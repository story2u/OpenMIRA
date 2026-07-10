from dataclasses import dataclass
from datetime import timedelta
import html
import json
from typing import Any
from urllib.parse import parse_qs, urlencode

import httpx
import structlog
from fastapi import APIRouter, Depends, HTTPException, Request, status
from fastapi.responses import HTMLResponse
from sqlalchemy.exc import DBAPIError, IntegrityError, OperationalError, ProgrammingError, SQLAlchemyError

from app.api.deps import get_user_repo, require_user
from app.application.dto import AuthTokenRead, AuthUserRead, OAuthAuthorizeRead
from app.application.mappers import to_auth_user_read
from app.core.config import Settings, get_settings
from app.core.security import (
    create_access_token,
    create_apple_client_secret,
    create_signed_token,
    decode_signed_token,
    verify_rs256_jwt,
)
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import UserRepository

router = APIRouter()
logger = structlog.get_logger(__name__)


@dataclass(frozen=True)
class OAuthProviderConfig:
    provider: str
    client_id: str
    client_secret: str
    redirect_uri: str
    authorize_url: str
    token_url: str
    jwks_url: str
    issuer: str
    scope: str
    extra_authorize_params: dict[str, str]


def auth_token_response(user: User, settings: Settings) -> AuthTokenRead:
    return AuthTokenRead(
        accessToken=create_access_token(subject=user.id, settings=settings),
        user=to_auth_user_read(user),
    )


def frontend_login_redirect(settings: Settings, token: str) -> str:
    return f"{settings.frontend_base_url.rstrip('/')}/login#{urlencode({'token': token})}"


def frontend_login_response(settings: Settings, token: str) -> HTMLResponse:
    redirect_url = frontend_login_redirect(settings, token)
    escaped_url = html.escape(redirect_url, quote=True)
    return HTMLResponse(
        content=(
            "<!doctype html>"
            '<html lang="zh-CN">'
            "<head>"
            '<meta charset="utf-8">'
            '<meta name="robots" content="noindex,nofollow">'
            f'<meta http-equiv="refresh" content="0;url={escaped_url}">'
            "<title>正在登录</title>"
            "</head>"
            "<body>"
            f"<script>window.location.replace({json.dumps(redirect_url)});</script>"
            f'<noscript><a href="{escaped_url}">继续登录</a></noscript>'
            "</body>"
            "</html>"
        ),
        headers={"Cache-Control": "no-store"},
    )


def provider_config(provider: str, settings: Settings) -> OAuthProviderConfig:
    if provider == "google":
        if not (
            settings.google_oauth_client_id
            and settings.google_oauth_client_secret
            and settings.google_oauth_redirect_uri
        ):
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="Google OAuth is not configured")
        return OAuthProviderConfig(
            provider="google",
            client_id=settings.google_oauth_client_id,
            client_secret=settings.google_oauth_client_secret,
            redirect_uri=settings.google_oauth_redirect_uri,
            authorize_url="https://accounts.google.com/o/oauth2/v2/auth",
            token_url="https://oauth2.googleapis.com/token",
            jwks_url="https://www.googleapis.com/oauth2/v3/certs",
            issuer="https://accounts.google.com",
            scope="openid email profile",
            extra_authorize_params={"prompt": "select_account"},
        )
    if provider == "apple":
        if not (settings.apple_oauth_client_id and settings.apple_oauth_redirect_uri):
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="Apple OAuth is not configured")
        try:
            client_secret = create_apple_client_secret(settings)
        except ValueError as exc:
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail=str(exc)) from exc
        return OAuthProviderConfig(
            provider="apple",
            client_id=settings.apple_oauth_client_id,
            client_secret=client_secret,
            redirect_uri=settings.apple_oauth_redirect_uri,
            authorize_url="https://appleid.apple.com/auth/authorize",
            token_url="https://appleid.apple.com/auth/token",
            jwks_url="https://appleid.apple.com/auth/keys",
            issuer="https://appleid.apple.com",
            scope="openid email name",
            extra_authorize_params={"response_mode": "form_post"},
        )
    raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="unsupported OAuth provider")


def oauth_persistence_error_detail(exc: SQLAlchemyError) -> str:
    if isinstance(exc, ProgrammingError):
        return "OAuth user persistence failed: database schema error"
    if isinstance(exc, IntegrityError):
        return "OAuth user persistence failed: account constraint conflict"
    if isinstance(exc, OperationalError):
        return "OAuth user persistence failed: database connection error"
    if isinstance(exc, DBAPIError):
        original = getattr(exc, "orig", None)
        if original:
            return f"OAuth user persistence failed: {original.__class__.__name__}"
    return f"OAuth user persistence failed: {exc.__class__.__name__}"


