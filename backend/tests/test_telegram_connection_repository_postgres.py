import os
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import TelegramConnectionType
from app.infrastructure.db.models import (
    TelegramConnection,
    TelegramConnectionAttempt,
    TelegramSource,
    TelegramWebhookEvent,
    User,
)
from app.infrastructure.db.repositories import TelegramConnectionRepository


TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL Telegram connection tests",
)


@pytest.fixture
async def connection_subject() -> AsyncIterator[tuple[async_sessionmaker[AsyncSession], User, int]]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    user = User(email=f"telegram-connection-{os.urandom(8).hex()}@example.test")
    async with session_factory() as session:
        session.add(user)
        await session.commit()

    webhook_update_id = int.from_bytes(os.urandom(6), "big")
    yield session_factory, user, webhook_update_id

    async with session_factory() as session:
        await session.exec(
            delete(TelegramWebhookEvent).where(TelegramWebhookEvent.update_id == webhook_update_id)
        )
        await session.exec(delete(TelegramConnectionAttempt).where(TelegramConnectionAttempt.owner_user_id == user.id))
        await session.exec(delete(TelegramSource).where(TelegramSource.owner_user_id == user.id))
        await session.exec(delete(TelegramConnection).where(TelegramConnection.owner_user_id == user.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


async def test_webhook_event_deduplication_reuses_completed_update(connection_subject) -> None:
    session_factory, _, webhook_update_id = connection_subject
    async with session_factory() as session:
        repo = TelegramConnectionRepository(session)
        first, should_process = await repo.reserve_webhook_event(
            update_id=webhook_update_id,
            payload_hash="a" * 64,
            event_type="message",
        )
        assert should_process is True
        await repo.finish_webhook_event(event=first)

        repeated, should_process_repeated = await repo.reserve_webhook_event(
            update_id=webhook_update_id,
            payload_hash="a" * 64,
            event_type="message",
        )

    assert repeated.id == first.id
    assert should_process_repeated is False


async def test_connection_attempt_is_owner_scoped(connection_subject) -> None:
    session_factory, user, _ = connection_subject
    group_request_id = int.from_bytes(os.urandom(4), "big") % 1_000_000_000 + 1
    async with session_factory() as session:
        repo = TelegramConnectionRepository(session)
        attempt = await repo.create_attempt(
            owner_user_id=user.id,
            connection_type=TelegramConnectionType.BOT_CHAT,
            token_hash=os.urandom(32).hex(),
            expires_at=datetime.now(UTC) + timedelta(minutes=10),
            group_request_id=group_request_id,
            channel_request_id=group_request_id + 1_000_000_000,
        )

        assert await repo.get_attempt_for_owner(owner_user_id=user.id, attempt_id=attempt.id)
        assert await repo.get_attempt_for_owner(owner_user_id=uuid4(), attempt_id=attempt.id) is None


async def test_mtproto_qr_pending_attempt_is_reused_and_owner_scoped(connection_subject) -> None:
    session_factory, user, _ = connection_subject
    async with session_factory() as session:
        repo = TelegramConnectionRepository(session)
        attempt = await repo.create_attempt(
            owner_user_id=user.id,
            connection_type=TelegramConnectionType.MTPROTO_QR,
            token_hash=os.urandom(32).hex(),
            expires_at=datetime.now(UTC) + timedelta(minutes=10),
        )

        assert (
            await repo.get_pending_attempt_for_owner(
                owner_user_id=user.id,
                connection_type=TelegramConnectionType.MTPROTO_QR,
            )
        ).id == attempt.id
        assert (
            await repo.get_pending_attempt_for_owner(
                owner_user_id=uuid4(),
                connection_type=TelegramConnectionType.MTPROTO_QR,
            )
            is None
        )

        with pytest.raises(IntegrityError):
            await repo.create_attempt(
                owner_user_id=user.id,
                connection_type=TelegramConnectionType.MTPROTO_QR,
                token_hash=os.urandom(32).hex(),
                expires_at=datetime.now(UTC) + timedelta(minutes=10),
            )
