import asyncio
import os
from collections.abc import AsyncIterator
from datetime import timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete, func
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.core.config import Settings
from app.core.security import derive_interactive_agent_turn_nonce
from app.domain.enums import (
    DevicePlatform,
    DeviceStatus,
    InteractiveAgentTurnStatus,
    InteractiveAgentProviderRequestStatus,
    UsageFeature,
    UsageStatus,
)
from app.domain.services.interactive_agent import (
    InteractiveAgentTurnConflictError,
    InteractiveAgentTurnQuotaExceededError,
    InteractiveAgentTurnTokenRejectedError,
)
from app.domain.services.subscription_policy import utc_calendar_month
from app.infrastructure.db.interactive_agent_repository import (
    InteractiveAgentTurnRepository,
)
from app.infrastructure.db.interactive_agent_gateway_repository import (
    InteractiveAgentGatewayRepository,
)
from app.infrastructure.db.models import (
    Device,
    InteractiveAgentProviderRequest,
    InteractiveAgentTurn,
    UsageLedger,
    User,
    utc_now,
)
from app.infrastructure.db.repositories import SubscriptionRepository

TEST_DATABASE_URL = os.getenv("INTERACTIVE_AGENT_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="INTERACTIVE_AGENT_TEST_DATABASE_URL is required for PostgreSQL turn tests",
)


def interactive_capabilities() -> dict[str, bool | int | str]:
    return {
        "client.reactNative": True,
        "sqlite.schema": 5,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.interactive": True,
        "agent.interactiveSchema": 1,
    }


@pytest.fixture
async def interactive_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, Device]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL, pool_size=10, max_overflow=20)
    session_factory = async_sessionmaker(
        engine,
        class_=AsyncSession,
        expire_on_commit=False,
    )
    user = User(email=f"interactive-{os.urandom(8).hex()}@example.test")
    device = Device(
        owner_user_id=user.id,
        installation_id_hash=os.urandom(32).hex(),
        platform=DevicePlatform.IOS,
        app_variant="development",
        app_version="1.0.0",
        app_build="1",
        capabilities=interactive_capabilities(),
    )
    async with session_factory() as session:
        session.add(user)
        await session.commit()
        session.add(device)
        await session.commit()

    yield session_factory, user, device

    async with session_factory() as session:
        await session.exec(
            delete(InteractiveAgentProviderRequest).where(
                InteractiveAgentProviderRequest.owner_user_id == user.id
            )
        )
        await session.exec(
            delete(InteractiveAgentTurn).where(
                InteractiveAgentTurn.owner_user_id == user.id
            )
        )
        await session.exec(delete(UsageLedger).where(UsageLedger.user_id == user.id))
        await session.exec(delete(Device).where(Device.owner_user_id == user.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


def settings_for(device: Device, *, limit: int = 10) -> Settings:
    assert TEST_DATABASE_URL
    return Settings(
        database_url=TEST_DATABASE_URL,
        admin_api_token="test-admin-token",
        interactive_agent_beta_enabled=True,
        interactive_agent_gateway_enabled=True,
        interactive_agent_beta_monthly_turn_limit=limit,
        interactive_agent_device_allowlist=str(device.id),
        device_agent_gateway_api_key="test-provider-key",
    )


def service_for(
    session: AsyncSession,
    settings: Settings,
) -> InteractiveAgentTurnService:
    return InteractiveAgentTurnService(
        turn_repo=InteractiveAgentTurnRepository(session),
        subscription_repo=SubscriptionRepository(session),
        settings=settings,
        routing_service=InteractiveAgentRoutingService(settings=settings),
    )


def principal_for(
    turn: InteractiveAgentTurn,
    settings: Settings,
) -> InteractiveAgentTurnTokenPrincipal:
    return InteractiveAgentTurnTokenPrincipal(
        turn_id=turn.id,
        owner_user_id=turn.owner_user_id,
        device_id=turn.device_id,
        nonce=derive_interactive_agent_turn_nonce(
            turn_id=turn.id,
            owner_user_id=turn.owner_user_id,
            device_id=turn.device_id,
            settings=settings,
        ),
    )


async def test_claim_retry_heartbeat_and_complete_consume_exactly_once(
    interactive_subject,
) -> None:
    session_factory, user, device = interactive_subject
    settings = settings_for(device)
    local_session_id = uuid4()

    async with session_factory() as session:
        first = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=local_session_id,
            idempotency_key="turn-retry-1",
        )
    async with session_factory() as session:
        retried = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=local_session_id,
            idempotency_key="turn-retry-1",
        )

    assert first.created is True
    assert retried.created is False
    assert retried.turn.id == first.turn.id
    assert first.turn.status == InteractiveAgentTurnStatus.CLAIMED

    principal = principal_for(first.turn, settings)
    async with session_factory() as session:
        running = await service_for(session, settings).heartbeat(
            principal,
            expected_lock_version=1,
        )
    async with session_factory() as session:
        stale_retry = await service_for(session, settings).heartbeat(
            principal,
            expected_lock_version=1,
        )
    async with session_factory() as session:
        completed = await service_for(session, settings).complete(
            principal,
            expected_lock_version=2,
        )
    async with session_factory() as session:
        replayed = await service_for(session, settings).complete(
            principal,
            expected_lock_version=2,
        )

    assert running.status == InteractiveAgentTurnStatus.RUNNING
    assert running.lock_version == 2
    assert stale_retry.lock_version == 2
    assert completed.status == InteractiveAgentTurnStatus.COMPLETED
    assert completed.lock_version == 3
    assert replayed.status == InteractiveAgentTurnStatus.COMPLETED
    async with session_factory() as session:
        ledger = await session.get(UsageLedger, completed.usage_ledger_id)
        assert ledger is not None
        assert ledger.feature == UsageFeature.INTERACTIVE_AGENT_TURN
        assert ledger.status == UsageStatus.CONSUMED
        assert ledger.source_message_id is None
        turn_count = await session.scalar(
            select(func.count()).select_from(InteractiveAgentTurn).where(
                InteractiveAgentTurn.owner_user_id == user.id
            )
        )
        ledger_count = await session.scalar(
            select(func.count()).select_from(UsageLedger).where(
                UsageLedger.user_id == user.id,
                UsageLedger.feature == UsageFeature.INTERACTIVE_AGENT_TURN,
            )
        )
        assert turn_count == 1
        assert ledger_count == 1


