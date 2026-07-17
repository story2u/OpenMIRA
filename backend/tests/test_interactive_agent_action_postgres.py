import asyncio
import os
from collections.abc import AsyncIterator
from dataclasses import replace
from datetime import timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.application.use_cases.interactive_agent_action import (
    InteractiveAgentActionService,
    InteractiveAgentApprovalTokenPrincipal,
)
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.application.use_cases.manual_reply import ManualReplyUseCase
from app.core.config import Settings
from app.core.security import (
    derive_interactive_agent_approval_nonce,
    derive_interactive_agent_turn_nonce,
    hash_interactive_agent_turn_nonce,
)
from app.domain.enums import (
    DevicePlatform,
    DeviceStatus,
    IMChannel,
    InteractiveAgentApprovalStatus,
    InteractiveAgentTurnStatus,
    OpportunityStatus,
    Priority,
    UsageFeature,
    UsageStatus,
)
from app.domain.ports import SendReceipt
from app.domain.services.interactive_agent_action import (
    CanonicalSendReplyArguments,
    InteractiveAgentActionConflictError,
    InteractiveAgentActionExpiredError,
    InteractiveAgentActionRejectedError,
    InteractiveAgentActionUncertainError,
    InteractiveAgentActionUnavailableError,
    canonical_send_reply_arguments_hash,
)
from app.infrastructure.db.interactive_agent_action_repository import (
    InteractiveAgentActionRepository,
)
from app.infrastructure.db.models import (
    Device,
    InteractiveAgentActionApproval,
    InteractiveAgentTurn,
    ManualReplyDelivery,
    Message,
    Opportunity,
    SyncChange,
    UsageLedger,
    User,
    utc_now,
)
from app.infrastructure.db.repositories import (
    ManualReplyDeliveryRepository,
    MessageRepository,
    OpportunityRepository,
)
from app.infrastructure.im.base import AdapterRegistry

TEST_DATABASE_URL = os.getenv("INTERACTIVE_AGENT_ACTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="INTERACTIVE_AGENT_ACTION_TEST_DATABASE_URL is required",
)


class FakeAdapter:
    channel = IMChannel.TELEGRAM

    def __init__(self, error: Exception | None = None) -> None:
        self.error = error
        self.calls: list[tuple[str, str]] = []

    async def send_message(self, conversation_id: str, text: str, **_: object) -> SendReceipt:
        self.calls.append((conversation_id, text))
        await asyncio.sleep(0)
        if self.error:
            raise self.error
        return SendReceipt(provider_message_id=f"provider-{len(self.calls)}")


def capabilities() -> dict[str, bool | int | str]:
    return {
        "client.reactNative": True,
        "sqlite.schema": 5,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.interactive": True,
        "agent.interactiveSchema": 3,
    }


