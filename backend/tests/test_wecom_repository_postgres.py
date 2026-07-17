import asyncio
import os
from collections.abc import AsyncIterator

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    IMChannel,
    ManualReplyDeliveryStatus,
    WeComConnectionStatus,
    WeComDeliveryStatus,
)
from app.domain.services.opportunity_state import OpportunityClaimConflict
from app.infrastructure.db.models import (
    ManualReplyDelivery,
    Opportunity,
    User,
    WeComConnection,
    WeComOutboundDelivery,
    WeComSource,
    WeComWebhookEvent,
)
from app.infrastructure.db.repositories import (
    ManualReplyDeliveryRepository,
    OpportunityRepository,
    WeComConnectionRepository,
    WeComDeliveryRepository,
    WeComEventRepository,
)

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL WeCom tests",
)


@pytest.fixture
async def wecom_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, WeComConnection, Opportunity]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    user = User(email=f"wecom-{os.urandom(8).hex()}@example.test")
    connection = WeComConnection(
        owner_user_id=user.id,
        status=WeComConnectionStatus.ACTIVE,
        corp_id=f"ww-{os.urandom(8).hex()}",
        agent_id="1000002",
        secret_encrypted="encrypted-secret",
        token_encrypted="encrypted-token",
        aes_key_encrypted="encrypted-key",
    )
    opportunity = Opportunity(
        owner_user_id=user.id,
        channel=IMChannel.WECOM,
        conversation_id=f"wecom:{connection.id}:zhangsan",
        title="采购咨询",
    )
    async with factory() as session:
        session.add(user)
        await session.commit()
        session.add(connection)
        await session.commit()
        session.add(opportunity)
        await session.commit()

    yield factory, user, connection, opportunity

    async with factory() as session:
        await session.exec(
            delete(ManualReplyDelivery).where(ManualReplyDelivery.owner_user_id == user.id)
        )
        await session.exec(
            delete(WeComOutboundDelivery).where(WeComOutboundDelivery.owner_user_id == user.id)
        )
        await session.exec(
            delete(WeComWebhookEvent).where(WeComWebhookEvent.owner_user_id == user.id)
        )
        await session.exec(delete(WeComSource).where(WeComSource.owner_user_id == user.id))
        await session.exec(delete(Opportunity).where(Opportunity.id == opportunity.id))
        await session.exec(delete(WeComConnection).where(WeComConnection.id == connection.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


async def test_event_and_source_are_idempotent_and_payload_is_cleared(wecom_subject) -> None:
    factory, _, connection, _ = wecom_subject
    async with factory() as session:
        connection_repo = WeComConnectionRepository(session)
        source_one = await connection_repo.ensure_private_source(
            connection=connection,
            external_conversation_id="zhangsan",
            display_name="张三",
        )
        source_two = await connection_repo.ensure_private_source(
            connection=connection,
            external_conversation_id="zhangsan",
            display_name="张三",
        )
        assert source_two.id == source_one.id

        event_repo = WeComEventRepository(session)
        event, should_queue = await event_repo.reserve(
            connection=connection,
            provider_event_id="message-001",
            event_type="text",
            payload_hash="a" * 64,
            normalized_payload_encrypted="encrypted-normalized-payload",
        )
        assert should_queue is True
        await event_repo.mark_queued(event)
        assert await event_repo.begin_processing(event.id)
        await event_repo.finish(event.id)

        repeated, should_queue_again = await event_repo.reserve(
            connection=connection,
            provider_event_id="message-001",
            event_type="text",
            payload_hash="a" * 64,
            normalized_payload_encrypted="encrypted-normalized-payload",
        )
        assert repeated.id == event.id
        assert should_queue_again is False
        assert repeated.normalized_payload_encrypted is None


async def test_delivery_idempotency_is_scoped_to_owner(wecom_subject) -> None:
    factory, user, connection, opportunity = wecom_subject
    async with factory() as session:
        source = await WeComConnectionRepository(session).ensure_private_source(
            connection=connection,
            external_conversation_id="zhangsan",
            display_name="张三",
        )
        repo = WeComDeliveryRepository(session)
        first, should_send = await repo.reserve(
            owner_user_id=user.id,
            connection_id=connection.id,
            source_id=source.id,
            opportunity_id=opportunity.id,
            idempotency_key="manual-reply-001",
            content_hash="b" * 64,
        )
        assert should_send is True
        await repo.mark_sending(first)
        await repo.mark_sent(first, "provider-message-001")

        repeated, should_send_again = await repo.reserve(
            owner_user_id=user.id,
            connection_id=connection.id,
            source_id=source.id,
            opportunity_id=opportunity.id,
            idempotency_key="manual-reply-001",
            content_hash="b" * 64,
        )

    assert repeated.id == first.id
    assert repeated.status == WeComDeliveryStatus.SENT
    assert should_send_again is False


async def test_manual_reply_delivery_claim_is_atomic_and_reuses_completed_record(
    wecom_subject,
) -> None:
    factory, user, _, opportunity = wecom_subject
    async with factory() as session:
        repo = ManualReplyDeliveryRepository(session)
        first = await repo.reserve(
            owner_user_id=user.id,
            opportunity_id=opportunity.id,
            idempotency_key="generic-manual-reply-001",
            content_hash="c" * 64,
        )
        claimed = await repo.claim_send_attempt(first.id)
        assert claimed is not None
        assert claimed.status == ManualReplyDeliveryStatus.SENDING
        assert await repo.claim_send_attempt(first.id) is None
        delivered = await repo.mark_delivered(claimed, "provider-message-generic-001")
        await repo.mark_completed(delivered.id)

        repeated = await repo.reserve(
            owner_user_id=user.id,
            opportunity_id=opportunity.id,
            idempotency_key="generic-manual-reply-001",
            content_hash="c" * 64,
        )

    assert repeated.id == first.id
    assert repeated.status == ManualReplyDeliveryStatus.COMPLETED


async def test_opportunity_claim_allows_only_one_concurrent_operator(wecom_subject) -> None:
    factory, user, _, opportunity = wecom_subject

    async def attempt(operator_id: str):
        async with factory() as session:
            return await OpportunityRepository(session).claim(
                opportunity_id=opportunity.id,
                owner_user_id=user.id,
                operator_id=operator_id,
            )

    results = await asyncio.gather(
        attempt("operator-a"),
        attempt("operator-b"),
        return_exceptions=True,
    )

    assert sum(not isinstance(result, Exception) for result in results) == 1
    assert sum(isinstance(result, OpportunityClaimConflict) for result in results) == 1
