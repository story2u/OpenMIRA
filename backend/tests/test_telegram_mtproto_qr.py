import os
from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from uuid import uuid4

import pytest
from fastapi import HTTPException

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost:5432/im")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from telethon.tl.types import Chat, ChatPhotoEmpty

from app.api.v1.routes import telegram_connections
from app.api.v1.routes.telegram_connections import add_mtproto_source, start_mtproto_qr_connection
from app.core.config import Settings
from app.core.security import decrypt_secret
from app.domain.enums import TelegramConnectionAttemptStatus, TelegramConnectionType
from app.domain.services.subscription_policy import GroupQuotaExceeded
from app.worker import telegram_mtproto_qr_worker


def qr_settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        jwt_secret_key="test-jwt-secret",
        telegram_mtproto_qr_enabled=True,
        telegram_mtproto_qr_worker_enabled=True,
        telegram_mtproto_api_id=12345,
        telegram_mtproto_api_hash="test-api-hash",
    )


def pending_attempt(owner_user_id):
    now = datetime.now(UTC)
    return SimpleNamespace(
        id=uuid4(),
        owner_user_id=owner_user_id,
        connection_type=TelegramConnectionType.MTPROTO_QR,
        status=TelegramConnectionAttemptStatus.PENDING,
        expires_at=now + timedelta(minutes=5),
        connection_id=None,
        error=None,
    )


class ExistingAttemptRepo:
    def __init__(self, attempt):
        self.attempt = attempt
        self.create_calls = 0

    async def get_pending_attempt_for_owner(self, *, owner_user_id, connection_type):
        assert owner_user_id == self.attempt.owner_user_id
        assert connection_type == TelegramConnectionType.MTPROTO_QR
        return self.attempt

    async def create_attempt(self, **_):
        self.create_calls += 1
        raise AssertionError("a second pending QR attempt must not be created")


@pytest.mark.asyncio
async def test_start_mtproto_qr_reuses_existing_pending_attempt() -> None:
    user = SimpleNamespace(id=uuid4())
    repo = ExistingAttemptRepo(pending_attempt(user.id))

    response = await start_mtproto_qr_connection(
        settings=qr_settings(),
        current_user=user,
        connection_repo=repo,
    )

    assert response.id == repo.attempt.id
    assert response.qrCodeUrl is None
    assert repo.create_calls == 0


@pytest.mark.asyncio
async def test_start_mtproto_qr_refuses_when_worker_is_disabled() -> None:
    user = SimpleNamespace(id=uuid4())
    settings = qr_settings()
    settings.telegram_mtproto_qr_worker_enabled = False

    with pytest.raises(HTTPException) as raised:
        await start_mtproto_qr_connection(
            settings=settings,
            current_user=user,
            connection_repo=ExistingAttemptRepo(pending_attempt(user.id)),
        )

    assert raised.value.status_code == 503


class FakeDialogClient:
    async def connect(self):
        return None

    async def disconnect(self):
        return None

    async def iter_dialogs(self, limit):
        assert limit == 100
        entity = Chat(
            id=123,
            title="Test group",
            photo=ChatPhotoEmpty(),
            participants_count=2,
            date=None,
            version=1,
        )
        yield SimpleNamespace(id=-123, entity=entity)


class QuotaRejectingRepo:
    async def add_mtproto_source(self, **_):
        raise GroupQuotaExceeded("current plan allows 1 Telegram group")


class FakeLegacyRepo:
    async def count_active_monitors_by_user(self, _):
        return 1


class FakeSubscriptionRepo:
    async def get_snapshot(self, _):
        return SimpleNamespace(entitlements=SimpleNamespace())


@pytest.mark.asyncio
async def test_add_mtproto_source_returns_403_when_group_quota_is_full(monkeypatch) -> None:
    async def fake_client(**_):
        return FakeDialogClient()

    monkeypatch.setattr(
        telegram_connections,
        "mtproto_client_for_connection",
        fake_client,
    )

    with pytest.raises(HTTPException) as raised:
        await add_mtproto_source(
            connection_id=uuid4(),
            payload=SimpleNamespace(chatId="-123"),
            settings=qr_settings(),
            current_user=SimpleNamespace(id=uuid4()),
            connection_repo=QuotaRejectingRepo(),
            legacy_repo=FakeLegacyRepo(),
            subscription_repo=FakeSubscriptionRepo(),
        )

    assert raised.value.status_code == 403
    assert raised.value.detail == "current plan allows 1 Telegram group"


class FakeSessionContext:
    async def __aenter__(self):
        return object()

    async def __aexit__(self, *_):
        return None


class FakeQrRepository:
    def __init__(self, attempt):
        self.attempt = attempt
        self.qr_url_encrypted = None
        self.completed = None

    async def set_qr_url_encrypted(self, *, attempt_id, qr_url_encrypted):
        assert attempt_id == self.attempt.id
        self.qr_url_encrypted = qr_url_encrypted
        return self.attempt

    async def complete_mtproto_qr_attempt(self, **kwargs):
        self.completed = kwargs
        return SimpleNamespace(id=uuid4())


class FakeQrLogin:
    url = "tg://login?token=sensitive-qr-grant"

    async def wait(self):
        return None


class FakeTelegramClient:
    def __init__(self, *_):
        self.session = SimpleNamespace(save=lambda: "sensitive-session")
        self.disconnected = False

    async def connect(self):
        return None

    async def qr_login(self):
        return FakeQrLogin()

    async def get_me(self):
        return SimpleNamespace(id=42, first_name="Test")

    async def disconnect(self):
        self.disconnected = True


@pytest.mark.asyncio
async def test_qr_worker_encrypts_grant_and_session_before_persistence(monkeypatch) -> None:
    settings = qr_settings()
    attempt = pending_attempt(uuid4())
    repo = FakeQrRepository(attempt)
    monkeypatch.setattr(telegram_mtproto_qr_worker, "TelegramClient", FakeTelegramClient)
    monkeypatch.setattr(telegram_mtproto_qr_worker, "AsyncSessionLocal", FakeSessionContext)
    monkeypatch.setattr(
        telegram_mtproto_qr_worker,
        "TelegramConnectionRepository",
        lambda _: repo,
    )

    pending = await telegram_mtproto_qr_worker.start_pending_login(attempt.id, settings)
    assert pending is not None
    await pending.task

    assert repo.qr_url_encrypted != FakeQrLogin.url
    assert decrypt_secret(repo.qr_url_encrypted, settings) == FakeQrLogin.url
    assert repo.completed is not None
    encrypted_session = repo.completed["credential_encrypted"]
    assert encrypted_session != "sensitive-session"
    assert decrypt_secret(encrypted_session, settings) == "sensitive-session"
    assert pending.client.disconnected is True