@pytest.fixture
async def action_subject() -> AsyncIterator[
    tuple[
        async_sessionmaker[AsyncSession],
        Settings,
        User,
        Device,
        InteractiveAgentTurn,
        Opportunity,
    ]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL, pool_size=10, max_overflow=20)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    now = utc_now()
    user = User(email=f"interactive-action-{uuid4()}@example.test")
    device = Device(
        owner_user_id=user.id,
        installation_id_hash=os.urandom(32).hex(),
        platform=DevicePlatform.IOS,
        app_variant="development",
        app_version="1.0.0",
        app_build="1",
        capabilities=capabilities(),
    )
    ledger = UsageLedger(
        user_id=user.id,
        feature=UsageFeature.INTERACTIVE_AGENT_TURN,
        period_start=now.replace(day=1, hour=0, minute=0, second=0, microsecond=0),
        period_end=now + timedelta(days=31),
        idempotency_key=f"interactive-action-{uuid4()}",
        status=UsageStatus.RESERVED,
    )
    settings = Settings(
        database_url=TEST_DATABASE_URL,
        admin_api_token="test-admin-token",
        jwt_secret_key="interactive-action-test-secret",
        interactive_agent_beta_enabled=True,
        interactive_agent_gateway_enabled=True,
        interactive_agent_external_actions_enabled=True,
        interactive_agent_beta_monthly_turn_limit=10,
        interactive_agent_device_allowlist=str(device.id),
        interactive_agent_schema_version=3,
        interactive_agent_policy_version="interactive-approved-send-v3",
        device_agent_gateway_api_key="test-provider-key",
        im_send_enabled=True,
    )
    turn_nonce = derive_interactive_agent_turn_nonce(
        turn_id=(turn_id := uuid4()),
        owner_user_id=user.id,
        device_id=device.id,
        settings=settings,
    )
    turn = InteractiveAgentTurn(
        id=turn_id,
        owner_user_id=user.id,
        device_id=device.id,
        local_session_id=uuid4(),
        idempotency_key=f"turn-{uuid4()}",
        usage_ledger_id=ledger.id,
        status=InteractiveAgentTurnStatus.RUNNING,
        runtime_version="pi-0.80.6",
        schema_version=3,
        model_alias="radar-interactive-v1",
        policy_version="interactive-approved-send-v3",
        lock_version=2,
        request_count=1,
        token_nonce_hash=hash_interactive_agent_turn_nonce(turn_nonce),
        lease_expires_at=now + timedelta(minutes=5),
        claimed_at=now,
        heartbeat_at=now,
    )
    opportunity = Opportunity(
        owner_user_id=user.id,
        channel=IMChannel.TELEGRAM,
        conversation_id=f"approved-send-{uuid4()}",
        title="Approved send target",
        summary="Only one approved reply may be sent",
        matched_keywords=["approved"],
        raw_message_links=[],
        confidence=0.95,
        priority=Priority.NORMAL,
        status=OpportunityStatus.PENDING_HUMAN,
        last_message_preview="Please reply",
    )
    async with factory() as session:
        session.add(user)
        await session.commit()
        session.add(device)
        session.add(ledger)
        session.add(opportunity)
        await session.commit()
        session.add(turn)
        await session.commit()

    yield factory, settings, user, device, turn, opportunity

    async with factory() as session:
        await session.exec(
            delete(InteractiveAgentActionApproval).where(
                InteractiveAgentActionApproval.owner_user_id == user.id
            )
        )
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id == user.id))
        await session.exec(delete(Message).where(Message.owner_user_id == user.id))
        await session.exec(
            delete(ManualReplyDelivery).where(ManualReplyDelivery.owner_user_id == user.id)
        )
        await session.exec(
            delete(InteractiveAgentTurn).where(InteractiveAgentTurn.owner_user_id == user.id)
        )
        await session.exec(delete(Opportunity).where(Opportunity.owner_user_id == user.id))
        await session.exec(delete(UsageLedger).where(UsageLedger.user_id == user.id))
        await session.exec(delete(Device).where(Device.owner_user_id == user.id))
        await session.exec(delete(User).where(User.id == user.id))
        await session.commit()
    await engine.dispose()


