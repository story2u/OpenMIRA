from typing import Any

from fastapi import FastAPI
from fastapi.testclient import TestClient
from redis.exceptions import RedisError

from app.api.deps import get_redis_client, get_user_repo
from app.api.v1.routes import auth
from app.core.config import Settings, get_settings
from app.core.security import decode_access_token, hash_password
from app.infrastructure.db.models import User


class FakeUserRepository:
    def __init__(self, user: User | None) -> None:
        self.user = user
        self.marked_user: User | None = None

    async def get_by_email(self, email: str) -> User | None:
        if self.user and self.user.email == email.lower().strip():
            return self.user
        return None

    async def mark_login(self, user: User) -> User:
        self.marked_user = user
        return user


class FakeRedisPipeline:
    def __init__(self, redis: "FakeRedis") -> None:
        self.redis = redis

    def incr(self, key: str) -> "FakeRedisPipeline":
        self.redis.pending_key = key
        return self

    def expire(self, key: str, seconds: int) -> "FakeRedisPipeline":
        self.redis.expiry_seconds = seconds
        return self

    async def execute(self) -> list[int | bool]:
        assert self.redis.pending_key
        self.redis.attempts[self.redis.pending_key] = (
            self.redis.attempts.get(self.redis.pending_key, 0) + 1
        )
        return [self.redis.attempts[self.redis.pending_key], True]


class FakeRedis:
    def __init__(self) -> None:
        self.attempts: dict[str, int] = {}
        self.pending_key: str | None = None
        self.expiry_seconds: int | None = None

    def pipeline(self, transaction: bool) -> FakeRedisPipeline:
        assert transaction
        return FakeRedisPipeline(self)

    async def delete(self, key: str) -> None:
        self.attempts.pop(key, None)


class UnavailableRedisPipeline(FakeRedisPipeline):
    async def execute(self) -> list[int | bool]:
        raise RedisError("unavailable")


class UnavailableRedis(FakeRedis):
    def pipeline(self, transaction: bool) -> FakeRedisPipeline:
        assert transaction
        return UnavailableRedisPipeline(self)


def make_settings(**overrides: Any) -> Settings:
    values: dict[str, Any] = {
        "database_url": "postgresql+asyncpg://user:password@localhost:5432/im",
        "admin_api_token": "admin-secret",
        "jwt_secret_key": "jwt-secret",
    }
    values.update(overrides)
    return Settings(**values)


def make_client(
    repo: FakeUserRepository,
    *,
    settings: Settings | None = None,
    redis: FakeRedis | None = None,
) -> tuple[TestClient, Settings, FakeRedis]:
    settings = settings or make_settings()
    redis = redis or FakeRedis()
    app = FastAPI()
    app.include_router(auth.router, prefix="/auth")
    app.dependency_overrides[get_settings] = lambda: settings
    app.dependency_overrides[get_user_repo] = lambda: repo
    app.dependency_overrides[get_redis_client] = lambda: redis
    return TestClient(app), settings, redis


def test_password_login_returns_token_for_active_password_user() -> None:
    user = User(
        email="member@example.com",
        display_name="Member",
        password_hash=hash_password("correct horse battery staple"),
    )
    repo = FakeUserRepository(user)
    client, settings, redis = make_client(repo)

    response = client.post(
        "/auth/password/login",
        json={"email": " MEMBER@EXAMPLE.COM ", "password": "correct horse battery staple"},
    )

    assert response.status_code == 200, response.text
    assert response.json()["user"]["email"] == "member@example.com"
    assert decode_access_token(response.json()["accessToken"], settings)["sub"] == str(user.id)
    assert repo.marked_user is user
    assert redis.attempts == {}


def test_password_login_rejects_wrong_password() -> None:
    user = User(email="member@example.com", password_hash=hash_password("correct password"))
    client, _, _ = make_client(FakeUserRepository(user))

    response = client.post(
        "/auth/password/login",
        json={"email": "member@example.com", "password": "wrong password"},
    )

    assert response.status_code == 401
    assert response.json() == {"detail": "邮箱或密码错误"}


def test_password_login_unknown_email_runs_password_verification(monkeypatch: Any) -> None:
    checked_hashes: list[str] = []

    def record_verification(password: str, password_hash: str) -> bool:
        assert password == "any password"
        checked_hashes.append(password_hash)
        return False

    monkeypatch.setattr(auth, "verify_password", record_verification)
    client, _, _ = make_client(FakeUserRepository(None))

    response = client.post(
        "/auth/password/login",
        json={"email": "missing@example.com", "password": "any password"},
    )

    assert response.status_code == 401
    assert response.json() == {"detail": "邮箱或密码错误"}
    assert checked_hashes == [auth._DUMMY_PASSWORD_HASH]


def test_password_login_rejects_passwordless_and_inactive_accounts_identically() -> None:
    passwordless = User(email="oauth@example.com", password_hash=None)
    passwordless_client, _, _ = make_client(FakeUserRepository(passwordless))
    inactive = User(
        email="inactive@example.com",
        password_hash=hash_password("correct password"),
        is_active=False,
    )
    inactive_client, _, _ = make_client(FakeUserRepository(inactive))

    passwordless_response = passwordless_client.post(
        "/auth/password/login",
        json={"email": "oauth@example.com", "password": "any password"},
    )
    inactive_response = inactive_client.post(
        "/auth/password/login",
        json={"email": "inactive@example.com", "password": "correct password"},
    )

    assert passwordless_response.status_code == inactive_response.status_code == 401
    assert passwordless_response.json() == inactive_response.json() == {"detail": "邮箱或密码错误"}


def test_password_login_validates_request_bounds() -> None:
    client, _, _ = make_client(FakeUserRepository(None))

    invalid_email = client.post(
        "/auth/password/login",
        json={"email": "not-an-email", "password": "password"},
    )
    oversized_password = client.post(
        "/auth/password/login",
        json={"email": "member@example.com", "password": "x" * 129},
    )

    assert invalid_email.status_code == 422
    assert oversized_password.status_code == 422


def test_password_login_rate_limits_repeated_failures() -> None:
    settings = make_settings(password_login_max_attempts=1, password_login_window_seconds=60)
    client, _, redis = make_client(FakeUserRepository(None), settings=settings)

    first = client.post(
        "/auth/password/login",
        json={"email": "missing@example.com", "password": "any password"},
    )
    limited = client.post(
        "/auth/password/login",
        json={"email": "missing@example.com", "password": "any password"},
    )

    assert first.status_code == 401
    assert limited.status_code == 429
    assert limited.headers["Retry-After"] == "60"
    assert redis.expiry_seconds == 60


def test_password_login_fails_closed_when_rate_limit_store_is_unavailable() -> None:
    client, _, _ = make_client(FakeUserRepository(None), redis=UnavailableRedis())

    response = client.post(
        "/auth/password/login",
        json={"email": "missing@example.com", "password": "any password"},
    )

    assert response.status_code == 503
    assert response.json() == {"detail": "登录服务暂时不可用"}
