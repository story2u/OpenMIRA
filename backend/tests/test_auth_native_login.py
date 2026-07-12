import base64
import json
import time
from typing import Any

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.deps import get_user_repo
from app.api.v1.routes import auth
from app.core.config import Settings, get_settings
from app.core.security import decode_access_token
from app.infrastructure.db.models import User

PRIVATE_KEY = rsa.generate_private_key(public_exponent=65537, key_size=2048)
KID = "test-key"


def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def _int_to_b64url(value: int) -> str:
    return _b64url(value.to_bytes((value.bit_length() + 7) // 8, "big"))


def make_jwks() -> dict[str, Any]:
    numbers = PRIVATE_KEY.public_key().public_numbers()
    return {
        "keys": [
            {
                "kty": "RSA",
                "kid": KID,
                "n": _int_to_b64url(numbers.n),
                "e": _int_to_b64url(numbers.e),
            }
        ]
    }


def make_id_token(claims: dict[str, Any]) -> str:
    header = {"alg": "RS256", "kid": KID}
    signing_input = ".".join(
        [
            _b64url(json.dumps(header, separators=(",", ":")).encode()),
            _b64url(json.dumps(claims, separators=(",", ":")).encode()),
        ]
    )
    signature = PRIVATE_KEY.sign(signing_input.encode("ascii"), padding.PKCS1v15(), hashes.SHA256())
    return f"{signing_input}.{_b64url(signature)}"


def google_claims(**overrides: Any) -> dict[str, Any]:
    claims: dict[str, Any] = {
        "iss": "https://accounts.google.com",
        "aud": "ios-client-id",
        "sub": "google-sub-1",
        "email": "bruce@example.com",
        "email_verified": True,
        "name": "Bruce",
        "picture": "https://example.com/a.png",
        "exp": int(time.time()) + 600,
    }
    claims.update(overrides)
    return claims


class FakeUserRepository:
    def __init__(self) -> None:
        self.users: dict[tuple[str, str], User] = {}

    async def get_or_create_oauth_user(
        self,
        *,
        provider: str,
        provider_subject: str,
        email: str,
        display_name: str,
        avatar_url: str,
    ) -> User:
        key = (provider, provider_subject)
        if key not in self.users:
            self.users[key] = User(email=email, display_name=display_name, avatar_url=avatar_url)
        return self.users[key]

    async def get_by_auth_account(self, provider: str, provider_subject: str) -> User | None:
        return self.users.get((provider, provider_subject))

    async def mark_login(self, user: User) -> User:
        return user


def make_settings(**overrides: Any) -> Settings:
    values: dict[str, Any] = {
        "database_url": "postgresql+asyncpg://user:password@localhost:5432/im",
        "admin_api_token": "admin-secret",
        "jwt_secret_key": "jwt-secret",
        "google_native_client_ids": "ios-client-id",
        "apple_native_client_ids": "com.storyim.radar",
    }
    values.update(overrides)
    return Settings(**values)


def make_client(monkeypatch: Any, settings: Settings, repo: FakeUserRepository) -> TestClient:
    async def fake_fetch_jwks(provider: str, jwks_url: str, client: Any) -> dict[str, Any]:
        return make_jwks()

    monkeypatch.setattr(auth, "fetch_provider_jwks", fake_fetch_jwks)
    app = FastAPI()
    app.include_router(auth.router, prefix="/auth")
    app.dependency_overrides[get_settings] = lambda: settings
    app.dependency_overrides[get_user_repo] = lambda: repo
    return TestClient(app)


def test_native_login_creates_user_and_returns_token(monkeypatch: Any) -> None:
    settings = make_settings()
    repo = FakeUserRepository()
    client = make_client(monkeypatch, settings, repo)

    response = client.post(
        "/auth/oauth/google/native",
        json={"idToken": make_id_token(google_claims())},
    )

    assert response.status_code == 200, response.text
    body = response.json()
    user = repo.users[("google", "google-sub-1")]
    assert body["user"]["email"] == "bruce@example.com"
    assert decode_access_token(body["accessToken"], settings)["sub"] == str(user.id)


def test_native_login_accepts_any_configured_audience(monkeypatch: Any) -> None:
    settings = make_settings(google_native_client_ids="web-server-client, ios-client-id")
    client = make_client(monkeypatch, settings, FakeUserRepository())

    response = client.post(
        "/auth/oauth/google/native",
        json={"idToken": make_id_token(google_claims())},
    )

    assert response.status_code == 200, response.text


def test_native_login_apple_uses_bundle_id_audience(monkeypatch: Any) -> None:
    settings = make_settings()
    client = make_client(monkeypatch, settings, FakeUserRepository())
    claims = google_claims(
        iss="https://appleid.apple.com",
        aud="com.storyim.radar",
        sub="apple-sub-1",
        email_verified="true",
    )

    response = client.post("/auth/oauth/apple/native", json={"idToken": make_id_token(claims)})

    assert response.status_code == 200, response.text


def test_native_login_rejects_wrong_audience(monkeypatch: Any) -> None:
    client = make_client(monkeypatch, make_settings(), FakeUserRepository())

    response = client.post(
        "/auth/oauth/google/native",
        json={"idToken": make_id_token(google_claims(aud="other-app"))},
    )

    assert response.status_code == 401


def test_native_login_rejects_unverified_email(monkeypatch: Any) -> None:
    client = make_client(monkeypatch, make_settings(), FakeUserRepository())

    response = client.post(
        "/auth/oauth/google/native",
        json={"idToken": make_id_token(google_claims(email_verified=False))},
    )

    assert response.status_code == 403


def test_native_login_requires_configuration(monkeypatch: Any) -> None:
    settings = make_settings(google_native_client_ids="")
    client = make_client(monkeypatch, settings, FakeUserRepository())

    response = client.post(
        "/auth/oauth/google/native",
        json={"idToken": make_id_token(google_claims())},
    )

    assert response.status_code == 503


def test_native_login_rejects_unknown_provider(monkeypatch: Any) -> None:
    client = make_client(monkeypatch, make_settings(), FakeUserRepository())

    response = client.post(
        "/auth/oauth/github/native",
        json={"idToken": make_id_token(google_claims())},
    )

    assert response.status_code == 404
