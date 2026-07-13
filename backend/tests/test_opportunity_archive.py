import os
from collections.abc import AsyncIterator
from types import SimpleNamespace
from uuid import uuid4

import pytest
from fastapi import HTTPException
from sqlalchemy import delete, func
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost:5432/im")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.v1.routes.opportunities import ensure_opportunity_is_active
from app.domain.enums import (
    IMChannel,
    OpportunityArchiveScope,
    OpportunityStatus,
    Priority,
)
from app.infrastructure.db.models import Opportunity, OpportunityArchiveEvent, User
from app.infrastructure.db.repositories import OpportunityRepository


TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL opportunity archive tests",
)


def make_opportunity(owner_user_id, *, title: str) -> Opportunity:
    return Opportunity(
        owner_user_id=owner_user_id,
        channel=IMChannel.TELEGRAM,
        conversation_id=f"conversation-{uuid4()}",
        title=title,
        status=OpportunityStatus.FOLLOWING,
        priority=Priority.HIGH,
        last_message_preview=title,
    )


@pytest.fixture
async def archive_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, User, Opportunity, Opportunity]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"archive-owner-{os.urandom(8).hex()}@example.test")
    other_owner = User(email=f"archive-other-{os.urandom(8).hex()}@example.test")
    async with session_factory() as session:
        session.add(owner)
        session.add(other_owner)
        await session.commit()

        owned = make_opportunity(owner.id, title="owned")
        foreign = make_opportunity(other_owner.id, title="foreign")
        session.add(owned)
        session.add(foreign)
        await session.commit()

    yield session_factory, owner, other_owner, owned, foreign

    async with session_factory() as session:
        opportunity_ids = [owned.id, foreign.id]
        await session.exec(
            delete(OpportunityArchiveEvent).where(
                OpportunityArchiveEvent.opportunity_id.in_(opportunity_ids)
            )
        )
        await session.exec(delete(Opportunity).where(Opportunity.id.in_(opportunity_ids)))
        await session.exec(delete(User).where(User.id.in_([owner.id, other_owner.id])))
        await session.commit()
    await engine.dispose()


async def test_archive_is_idempotent_and_preserves_business_status(archive_subject) -> None:
    session_factory, owner, _, owned, _ = archive_subject
    async with session_factory() as session:
        repo = OpportunityRepository(session)
        current = await repo.get(owned.id)
        assert current
        archived = await repo.archive(current, actor_user_id=owner.id, reason="已完成跟进")
        archived_again = await repo.archive(archived, actor_user_id=owner.id, reason="重复请求")
        event_count = await session.scalar(
            select(func.count()).select_from(OpportunityArchiveEvent).where(
                OpportunityArchiveEvent.opportunity_id == owned.id
            )
        )

        assert archived_again.archived_at is not None
        assert archived_again.archive_reason == "已完成跟进"
        assert archived_again.status == OpportunityStatus.FOLLOWING
        assert event_count == 1


async def test_archive_scope_defaults_to_active_and_restore_keeps_status(archive_subject) -> None:
    session_factory, owner, _, owned, _ = archive_subject
    async with session_factory() as session:
        repo = OpportunityRepository(session)
        current = await repo.get(owned.id)
        assert current
        await repo.archive(current, actor_user_id=owner.id, reason=None)

        assert await repo.list(owner_user_id=owner.id) == []
        archived = await repo.list(
            owner_user_id=owner.id,
            archive_scope=OpportunityArchiveScope.ARCHIVED,
        )
        all_records = await repo.list(
            owner_user_id=owner.id,
            archive_scope=OpportunityArchiveScope.ALL,
        )
        dashboard_items, dashboard_total = await repo.dashboard(owner_user_id=owner.id)
        pending_count = await repo.count_pending(owner.id)
        attention_items = await repo.list_attention(owner.id)
        restored = await repo.restore(archived[0], actor_user_id=owner.id)

        assert [item.id for item in archived] == [owned.id]
        assert [item.id for item in all_records] == [owned.id]
        assert dashboard_items == []
        assert dashboard_total == 0
        assert pending_count == 0
        assert attention_items == []
        assert restored.archived_at is None
        assert restored.status == OpportunityStatus.FOLLOWING


async def test_bulk_archive_rejects_foreign_id_without_partial_update(archive_subject) -> None:
    session_factory, owner, _, owned, foreign = archive_subject
    async with session_factory() as session:
        repo = OpportunityRepository(session)
        with pytest.raises(LookupError):
            await repo.archive_many(
                owner_user_id=owner.id,
                opportunity_ids=[owned.id, foreign.id],
                reason="batch",
            )

        owned_after = await repo.get(owned.id)
        foreign_after = await repo.get(foreign.id)
        assert owned_after and owned_after.archived_at is None
        assert foreign_after and foreign_after.archived_at is None


def test_archived_opportunity_must_be_restored_before_business_mutation() -> None:
    archived = SimpleNamespace(archived_at=object())

    with pytest.raises(HTTPException) as raised:
        ensure_opportunity_is_active(archived)

    assert raised.value.status_code == 409
