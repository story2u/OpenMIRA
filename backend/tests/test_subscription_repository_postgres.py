import asyncio
import os
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    IMChannel,
    MessageDirection,
    PlanCode,
    SubscriptionStatus,
)
from app.domain.services.subscription_policy import GroupQuotaExceeded, get_plan_entitlements
from app.infrastructure.db.models import (
    Message,
    SubscriptionAccount,
    TelegramMonitor,
    TelegramUserConfig,
    UsageLedger,
    User,
)
from app.infrastructure.db.repositories import SubscriptionRepository, TelegramUserConfigRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL quota tests",
)


@pytest.fixture
async def quota_subject() -> AsyncIterator[tuple[async_sessionmaker[AsyncSession], User, Message]]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL, pool_size=10, max_overflow=20)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    user = User(email=f"quota-{os.urandom(8).hex()}@example.test")
    message = Message(
        owner_user_id=user.id,
        channel=IMChannel.TELEGRAM,
        external_message_id=f"quota-{os.urandom(8).hex()}",
        conversation_id="quota-test",
        direction=MessageDirection.INCOMING,
    )
    async with session_factory() as session:
        session.add(user)
        await session.commit()
        session.add(message)
        await session.commit()

    yield session_factory, user, message

    async with session_factory() as session:
        await session.exec(delete(UsageLedger).where(UsageLedger.user_id == user.id))
        await session.exec(delete(TelegramMonitor).where(TelegramMonitor.user_id == user.id))
        await session.exec(
            delete(TelegramUserConfig).where(TelegramUserConfig.user_id == user.id)
        )
        await session.exec(
            delete(SubscriptionAccount).where(SubscriptionAccount.user_id == user.id)
        )
        await session.exec(delete(Message).where(Message.owner_user_id == user.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


async def test_free_quota_is_atomic_under_concurrent_reservations(quota_subject) -> None:
    session_factory, user, _ = quota_subject
    messages = [
        Message(
            owner_user_id=user.id,
            channel=IMChannel.TELEGRAM,
            external_message_id=f"concurrent-{os.urandom(8).hex()}",
            conversation_id="quota-concurrent",
            direction=MessageDirection.INCOMING,
        )
        for _ in range(105)
    ]
    async with session_factory() as session:
        session.add_all(messages)
        await session.commit()

    async def reserve(index: int) -> bool:
        async with session_factory() as session:
            result = await SubscriptionRepository(session).reserve_agent_analysis(
                user_id=user.id,
                message_id=messages[index].id,
                idempotency_key=f"concurrent-{index}",
            )
            return result.allowed

    results = await asyncio.gather(*(reserve(index) for index in range(105)))

    assert sum(results) == 100


async def test_repeated_idempotency_key_only_allocates_once(quota_subject) -> None:
    session_factory, user, message = quota_subject

    async with session_factory() as session:
        first = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="same-request",
        )
    async with session_factory() as session:
        repeated = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="same-request",
        )

    assert first.allowed is True and first.created is True
    assert repeated.allowed is True and repeated.created is False
    assert repeated.allocated == 1

    assert first.ledger is not None
    async with session_factory() as session:
        repo = SubscriptionRepository(session)
        await repo.consume_usage(first.ledger.id)
        snapshot = await repo.get_snapshot(user.id)
        assert await repo.usage_counts(user_id=user.id, period=snapshot.period) == (1, 0)

        second = await repo.reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="released-request",
        )
        assert second.ledger is not None
        await repo.release_usage(second.ledger.id, "provider unavailable")
        assert await repo.usage_counts(user_id=user.id, period=snapshot.period) == (1, 0)


async def test_different_keys_share_one_active_message_reservation(quota_subject) -> None:
    session_factory, user, message = quota_subject

    async with session_factory() as session:
        first = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="automatic",
        )
    async with session_factory() as session:
        second = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="manual-different-key",
        )

    assert first.ledger is not None and second.ledger is not None
    assert first.created is True
    assert second.created is False
    assert second.ledger.id == first.ledger.id


