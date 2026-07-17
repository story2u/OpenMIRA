import os
from collections.abc import AsyncIterator
from datetime import timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.application.use_cases.sync_feed import InvalidSyncPageToken, SyncFeedService
from app.core.config import Settings
from app.domain.enums import (
    DevicePlatform,
    IMChannel,
    MessageDirection,
    SyncAggregateType,
    SyncOperation,
)
from app.infrastructure.db.models import (
    Device,
    Message,
    SyncChange,
    User,
    UserDetectionPreference,
    utc_now,
)
from app.infrastructure.db.repositories import UserSettingsRepository
from app.infrastructure.db.sync_repository import (
    SyncCursorAheadError,
    SyncDeviceUnavailableError,
    SyncFeedRepository,
)

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for sync feed tests",
)


def settings() -> Settings:
    assert TEST_DATABASE_URL
    return Settings(
        database_url=TEST_DATABASE_URL,
        admin_api_token="test-admin-token",
        jwt_secret_key="sync-feed-test-secret",
        sync_change_retention_days=30,
        sync_bootstrap_token_minutes=60,
    )


def device(owner: User) -> Device:
    return Device(
        owner_user_id=owner.id,
        installation_id_hash=uuid4().hex + uuid4().hex,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={"sync.schema": 1},
    )


def message(owner: User, index: int) -> Message:
    return Message(
        owner_user_id=owner.id,
        channel=IMChannel.TELEGRAM,
        external_message_id=f"sync-feed-{owner.id}-{index}",
        conversation_id=f"conversation-{owner.id}",
        direction=MessageDirection.INCOMING,
        sender_display_name="Customer",
        text=f"Message {index}",
    )


@pytest.fixture
async def sync_feed_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, User, Device, Device]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"sync-feed-owner-{uuid4()}@example.test")
    other = User(email=f"sync-feed-other-{uuid4()}@example.test")
    async with factory() as session:
        session.add_all([owner, other])
        await session.commit()
        owner_device = device(owner)
        other_device = device(other)
        session.add_all([owner_device, other_device])
        await session.commit()
        session.add_all([message(owner, 1), message(owner, 2), message(other, 1)])
        await session.commit()
        await UserSettingsRepository(session).upsert_detection(
            user_id=owner.id,
            keywords=["quote"],
            ai_semantics_enabled=True,
        )

    yield factory, owner, other, owner_device, other_device

    async with factory() as session:
        owner_ids = [owner.id, other.id]
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id.in_(owner_ids)))
        await session.exec(delete(Message).where(Message.owner_user_id.in_(owner_ids)))
        await session.exec(
            delete(UserDetectionPreference).where(
                UserDetectionPreference.user_id.in_(owner_ids)
            )
        )
        await session.exec(delete(Device).where(Device.owner_user_id.in_(owner_ids)))
        await session.exec(delete(User).where(User.id.in_(owner_ids)))
        await session.commit()
    await engine.dispose()


async def collect_bootstrap(
    service: SyncFeedService,
    owner_id,
    *,
    limit: int,
):
    token = None
    watermark = None
    items = []
    while True:
        page = await service.bootstrap(
            owner_user_id=owner_id,
            limit=limit,
            page_token=token,
        )
        watermark = page.watermarkCursor if watermark is None else watermark
        assert page.watermarkCursor == watermark
        items.extend(page.items)
        if not page.hasMore:
            assert page.nextPageToken is None
            return watermark, items
        assert page.nextPageToken
        token = page.nextPageToken


