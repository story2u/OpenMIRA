import os
from collections.abc import AsyncIterator

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import PlanCode, WeComConnectionStatus, WeComSourceType
from app.domain.services.subscription_policy import GroupQuotaExceeded, get_plan_entitlements
from app.infrastructure.db.models import (
    TelegramMonitor,
    TelegramUserConfig,
    User,
    WeComArchiveConnection,
    WeComArchiveCursor,
    WeComArchiveEvent,
    WeComArchiveMemberBinding,
    WeComSource,
)
from app.infrastructure.db.repositories import (
    TelegramUserConfigRepository,
    WeComArchiveRepository,
)

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL WeCom archive tests",
)


@pytest.fixture
async def archive_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, WeComArchiveConnection]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    user = User(email=f"wecom-archive-{os.urandom(8).hex()}@example.test")
    async with factory() as session:
        session.add(user)
        await session.commit()
        connection, _, _ = await WeComArchiveRepository(session).create_with_owner_binding(
            connection=WeComArchiveConnection(
                owner_user_id=user.id,
                status=WeComConnectionStatus.ACTIVE,
                corp_id=f"ww-{os.urandom(8).hex()}",
                secret_encrypted="encrypted-secret",
                private_key_encrypted="encrypted-private-key",
                public_key_version=1,
            ),
            wecom_user_id="member-a",
            member_display_name="成员 A",
        )

    yield factory, user, connection

    async with factory() as session:
        await session.exec(
            delete(WeComSource).where(WeComSource.archive_connection_id == connection.id)
        )
        await session.exec(
            delete(WeComArchiveEvent).where(
                WeComArchiveEvent.connection_id == connection.id
            )
        )
        await session.exec(
            delete(WeComArchiveCursor).where(
                WeComArchiveCursor.connection_id == connection.id
            )
        )
        await session.exec(
            delete(WeComArchiveMemberBinding).where(
                WeComArchiveMemberBinding.connection_id == connection.id
            )
        )
        await session.exec(
            delete(WeComArchiveConnection).where(
                WeComArchiveConnection.id == connection.id
            )
        )
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


async def test_archive_cursor_event_and_source_are_idempotent(archive_subject) -> None:
    factory, user, connection = archive_subject
    async with factory() as session:
        repository = WeComArchiveRepository(session)
        cursor = await repository.acquire_poll_lease(connection.id, lease_seconds=120)
        assert cursor and cursor.last_seq == 0
        assert await repository.acquire_poll_lease(connection.id, lease_seconds=120) is None

        source_one = await repository.ensure_archive_source(
            connection_id=connection.id,
            owner_user_id=user.id,
            external_conversation_id="room:group-1",
            display_name="group-1",
            source_type=WeComSourceType.INTERNAL_GROUP,
        )
        source_two = await repository.ensure_archive_source(
            connection_id=connection.id,
            owner_user_id=user.id,
            external_conversation_id="room:group-1",
            display_name="group-1",
            source_type=WeComSourceType.INTERNAL_GROUP,
        )
        assert source_one.id == source_two.id
        assert source_one.connection_id is None
        assert source_one.archive_connection_id == connection.id

        event, should_process = await repository.reserve_event(
            connection_id=connection.id,
            provider_message_id="message-1",
            sequence=1,
            message_type="text",
            payload_hash="a" * 64,
        )
        assert should_process is True
        await repository.complete_event(event, matched_user_count=1)
        repeated, should_process_again = await repository.reserve_event(
            connection_id=connection.id,
            provider_message_id="message-1",
            sequence=1,
            message_type="text",
            payload_hash="a" * 64,
        )
        assert repeated.id == event.id
        assert should_process_again is False

        await repository.finish_poll(
            connection=connection,
            cursor=cursor,
            last_sequence=1,
            batch_size=1,
        )
        refreshed = await repository.cursor_for_connection(connection.id)
        assert refreshed and refreshed.last_seq == 1

        await repository.disable_and_clear_secrets(
            connection, cleared_secret="cleared"
        )
        assert await repository.list_for_owner(user.id) == []
        recreated, binding, recreated_cursor = await repository.create_with_owner_binding(
            connection=WeComArchiveConnection(
                owner_user_id=user.id,
                corp_id=connection.corp_id,
                secret_encrypted="new-encrypted-secret",
                private_key_encrypted="new-encrypted-private-key",
                public_key_version=2,
            ),
            wecom_user_id="member-a",
            member_display_name="成员 A",
        )
        assert recreated.id == connection.id
        assert recreated.enabled is True
        assert recreated.status == WeComConnectionStatus.PENDING
        assert binding.enabled is True
        assert recreated_cursor.last_seq == 1


async def test_telegram_monitor_creation_counts_active_wecom_groups(
    archive_subject,
) -> None:
    factory, user, connection = archive_subject
    async with factory() as session:
        archive_repository = WeComArchiveRepository(session)
        for index in range(10):
            await archive_repository.ensure_archive_source(
                connection_id=connection.id,
                owner_user_id=user.id,
                external_conversation_id=f"room:group-{index}",
                display_name=f"group-{index}",
                source_type=WeComSourceType.INTERNAL_GROUP,
            )
        config = TelegramUserConfig(user_id=user.id, enabled=True)
        session.add(config)
        await session.commit()
        await session.refresh(config)

        with pytest.raises(GroupQuotaExceeded, match="10 monitored groups in total"):
            await TelegramUserConfigRepository(session).replace_monitors_for_user(
                user_id=user.id,
                telegram_config_id=config.id,
                chats=["telegram-group-1"],
                enabled=True,
                backfill_limit=30,
                entitlements=get_plan_entitlements(PlanCode.PLUS),
            )

        await session.exec(
            delete(TelegramMonitor).where(
                TelegramMonitor.telegram_config_id == config.id
            )
        )
        await session.exec(
            delete(TelegramUserConfig).where(TelegramUserConfig.id == config.id)
        )
        await session.commit()
