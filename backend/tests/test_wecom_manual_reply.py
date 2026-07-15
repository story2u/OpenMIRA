from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.manual_reply import ManualReplyUseCase, MessageDeliveryError
from app.domain.enums import IMChannel, OpportunityStatus
from app.domain.ports import SendReceipt
from app.infrastructure.db.models import Opportunity
from app.infrastructure.im.base import AdapterRegistry


class FakeWeComAdapter:
    channel = IMChannel.WECOM

    def __init__(self) -> None:
        self.calls: list[dict] = []

    async def send_message(self, conversation_id, text, **kwargs):
        self.calls.append({"conversation_id": conversation_id, "text": text, **kwargs})
        return SendReceipt(raw_response={"dry_run": True})


class DisabledWeComAdapter(FakeWeComAdapter):
    async def send_message(self, conversation_id, text, **kwargs):
        self.calls.append({"conversation_id": conversation_id, "text": text, **kwargs})
        return SendReceipt(delivered=False, raw_response={"dry_run": True})


class FakeMessageRepository:
    def __init__(self) -> None:
        self.messages: dict[tuple[IMChannel, str], SimpleNamespace] = {}

    async def get_by_external_id(self, channel, external_message_id):
        return self.messages.get((channel, external_message_id))

    async def create_outgoing(self, **kwargs):
        message = SimpleNamespace(**kwargs)
        self.messages[(kwargs["channel"], kwargs["external_message_id"])] = message
        return message


class FakeOpportunityRepository:
    def __init__(self, opportunity) -> None:
        self.opportunity = opportunity

    async def claim_for_manual_reply(self, *, opportunity_id, operator_id):
        assert opportunity_id == self.opportunity.id
        self.opportunity.assigned_to = operator_id
        return self.opportunity

    async def update_status(self, opportunity, status, **kwargs):
        opportunity.status = status
        opportunity.final_reply = kwargs.get("final_reply")
        opportunity.assigned_to = kwargs.get("assigned_to")
        return opportunity


@pytest.mark.asyncio
async def test_wecom_manual_reply_passes_owner_context_and_is_idempotent() -> None:
    owner_id = uuid4()
    connection_id = uuid4()
    opportunity = Opportunity(
        owner_user_id=owner_id,
        channel=IMChannel.WECOM,
        conversation_id=f"wecom:{connection_id}:zhangsan",
        title="采购咨询",
        status=OpportunityStatus.PENDING_HUMAN,
    )
    adapter = FakeWeComAdapter()
    messages = FakeMessageRepository()
    use_case = ManualReplyUseCase(
        opportunity_repo=FakeOpportunityRepository(opportunity),
        message_repo=messages,
        adapters=AdapterRegistry([adapter]),
    )

    for _ in range(2):
        await use_case.execute(
            opportunity=opportunity,
            text="您好，可以安排产品演示。",
            operator_id=str(owner_id),
            mark_following=True,
            idempotency_key="reply-request-001",
        )

    assert len(messages.messages) == 1
    assert len(adapter.calls) == 2
    assert adapter.calls[0]["owner_user_id"] == owner_id
    assert adapter.calls[0]["opportunity_id"] == opportunity.id
    assert adapter.calls[0]["idempotency_key"] == "reply-request-001"


@pytest.mark.asyncio
async def test_wecom_archive_manual_reply_is_rejected_before_adapter_send() -> None:
    owner_id = uuid4()
    adapter = FakeWeComAdapter()
    opportunity = Opportunity(
        owner_user_id=owner_id,
        channel=IMChannel.WECOM,
        conversation_id=f"wecom-archive:{uuid4()}:{owner_id}:room-001",
        title="采购咨询",
        status=OpportunityStatus.PENDING_HUMAN,
    )
    use_case = ManualReplyUseCase(
        opportunity_repo=FakeOpportunityRepository(opportunity),
        message_repo=FakeMessageRepository(),
        adapters=AdapterRegistry([adapter]),
    )

    with pytest.raises(ValueError, match="read-only"):
        await use_case.execute(
            opportunity=opportunity,
            text="这条消息不得通过存档连接发送",
            operator_id=str(owner_id),
            mark_following=True,
        )

    assert adapter.calls == []


@pytest.mark.asyncio
async def test_manual_reply_dry_run_does_not_create_outgoing_or_mark_replied() -> None:
    owner_id = uuid4()
    adapter = DisabledWeComAdapter()
    opportunity = Opportunity(
        owner_user_id=owner_id,
        channel=IMChannel.WECOM,
        conversation_id=f"wecom:{uuid4()}:zhangsan",
        title="采购咨询",
        status=OpportunityStatus.PENDING_HUMAN,
    )
    messages = FakeMessageRepository()
    use_case = ManualReplyUseCase(
        opportunity_repo=FakeOpportunityRepository(opportunity),
        message_repo=messages,
        adapters=AdapterRegistry([adapter]),
    )

    with pytest.raises(MessageDeliveryError, match="no message was delivered"):
        await use_case.execute(
            opportunity=opportunity,
            text="您好，可以安排产品演示。",
            operator_id=str(owner_id),
            mark_following=False,
            idempotency_key="reply-request-002",
        )

    assert messages.messages == {}
    assert opportunity.status == OpportunityStatus.PENDING_HUMAN