async def test_bootstrap_is_bounded_owner_scoped_and_has_stable_defaults(
    sync_feed_subject,
) -> None:
    factory, owner, other, _, _ = sync_feed_subject
    async with factory() as session:
        service = SyncFeedService(SyncFeedRepository(session), settings())
        watermark, items = await collect_bootstrap(service, owner.id, limit=2)
        other_page = await service.bootstrap(
            owner_user_id=other.id,
            limit=20,
            page_token=None,
        )

    assert watermark > 0
    assert len(items) == 5
    assert [item.aggregateType for item in items[:3]] == [
        SyncAggregateType.USER_DETECTION_PREFERENCE,
        SyncAggregateType.USER_WORK_SCHEDULE,
        SyncAggregateType.USER_NOTIFICATION_PREFERENCE,
    ]
    detection = items[0]
    work_schedule = items[1]
    notifications = items[2]
    assert detection.aggregateId == owner.id
    assert detection.aggregateVersion == 1
    assert detection.payload == {"keywords": ["quote"], "aiSemanticsEnabled": True}
    assert work_schedule.aggregateVersion == 0
    assert work_schedule.payload["isDefault"] is True
    assert notifications.aggregateVersion == 0
    assert sum(item.aggregateType == SyncAggregateType.MESSAGE for item in items) == 2
    assert sum(
        item.aggregateType == SyncAggregateType.MESSAGE for item in other_page.items
    ) == 1


async def test_bootstrap_token_is_owner_bound(sync_feed_subject) -> None:
    factory, owner, other, _, _ = sync_feed_subject
    async with factory() as session:
        service = SyncFeedService(SyncFeedRepository(session), settings())
        first = await service.bootstrap(owner_user_id=owner.id, limit=1, page_token=None)
        assert first.nextPageToken
        with pytest.raises(InvalidSyncPageToken):
            await service.bootstrap(
                owner_user_id=other.id,
                limit=1,
                page_token=first.nextPageToken,
            )


async def test_changes_page_and_retention_reset_are_explicit(sync_feed_subject) -> None:
    factory, owner, other, _, _ = sync_feed_subject
    async with factory() as session:
        repository = SyncFeedRepository(session)
        current = await repository.latest_cursor(owner.id)
        old = SyncChange(
            owner_user_id=owner.id,
            aggregate_type=SyncAggregateType.MESSAGE,
            aggregate_id=uuid4(),
            aggregate_version=1,
            operation=SyncOperation.DELETE,
            payload=None,
            created_at=utc_now() - timedelta(days=31),
        )
        session.add(old)
        await session.commit()
        await session.refresh(old)
        assert old.cursor is not None and old.cursor > current

        service = SyncFeedService(repository, settings())
        reset = await service.changes(owner_user_id=owner.id, after=0, limit=20)
        after_old = await service.changes(
            owner_user_id=owner.id,
            after=int(old.cursor),
            limit=1,
        )
        ahead = await service.changes(
            owner_user_id=owner.id,
            after=reset.serverCursor + 1,
            limit=20,
        )
        other_changes = await service.changes(
            owner_user_id=other.id,
            after=0,
            limit=20,
        )

    assert reset.resetRequired is True
    assert reset.resetReason == "cursor_expired"
    assert reset.changes == []
    assert after_old.resetRequired is False
    assert after_old.changes == []
    assert ahead.resetReason == "cursor_ahead"
    assert all(change.aggregateId != old.aggregate_id for change in other_changes.changes)


async def test_ack_is_monotonic_bounded_and_device_scoped(sync_feed_subject) -> None:
    factory, owner, _, owner_device, other_device = sync_feed_subject
    async with factory() as session:
        repository = SyncFeedRepository(session)
        latest = await repository.latest_cursor(owner.id)
        acknowledged = await repository.acknowledge(
            owner_user_id=owner.id,
            device_id=owner_device.id,
            cursor=latest,
            error_code=None,
        )
        repeated = await repository.acknowledge(
            owner_user_id=owner.id,
            device_id=owner_device.id,
            cursor=max(0, latest - 1),
            error_code="projection.retry",
        )
        acknowledged_cursor = acknowledged.last_sync_cursor
        repeated_cursor = repeated.last_sync_cursor
        repeated_error = repeated.last_sync_error_code
        with pytest.raises(SyncCursorAheadError):
            await repository.acknowledge(
                owner_user_id=owner.id,
                device_id=owner_device.id,
                cursor=latest + 1,
                error_code=None,
            )
        with pytest.raises(SyncDeviceUnavailableError):
            await repository.acknowledge(
                owner_user_id=owner.id,
                device_id=other_device.id,
                cursor=latest,
                error_code=None,
            )

    assert acknowledged_cursor == latest
    assert repeated_cursor == latest
    assert repeated_error == "projection.retry"