async def exchange_code_for_profile(
    provider: str,
    code: str,
    settings: Settings,
) -> dict[str, Any]:
    config = provider_config(provider, settings)
    async with httpx.AsyncClient(timeout=15.0) as client:
        try:
            token_response = await client.post(
                config.token_url,
                data={
                    "grant_type": "authorization_code",
                    "code": code,
                    "client_id": config.client_id,
                    "client_secret": config.client_secret,
                    "redirect_uri": config.redirect_uri,
                },
                headers={"Accept": "application/json"},
            )
        except httpx.RequestError as exc:
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail=f"{provider} token endpoint is unavailable",
            ) from exc
        try:
            token_data = token_response.json()
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail=f"{provider} token endpoint returned invalid JSON",
            ) from exc
        if token_response.status_code >= 400:
            error = token_data.get("error") if isinstance(token_data, dict) else None
            error_description = token_data.get("error_description") if isinstance(token_data, dict) else None
            detail = f"{provider} token exchange failed"
            if error:
                detail = f"{detail}: {error}"
            if error_description:
                detail = f"{detail} ({error_description})"
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=detail,
            )
        id_token = token_data.get("id_token")
        if not id_token:
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="OAuth id_token missing")

        try:
            jwks_response = await client.get(config.jwks_url, headers={"Accept": "application/json"})
        except httpx.RequestError as exc:
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail=f"{provider} JWKS endpoint is unavailable",
            ) from exc
        if jwks_response.status_code >= 400:
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail=f"{provider} JWKS endpoint returned {jwks_response.status_code}",
            )
        try:
            jwks = jwks_response.json()
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail=f"{provider} JWKS endpoint returned invalid JSON",
            ) from exc

    try:
        claims = verify_rs256_jwt(
            id_token,
            jwks=jwks,
            issuer=config.issuer,
            audience=config.client_id,
        )
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail=str(exc)) from exc

    subject = claims.get("sub")
    if not subject:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="OAuth subject is missing")

    return {
        "provider": provider,
        "subject": str(subject),
        "email": claims.get("email"),
        "email_verified": claims.get("email_verified") in {True, "true", "1"},
        "name": claims.get("name") or "",
        "avatar_url": claims.get("picture") or "",
    }


async def complete_oauth_login(
    provider: str,
    code: str,
    state: str,
    settings: Settings,
    repo: UserRepository,
) -> HTMLResponse:
    try:
        state_payload = decode_signed_token(state, settings)
        if state_payload.get("provider") != provider:
            raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="OAuth state mismatch")

        profile = await exchange_code_for_profile(provider, code, settings)
        email = profile.get("email")
        if email:
            if not profile.get("email_verified"):
                raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="OAuth email is not verified")
            user = await repo.get_or_create_oauth_user(
                provider=provider,
                provider_subject=profile["subject"],
                email=email,
                display_name=profile.get("name") or email.split("@", 1)[0],
                avatar_url=profile.get("avatar_url") or "",
            )
        else:
            existing_user = await repo.get_by_auth_account(provider, profile["subject"])
            if not existing_user:
                raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="OAuth email is missing")
            user = await repo.mark_login(existing_user)

        return frontend_login_response(settings, create_access_token(subject=user.id, settings=settings))
    except HTTPException:
        raise
    except SQLAlchemyError as exc:
        logger.exception(
            "oauth.user_persistence_failed",
            provider=provider,
            error_class=exc.__class__.__name__,
        )
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=oauth_persistence_error_detail(exc),
        ) from exc
    except Exception as exc:
        logger.exception("oauth.callback_failed", provider=provider)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="OAuth callback failed",
        ) from exc


@router.get("/oauth/{provider}/authorize", response_model=OAuthAuthorizeRead)
async def oauth_authorize(
    provider: str,
    settings: Settings = Depends(get_settings),
) -> OAuthAuthorizeRead:
    config = provider_config(provider, settings)
    state = create_signed_token(
        {"provider": provider},
        settings=settings,
        expires_delta=timedelta(minutes=10),
    )
    params = {
        "client_id": config.client_id,
        "redirect_uri": config.redirect_uri,
        "response_type": "code",
        "scope": config.scope,
        "state": state,
        **config.extra_authorize_params,
    }
    return OAuthAuthorizeRead(authorizationUrl=f"{config.authorize_url}?{urlencode(params)}")


@router.get("/oauth/{provider}/callback")
async def oauth_callback(
    provider: str,
    code: str,
    state: str,
    settings: Settings = Depends(get_settings),
    repo: UserRepository = Depends(get_user_repo),
) -> HTMLResponse:
    return await complete_oauth_login(provider, code, state, settings, repo)


@router.post("/oauth/apple/callback")
async def apple_oauth_form_post_callback(
    request: Request,
    settings: Settings = Depends(get_settings),
    repo: UserRepository = Depends(get_user_repo),
) -> HTMLResponse:
    form = parse_qs((await request.body()).decode("utf-8"))
    code = form.get("code", [""])[0]
    state_value = form.get("state", [""])[0]
    if not code or not state_value:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="OAuth callback is missing code or state")
    return await complete_oauth_login("apple", code, state_value, settings, repo)


@router.get("/me", response_model=AuthUserRead)
async def me(current_user: User = Depends(require_user)) -> AuthUserRead:
    return to_auth_user_read(current_user)
