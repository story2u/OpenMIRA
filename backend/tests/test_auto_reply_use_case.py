from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.ai_reply import AIAutoReplyUseCase
from app.core.config import Settings
from app.domain.enums import (
    AgentAnalysisStatus,
    AutoReplyDeliveryStatus,
    IMChannel,
    OpportunityStatus,
    Priority,
    TelegramConnectionStatus,
)
from app.domain.ports import SendReceipt
from app.infrastructure.db.models import Message, Opportunity, UserWorkSchedule
from app.infrastructure.im.base import AdapterRegistry


class FakeOpportunityRepository:
    def __init__(self, opportunity: Opportunity) -> None:
        self.opportunity = opportunity

    async def get(self, opportunity_id):
        return self.opportunity if opportunity_id == self.opportunity.id else None

    async def save_ai_draft(self, opportunity, draft):
        opportunity.ai_reply_draft = draft
        return opportunity

    async def update_status(self, opportunity, status, **kwargs):
        opportunity.status = status
        opportunity.final_reply = kwargs.get("final_reply")
        if kwargs.get("clear_assignment"):
            opportunity.assigned_to = None
        return opportunity


class FakeMessageRepository:
    def __init__(self, message: Message) -> None:
        self.message = message
        self.outgoing: list[dict] = []

    async def get(self, message_id):
        return self.message if message_id == self.message.id else None

    async def create_outgoing(self, **kwargs):
        self.outgoing.append(kwargs)

    async def get_by_external_id(self, channel, external_message_id):
        return next(
            (
                item
                for item in self.outgoing
                if item["channel"] == channel and item["external_message_id"] == external_message_id
            ),
            None,
        )


class FakeDeliveryRepository:
    def __init__(self) -> None:
        self.opportunity = None
        self.created = True
        self.delivery = SimpleNamespace(
            id=uuid4(),
            idempotency_key="auto-reply:test",
            status=AutoReplyDeliveryStatus.CANDIDATE,
            provider_message_id=None,
        )

    async def reserve(self, **_kwargs):
        return self.delivery, self.created

    async def claim_candidate(self, _delivery_id):
        self.delivery.status = AutoReplyDeliveryStatus.GENERATING
        return self.delivery

    async def count_sent_since(self, **_kwargs):
        return 0

    async def mark_ready(self, delivery, **_kwargs):
        delivery.status = AutoReplyDeliveryStatus.READY
        return delivery

    async def mark_sending(self, delivery):
        delivery.status = AutoReplyDeliveryStatus.SENDING
        return delivery

    async def claim_ready_for_send(self, delivery):
        delivery.status = AutoReplyDeliveryStatus.SENDING
        assert self.opportunity is not None
        self.opportunity.assigned_to = "ai:auto_reply"
        return delivery, self.opportunity

    async def mark_dry_run(self, delivery):
        delivery.status = AutoReplyDeliveryStatus.DRY_RUN
        return delivery

    async def mark_blocked(self, delivery, _reason):
        delivery.status = AutoReplyDeliveryStatus.BLOCKED
        return delivery

    async def mark_failed(self, delivery, _error):
        delivery.status = AutoReplyDeliveryStatus.FAILED
        return delivery

    async def mark_sending_uncertain(self, delivery, _error):
        delivery.status = AutoReplyDeliveryStatus.SENDING
        return delivery

    async def mark_sent(self, delivery, **_kwargs):
        delivery.status = AutoReplyDeliveryStatus.SENT
        delivery.provider_message_id = _kwargs["provider_message_id"]
        return delivery

    async def reload_after_rollback(self, _delivery_id):
        return self.delivery


class FakeTelegramRepository:
    async def get_auto_reply_source(self, **_kwargs):
        source = SimpleNamespace(
            auto_reply_enabled=True,
            enabled=True,
            quota_paused=False,
        )
        connection = SimpleNamespace(
            enabled=True,
            status=TelegramConnectionStatus.CONNECTED,
            capabilities={"can_reply": True},
        )
        return source, connection


class FakeUserSettingsRepository:
    async def get_work_schedule(self, _user_id):
        return UserWorkSchedule(
            user_id=uuid4(),
            timezone="UTC",
            slots=[],
            auto_reply_outside_hours=True,
        )


class FakeReplyGenerator:
    async def generate_reply(self, _opportunity_id):
        return "人工草稿"

    async def generate_auto_reply(self, _opportunity_id):
        return "您好，需求已收到。方便补充一下预计使用人数吗？"


class DryRunTelegramAdapter:
    channel = IMChannel.TELEGRAM

    async def send_message(self, *_args, **_kwargs):
        return SendReceipt(delivered=False, raw_response={"dry_run": True})


