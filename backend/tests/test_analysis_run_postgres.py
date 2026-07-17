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

from app.application.use_cases.analysis_run import (
    AnalysisRunService,
    AnalysisRunTokenPrincipal,
    DeviceAgentRoutingService,
)
from app.core.config import Settings
from app.core.security import decode_analysis_run_token, derive_analysis_run_nonce
from app.domain.enums import (
    AgentAnalysisStatus,
    AnalysisProviderRequestStatus,
    AnalysisRunExecutor,
    AnalysisRunMode,
    AnalysisRunStatus,
    DevicePlatform,
    DeviceStatus,
    IMChannel,
    LinkSafetyStatus,
    MessageDirection,
    UsageStatus,
)
from app.domain.ports import AgentAnalysisResult, AgentExecutionMetadata, LinkInspection
from app.domain.services.agent_policy import project_agent_result
from app.domain.services.analysis_run import (
    AnalysisRunConflictError,
    AnalysisRunLeaseExpiredError,
    AnalysisRunTokenRejectedError,
    AnalysisRunUnavailableError,
)
from app.infrastructure.db.analysis_run_repository import AnalysisRunRepository
from app.infrastructure.db.models import (
    AnalysisProviderRequest,
    AnalysisRun,
    Device,
    Message,
    Opportunity,
    SyncChange,
    UsageLedger,
    User,
    utc_now,
)
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    SubscriptionRepository,
)

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for analysis run tests",
)


class FakeTaskQueue:
    def __init__(self) -> None:
        self.notified: list = []
        self.agent_jobs: list[tuple] = []

    def enqueue_ai_reply(self, opportunity_id) -> None:
        pass

    def notify_reviewers(self, opportunity_id) -> None:
        self.notified.append(opportunity_id)

    def enqueue_agent_analysis(
        self,
        message_id,
        *,
        force: bool = False,
        usage_ledger_id=None,
        delay_seconds: int = 0,
    ) -> bool:
        return False


class RecordingTaskQueue(FakeTaskQueue):
    def enqueue_agent_analysis(
        self,
        message_id,
        *,
        force: bool = False,
        usage_ledger_id=None,
        delay_seconds: int = 0,
    ) -> bool:
        self.agent_jobs.append((message_id, force, usage_ledger_id, delay_seconds))
        return True


def run_settings() -> Settings:
    assert TEST_DATABASE_URL
    return Settings(
        database_url=TEST_DATABASE_URL,
        admin_api_token="test-admin-token",
        jwt_secret_key="analysis-run-test-secret",
        rn_sync_rollout_enabled=True,
        rn_device_agent_rollout_enabled=True,
        rn_device_agent_rollout_percentage=100,
        device_agent_rollout_require_shadow_ready=False,
        device_agent_fallback_enabled=True,
        device_agent_gateway_enabled=True,
        device_agent_gateway_api_key="test-provider-key",
        pi_agent_api_key="test-server-key",
        device_agent_lease_seconds=60,
    )


def capable_device(owner_id, suffix: str) -> Device:
    return Device(
        owner_user_id=owner_id,
        installation_id_hash=(suffix * 64)[:64],
        platform=DevicePlatform.IOS,
        display_name=f"Agent device {suffix}",
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={
            "client.reactNative": True,
            "agent.submitAnalysis": True,
            "agent.streaming": True,
            "agent.runtime": "pi-0.80.6",
            "agent.schema": 1,
        },
    )


def incoming_message(owner_id) -> Message:
    marker = uuid4()
    return Message(
        owner_user_id=owner_id,
        channel=IMChannel.TELEGRAM,
        external_message_id=f"analysis-run-{marker}",
        conversation_id=f"analysis-run-conversation-{marker}",
        sender_external_id="candidate-contact",
        sender_display_name="Candidate Contact",
        direction=MessageDirection.INCOMING,
        text="Need a secure deployment proposal this week.",
        raw_message_links=[],
        raw_payload={},
    )


