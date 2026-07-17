import os
from collections.abc import AsyncIterator
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    IMChannel,
    MessageDirection,
    OpportunityStatus,
    Priority,
    SyncAggregateType,
    SyncOperation,
)
from app.infrastructure.db.models import (
    Message,
    Opportunity,
    SyncChange,
    User,
    UserDetectionPreference,
)
from app.infrastructure.db.repositories import UserSettingsRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for sync capture tests",
)


@pytest.fixture
async def sync_subject() -> AsyncIterator[tuple[async_sessionmaker[AsyncSession], User]]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"sync-capture-{uuid4()}@example.test")
    async with factory() as session:
        session.add(owner)
        await session.commit()

    yield factory, owner

    async with factory() as session:
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id == owner.id))
        await session.exec(
            delete(UserDetectionPreference).where(UserDetectionPreference.user_id == owner.id)
        )
        await session.exec(delete(Message).where(Message.owner_user_id == owner.id))
        await session.exec(delete(Opportunity).where(Opportunity.owner_user_id == owner.id))
        await session.exec(delete(User).where(User.id == owner.id))
        await session.commit()
    await engine.dispose()


def incoming(owner: User, *, external_id: str | None = None) -> Message:
    return Message(
        owner_user_id=owner.id,
        channel=IMChannel.TELEGRAM,
        external_message_id=external_id or f"sync-message-{uuid4()}",
        conversation_id=f"sync-conversation-{uuid4()}",
        direction=MessageDirection.INCOMING,
        sender_display_name="Sync customer",
        text="Need a quote",
        raw_payload={"providerSecret": "must-not-enter-sync-feed"},
    )


def opportunity(owner: User, source_message: Message) -> Opportunity:
    return Opportunity(
        owner_user_id=owner.id,
        channel=IMChannel.TELEGRAM,
        conversation_id=source_message.conversation_id,
        source_message_id=source_message.id,
        title="Quote request",
        summary="Customer requested a quote",
        matched_keywords=["quote"],
        raw_message_links=[],
        confidence=0.9,
        priority=Priority.HIGH,
        status=OpportunityStatus.PENDING_HUMAN,
        last_message_preview="Need a quote",
    )


async def test_create_and_update_append_public_payloads_in_the_same_commit(sync_subject) -> None:
    factory, owner = sync_subject
    message = incoming(owner)
    item = opportunity(owner, message)

    async with factory() as session:
        session.add(message)
        await session.commit()
        session.add(item)
        await session.commit()

        message.opportunity_id = item.id
        message.text = "Need a formal quote"
        item.summary = "Formal quote requested"
        session.add_all([message, item])
        await session.commit()

        changes = list(
            (
                await session.exec(
                    select(SyncChange)
                    .where(SyncChange.owner_user_id == owner.id)
                    .order_by(SyncChange.cursor)
                )
            ).all()
        )

    assert message.aggregate_version == 2
    assert item.aggregate_version == 2
    assert len(changes) == 4
    assert sorted(
        (change.aggregate_type.value, change.aggregate_version) for change in changes
    ) == [
        (SyncAggregateType.MESSAGE.value, 1),
        (SyncAggregateType.MESSAGE.value, 2),
        (SyncAggregateType.OPPORTUNITY.value, 1),
        (SyncAggregateType.OPPORTUNITY.value, 2),
    ]
    message_change = next(
        change
        for change in changes
        if change.aggregate_type == SyncAggregateType.MESSAGE
        and change.aggregate_version == 2
    )
    assert message_change.payload is not None
    assert message_change.payload["content"] == "Need a formal quote"
    assert message_change.payload["opportunityId"] == str(item.id)
    assert "providerSecret" not in str(message_change.payload)


async def test_user_setting_uses_owner_as_stable_aggregate_identity_and_delete_tombstone(
    sync_subject,
) -> None:
    factory, owner = sync_subject
    async with factory() as session:
        preference = await UserSettingsRepository(session).upsert_detection(
            user_id=owner.id,
            keywords=["quote"],
            ai_semantics_enabled=True,
        )
        assert preference.aggregate_version == 1
        await session.delete(preference)
        await session.commit()
        changes = list(
            (
                await session.exec(
                    select(SyncChange)
                    .where(
                        SyncChange.owner_user_id == owner.id,
                        SyncChange.aggregate_type
                        == SyncAggregateType.USER_DETECTION_PREFERENCE,
                    )
                    .order_by(SyncChange.cursor)
                )
            ).all()
        )

    assert [change.aggregate_id for change in changes] == [owner.id, owner.id]
    assert [change.aggregate_version for change in changes] == [1, 2]
    assert changes[0].operation == SyncOperation.UPSERT
    assert changes[0].payload == {"keywords": ["quote"], "aiSemanticsEnabled": True}
    assert changes[1].operation == SyncOperation.DELETE
    assert changes[1].payload is None


async def test_failed_business_write_rolls_back_its_change(sync_subject) -> None:
    factory, owner = sync_subject
    external_id = f"duplicate-{uuid4()}"
    async with factory() as session:
        session.add(incoming(owner, external_id=external_id))
        await session.commit()

        session.add(incoming(owner, external_id=external_id))
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        messages = list(
            (
                await session.exec(
                    select(Message).where(
                        Message.owner_user_id == owner.id,
                        Message.external_message_id == external_id,
                    )
                )
            ).all()
        )
        changes = list(
            (
                await session.exec(
                    select(SyncChange).where(
                        SyncChange.owner_user_id == owner.id,
                        SyncChange.aggregate_type == SyncAggregateType.MESSAGE,
                    )
                )
            ).all()
        )

    assert len(messages) == 1
    assert len(changes) == 1


async def test_concurrent_same_version_is_rejected_instead_of_silently_forking(
    sync_subject,
) -> None:
    factory, owner = sync_subject
    message = incoming(owner)
    async with factory() as session:
        session.add(message)
        await session.commit()

    first = factory()
    second = factory()
    try:
        first_message = await first.get(Message, message.id)
        second_message = await second.get(Message, message.id)
        assert first_message is not None and second_message is not None
        first_message.text = "first update"
        second_message.text = "second update"
        await first.commit()
        with pytest.raises(IntegrityError):
            await second.commit()
        await second.rollback()
    finally:
        await first.close()
        await second.close()

    async with factory() as session:
        changes = list(
            (
                await session.exec(
                    select(SyncChange)
                    .where(
                        SyncChange.owner_user_id == owner.id,
                        SyncChange.aggregate_type == SyncAggregateType.MESSAGE,
                    )
                    .order_by(SyncChange.aggregate_version)
                )
            ).all()
        )
    assert [change.aggregate_version for change in changes] == [1, 2]