class MustNotSendTelegramAdapter:
    channel = IMChannel.TELEGRAM

    async def send_message(self, *_args, **_kwargs):
        raise AssertionError("sent delivery reconciliation must not call Telegram")


class FakeQueue:
    def __init__(self) -> None:
        self.notifications = []

    def notify_reviewers(self, opportunity_id):
        self.notifications.append(opportunity_id)


@pytest.mark.asyncio
async def test_dry_run_never_marks_opportunity_replied_or_creates_outgoing_message() -> None:
    owner_id = uuid4()
    source_message = Message(
        id=uuid4(),
        owner_user_id=owner_id,
        channel=IMChannel.TELEGRAM,
        external_message_id="business:1:2",
        conversation_id="12345",
        text="我们想采购一批设备，请先了解需求。",
    )
    opportunity = Opportunity(
        owner_user_id=owner_id,
        source_message_id=source_message.id,
        channel=IMChannel.TELEGRAM,
        conversation_id="12345",
        source_type="private",
        title="设备采购",
        status=OpportunityStatus.AI_AUTO_REPLY,
        priority=Priority.NORMAL,
        confidence=0.95,
        agent_analysis_status=AgentAnalysisStatus.COMPLETED,
    )
    messages = FakeMessageRepository(source_message)
    deliveries = FakeDeliveryRepository()
    deliveries.opportunity = opportunity
    queue = FakeQueue()
    use_case = AIAutoReplyUseCase(
        settings=Settings(
            database_url="postgresql+asyncpg://test:test@localhost/test",
            admin_api_token="test-admin-token",
            ai_enabled=True,
            ai_auto_reply_enabled=True,
            im_send_enabled=True,
        ),
        opportunity_repo=FakeOpportunityRepository(opportunity),  # type: ignore[arg-type]
        message_repo=messages,  # type: ignore[arg-type]
        delivery_repo=deliveries,  # type: ignore[arg-type]
        telegram_repo=FakeTelegramRepository(),  # type: ignore[arg-type]
        user_settings_repo=FakeUserSettingsRepository(),  # type: ignore[arg-type]
        adapters=AdapterRegistry([DryRunTelegramAdapter()]),
        reply_generator=FakeReplyGenerator(),
        task_queue=queue,  # type: ignore[arg-type]
    )

    result = await use_case.execute(opportunity)

    assert result.status == OpportunityStatus.PENDING_HUMAN
    assert result.assigned_to is None
    assert deliveries.delivery.status == AutoReplyDeliveryStatus.DRY_RUN
    assert messages.outgoing == []
    assert queue.notifications == [opportunity.id]


@pytest.mark.asyncio
async def test_sent_delivery_reconciles_local_projection_without_resending() -> None:
    owner_id = uuid4()
    source_message = Message(
        id=uuid4(),
        owner_user_id=owner_id,
        channel=IMChannel.TELEGRAM,
        external_message_id="business:1:3",
        conversation_id="12345",
        text="需要采购设备",
    )
    opportunity = Opportunity(
        owner_user_id=owner_id,
        source_message_id=source_message.id,
        channel=IMChannel.TELEGRAM,
        conversation_id="12345",
        source_type="private",
        title="设备采购",
        status=OpportunityStatus.AI_AUTO_REPLY,
        assigned_to="ai:auto_reply",
        ai_reply_draft="您好，需求已收到。方便补充一下预计使用人数吗？",
    )
    messages = FakeMessageRepository(source_message)
    deliveries = FakeDeliveryRepository()
    deliveries.created = False
    deliveries.delivery.status = AutoReplyDeliveryStatus.SENT
    deliveries.delivery.provider_message_id = "provider-message-1"
    use_case = AIAutoReplyUseCase(
        settings=Settings(
            database_url="postgresql+asyncpg://test:test@localhost/test",
            admin_api_token="test-admin-token",
        ),
        opportunity_repo=FakeOpportunityRepository(opportunity),  # type: ignore[arg-type]
        message_repo=messages,  # type: ignore[arg-type]
        delivery_repo=deliveries,  # type: ignore[arg-type]
        telegram_repo=FakeTelegramRepository(),  # type: ignore[arg-type]
        user_settings_repo=FakeUserSettingsRepository(),  # type: ignore[arg-type]
        adapters=AdapterRegistry([MustNotSendTelegramAdapter()]),
        reply_generator=FakeReplyGenerator(),
        task_queue=FakeQueue(),  # type: ignore[arg-type]
    )

    result = await use_case.execute(opportunity)

    assert result.status == OpportunityStatus.REPLIED
    assert result.assigned_to is None
    assert len(messages.outgoing) == 1
    assert messages.outgoing[0]["external_message_id"] == "provider-message-1"