def result(*, title: str = "Deployment opportunity") -> AgentAnalysisResult:
    return AgentAnalysisResult.model_validate(
        {
            "is_opportunity": True,
            "confidence": 0.92,
            "title": title,
            "summary": "The contact requested a deployment proposal.",
            "priority": "high",
            "trust_score": 80,
            "attention_required": True,
            "link_status": "unverified",
            "link_summary": None,
            "risk_flags": [],
            "contacts": {
                "email": None,
                "phone": None,
                "telegram_handle": None,
                "wecom_id": None,
                "extraction_source": None,
            },
            "actions": [
                {
                    "action_type": "private_message",
                    "reason": "Clarify deployment scope.",
                    "target": "candidate-contact",
                    "draft": "Could you share the target environment?",
                    "requires_approval": True,
                }
            ],
        }
    )


@pytest.fixture
async def analysis_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, User, Device, Device, Message]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"analysis-owner-{uuid4()}@example.test")
    other = User(email=f"analysis-other-{uuid4()}@example.test")
    first_device = capable_device(owner.id, "a")
    second_device = capable_device(owner.id, "b")
    message = incoming_message(owner.id)
    async with factory() as session:
        session.add_all([owner, other])
        await session.commit()
        session.add_all([first_device, second_device, message])
        await session.commit()

    yield factory, owner, other, first_device, second_device, message

    async with factory() as session:
        await session.exec(
            delete(AnalysisProviderRequest).where(
                AnalysisProviderRequest.owner_user_id == owner.id
            )
        )
        await session.exec(delete(AnalysisRun).where(AnalysisRun.owner_user_id == owner.id))
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id == owner.id))
        await session.exec(delete(UsageLedger).where(UsageLedger.user_id == owner.id))
        await session.exec(delete(Message).where(Message.owner_user_id == owner.id))
        await session.exec(delete(Opportunity).where(Opportunity.owner_user_id == owner.id))
        await session.exec(delete(Device).where(Device.owner_user_id == owner.id))
        await session.exec(delete(User).where(User.id.in_([owner.id, other.id])))
        await session.commit()
    await engine.dispose()


def service(
    session: AsyncSession,
    queue: FakeTaskQueue | None = None,
    settings: Settings | None = None,
) -> AnalysisRunService:
    return AnalysisRunService(
        run_repo=AnalysisRunRepository(session),
        subscription_repo=SubscriptionRepository(session),
        message_repo=MessageRepository(session),
        opportunity_repo=OpportunityRepository(session),
        task_queue=queue or FakeTaskQueue(),
        settings=settings or run_settings(),
    )


async def complete_server_baseline(
    session: AsyncSession,
    owner: User,
    message: Message,
    analysis: AgentAnalysisResult,
) -> UsageLedger:
    subscription_repo = SubscriptionRepository(session)
    reservation = await subscription_repo.reserve_agent_analysis(
        user_id=owner.id,
        message_id=message.id,
        idempotency_key=f"server-baseline:{message.id}",
    )
    assert reservation.ledger is not None
    message_repo = MessageRepository(session)
    await message_repo.mark_agent_queued(message.id)
    claimed = await message_repo.claim_agent_analysis(message.id)
    assert claimed is not None
    projection = project_agent_result(analysis, [], analyzed_at=utc_now())
    await message_repo.complete_agent_analysis(
        claimed,
        projection,
        execution=AgentExecutionMetadata(
            executed_by=AnalysisRunExecutor.SERVER,
            run_id=reservation.ledger.id,
            runtime_version="node-pi-0.80.6",
            schema_version=1,
            model_version="test-server-model",
            policy_version="agent-policy-v1",
        ),
    )
    ledger = await subscription_repo.consume_usage(reservation.ledger.id)
    assert ledger is not None
    return ledger


def principal(run: AnalysisRun) -> AnalysisRunTokenPrincipal:
    return AnalysisRunTokenPrincipal(
        run_id=run.id,
        owner_user_id=run.owner_user_id,
        device_id=run.device_id,
        nonce=derive_analysis_run_nonce(
            run_id=run.id,
            owner_user_id=run.owner_user_id,
            device_id=run.device_id,
            settings=run_settings(),
        ),
    )


class FakeLinkInspector:
    def __init__(self) -> None:
        self.calls: list[list[str]] = []

    async def inspect_many(self, urls: list[str]) -> list[LinkInspection]:
        self.calls.append(urls)
        return [
            LinkInspection(
                url=url,
                final_url=url,
                status=LinkSafetyStatus.SUSPICIOUS,
                http_status=200,
                content_type="text/plain",
                title="Evidence",
                text="Contact link@example.test",
                emails=["link@example.test"],
                risk_reasons=["deterministic risk"],
            )
            for url in urls
        ]