async def test_annual_subscription_uses_utc_month_for_usage_and_upgrade_keeps_usage(quota_subject) -> None:
    session_factory, user, message = quota_subject
    now = datetime(2026, 7, 15, 12, tzinfo=UTC)
    async with session_factory() as session:
        account = SubscriptionAccount(
            user_id=user.id,
            plan_code=PlanCode.PLUS,
            status=SubscriptionStatus.ACTIVE,
            current_period_start=datetime(2026, 1, 1, tzinfo=UTC),
            current_period_end=datetime(2027, 1, 1, tzinfo=UTC),
        )
        session.add(account)
        await session.commit()
        repo = SubscriptionRepository(session)

        first = await repo.reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="annual-plus-july",
            now=now,
        )
        assert first.ledger is not None
        assert first.ledger.period_start == datetime(2026, 7, 1, tzinfo=UTC)
        assert first.ledger.period_end == datetime(2026, 8, 1, tzinfo=UTC)
        await repo.consume_usage(first.ledger.id)

        account.plan_code = PlanCode.PRO
        account.updated_at = now
        session.add(account)
        await session.commit()
        upgraded = await repo.reserve_agent_analysis(
            user_id=user.id,
            message_id=message.id,
            idempotency_key="annual-pro-july",
            now=now,
        )

        assert upgraded.allowed is True
        assert upgraded.allocated == 2
        snapshot = await repo.get_snapshot(user.id, now=now)
        assert snapshot.plan_code == PlanCode.PRO
        assert snapshot.billing_period_start == datetime(2026, 1, 1, tzinfo=UTC)
        assert snapshot.billing_period_end == datetime(2027, 1, 1, tzinfo=UTC)
        assert snapshot.usage_period_start == datetime(2026, 7, 1, tzinfo=UTC)
        assert snapshot.usage_period_end == datetime(2026, 8, 1, tzinfo=UTC)


async def test_telegram_monitor_write_uses_effective_plan_limit(quota_subject) -> None:
    session_factory, user, _ = quota_subject
    now = datetime.now(UTC)

    async with session_factory() as session:
        config = TelegramUserConfig(user_id=user.id)
        session.add(config)
        await session.commit()
        await session.refresh(config)
        config_id = config.id
        telegram_repo = TelegramUserConfigRepository(session)

        with pytest.raises(GroupQuotaExceeded, match="1 Telegram group"):
            await telegram_repo.replace_monitors_for_user(
                user_id=user.id,
                telegram_config_id=config_id,
                chats=["group-1", "group-2"],
                enabled=True,
                backfill_limit=30,
                entitlements=get_plan_entitlements(PlanCode.FREE),
            )
        await session.rollback()

        session.add(
            SubscriptionAccount(
                user_id=user.id,
                plan_code=PlanCode.PLUS,
                status=SubscriptionStatus.ACTIVE,
                current_period_start=now - timedelta(days=1),
                current_period_end=now + timedelta(days=29),
            )
        )
        await session.commit()
        snapshot = await SubscriptionRepository(session).get_snapshot(user.id, now=now)
        monitors = await telegram_repo.replace_monitors_for_user(
            user_id=user.id,
            telegram_config_id=config_id,
            chats=[f"group-{index}" for index in range(10)],
            enabled=True,
            backfill_limit=30,
            entitlements=snapshot.entitlements,
        )

    assert snapshot.plan_code == PlanCode.PLUS
    assert len(monitors) == 10

    retained_id = monitors[-1].id
    async with session_factory() as session:
        account = await SubscriptionRepository(session).get_account(user.id)
        assert account is not None
        account.plan_code = PlanCode.FREE
        account.status = SubscriptionStatus.INACTIVE
        session.add(account)
        await session.commit()

        telegram_repo = TelegramUserConfigRepository(session)
        active = await telegram_repo.reconcile_monitor_quota_for_user(
            user_id=user.id,
            capacity=1,
        )
        assert len(active) == 1
        assert len([monitor for monitor in await telegram_repo.list_monitors_by_user(user.id) if monitor.quota_paused]) == 9

        with pytest.raises(ValueError, match="belong to the current user"):
            await telegram_repo.select_retained_monitors(
                user_id=user.id,
                monitor_ids=[uuid4()],
                capacity=1,
            )
        await session.rollback()

        selected = await telegram_repo.select_retained_monitors(
            user_id=user.id,
            monitor_ids=[retained_id],
            capacity=1,
        )
        retained = next(monitor for monitor in selected if monitor.id == retained_id)
        assert retained.quota_paused is False
        assert retained.retention_priority == 100
        config = await telegram_repo.get_by_user(user.id)
        assert config is not None
        assert config.retention_limit == 1
        assert config.retention_selected_at is not None

        active_after_refresh = await telegram_repo.reconcile_monitor_quota_for_user(
            user_id=user.id,
            capacity=1,
        )
        assert [monitor.id for _, monitor in active_after_refresh] == [retained_id]

        retained_chat_id = next(
            monitor.chat_id for monitor in selected if monitor.id == retained_id
        )
        await telegram_repo.replace_monitors_for_user(
            user_id=user.id,
            telegram_config_id=config_id,
            chats=[retained_chat_id],
            enabled=True,
            backfill_limit=30,
            entitlements=get_plan_entitlements(PlanCode.FREE),
        )
        persisted = await telegram_repo.list_monitors_by_user(user.id)
        assert len(persisted) == 10
        config = await telegram_repo.get_by_user(user.id)
        assert config is not None and config.retention_limit == 1