def turn_principal(
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


def approval_principal(
    approval: InteractiveAgentActionApproval,
    settings: Settings,
) -> InteractiveAgentApprovalTokenPrincipal:
    return InteractiveAgentApprovalTokenPrincipal(
        approval_id=approval.id,
        turn_id=approval.turn_id,
        owner_user_id=approval.owner_user_id,
        device_id=approval.device_id,
        nonce=derive_interactive_agent_approval_nonce(
            approval_id=approval.id,
            turn_id=approval.turn_id,
            owner_user_id=approval.owner_user_id,
            device_id=approval.device_id,
            settings=settings,
        ),
    )


def service_for(
    session: AsyncSession,
    settings: Settings,
    adapter: FakeAdapter,
) -> InteractiveAgentActionService:
    adapters = AdapterRegistry([adapter])
    return InteractiveAgentActionService(
        repository=InteractiveAgentActionRepository(session),
        manual_reply=ManualReplyUseCase(
            opportunity_repo=OpportunityRepository(session),
            message_repo=MessageRepository(session),
            delivery_repo=ManualReplyDeliveryRepository(session),
            adapters=adapters,
        ),
        adapters=adapters,
        settings=settings,
        routing_service=InteractiveAgentRoutingService(settings=settings),
    )


async def grant(
    factory: async_sessionmaker[AsyncSession],
    settings: Settings,
    turn: InteractiveAgentTurn,
    opportunity: Opportunity,
    adapter: FakeAdapter,
    *,
    text: str = "Approved exact reply",
    tool_call_id: str = "call-approved-send",
) -> InteractiveAgentActionApproval:
    async with factory() as session:
        decision = await service_for(session, settings, adapter).decide(
            turn_principal(turn, settings),
            approved=True,
            expected_version=opportunity.aggregate_version,
            idempotency_key=f"agent-reply:{uuid4()}",
            opportunity_id=opportunity.id,
            text=text,
            tool_call_id=tool_call_id,
        )
        assert decision.approval_token
        return decision.approval


def test_canonical_approved_reply_hash_is_exact_and_content_free() -> None:
    opportunity_id = uuid4()
    first = canonical_send_reply_arguments_hash(
        CanonicalSendReplyArguments(opportunity_id=opportunity_id, text="exact")
    )
    assert first == canonical_send_reply_arguments_hash(
        CanonicalSendReplyArguments(opportunity_id=opportunity_id, text="exact")
    )
    assert first != canonical_send_reply_arguments_hash(
        CanonicalSendReplyArguments(opportunity_id=opportunity_id, text="exact ")
    )
    assert len(first) == 64
    assert "exact" not in first


async def test_approved_reply_executes_once_and_replay_is_rejected(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    approval = await grant(factory, settings, turn, opportunity, adapter)
    async with factory() as session:
        result = await service_for(session, settings, adapter).execute_send_reply(
            approval_principal(approval, settings),
            expected_version=approval.expected_version,
            idempotency_key=approval.idempotency_key,
            opportunity_id=approval.opportunity_id,
            text="Approved exact reply",
        )
    assert result.opportunity.status == OpportunityStatus.FOLLOWING
    assert adapter.calls == [(opportunity.conversation_id, "Approved exact reply")]

    async with factory() as session:
        with pytest.raises(InteractiveAgentActionConflictError):
            await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(approval, settings),
                expected_version=approval.expected_version,
                idempotency_key=approval.idempotency_key,
                opportunity_id=approval.opportunity_id,
                text="Approved exact reply",
            )
        stored = await session.get(InteractiveAgentActionApproval, approval.id)
        assert stored is not None
        assert stored.status == InteractiveAgentApprovalStatus.CONSUMED
        assert (
            stored.token_nonce_hash
            and stored.token_nonce_hash != approval_principal(approval, settings).nonce
        )
    assert len(adapter.calls) == 1


async def test_approval_binds_exact_text_version_and_idempotency_key(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    approval = await grant(factory, settings, turn, opportunity, adapter)
    for overrides in (
        {"text": "tampered"},
        {"expected_version": approval.expected_version + 1},
        {"idempotency_key": f"agent-reply:{uuid4()}"},
    ):
        async with factory() as session:
            with pytest.raises(InteractiveAgentActionRejectedError):
                await service_for(session, settings, adapter).execute_send_reply(
                    approval_principal(approval, settings),
                    expected_version=overrides.get("expected_version", approval.expected_version),
                    idempotency_key=overrides.get("idempotency_key", approval.idempotency_key),
                    opportunity_id=approval.opportunity_id,
                    text=overrides.get("text", "Approved exact reply"),
                )
    assert adapter.calls == []


async def test_changed_resource_or_revoked_device_is_rejected_before_provider(
    action_subject,
) -> None:
    factory, settings, _, device, turn, opportunity = action_subject
    adapter = FakeAdapter()
    approval = await grant(factory, settings, turn, opportunity, adapter)
    async with factory() as session:
        current = await session.get(Opportunity, opportunity.id)
        assert current is not None
        current.aggregate_version += 1
        session.add(current)
        await session.commit()
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionConflictError):
            await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(approval, settings),
                expected_version=approval.expected_version,
                idempotency_key=approval.idempotency_key,
                opportunity_id=approval.opportunity_id,
                text="Approved exact reply",
            )
    assert adapter.calls == []

    second = Opportunity(
        owner_user_id=opportunity.owner_user_id,
        channel=IMChannel.TELEGRAM,
        conversation_id=f"revoked-{uuid4()}",
        title="Revoked device target",
        summary="No send after revoke",
        matched_keywords=[],
        raw_message_links=[],
        confidence=0.9,
        priority=Priority.NORMAL,
        status=OpportunityStatus.PENDING_HUMAN,
        last_message_preview="reply",
    )
    async with factory() as session:
        session.add(second)
        await session.commit()
    second_approval = await grant(
        factory,
        settings,
        turn,
        second,
        adapter,
        tool_call_id="call-revoked-device",
    )
    async with factory() as session:
        stored_device = await session.get(Device, device.id)
        assert stored_device is not None
        stored_device.status = DeviceStatus.REVOKED
        stored_device.revoked_at = utc_now()
        stored_device.revocation_reason = "approval_execution_test"
        session.add(stored_device)
        await session.commit()
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionRejectedError):
            await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(second_approval, settings),
                expected_version=second_approval.expected_version,
                idempotency_key=second_approval.idempotency_key,
                opportunity_id=second.id,
                text="Approved exact reply",
            )
    assert adapter.calls == []