async def test_claim_is_owner_bound_hash_only_and_idempotent(analysis_subject) -> None:
    factory, owner, other, first_device, _, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        repeated = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )

    assert repeated.run.id == claimed.run.id
    assert decode_analysis_run_token(claimed.run_token, run_settings())["rid"] == str(
        claimed.run.id
    )
    assert decode_analysis_run_token(repeated.run_token, run_settings())["rid"] == str(
        claimed.run.id
    )
    assert claimed.run_token not in claimed.run.token_nonce_hash
    assert len(claimed.run.token_nonce_hash) == 64

    async with factory() as session:
        ledger = await session.get(UsageLedger, claimed.run.usage_ledger_id)
        stored_message = await session.get(Message, message.id)
        run_count = await session.scalar(
            select(func.count())
            .select_from(AnalysisRun)
            .where(AnalysisRun.message_id == message.id)
        )
    assert ledger is not None and ledger.status == UsageStatus.RESERVED
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.RUNNING
    assert run_count == 1

    async with factory() as session:
        with pytest.raises(AnalysisRunUnavailableError):
            await service(session).claim(
                owner_user_id=other.id,
                device=first_device,
                message_id=message.id,
            )


async def test_claim_requires_rollout_and_exact_device_capabilities(analysis_subject) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    first_device.capabilities["agent.schema"] = 2
    async with factory() as session:
        with pytest.raises(AnalysisRunUnavailableError):
            await service(session).claim(
                owner_user_id=owner.id,
                device=first_device,
                message_id=message.id,
            )


