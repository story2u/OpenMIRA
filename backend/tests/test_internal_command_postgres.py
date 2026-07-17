import hashlib
import os
import asyncio
from collections.abc import AsyncIterator
from uuid import uuid4

import pytest
from sqlalchemy import delete, func
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import IMChannel, OpportunityStatus, Priority, SyncAggregateType
from app.domain.services.opportunity_state import (
    InternalCommandIdempotencyConflict,
    OpportunityVersionConflict,
)
from app.infrastructure.db.models import (
    InternalCommandReceipt,
    Opportunity,
    SyncChange,
    User,
)
from app.infrastructure.db.repositories import OpportunityRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for internal command tests",
)


@pytest.fixture
async def command_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, Opportunity]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"internal-command-{uuid4()}@example.test")
    item = Opportunity(
        owner_user_id=owner.id,
        channel=IMChannel.TELEGRAM,
        conversation_id=f"command-conversation-{uuid4()}",
        title="Internal command target",
        summary="Status can be changed without an external side effect",
        matched_keywords=["status"],
        raw_message_links=[],
        confidence=0.9,
        priority=Priority.NORMAL,
        status=OpportunityStatus.PENDING_HUMAN,
        last_message_preview="status",
    )
    async with factory() as session:
        session.add(owner)
        await session.commit()
        session.add(item)
        await session.commit()

    yield factory, owner, item

    async with factory() as session:
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id == owner.id))
        await session.exec(delete(Opportunity).where(Opportunity.id == item.id))
        await session.exec(delete(User).where(User.id == owner.id))
        await session.commit()
    await engine.dispose()


def status_hash(status: OpportunityStatus) -> str:
    return hashlib.sha256(status.value.encode("utf-8")).hexdigest()


async def test_internal_status_command_is_atomic_versioned_and_idempotent(command_subject) -> None:
    factory, owner, item = command_subject
    key = f"status-{uuid4()}"
    async with factory() as session:
        repository = OpportunityRepository(session)
        first = await repository.transition_status(
            opportunity_id=item.id,
            owner_user_id=owner.id,
            status=OpportunityStatus.FOLLOWING,
            expected_version=1,
            idempotency_key=key,
            payload_hash=status_hash(OpportunityStatus.FOLLOWING),
        )
        assert first.aggregate_version == 2

        changed_again = await repository.transition_status(
            opportunity_id=item.id,
            owner_user_id=owner.id,
            status=OpportunityStatus.REPLIED,
        )
        assert changed_again.aggregate_version == 3

        replay = await repository.transition_status(
            opportunity_id=item.id,
            owner_user_id=owner.id,
            status=OpportunityStatus.FOLLOWING,
            expected_version=1,
            idempotency_key=key,
            payload_hash=status_hash(OpportunityStatus.FOLLOWING),
        )
        receipt_count = await session.scalar(
            select(func.count())
            .select_from(InternalCommandReceipt)
            .where(InternalCommandReceipt.owner_user_id == owner.id)
        )
        changes = list(
            (
                await session.exec(
                    select(SyncChange).where(
                        SyncChange.owner_user_id == owner.id,
                        SyncChange.aggregate_type == SyncAggregateType.OPPORTUNITY,
                    )
                )
            ).all()
        )

    assert replay.status == OpportunityStatus.REPLIED
    assert replay.aggregate_version == 3
    assert receipt_count == 1
    assert sorted(change.aggregate_version for change in changes) == [1, 2, 3]


async def test_internal_status_command_rejects_stale_version_without_receipt(command_subject) -> None:
    factory, owner, item = command_subject
    async with factory() as session:
        repository = OpportunityRepository(session)
        with pytest.raises(OpportunityVersionConflict):
            await repository.transition_status(
                opportunity_id=item.id,
                owner_user_id=owner.id,
                status=OpportunityStatus.FOLLOWING,
                expected_version=99,
                idempotency_key=f"status-{uuid4()}",
                payload_hash=status_hash(OpportunityStatus.FOLLOWING),
            )
        receipt_count = await session.scalar(
            select(func.count())
            .select_from(InternalCommandReceipt)
            .where(InternalCommandReceipt.owner_user_id == owner.id)
        )

    assert receipt_count == 0


async def test_internal_status_command_rejects_idempotency_rebinding(command_subject) -> None:
    factory, owner, item = command_subject
    key = f"status-{uuid4()}"
    async with factory() as session:
        repository = OpportunityRepository(session)
        await repository.transition_status(
            opportunity_id=item.id,
            owner_user_id=owner.id,
            status=OpportunityStatus.FOLLOWING,
            expected_version=1,
            idempotency_key=key,
            payload_hash=status_hash(OpportunityStatus.FOLLOWING),
        )
        with pytest.raises(InternalCommandIdempotencyConflict):
            await repository.transition_status(
                opportunity_id=item.id,
                owner_user_id=owner.id,
                status=OpportunityStatus.CLOSED,
                expected_version=1,
                idempotency_key=key,
                payload_hash=status_hash(OpportunityStatus.CLOSED),
            )


async def test_concurrent_same_internal_command_applies_only_once(command_subject) -> None:
    factory, owner, item = command_subject
    key = f"status-{uuid4()}"

    async def execute() -> int:
        async with factory() as session:
            result = await OpportunityRepository(session).transition_status(
                opportunity_id=item.id,
                owner_user_id=owner.id,
                status=OpportunityStatus.FOLLOWING,
                expected_version=1,
                idempotency_key=key,
                payload_hash=status_hash(OpportunityStatus.FOLLOWING),
            )
            return result.aggregate_version

    versions = await asyncio.gather(execute(), execute())

    async with factory() as session:
        receipt_count = await session.scalar(
            select(func.count())
            .select_from(InternalCommandReceipt)
            .where(InternalCommandReceipt.owner_user_id == owner.id)
        )
        change_count = await session.scalar(
            select(func.count())
            .select_from(SyncChange)
            .where(
                SyncChange.owner_user_id == owner.id,
                SyncChange.aggregate_type == SyncAggregateType.OPPORTUNITY,
            )
        )

    assert versions == [2, 2]
    assert receipt_count == 1
    assert change_count == 2