async def test_denial_and_changed_decision_never_issue_execution_token(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    key = f"agent-reply:{uuid4()}"
    async with factory() as session:
        service = service_for(session, settings, adapter)
        denied = await service.decide(
            turn_principal(turn, settings),
            approved=False,
            expected_version=opportunity.aggregate_version,
            idempotency_key=key,
            opportunity_id=opportunity.id,
            text="Do not send",
            tool_call_id="call-denied",
        )
        assert denied.approval.status == InteractiveAgentApprovalStatus.DENIED
        assert denied.approval_token is None
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionConflictError):
            await service_for(session, settings, adapter).decide(
                turn_principal(turn, settings),
                approved=True,
                expected_version=opportunity.aggregate_version,
                idempotency_key=key,
                opportunity_id=opportunity.id,
                text="Do not send",
                tool_call_id="call-denied",
            )
    assert adapter.calls == []


async def test_provider_uncertainty_consumes_approval_without_retry(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter(RuntimeError("provider timeout after acceptance"))
    approval = await grant(factory, settings, turn, opportunity, adapter)
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionUncertainError):
            await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(approval, settings),
                expected_version=approval.expected_version,
                idempotency_key=approval.idempotency_key,
                opportunity_id=approval.opportunity_id,
                text="Approved exact reply",
            )
    async with factory() as session:
        stored = await session.get(InteractiveAgentActionApproval, approval.id)
    assert stored is not None
    assert stored.status == InteractiveAgentApprovalStatus.UNCERTAIN
    assert len(adapter.calls) == 1


async def test_concurrent_execution_contacts_provider_once(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    approval = await grant(factory, settings, turn, opportunity, adapter)

    async def execute() -> object:
        async with factory() as session:
            return await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(approval, settings),
                expected_version=approval.expected_version,
                idempotency_key=approval.idempotency_key,
                opportunity_id=approval.opportunity_id,
                text="Approved exact reply",
            )

    results = await asyncio.gather(execute(), execute(), return_exceptions=True)
    assert sum(not isinstance(result, Exception) for result in results) == 1
    assert sum(isinstance(result, InteractiveAgentActionConflictError) for result in results) == 1
    assert len(adapter.calls) == 1


async def test_expired_or_cross_owner_approval_is_rejected_before_provider(
    action_subject,
) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    approval = await grant(factory, settings, turn, opportunity, adapter)
    async with factory() as session:
        stored = await session.get(InteractiveAgentActionApproval, approval.id)
        assert stored is not None
        stored.decided_at = utc_now() - timedelta(seconds=10)
        stored.expires_at = utc_now() - timedelta(seconds=1)
        session.add(stored)
        await session.commit()
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionExpiredError):
            await service_for(session, settings, adapter).execute_send_reply(
                approval_principal(approval, settings),
                expected_version=approval.expected_version,
                idempotency_key=approval.idempotency_key,
                opportunity_id=approval.opportunity_id,
                text="Approved exact reply",
            )
    assert adapter.calls == []

    second = await grant(
        factory,
        settings,
        turn,
        opportunity,
        adapter,
        tool_call_id="call-cross-owner",
    )
    principal = approval_principal(second, settings)
    async with factory() as session:
        with pytest.raises(InteractiveAgentActionRejectedError):
            await service_for(session, settings, adapter).execute_send_reply(
                replace(principal, owner_user_id=uuid4()),
                expected_version=second.expected_version,
                idempotency_key=second.idempotency_key,
                opportunity_id=second.opportunity_id,
                text="Approved exact reply",
            )
    assert adapter.calls == []


async def test_external_action_and_im_send_gates_fail_closed(action_subject) -> None:
    factory, settings, _, _, turn, opportunity = action_subject
    adapter = FakeAdapter()
    for gated in (
        settings.model_copy(update={"interactive_agent_external_actions_enabled": False}),
        settings.model_copy(update={"im_send_enabled": False}),
    ):
        async with factory() as session:
            with pytest.raises(InteractiveAgentActionUnavailableError):
                await service_for(session, gated, adapter).decide(
                    turn_principal(turn, settings),
                    approved=True,
                    expected_version=opportunity.aggregate_version,
                    idempotency_key=f"agent-reply:{uuid4()}",
                    opportunity_id=opportunity.id,
                    text="Never approved while a gate is closed",
                    tool_call_id=f"call-gated-{uuid4()}",
                )
    assert adapter.calls == []