async def test_concurrent_devices_create_one_active_run_and_one_reservation(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, second_device, message = analysis_subject

    async def claim(device: Device):
        async with factory() as session:
            try:
                return await service(session).claim(
                    owner_user_id=owner.id,
                    device=device,
                    message_id=message.id,
                )
            except AnalysisRunConflictError:
                return None

    outcomes = await asyncio.gather(claim(first_device), claim(second_device))

    assert sum(item is not None for item in outcomes) == 1
    async with factory() as session:
        run_count = await session.scalar(
            select(func.count())
            .select_from(AnalysisRun)
            .where(AnalysisRun.message_id == message.id)
        )
        ledger_count = await session.scalar(
            select(func.count())
            .select_from(UsageLedger)
            .where(UsageLedger.source_message_id == message.id)
        )
    assert run_count == 1
    assert ledger_count == 1


async def test_claim_next_reuses_the_scheduled_reservation_without_second_quota(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        reservation = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=owner.id,
            message_id=message.id,
            idempotency_key=f"message:{message.id}:automatic",
        )
        assert reservation.ledger is not None
        reserved_ledger_id = reservation.ledger.id
        await MessageRepository(session).mark_agent_queued(message.id)
        claimed = await service(session).claim_next(
            owner_user_id=owner.id,
            device=first_device,
        )
        assert claimed is not None
        stored_message = await session.get(Message, message.id, with_for_update=True)
        assert stored_message is not None
        stored_message.agent_started_at = utc_now() - timedelta(minutes=11)
        session.add(stored_message)
        await session.commit()
        claimed_ledger_id = claimed.run.usage_ledger_id
        empty = await service(session).claim_next(
            owner_user_id=owner.id,
            device=first_device,
        )

    assert claimed_ledger_id == reserved_ledger_id
    assert empty is None
    async with factory() as session:
        ledger_count = await session.scalar(
            select(func.count())
            .select_from(UsageLedger)
            .where(UsageLedger.source_message_id == message.id)
        )
    assert ledger_count == 1


async def test_delayed_server_worker_does_not_consume_an_active_device_ledger(
    analysis_subject,
    monkeypatch,
) -> None:
    assert TEST_DATABASE_URL
    monkeypatch.setenv("DATABASE_URL", TEST_DATABASE_URL)
    monkeypatch.setenv("ADMIN_API_TOKEN", "test-admin-token")
    from app.worker import tasks as worker_tasks

    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        reservation = await SubscriptionRepository(session).reserve_agent_analysis(
            user_id=owner.id,
            message_id=message.id,
            idempotency_key=f"message:{message.id}:automatic",
        )
        assert reservation.ledger is not None
        ledger_id = reservation.ledger.id
        await MessageRepository(session).mark_agent_queued(message.id)
        claimed = await service(session).claim_next(
            owner_user_id=owner.id,
            device=first_device,
        )
        assert claimed is not None

    monkeypatch.setattr(worker_tasks, "AsyncSessionLocal", factory)
    monkeypatch.setattr(worker_tasks, "get_settings", run_settings)
    await worker_tasks._analyze_message(
        message.id,
        force=False,
        usage_ledger_id=ledger_id,
    )

    async with factory() as session:
        await MessageRepository(session).fail_agent_analysis(
            message.id,
            "stale server retry must not overwrite an active device run",
        )
        released = await SubscriptionRepository(session).release_usage(
            ledger_id,
            "stale server retry must not release an active device run",
        )

    async with factory() as session:
        ledger = await session.get(UsageLedger, ledger_id)
        stored_message = await session.get(Message, message.id)
    assert released is None
    assert ledger is not None and ledger.status == UsageStatus.RESERVED
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.RUNNING


async def test_primary_routing_requires_a_recent_selected_capable_device(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, second_device, _ = analysis_subject
    async with factory() as session:
        first = await session.get(Device, first_device.id)
        second = await session.get(Device, second_device.id)
        assert first is not None and second is not None
        first.last_seen_at = utc_now() - timedelta(hours=1)
        second.last_seen_at = utc_now() - timedelta(hours=1)
        session.add_all([first, second])
        await session.commit()

    async with factory() as session:
        routing = DeviceAgentRoutingService(
            run_repo=AnalysisRunRepository(session),
            settings=run_settings(),
        )
        assert await routing.has_primary_device(owner.id) is False
        first = await session.get(Device, first_device.id)
        assert first is not None
        first.last_seen_at = utc_now()
        session.add(first)
        await session.commit()

    async with factory() as session:
        routing = DeviceAgentRoutingService(
            run_repo=AnalysisRunRepository(session),
            settings=run_settings(),
        )
        assert await routing.has_primary_device(owner.id) is True


async def test_heartbeat_is_monotonic_and_stale_retry_is_idempotent(analysis_subject) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        renewed = await manager.heartbeat(principal(claimed.run), expected_lock_version=1)
        replayed = await manager.heartbeat(principal(claimed.run), expected_lock_version=1)

    assert renewed.status == AnalysisRunStatus.RUNNING
    assert renewed.lock_version == 2
    assert replayed.lock_version == 2
    assert replayed.heartbeat_at == renewed.heartbeat_at


async def test_complete_projects_once_and_consumes_the_reserved_ledger(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    queue = FakeTaskQueue()
    async with factory() as session:
        manager = service(session, queue)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        completed = await manager.complete(
            principal(claimed.run),
            expected_lock_version=1,
            result=result(),
        )
        replayed = await manager.complete(
            principal(claimed.run),
            expected_lock_version=1,
            result=result(),
        )
        with pytest.raises(AnalysisRunConflictError):
            await manager.complete(
                principal(claimed.run),
                expected_lock_version=1,
                result=result(title="Different result"),
            )

    assert completed.status == AnalysisRunStatus.COMPLETED
    assert replayed.id == completed.id
    assert len(queue.notified) == 1
    async with factory() as session:
        ledger = await session.get(UsageLedger, completed.usage_ledger_id)
        stored_message = await session.get(Message, message.id)
        opportunities = (
            await session.exec(
                select(Opportunity).where(Opportunity.source_message_id == message.id)
            )
        ).all()
    assert ledger is not None and ledger.status == UsageStatus.CONSUMED
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.COMPLETED
    assert stored_message.agent_execution == {
        "executed_by": "device",
        "run_id": str(completed.id),
        "device_id": str(first_device.id),
        "runtime_version": "pi-0.80.6",
        "schema_version": 1,
        "model_version": "radar-analysis-v1",
        "policy_version": "agent-policy-v1",
    }
    assert len(opportunities) == 1
    assert opportunities[0].agent_analysis_status == AgentAnalysisStatus.COMPLETED
    assert opportunities[0].agent_actions[0]["requires_approval"] is True


async def test_fail_and_expire_release_usage_without_projecting(analysis_subject) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        failed = await manager.fail(
            principal(claimed.run),
            expected_lock_version=1,
            failure_code="gateway_unavailable",
        )
        replayed = await manager.fail(
            principal(claimed.run),
            expected_lock_version=1,
            failure_code="gateway_unavailable",
        )
    assert failed.status == AnalysisRunStatus.FAILED
    assert replayed.id == failed.id
    async with factory() as session:
        ledger = await session.get(UsageLedger, failed.usage_ledger_id)
    assert ledger is not None and ledger.status == UsageStatus.RELEASED


async def test_expired_lease_releases_usage_and_requeues_message(analysis_subject) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        claimed.run.claimed_at = utc_now() - timedelta(minutes=2)
        claimed.run.lease_expires_at = utc_now() - timedelta(seconds=1)
        session.add(claimed.run)
        await session.commit()
        manager.settings.device_agent_fallback_enabled = False
        with pytest.raises(AnalysisRunLeaseExpiredError):
            await manager.heartbeat(principal(claimed.run), expected_lock_version=1)

    async with factory() as session:
        stored_run = await session.get(AnalysisRun, claimed.run.id)
        ledger = await session.get(UsageLedger, claimed.run.usage_ledger_id)
        stored_message = await session.get(Message, message.id)
    assert stored_run is not None and stored_run.status == AnalysisRunStatus.EXPIRED
    assert ledger is not None and ledger.status == UsageStatus.RELEASED
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.QUEUED


async def test_run_token_nonce_is_checked_against_the_bound_device(analysis_subject) -> None:
    factory, owner, _, first_device, second_device, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        forged = AnalysisRunTokenPrincipal(
            run_id=claimed.run.id,
            owner_user_id=owner.id,
            device_id=first_device.id,
            nonce=derive_analysis_run_nonce(
                run_id=claimed.run.id,
                owner_user_id=owner.id,
                device_id=second_device.id,
                settings=run_settings(),
            ),
        )
        with pytest.raises(AnalysisRunTokenRejectedError):
            await manager.heartbeat(forged, expected_lock_version=1)


async def test_revoked_device_immediately_invalidates_an_existing_run_token(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        stored_device = await session.get(Device, first_device.id, with_for_update=True)
        assert stored_device is not None
        stored_device.status = DeviceStatus.REVOKED
        stored_device.revoked_at = utc_now()
        session.add(stored_device)
        await session.commit()

        with pytest.raises(AnalysisRunTokenRejectedError):
            await manager.heartbeat(principal(claimed.run), expected_lock_version=1)


async def test_provider_request_audit_allows_only_one_active_stream_per_run(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    async with factory() as session:
        claimed = await service(session).claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )

    first = AnalysisProviderRequest(
        owner_user_id=owner.id,
        run_id=claimed.run.id,
        device_id=first_device.id,
        status=AnalysisProviderRequestStatus.STARTED,
        provider="openai",
        provider_model="provider-model",
        model_alias=claimed.run.model_alias,
    )
    async with factory() as session:
        session.add(first)
        await session.commit()

    second = AnalysisProviderRequest(
        owner_user_id=owner.id,
        run_id=claimed.run.id,
        device_id=first_device.id,
        status=AnalysisProviderRequestStatus.STARTED,
        provider="openai",
        provider_model="provider-model",
        model_alias=claimed.run.model_alias,
    )
    async with factory() as session:
        session.add(second)
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

    async with factory() as session:
        stored = await session.get(AnalysisProviderRequest, first.id)
        assert stored is not None
        stored.status = AnalysisProviderRequestStatus.COMPLETED
        stored.finished_at = utc_now()
        session.add(stored)
        await session.commit()
        session.add(second)
        await session.commit()
        count = await session.scalar(
            select(func.count())
            .select_from(AnalysisProviderRequest)
            .where(AnalysisProviderRequest.run_id == claimed.run.id)
        )

    assert count == 2


async def test_link_proxy_uses_server_message_caches_evidence_and_clamps_completion(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    message.raw_message_links = ["https://public.example/rfp"]
    async with factory() as session:
        stored_message = await session.get(Message, message.id)
        assert stored_message is not None
        stored_message.raw_message_links = message.raw_message_links
        session.add(stored_message)
        await session.commit()
        manager = service(session)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )

    async with factory() as session:
        with pytest.raises(AnalysisRunConflictError):
            await service(session).complete(
                principal(claimed.run),
                expected_lock_version=1,
                result=result(),
            )

    inspector = FakeLinkInspector()
    async with factory() as session:
        manager = service(session)
        fetched_run, evidence = await manager.inspect_links(
            principal(claimed.run),
            inspector=inspector,
        )
        cached_run, cached = await manager.inspect_links(
            principal(claimed.run),
            inspector=inspector,
        )
        safe_result_data = result().model_dump(mode="json")
        safe_result_data["link_status"] = LinkSafetyStatus.SAFE
        safe_result_data["contacts"] = {
            "email": "link@example.test",
            "phone": None,
            "telegram_handle": None,
            "wecom_id": None,
            "extraction_source": "link_content",
        }
        safe_result = AgentAnalysisResult.model_validate(safe_result_data)
        completed = await manager.complete(
            principal(claimed.run),
            expected_lock_version=1,
            result=safe_result,
        )

    assert inspector.calls == [["https://public.example/rfp"]]
    assert fetched_run.link_evidence_fetched_at is not None
    assert cached_run.id == fetched_run.id
    assert cached == evidence
    async with factory() as session:
        stored_run = await session.get(AnalysisRun, completed.id)
        opportunity = (
            await session.exec(
                select(Opportunity).where(Opportunity.source_message_id == message.id)
            )
        ).one()
    assert stored_run is not None and stored_run.link_evidence is not None
    assert opportunity.link_verification["status"] == LinkSafetyStatus.SUSPICIOUS
    assert opportunity.extracted_contacts["email"] == "link@example.test"


async def test_shadow_run_reuses_consumed_ledger_and_never_projects_twice(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    baseline = result()
    async with factory() as session:
        ledger = await complete_server_baseline(session, owner, message, baseline)
        stored_message = await session.get(Message, message.id)
        assert stored_message is not None
        baseline_projection = stored_message.agent_result.copy()

    shadow_settings = run_settings().model_copy(
        update={
            "rn_device_agent_rollout_enabled": False,
            "rn_device_agent_rollout_percentage": 0,
            "device_agent_fallback_enabled": False,
            "device_agent_shadow_enabled": True,
        }
    )
    async with factory() as session:
        manager = service(session, settings=shadow_settings)
        claimed = await manager.claim_shadow(
            owner_user_id=owner.id,
            device=first_device,
        )
        assert claimed is not None
        completed = await manager.complete(
            principal(claimed.run),
            expected_lock_version=1,
            result=baseline,
        )

    assert completed.mode == AnalysisRunMode.SHADOW
    assert completed.shadow_match is True
    assert completed.shadow_difference_count == 0
    async with factory() as session:
        stored_message = await session.get(Message, message.id)
        stored_ledger = await session.get(UsageLedger, ledger.id)
        ledger_count = await session.scalar(
            select(func.count())
            .select_from(UsageLedger)
            .where(UsageLedger.source_message_id == message.id)
        )
    assert stored_message is not None
    assert stored_message.agent_result == baseline_projection
    assert stored_message.agent_execution is not None
    assert stored_message.agent_execution["executed_by"] == "server"
    assert stored_message.agent_execution["run_id"] == str(ledger.id)
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.COMPLETED
    assert stored_ledger is not None and stored_ledger.status == UsageStatus.CONSUMED
    assert ledger_count == 1

    readiness_settings = shadow_settings.model_copy(
        update={
            "rn_device_agent_rollout_enabled": True,
            "rn_device_agent_rollout_percentage": 100,
            "device_agent_fallback_enabled": True,
            "device_agent_rollout_require_shadow_ready": True,
            "device_agent_rollout_min_shadow_samples": 1,
            "device_agent_rollout_min_shadow_success_rate": 1.0,
            "device_agent_rollout_min_shadow_match_rate": 1.0,
            "device_agent_rollout_max_p95_seconds": 120.0,
        }
    )
    async with factory() as session:
        routing = DeviceAgentRoutingService(
            run_repo=AnalysisRunRepository(session),
            settings=readiness_settings,
        )
        readiness = await routing.rollout_readiness()
        has_primary_device = await routing.has_primary_device(owner.id)

    assert readiness.ready is True
    assert readiness.evidence.terminal_samples == 1
    assert readiness.evidence.completed_samples == 1
    assert readiness.evidence.matched_samples == 1
    assert readiness.evidence.p95_seconds is not None
    assert has_primary_device is True

    async with factory() as session:
        stored_run = await session.get(AnalysisRun, completed.id, with_for_update=True)
        assert stored_run is not None
        stored_run.runtime_version = "legacy-runtime"
        session.add(stored_run)
        await session.commit()
        version_scoped = await DeviceAgentRoutingService(
            run_repo=AnalysisRunRepository(session),
            settings=readiness_settings,
        ).rollout_readiness()

    assert version_scoped.ready is False
    assert version_scoped.evidence.terminal_samples == 0
    assert version_scoped.reasons == (
        "insufficient_shadow_samples",
        "shadow_success_rate_below_threshold",
        "shadow_match_rate_below_threshold",
        "shadow_p95_above_threshold",
    )


async def test_shadow_claim_is_single_and_expiry_preserves_server_truth(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, second_device, message = analysis_subject
    async with factory() as session:
        ledger = await complete_server_baseline(session, owner, message, result())
    shadow_settings = run_settings().model_copy(
        update={
            "rn_device_agent_rollout_enabled": False,
            "rn_device_agent_rollout_percentage": 0,
            "device_agent_fallback_enabled": False,
            "device_agent_shadow_enabled": True,
        }
    )

    async def claim_shadow(device: Device):
        async with factory() as session:
            return await service(session, settings=shadow_settings).claim_shadow(
                owner_user_id=owner.id,
                device=device,
            )

    claims = await asyncio.gather(
        claim_shadow(first_device),
        claim_shadow(second_device),
    )
    claimed = next(item for item in claims if item is not None)
    assert sum(item is not None for item in claims) == 1

    async with factory() as session:
        stored = await session.get(AnalysisRun, claimed.run.id, with_for_update=True)
        assert stored is not None
        stored.claimed_at = utc_now() - timedelta(minutes=2)
        stored.lease_expires_at = utc_now() - timedelta(seconds=1)
        session.add(stored)
        await session.commit()
        expired = await service(session, settings=shadow_settings).expire_stale()

    assert expired == 1
    async with factory() as session:
        stored_run = await session.get(AnalysisRun, claimed.run.id)
        stored_message = await session.get(Message, message.id)
        stored_ledger = await session.get(UsageLedger, ledger.id)
    assert stored_run is not None and stored_run.status == AnalysisRunStatus.EXPIRED
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.COMPLETED
    assert stored_ledger is not None and stored_ledger.status == UsageStatus.CONSUMED


async def test_primary_failure_releases_before_single_fallback_reservation(
    analysis_subject,
) -> None:
    factory, owner, _, first_device, _, message = analysis_subject
    queue = RecordingTaskQueue()
    fallback_settings = run_settings().model_copy(
        update={"device_agent_fallback_enabled": True}
    )
    async with factory() as session:
        manager = service(session, queue=queue, settings=fallback_settings)
        claimed = await manager.claim(
            owner_user_id=owner.id,
            device=first_device,
            message_id=message.id,
        )
        failed = await manager.fail(
            principal(claimed.run),
            expected_lock_version=1,
            failure_code="agent_retry_exhausted",
        )

    assert failed.status == AnalysisRunStatus.FAILED
    assert len(queue.agent_jobs) == 1
    assert queue.agent_jobs[0][:2] == (message.id, False)
    assert queue.agent_jobs[0][2] is not None
    assert queue.agent_jobs[0][3] == 0
    async with factory() as session:
        ledgers = list(
            (
                await session.exec(
                    select(UsageLedger)
                    .where(UsageLedger.source_message_id == message.id)
                    .order_by(UsageLedger.created_at)
                )
            ).all()
        )
        stored_message = await session.get(Message, message.id)
    assert [ledger.status for ledger in ledgers] == [
        UsageStatus.RELEASED,
        UsageStatus.RESERVED,
    ]
    assert stored_message is not None
    assert stored_message.agent_analysis_status == AgentAnalysisStatus.QUEUED
