import base64
import hashlib
import hmac
import json
import os
import re
import secrets
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

from cryptography.exceptions import InvalidSignature
from cryptography.fernet import Fernet, InvalidToken
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, padding, rsa, utils
from fastapi import HTTPException, status

from app.core.config import Settings

PASSWORD_SCHEME = "pbkdf2_sha256"
PASSWORD_ITERATIONS = 390000
DEVICE_REFRESH_TOKEN_PREFIX = "radar_device_1_"
DEVICE_REFRESH_TOKEN_PATTERN = re.compile(r"^radar_device_1_[A-Za-z0-9_-]{43}$")
ANALYSIS_RUN_TOKEN_PURPOSE = "analysis_run"
INTERACTIVE_AGENT_TURN_TOKEN_PURPOSE = "interactive_agent_turn"
INTERACTIVE_AGENT_APPROVAL_TOKEN_PURPOSE = "interactive_agent_approval"


def constant_time_equals(left: str, right: str) -> bool:
    return hmac.compare_digest(left.encode("utf-8"), right.encode("utf-8"))


def require_secret(actual: str, expected: str, detail: str = "invalid signature") -> None:
    if not expected or not constant_time_equals(actual, expected):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail=detail)


def hash_password(password: str) -> str:
    salt = os.urandom(16)
    digest = hashlib.pbkdf2_hmac(
        "sha256",
        password.encode("utf-8"),
        salt,
        PASSWORD_ITERATIONS,
    )
    return (
        f"{PASSWORD_SCHEME}${PASSWORD_ITERATIONS}$"
        f"{base64.b64encode(salt).decode()}${base64.b64encode(digest).decode()}"
    )


def verify_password(password: str, password_hash: str) -> bool:
    try:
        scheme, iterations, salt_b64, digest_b64 = password_hash.split("$", 3)
        if scheme != PASSWORD_SCHEME:
            return False
        salt = base64.b64decode(salt_b64.encode())
        expected = base64.b64decode(digest_b64.encode())
        actual = hashlib.pbkdf2_hmac(
            "sha256",
            password.encode("utf-8"),
            salt,
            int(iterations),
        )
        return hmac.compare_digest(actual, expected)
    except (ValueError, TypeError):
        return False


def create_device_refresh_token() -> str:
    return f"{DEVICE_REFRESH_TOKEN_PREFIX}{secrets.token_urlsafe(32)}"


def hash_device_refresh_token(token: str) -> str:
    if not DEVICE_REFRESH_TOKEN_PATTERN.fullmatch(token):
        raise ValueError("invalid device refresh token")
    return hashlib.sha256(token.encode("ascii")).hexdigest()


def hash_device_installation_id(installation_id: UUID, settings: Settings) -> str:
    """Keep the stable installation identifier unlinkable without the server auth secret."""
    return hmac.new(
        _auth_secret(settings).encode("utf-8"),
        f"device-installation\0{installation_id}".encode("ascii"),
        hashlib.sha256,
    ).hexdigest()


