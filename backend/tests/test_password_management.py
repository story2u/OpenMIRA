from datetime import timedelta
from typing import Any
from uuid import uuid4

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api import deps
from app.api.deps import (
    get_password_reset_repo,
    get_redis_client,
    get_task_queue,
    get_user_repo,
    require_user,
)
from app.api.v1.routes import auth
from app.application.use_cases.password_management import (
    CurrentPasswordInvalid,
    PasswordManagementUseCase,
    PasswordResetInvalid,
    PasswordResetRequired,
)
from app.core.config import Settings, get_settings
from app.core.password_reset import reset_credential_digest
from app.core.security import create_access_token, hash_password, verify_password
from app.infrastructure.db.models import PasswordResetChallenge, User, utc_now


def make_settings(**overrides: Any) -> Settings:
    values: dict[str, Any] = {
        "database_url": "postgresql+asyncpg://user:password@localhost:5432/im",
        "admin_api_token": "admin-secret",
        "jwt_secret_key": "jwt-secret-for-password-tests",
        "smtp_host": "smtp.example.test",
        "smtp_from_email": "no-reply@example.test",
    }
    values.update(overrides)
    return Settings(**values)


def test_password_reset_is_enabled_by_default_but_requires_email_configuration() -> None:
    settings = make_settings(smtp_host="", smtp_from_email="")

    assert settings.password_reset_enabled is True
    assert settings.password_reset_email_configured is False


class FakeQueue:
    def __init__(self, succeeds: bool = True) -> None:
        self.succeeds = succeeds
        self.emails: list[str] = []

    def enqueue_password_reset(self, email: str) -> bool:
        self.emails.append(email)
        return self.succeeds


class FakeUserRepository:
    def __init__(self, user: User | None = None) -> None:
        self.user = user

    async def get(self, user_id):
        return self.user if self.user and self.user.id == user_id else None

    async def get_by_email(self, email: str) -> User | None:
        return self.user if self.user and self.user.email == email else None


class FakeResetRepository:
    def __init__(self, settings: Settings, user: User | None = None, code: str = "ABCDEFGH23") -> None:
        self.settings = settings
        self.user = user
        self.code = code
        self.challenge = PasswordResetChallenge(
            user_id=user.id if user else uuid4(),
            token_digest=reset_credential_digest("x" * 43, settings),
            code_digest=reset_credential_digest(code, settings),
            expires_at=utc_now() + timedelta(minutes=15),
        )

    async def active_by_token(self, digest: str):
        if self.challenge.used_at is None and digest == self.challenge.token_digest:
            return self.challenge
        return None

    async def latest_active_for_email(self, email: str):
        if self.user and self.user.email == email and self.challenge.used_at is None:
            return self.challenge, self.user
        return None

    async def user_for_challenge(self, challenge):
        return self.user

    async def register_failed_attempt(self, challenge, *, max_attempts: int) -> None:
        challenge.failed_attempts += 1
        if challenge.failed_attempts >= max_attempts:
            challenge.used_at = utc_now()

    async def replace_password(self, *, user, password_hash: str, challenge=None):
        user.password_hash = password_hash
        user.auth_version += 1
        if challenge:
            challenge.used_at = utc_now()
        return user


def make_use_case(user: User | None = None):
    settings = make_settings()
    reset_repo = FakeResetRepository(settings, user)
    queue = FakeQueue()
    use_case = PasswordManagementUseCase(
        settings=settings,
        user_repo=FakeUserRepository(user),  # type: ignore[arg-type]
        reset_repo=reset_repo,  # type: ignore[arg-type]
        task_queue=queue,  # type: ignore[arg-type]
    )
    return use_case, reset_repo, queue


@pytest.mark.asyncio
async def test_change_password_replaces_hash_and_increments_auth_version() -> None:
    user = User(email="member@example.test", password_hash=hash_password("old-password"))
    use_case, _, _ = make_use_case(user)

    await use_case.change_password(
        user=user,
        current_password="old-password",
        new_password="new-password-123",
    )

    assert user.auth_version == 1
    assert verify_password("new-password-123", user.password_hash or "")
    assert not verify_password("old-password", user.password_hash or "")


