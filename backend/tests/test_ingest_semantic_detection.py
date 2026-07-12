from uuid import uuid4

import pytest

from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.domain.enums import IMChannel, MessageDirection, OpportunityStatus, Priority
from app.domain.ports import (
    DetectionResult,
    InboundMessage,
    OpportunityClassificationRequest,
)
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.db.models import Message, Opportunity


class SemanticClassifier:
    def __init__(self) -> None:
        self.request: OpportunityClassificationRequest | None = None

    async def classify(self, request: OpportunityClassificationRequest) -> DetectionResult:
        self.request = request
        return DetectionResult(
            is_opportunity=True,
            confidence=0.9,
            title="上下文商机",
            summary="客户结合上文表达了明确需求",
            reason="上文说明规模，当前询问解决方案",
            priority=Priority.HIGH,
        )


class FakeMessageRepository:
    def __init__(self) -> None:
        self.current: Message | None = None
        self.attached_opportunity_id = None
        self.history = [
            Message(
                id=uuid4(),
                channel=IMChannel.TELEGRAM,
                external_message_id="previous-1",
                conversation_id="chat-1",
                sender_display_name="Alice",
                direction=MessageDirection.INCOMING,
                text="我们团队大约 200 人，本月要确定。",
                source_type="group",
                group_name="产品交流群",
            )
        ]

    async def get_by_external_id(self, channel, external_message_id):
        return None

    async def create_incoming(self, inbound):
        self.current = Message(
            id=uuid4(),
            owner_user_id=None,
            channel=inbound.channel,
            external_message_id=inbound.external_message_id,
            conversation_id=inbound.conversation_id,
            sender_display_name=inbound.sender_display_name,
            direction=MessageDirection.INCOMING,
            text=inbound.text,
            source_type=inbound.source_type,
            group_name=inbound.group_name,
        )
        return self.current

    async def list_by_conversation(self, channel, conversation_id, owner_user_id, limit=20):
        assert limit == 7
        assert owner_user_id is None
        return [*self.history, self.current]

    async def attach_opportunity(self, message_id, opportunity_id):
        self.attached_opportunity_id = opportunity_id


class FakeOpportunityRepository:
    def __init__(self) -> None:
        self.created: Opportunity | None = None

    async def create(self, **values):
        self.created = Opportunity(id=uuid4(), **values)
        return self.created


class FakeRuleRepository:
    async def enabled_detection_rules(self):
        return []


class AfterHours:
    def is_working_time(self):
        return False


class FakeQueue:
    def __init__(self) -> None:
        self.reviews = []
        self.ai_replies = []

    def notify_reviewers(self, opportunity_id):
        self.reviews.append(opportunity_id)

    def enqueue_ai_reply(self, opportunity_id):
        self.ai_replies.append(opportunity_id)


@pytest.mark.asyncio
async def test_ai_discovered_opportunity_uses_context_and_never_auto_replies() -> None:
    classifier = SemanticClassifier()
    message_repo = FakeMessageRepository()
    opportunity_repo = FakeOpportunityRepository()
    queue = FakeQueue()
    use_case = IngestMessageUseCase(
        message_repo=message_repo,  # type: ignore[arg-type]
        opportunity_repo=opportunity_repo,  # type: ignore[arg-type]
        rule_repo=FakeRuleRepository(),  # type: ignore[arg-type]
        detector=OpportunityDetector(ai_classifier=classifier),
        work_time=AfterHours(),  # type: ignore[arg-type]
        task_queue=queue,  # type: ignore[arg-type]
        subscription_repo=object(),  # type: ignore[arg-type]
    )

    opportunity = await use_case.execute(
        InboundMessage(
            channel=IMChannel.TELEGRAM,
            external_message_id="current-1",
            conversation_id="chat-1",
            sender_display_name="Alice",
            text="你们有类似方案吗？",
            source_type="group",
            group_name="产品交流群",
        )
    )

    assert isinstance(opportunity, Opportunity)
    assert opportunity.status == OpportunityStatus.PENDING_HUMAN
    assert queue.reviews == [opportunity.id]
    assert queue.ai_replies == []
    assert classifier.request is not None
    assert [turn.text for turn in classifier.request.conversation] == [
        "我们团队大约 200 人，本月要确定。"
    ]
    assert classifier.request.group_name == "产品交流群"


def test_detection_context_keeps_recent_messages_with_bounded_size() -> None:
    use_case = IngestMessageUseCase.__new__(IngestMessageUseCase)
    messages = [
        Message(
            id=uuid4(),
            channel=IMChannel.TELEGRAM,
            external_message_id=f"message-{index}",
            conversation_id="chat-1",
            sender_display_name="Alice",
            direction=MessageDirection.INCOMING,
            text=f"{index}:" + "x" * 1200,
        )
        for index in range(8)
    ]
    current = Message(
        id=uuid4(),
        channel=IMChannel.TELEGRAM,
        external_message_id="current",
        conversation_id="chat-1",
        sender_display_name="Alice",
        direction=MessageDirection.INCOMING,
        text="current message",
    )

    context = use_case._detection_context([*messages, current], current_message_id=current.id)

    assert len(context) == 4
    assert sum(len(turn.text) for turn in context) == 4000
    assert context[-1].text.startswith("7:")
    assert all("current message" not in turn.text for turn in context)