async def test_concurrent_claims_serialize_idempotency_and_session_ownership(
    interactive_subject,
) -> None:
    session_factory, user, device = interactive_subject
    settings = settings_for(device)
    local_session_id = uuid4()

    async def claim(key: str):
        async with session_factory() as session:
            return await service_for(session, settings).claim(
                owner_user_id=user.id,
                device=device,
                local_session_id=local_session_id,
                idempotency_key=key,
            )

    same_key = await asyncio.gather(claim("concurrent-same"), claim("concurrent-same"))
    assert {item.turn.id for item in same_key} == {same_key[0].turn.id}
    assert sorted(item.created for item in same_key) == [False, True]

    results = await asyncio.gather(
        claim("concurrent-other-a"),
        claim("concurrent-other-b"),
        return_exceptions=True,
    )
    assert all(isinstance(item, InteractiveAgentTurnConflictError) for item in results)

    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count()).select_from(InteractiveAgentTurn).where(
                    InteractiveAgentTurn.owner_user_id == user.id
                )
            )
            == 1
        )
        assert (
            await session.scalar(
                select(func.count()).select_from(UsageLedger).where(
                    UsageLedger.user_id == user.id,
                    UsageLedger.feature == UsageFeature.INTERACTIVE_AGENT_TURN,
                )
            )
            == 1
        )


async def test_interactive_quota_is_independent_from_analysis_usage(
    interactive_subject,
) -> None:
    session_factory, user, device = interactive_subject
    settings = settings_for(device, limit=1)
    now = utc_now()
    period = utc_calendar_month(now)
    async with session_factory() as session:
        session.add(
            UsageLedger(
                user_id=user.id,
                feature=UsageFeature.PI_AGENT_ANALYSIS,
                quantity=1,
                period_start=period.start,
                period_end=period.end,
                idempotency_key="unrelated-analysis",
                status=UsageStatus.CONSUMED,
                consumed_at=now,
            )
        )
        await session.commit()
        await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=uuid4(),
            idempotency_key="interactive-one",
        )
    async with session_factory() as session:
        with pytest.raises(InteractiveAgentTurnQuotaExceededError) as exc_info:
            await service_for(session, settings).claim(
                owner_user_id=user.id,
                device=device,
                local_session_id=uuid4(),
                idempotency_key="interactive-two",
            )

    assert exc_info.value.limit == 1
    assert exc_info.value.allocated == 1


