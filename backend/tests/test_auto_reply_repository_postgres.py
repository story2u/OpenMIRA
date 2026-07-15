import os
from collections.abc import AsyncIterator

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import IMChannel, MessageDirection, OpportunityStatus
from app.domain.services.opportunity_state import InvalidOpportunityTransition
from app.infrastructure.db.models import AutoReplyDelivery, Message, Opportunity, User
from app.infrastructure.db.repositories import AutoReplyDeliveryRepository, OpportunityRepository


TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL auto-reply tests",
)


@pytest.fixture
async def auto_reply_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, Message, Opportunity]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    user = User(email=f"auto-reply-{os.urandom(8).hex()}@example.test")
    async with session_factory() as session:
        session.add(user)
        await session.commit()
        message = Message(
            owner_user_id=user.id,
            channel=IMChannel.TELEGRAM,
            external_message_id=f"business-{os.urandom(8).hex()}",
            conversation_id="customer-1",
            direction=MessageDirection.INCOMING,
            text="需要采购设备",
        )
        session.add(message)
        await session.commit()
        opportunity = Opportunity(
            owner_user_id=user.id,
            source_message_id=message.id,
            channel=IMChannel.TELEGRAM,
            conversation_id=message.conversation_id,
            source_type="private",
            title="设备采购",
            status=OpportunityStatus.AI_AUTO_REPLY,
        )
        session.add(opportunity)
        await session.commit()

    yield session_factory, user, message, opportunity

    async with session_factory() as session:
        await session.exec(
            delete(AutoReplyDelivery).where(AutoReplyDelivery.owner_user_id == user.id)
        )
        await session.exec(delete(Opportunity).where(Opportunity.id == opportunity.id))
        await session.exec(delete(Message).where(Message.id == message.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


async def test_delivery_reservation_is_idempotent_and_blocks_manual_send_while_sending(
    auto_reply_subject,
) -> None:
    session_factory, user, message, opportunity = auto_reply_subject
    async with session_factory() as session:
        delivery_repo = AutoReplyDeliveryRepository(session)
        first, created = await delivery_repo.reserve(
            owner_user_id=user.id,
            opportunity_id=opportunity.id,
            source_message_id=message.id,
            channel=IMChannel.TELEGRAM,
            conversation_id=message.conversation_id,
            idempotency_key=f"auto-reply:{opportunity.id}",
        )
        repeated, repeated_created = await delivery_repo.reserve(
            owner_user_id=user.id,
            opportunity_id=opportunity.id,
            source_message_id=message.id,
            channel=IMChannel.TELEGRAM,
            conversation_id=message.conversation_id,
            idempotency_key=f"auto-reply:{opportunity.id}",
        )
        generating = await delivery_repo.claim_candidate(first.id)
        assert generating is not None
        ready = await delivery_repo.mark_ready(generating, content_hash="a" * 64)
        claimed = await delivery_repo.claim_ready_for_send(ready)

        assert created is True
        assert repeated_created is False
        assert repeated.id == first.id
        assert claimed is not None
        assert claimed[1].assigned_to == "ai:auto_reply"
        with pytest.raises(InvalidOpportunityTransition, match="already in progress"):
            await OpportunityRepository(session).claim_for_manual_reply(
                opportunity_id=opportunity.id,
                operator_id="operator-1",
            )
