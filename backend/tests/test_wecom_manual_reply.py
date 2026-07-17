from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.manual_reply import (
    ManualReplyIdempotencyConflict,
    ManualReplyOutcomeUncertain,
    ManualReplyUseCase,
)
from app.domain.enums import (
    IMChannel,
    ManualReplyDeliveryStatus,
    OpportunityStatus,
)
from app.domain.ports import SendReceipt
from app.infrastructure.db.models import Opportunity
from app.infrastructure.im.base import AdapterRegistry


class FakeWeComAdapter:
    channel = IMChannel.WECOM

    def __init__(self, error: Exception | None = None) -> None:
        self.calls: list[dict] = []
        self.error = error

    async def send_message(self, conversation_id, text, **kwargs):
        self.calls.append({"conversation_id": conversation_id, "text": text, **kwargs})
        if self.error:
            raise self.error
        return SendReceipt(provider_message_id="provider-message-1", raw_response={})


class DisabledWeComAdapter(FakeWeComAdapter):
    async def send_message(self, conversation_id, text, **kwargs):
        self.calls.append({"conversation_id": conversation_id, "text": text, **kwargs})
        return SendReceipt(delivered=False, raw_response={"dry_run": True})


class FakeMessageRepository:
    def __init__(self) -> None:
        self.messages: dict[tuple[IMChannel, str], SimpleNamespace] = {}

    async def get_by_external_id(self, channel, external_message_id):
        return self.messages.get((channel, external_message_id))

    async def create_outgoing_idempotent(self, **kwargs):
        key = (kwargs["channel"], kwargs["external_message_id"])
        message = self.messages.get(key)
        if message:
            return message
        message = SimpleNamespace(id=uuid4(), **kwargs)
        self.messages[key] = message
        return message


class FakeDeliveryRepository:
    def __init__(self) -> None:
        self.deliveries: dict[tuple[object, str], SimpleNamespace] = {}

    async def reserve(self, **kwargs):
        key = (kwargs["owner_user_id"], kwargs["idempotency_key"])
        delivery = self.deliveries.get(key)
        if delivery:
            return delivery
        delivery = SimpleNamespace(
            id=uuid4(),
            status=ManualReplyDeliveryStatus.PENDING,
            provider_message_id=None,
            **kwargs,
        )
        self.deliveries[key] = delivery
        return delivery

    async def claim_send_attempt(self, delivery_id):
        delivery = self._get(delivery_id)
        if delivery.status not in {
            ManualReplyDeliveryStatus.PENDING,
            ManualReplyDeliveryStatus.FAILED,
        }:
            return None
        delivery.status = ManualReplyDeliveryStatus.SENDING
        return delivery

    async def mark_delivered(self, delivery, provider_message_id):
        delivery.status = ManualReplyDeliveryStatus.DELIVERED
        delivery.provider_message_id = provider_message_id
        return delivery

    async def mark_completed(self, delivery_id):
        delivery = self._get(delivery_id)
        delivery.status = ManualReplyDeliveryStatus.COMPLETED
        return delivery

    async def mark_failed(self, delivery_id, error_class):
        delivery = self._get(delivery_id)
        delivery.status = ManualReplyDeliveryStatus.FAILED
        delivery.error_class = error_class
        return delivery

    async def mark_uncertain(self, delivery_id, error_class):
        delivery = self._get(delivery_id)
        delivery.status = ManualReplyDeliveryStatus.UNCERTAIN
        delivery.error_class = error_class
        return delivery

    def _get(self, delivery_id):
        return next(item for item in self.deliveries.values() if item.id == delivery_id)


class FakeOpportunityRepository:
    def __init__(self, opportunity: Opportunity) -> None:
        self.opportunity = opportunity

    async def get(self, opportunity_id):
        return self.opportunity if self.opportunity.id == opportunity_id else None

    async def finalize_manual_reply(self, *, target_status, text, operator_id, **kwargs):
        self.opportunity.status = target_status
        self.opportunity.final_reply = text
        self.opportunity.assigned_to = operator_id
        return self.opportunity


def make_subject(adapter: FakeWeComAdapter | None = None):
    owner_id = uuid4()
    connection_id = uuid4()
    opportunity = Opportunity(
        owner_user_id=owner_id,
        channel=IMChannel.WECOM,
        conversation_id=f"wecom:{connection_id}:zhangsan",
        title="采购咨询",
        status=OpportunityStatus.PENDING_HUMAN,
    )
    selected_adapter = adapter or FakeWeComAdapter()
    messages = FakeMessageRepository()
    deliveries = FakeDeliveryRepository()
    use_case = ManualReplyUseCase(
        opportunity_repo=FakeOpportunityRepository(opportunity),
        message_repo=messages,
        delivery_repo=deliveries,
        adapters=AdapterRegistry([selected_adapter]),
    )
    return owner_id, opportunity, selected_adapter, messages, deliveries, use_case


@pytest.mark.asyncio
async def test_manual_reply_reuses_completed_delivery_without_resending() -> None:
    owner_id, opportunity, adapter, messages, deliveries, use_case = make_subject()

    first = await use_case.execute(
        opportunity=opportunity,
        text="您好，可以安排产品演示。",
        operator_id=str(owner_id),
        mark_following=True,
        idempotency_key="reply-request-001",
    )
    repeated = await use_case.execute(
        opportunity=opportunity,
        text="您好，可以安排产品演示。",
        operator_id=str(owner_id),
        mark_following=True,
        idempotency_key="reply-request-001",
    )

    assert first.message.id == repeated.message.id
    assert len(messages.messages) == 1
    assert len(adapter.calls) == 1
    assert adapter.calls[0]["owner_user_id"] == owner_id
    assert adapter.calls[0]["opportunity_id"] == opportunity.id
    assert adapter.calls[0]["idempotency_key"] == "reply-request-001"
    assert next(iter(deliveries.deliveries.values())).status == (
        ManualReplyDeliveryStatus.COMPLETED
    )


@pytest.mark.asyncio
async def test_manual_reply_rejects_same_key_with_different_payload() -> None:
    owner_id, opportunity, adapter, _, _, use_case = make_subject()
    await use_case.execute(
        opportunity=opportunity,
        text="第一条回复",
        operator_id=str(owner_id),
        mark_following=True,
        idempotency_key="reply-request-002",
    )

    with pytest.raises(ManualReplyIdempotencyConflict):
        await use_case.execute(
            opportunity=opportunity,
            text="不同内容",
            operator_id=str(owner_id),
            mark_following=True,
            idempotency_key="reply-request-002",
        )

    assert len(adapter.calls) == 1


@pytest.mark.asyncio
async def test_manual_reply_provider_error_is_fail_closed_and_never_retried() -> None:
    adapter = FakeWeComAdapter(RuntimeError("provider timeout after request"))
    owner_id, opportunity, _, _, deliveries, use_case = make_subject(adapter)

    for _ in range(2):
        with pytest.raises(ManualReplyOutcomeUncertain):
            await use_case.execute(
                opportunity=opportunity,
                text="请确认是否收到。",
                operator_id=str(owner_id),
                mark_following=True,
                idempotency_key="reply-request-003",
            )

    assert len(adapter.calls) == 1
    assert next(iter(deliveries.deliveries.values())).status == (
        ManualReplyDeliveryStatus.UNCERTAIN
    )
