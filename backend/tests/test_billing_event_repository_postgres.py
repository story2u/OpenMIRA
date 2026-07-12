import os
from collections.abc import AsyncIterator
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import BillingEventStatus
from app.infrastructure.db.models import BillingEvent
from app.infrastructure.db.repositories import BillingEventRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(not TEST_DATABASE_URL, reason="PostgreSQL billing event tests require SUBSCRIPTION_TEST_DATABASE_URL")


@pytest.fixture
async def billing_event_subject() -> AsyncIterator[async_sessionmaker[AsyncSession]]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    yield factory
    async with factory() as session:
        await session.exec(delete(BillingEvent).where(BillingEvent.provider_event_id.like("test-event-%")))
        await session.commit()
    await engine.dispose()


async def test_billing_event_reservation_is_idempotent(billing_event_subject) -> None:
    event_id = f"test-event-{uuid4()}"
    user_id = uuid4()
    async with billing_event_subject() as session:
        first = await BillingEventRepository(session).reserve_revenuecat_event(
            provider_event_id=event_id,
            event_type="FUTURE_EVENT",
            app_user_ids=[user_id],
            environment="sandbox",
            payload_hash="a" * 64,
        )
    async with billing_event_subject() as session:
        repeated = await BillingEventRepository(session).reserve_revenuecat_event(
            provider_event_id=event_id,
            event_type="FUTURE_EVENT",
            app_user_ids=[user_id],
            environment="sandbox",
            payload_hash="a" * 64,
        )
        processing = await BillingEventRepository(session).begin_processing(first.event.id)
        assert processing is not None
        duplicate_processing = await BillingEventRepository(session).begin_processing(first.event.id)

    assert first.should_enqueue is True and first.duplicate is False
    assert repeated.should_enqueue is False and repeated.duplicate is True
    assert duplicate_processing is None
    assert processing.status == BillingEventStatus.PROCESSING