async def test_fail_expire_release_and_device_revocation_invalidates_turn_token(
    interactive_subject,
) -> None:
    session_factory, user, device = interactive_subject
    settings = settings_for(device)

    async with session_factory() as session:
        failed_claim = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=uuid4(),
            idempotency_key="failure-one",
        )
    async with session_factory() as session:
        failed = await service_for(session, settings).fail(
            principal_for(failed_claim.turn, settings),
            expected_lock_version=1,
            failure_code="provider_unavailable",
        )
        failed_ledger = await session.get(UsageLedger, failed.usage_ledger_id)
        assert failed_ledger is not None
        assert failed_ledger.status == UsageStatus.RELEASED

    async with session_factory() as session:
        expiring_claim = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=uuid4(),
            idempotency_key="expiry-one",
        )
        stored = await session.get(
            InteractiveAgentTurn,
            expiring_claim.turn.id,
            with_for_update=True,
        )
        assert stored is not None
        now = utc_now()
        stored.claimed_at = now - timedelta(minutes=2)
        stored.lease_expires_at = now - timedelta(seconds=1)
        session.add(stored)
        await session.commit()
    async with session_factory() as session:
        expired = await service_for(session, settings).expire(
            owner_user_id=user.id,
            device_id=device.id,
            turn_id=expiring_claim.turn.id,
        )
        expired_ledger = await session.get(UsageLedger, expired.usage_ledger_id)
        assert expired.status == InteractiveAgentTurnStatus.EXPIRED
        assert expired_ledger is not None
        assert expired_ledger.status == UsageStatus.RELEASED

    async with session_factory() as session:
        revoked_claim = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=uuid4(),
            idempotency_key="revocation-one",
        )
        stored_device = await session.get(Device, device.id, with_for_update=True)
        assert stored_device is not None
        stored_device.status = DeviceStatus.REVOKED
        stored_device.revoked_at = utc_now()
        session.add(stored_device)
        await session.commit()
    async with session_factory() as session:
        with pytest.raises(InteractiveAgentTurnTokenRejectedError):
            await service_for(session, settings).heartbeat(
                principal_for(revoked_claim.turn, settings),
                expected_lock_version=1,
            )


async def test_provider_audit_allows_only_one_active_request_and_monotonic_sequence(
    interactive_subject,
) -> None:
    session_factory, user, device = interactive_subject
    settings = settings_for(device)
    async with session_factory() as session:
        claim = await service_for(session, settings).claim(
            owner_user_id=user.id,
            device=device,
            local_session_id=uuid4(),
            idempotency_key="gateway-audit",
        )

    async with session_factory() as session:
        repository = InteractiveAgentGatewayRepository(session)
        turn = await repository.lock_turn_owned(
            turn_id=claim.turn.id,
            owner_user_id=user.id,
            device_id=device.id,
        )
        assert turn is not None
        turn.request_count = 1
        first = InteractiveAgentProviderRequest(
            owner_user_id=user.id,
            turn_id=turn.id,
            device_id=device.id,
            request_sequence=1,
            status=InteractiveAgentProviderRequestStatus.STARTED,
            provider="openai",
            provider_model="provider-model",
            model_alias=turn.model_alias,
        )
        await repository.add(turn, first)
        first_id = first.id

    async with session_factory() as session:
        repository = InteractiveAgentGatewayRepository(session)
        turn = await repository.lock_turn_owned(
            turn_id=claim.turn.id,
            owner_user_id=user.id,
            device_id=device.id,
        )
        assert turn is not None
        turn.request_count = 2
        conflicting = InteractiveAgentProviderRequest(
            owner_user_id=user.id,
            turn_id=turn.id,
            device_id=device.id,
            request_sequence=2,
            status=InteractiveAgentProviderRequestStatus.STARTED,
            provider="openai",
            provider_model="provider-model",
            model_alias=turn.model_alias,
        )
        with pytest.raises(IntegrityError):
            await repository.add(turn, conflicting)
        await repository.rollback()

    async with session_factory() as session:
        repository = InteractiveAgentGatewayRepository(session)
        first = await session.get(
            InteractiveAgentProviderRequest,
            first_id,
            with_for_update=True,
        )
        assert first is not None
        first.status = InteractiveAgentProviderRequestStatus.COMPLETED
        first.finished_at = utc_now()
        await repository.commit(first)
        turn = await repository.lock_turn_owned(
            turn_id=claim.turn.id,
            owner_user_id=user.id,
            device_id=device.id,
        )
        assert turn is not None
        turn.request_count = 2
        second = InteractiveAgentProviderRequest(
            owner_user_id=user.id,
            turn_id=turn.id,
            device_id=device.id,
            request_sequence=2,
            status=InteractiveAgentProviderRequestStatus.STARTED,
            provider="openai",
            provider_model="provider-model",
            model_alias=turn.model_alias,
        )
        await repository.add(turn, second)

    assert second.request_sequence == 2