@pytest.mark.asyncio
async def test_change_password_rejects_wrong_or_missing_current_password() -> None:
    password_user = User(email="member@example.test", password_hash=hash_password("old-password"))
    use_case, _, _ = make_use_case(password_user)
    with pytest.raises(CurrentPasswordInvalid):
        await use_case.change_password(
            user=password_user,
            current_password="wrong-password",
            new_password="new-password-123",
        )

    oauth_user = User(email="oauth@example.test", password_hash=None)
    use_case, _, _ = make_use_case(oauth_user)
    with pytest.raises(PasswordResetRequired):
        await use_case.change_password(
            user=oauth_user,
            current_password="anything",
            new_password="new-password-123",
        )


@pytest.mark.asyncio
async def test_reset_code_is_one_time_and_failed_codes_are_counted() -> None:
    user = User(email="member@example.test", password_hash=hash_password("old-password"))
    use_case, reset_repo, _ = make_use_case(user)

    with pytest.raises(PasswordResetInvalid):
        await use_case.confirm_reset(
            email=user.email,
            code="ZZZZZZZZZ2",
            new_password="new-password-123",
        )
    assert reset_repo.challenge.failed_attempts == 1

    await use_case.confirm_reset(
        email=user.email,
        code=reset_repo.code,
        new_password="new-password-123",
    )
    assert reset_repo.challenge.used_at is not None
    assert user.auth_version == 1

    with pytest.raises(PasswordResetInvalid):
        await use_case.confirm_reset(
            email=user.email,
            code=reset_repo.code,
            new_password="another-password-123",
        )


@pytest.mark.asyncio
async def test_auth_version_rejects_tokens_issued_before_password_change(monkeypatch: Any) -> None:
    settings = make_settings()
    user = User(id=uuid4(), email="member@example.test", auth_version=1)
    old_token = create_access_token(subject=user.id, settings=settings, auth_version=0)
    current_token = create_access_token(subject=user.id, settings=settings, auth_version=1)
    monkeypatch.setattr(deps, "UserRepository", lambda session: FakeUserRepository(user))

    with pytest.raises(Exception) as exc_info:
        await deps._user_from_token(old_token, settings, object())  # type: ignore[arg-type]
    assert getattr(exc_info.value, "status_code", None) == 401
    assert await deps._user_from_token(current_token, settings, object()) is user  # type: ignore[arg-type]


class FakeRedis:
    def __init__(self) -> None:
        self.values: dict[str, int] = {}

    def pipeline(self, transaction: bool = True):
        return self

    def incr(self, key: str):
        self.pending_key = key
        return self

    def expire(self, key: str, seconds: int):
        return self

    async def execute(self):
        self.values[self.pending_key] = self.values.get(self.pending_key, 0) + 1
        return self.values[self.pending_key], True

    async def delete(self, key: str):
        self.values.pop(key, None)


def make_client(*, queue: FakeQueue | None = None, settings: Settings | None = None) -> TestClient:
    app = FastAPI()
    app.include_router(auth.router, prefix="/auth")
    active_settings = settings or make_settings()
    active_queue = queue or FakeQueue()
    app.dependency_overrides[get_settings] = lambda: active_settings
    app.dependency_overrides[get_redis_client] = lambda: FakeRedis()
    app.dependency_overrides[get_user_repo] = lambda: FakeUserRepository()
    app.dependency_overrides[get_password_reset_repo] = lambda: FakeResetRepository(active_settings)
    app.dependency_overrides[get_task_queue] = lambda: active_queue
    app.dependency_overrides[require_user] = lambda: User(email="member@example.test")
    return TestClient(app)


def test_reset_request_uses_generic_response_and_never_queries_user_in_http_path() -> None:
    queue = FakeQueue()
    client = make_client(queue=queue)

    response = client.post("/auth/password/reset/request", json={"email": "missing@example.test"})

    assert response.status_code == 202
    assert response.json() == {
        "message": "如果该邮箱已注册，重置邮件将在几分钟内送达"
    }
    assert queue.emails == ["missing@example.test"]


def test_reset_request_fails_closed_when_email_or_queue_is_unavailable() -> None:
    disabled = make_client(settings=make_settings(password_reset_enabled=False))
    unavailable = make_client(queue=FakeQueue(succeeds=False))

    assert disabled.post(
        "/auth/password/reset/request", json={"email": "member@example.test"}
    ).status_code == 503
    assert unavailable.post(
        "/auth/password/reset/request", json={"email": "member@example.test"}
    ).status_code == 503