def derive_analysis_run_nonce(
    *,
    run_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    """Derive a repeatable nonce so retried claims can issue a usable token without storage."""
    return hmac.new(
        _auth_secret(settings).encode("utf-8"),
        f"analysis-run\0{run_id}\0{owner_user_id}\0{device_id}".encode("ascii"),
        hashlib.sha256,
    ).hexdigest()


def hash_analysis_run_nonce(nonce: str) -> str:
    if not re.fullmatch(r"[0-9a-f]{64}", nonce):
        raise ValueError("invalid analysis run nonce")
    return hashlib.sha256(nonce.encode("ascii")).hexdigest()


def derive_interactive_agent_turn_nonce(
    *,
    turn_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    return hmac.new(
        _auth_secret(settings).encode("utf-8"),
        f"interactive-agent-turn\0{turn_id}\0{owner_user_id}\0{device_id}".encode("ascii"),
        hashlib.sha256,
    ).hexdigest()


def hash_interactive_agent_turn_nonce(nonce: str) -> str:
    if not re.fullmatch(r"[0-9a-f]{64}", nonce):
        raise ValueError("invalid interactive Agent turn nonce")
    return hashlib.sha256(nonce.encode("ascii")).hexdigest()


def derive_interactive_agent_approval_nonce(
    *,
    approval_id: UUID,
    turn_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    material = f"interactive-agent-approval\0{approval_id}\0{turn_id}\0{owner_user_id}\0{device_id}"
    return hmac.new(
        _auth_secret(settings).encode("utf-8"),
        material.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()


def hash_interactive_agent_approval_nonce(nonce: str) -> str:
    if not re.fullmatch(r"[0-9a-f]{64}", nonce):
        raise ValueError("invalid interactive Agent approval nonce")
    return hashlib.sha256(nonce.encode("ascii")).hexdigest()


def _b64url_encode(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def _b64url_decode(data: str) -> bytes:
    padding = "=" * (-len(data) % 4)
    return base64.urlsafe_b64decode((data + padding).encode("ascii"))


def _auth_secret(settings: Settings) -> str:
    return settings.jwt_secret_key or settings.admin_api_token


def create_signed_token(
    payload: dict[str, Any],
    *,
    settings: Settings,
    expires_delta: timedelta,
) -> str:
    now = datetime.now(UTC)
    body = {
        **payload,
        "iat": int(now.timestamp()),
        "exp": int((now + expires_delta).timestamp()),
    }
    header = {"alg": "HS256", "typ": "JWT"}
    signing_input = ".".join(
        [
            _b64url_encode(json.dumps(header, separators=(",", ":")).encode()),
            _b64url_encode(json.dumps(body, separators=(",", ":")).encode()),
        ]
    )
    signature = hmac.new(
        _auth_secret(settings).encode("utf-8"),
        signing_input.encode("ascii"),
        hashlib.sha256,
    ).digest()
    return f"{signing_input}.{_b64url_encode(signature)}"


def decode_signed_token(token: str, settings: Settings) -> dict[str, Any]:
    try:
        header_b64, payload_b64, signature_b64 = token.split(".", 2)
        signing_input = f"{header_b64}.{payload_b64}"
        expected = hmac.new(
            _auth_secret(settings).encode("utf-8"),
            signing_input.encode("ascii"),
            hashlib.sha256,
        ).digest()
        actual = _b64url_decode(signature_b64)
        if not hmac.compare_digest(actual, expected):
            raise ValueError("invalid signature")
        header = json.loads(_b64url_decode(header_b64))
        if header.get("alg") != "HS256":
            raise ValueError("invalid algorithm")
        payload = json.loads(_b64url_decode(payload_b64))
        if int(payload.get("exp", 0)) < int(datetime.now(UTC).timestamp()):
            raise ValueError("token expired")
        return payload
    except (ValueError, json.JSONDecodeError, TypeError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc


def create_access_token(
    *,
    subject: UUID,
    settings: Settings,
    device_id: UUID | None = None,
    expires_delta: timedelta | None = None,
) -> str:
    return create_signed_token(
        {
            "sub": str(subject),
            **({"did": str(device_id)} if device_id is not None else {}),
        },
        settings=settings,
        expires_delta=expires_delta or timedelta(minutes=settings.access_token_expire_minutes),
    )


def decode_access_token(token: str, settings: Settings) -> dict[str, Any]:
    return decode_signed_token(token, settings)


def create_analysis_run_token(
    *,
    run_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    nonce = derive_analysis_run_nonce(
        run_id=run_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        settings=settings,
    )
    return create_signed_token(
        {
            "sub": str(owner_user_id),
            "did": str(device_id),
            "rid": str(run_id),
            "purpose": ANALYSIS_RUN_TOKEN_PURPOSE,
            "nonce": nonce,
        },
        settings=settings,
        expires_delta=timedelta(minutes=settings.device_agent_run_token_minutes),
    )


def decode_analysis_run_token(token: str, settings: Settings) -> dict[str, Any]:
    payload = decode_signed_token(token, settings)
    if payload.get("purpose") != ANALYSIS_RUN_TOKEN_PURPOSE:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token")
    return payload


def create_interactive_agent_turn_token(
    *,
    turn_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    nonce = derive_interactive_agent_turn_nonce(
        turn_id=turn_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        settings=settings,
    )
    return create_signed_token(
        {
            "sub": str(owner_user_id),
            "did": str(device_id),
            "tid": str(turn_id),
            "purpose": INTERACTIVE_AGENT_TURN_TOKEN_PURPOSE,
            "nonce": nonce,
        },
        settings=settings,
        expires_delta=timedelta(minutes=settings.interactive_agent_turn_token_minutes),
    )


def decode_interactive_agent_turn_token(
    token: str,
    settings: Settings,
) -> dict[str, Any]:
    payload = decode_signed_token(token, settings)
    if payload.get("purpose") != INTERACTIVE_AGENT_TURN_TOKEN_PURPOSE:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token")
    return payload


def create_interactive_agent_approval_token(
    *,
    approval_id: UUID,
    turn_id: UUID,
    owner_user_id: UUID,
    device_id: UUID,
    settings: Settings,
) -> str:
    nonce = derive_interactive_agent_approval_nonce(
        approval_id=approval_id,
        turn_id=turn_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        settings=settings,
    )
    return create_signed_token(
        {
            "sub": str(owner_user_id),
            "did": str(device_id),
            "tid": str(turn_id),
            "aid": str(approval_id),
            "purpose": INTERACTIVE_AGENT_APPROVAL_TOKEN_PURPOSE,
            "nonce": nonce,
        },
        settings=settings,
        expires_delta=timedelta(seconds=settings.interactive_agent_approval_token_seconds),
    )


def decode_interactive_agent_approval_token(
    token: str,
    settings: Settings,
) -> dict[str, Any]:
    payload = decode_signed_token(token, settings)
    if payload.get("purpose") != INTERACTIVE_AGENT_APPROVAL_TOKEN_PURPOSE:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token")
    return payload


def _fernet(settings: Settings) -> Fernet:
    digest = hashlib.sha256(_auth_secret(settings).encode("utf-8")).digest()
    return Fernet(base64.urlsafe_b64encode(digest))


def encrypt_secret(value: str, settings: Settings) -> str:
    return _fernet(settings).encrypt(value.encode("utf-8")).decode("utf-8")


def decrypt_secret(value: str, settings: Settings) -> str:
    try:
        return _fernet(settings).decrypt(value.encode("utf-8")).decode("utf-8")
    except InvalidToken as exc:
        raise ValueError("secret cannot be decrypted with current JWT_SECRET_KEY") from exc


def decode_unverified_jwt(token: str) -> tuple[dict[str, Any], dict[str, Any], bytes, bytes]:
    header_b64, payload_b64, signature_b64 = token.split(".", 2)
    header = json.loads(_b64url_decode(header_b64))
    payload = json.loads(_b64url_decode(payload_b64))
    return (
        header,
        payload,
        f"{header_b64}.{payload_b64}".encode("ascii"),
        _b64url_decode(signature_b64),
    )


def verify_rs256_jwt(
    token: str,
    *,
    jwks: dict[str, Any],
    issuer: str,
    audience: str,
) -> dict[str, Any]:
    try:
        header, payload, signing_input, signature = decode_unverified_jwt(token)
    except (ValueError, json.JSONDecodeError, TypeError) as exc:
        raise ValueError("invalid id token") from exc
    if header.get("alg") != "RS256":
        raise ValueError("unexpected id token algorithm")
    key = next(
        (item for item in jwks.get("keys", []) if item.get("kid") == header.get("kid")),
        None,
    )
    if not key:
        raise ValueError("id token key not found")
    try:
        public_key = rsa.RSAPublicNumbers(
            e=int.from_bytes(_b64url_decode(key["e"]), "big"),
            n=int.from_bytes(_b64url_decode(key["n"]), "big"),
        ).public_key()
    except (KeyError, TypeError, ValueError) as exc:
        raise ValueError("invalid id token key") from exc
    try:
        public_key.verify(signature, signing_input, padding.PKCS1v15(), hashes.SHA256())
    except (InvalidSignature, TypeError, ValueError) as exc:
        raise ValueError("invalid id token signature") from exc
    if payload.get("iss") != issuer:
        raise ValueError("invalid id token issuer")
    token_audience = payload.get("aud")
    if isinstance(token_audience, list):
        audience_valid = audience in token_audience
    else:
        audience_valid = token_audience == audience
    if not audience_valid:
        raise ValueError("invalid id token audience")
    try:
        expires_at = int(payload.get("exp", 0))
    except (TypeError, ValueError) as exc:
        raise ValueError("invalid id token expiry") from exc
    if expires_at < int(datetime.now(UTC).timestamp()):
        raise ValueError("id token expired")
    return payload


def create_apple_client_secret(settings: Settings) -> str:
    if settings.apple_oauth_client_secret:
        return settings.apple_oauth_client_secret
    if not (
        settings.apple_oauth_team_id
        and settings.apple_oauth_key_id
        and settings.apple_oauth_private_key
        and settings.apple_oauth_client_id
    ):
        raise ValueError("Apple OAuth client secret or signing key settings are missing")

    private_key_text = settings.apple_oauth_private_key.replace("\\n", "\n")
    private_key = serialization.load_pem_private_key(
        private_key_text.encode("utf-8"),
        password=None,
    )
    if not isinstance(private_key, ec.EllipticCurvePrivateKey):
        raise ValueError("Apple OAuth private key must be an EC private key")
    now = datetime.now(UTC)
    header = {"alg": "ES256", "kid": settings.apple_oauth_key_id}
    payload = {
        "iss": settings.apple_oauth_team_id,
        "iat": int(now.timestamp()),
        "exp": int((now + timedelta(days=180)).timestamp()),
        "aud": "https://appleid.apple.com",
        "sub": settings.apple_oauth_client_id,
    }
    signing_input = ".".join(
        [
            _b64url_encode(json.dumps(header, separators=(",", ":")).encode()),
            _b64url_encode(json.dumps(payload, separators=(",", ":")).encode()),
        ]
    )
    der_signature = private_key.sign(signing_input.encode("ascii"), ec.ECDSA(hashes.SHA256()))
    r, s = utils.decode_dss_signature(der_signature)
    signature = r.to_bytes(32, "big") + s.to_bytes(32, "big")
    return f"{signing_input}.{_b64url_encode(signature)}"
