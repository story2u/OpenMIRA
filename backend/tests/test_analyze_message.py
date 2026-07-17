from uuid import uuid4

from app.application.use_cases.analyze_message import (
    AnalyzeMessageUseCase,
    message_links,
)
from app.domain.enums import (
    AgentActionType,
    AnalysisRunExecutor,
    IMChannel,
    LinkSafetyStatus,
    MessageDirection,
    OpportunityStatus,
    Priority,
)
from app.domain.ports import (
    AgentActionRecommendation,
    AgentAnalysisResult,
    AgentContactExtraction,
    AgentExecutionMetadata,
    LinkInspection,
)
from app.domain.services.job_discovery import redact_source_sample
from app.infrastructure.db.models import Message, Opportunity


class FakeMessageRepository:
    def __init__(self, message: Message) -> None:
        self.message = message
        self.completed = False
        self.failed: str | None = None

    async def claim_agent_analysis(self, message_id, *, force=False):
        del force
        return self.message if message_id == self.message.id else None

    async def attach_opportunity(self, message_id, opportunity_id) -> None:
        assert message_id == self.message.id
        self.message.opportunity_id = opportunity_id

    async def complete_agent_analysis(self, message, projection, *, execution):
        assert message is self.message
        self.completed = True
        self.projection = projection
        self.execution = execution
        return message

    async def fail_agent_analysis(self, message_id, error) -> None:
        assert message_id == self.message.id
        self.failed = error


class FakeOpportunityRepository:
    def __init__(self) -> None:
        self.opportunity: Opportunity | None = None
        self.create_status: OpportunityStatus | None = None

    async def get(self, opportunity_id):
        return (
            self.opportunity
            if self.opportunity and self.opportunity.id == opportunity_id
            else None
        )

    async def get_by_source_message(self, message_id):
        return (
            self.opportunity
            if self.opportunity and self.opportunity.source_message_id == message_id
            else None
        )

    async def create(self, **values):
        self.create_status = values["status"]
        self.opportunity = Opportunity(id=uuid4(), **values)
        return self.opportunity

    async def apply_agent_projection(self, opportunity, projection):
        opportunity.attention_required = projection.attention_required
        opportunity.agent_actions = projection.actions
        return opportunity


class FakeAgent:
    async def analyze(self, request):
        assert request.text == "需要采购 500 套，详情 https://public.example/rfp"
        return AgentAnalysisResult(
            is_opportunity=True,
            confidence=0.95,
            title="批量采购",
            summary="客户需要采购 500 套",
            priority=Priority.URGENT,
            trust_score=88,
            attention_required=True,
            link_status=LinkSafetyStatus.SAFE,
            contacts=AgentContactExtraction(email="buyer@example.com"),
            actions=[
                AgentActionRecommendation(
                    action_type=AgentActionType.SEND_EMAIL,
                    reason="采购量大且时间紧",
                    target="buyer@example.com",
                    requires_approval=False,
                )
            ],
        )


class FakeLinkInspector:
    async def inspect_many(self, urls):
        assert urls == ["https://public.example/rfp"]
        return [
            LinkInspection(
                url=urls[0],
                status=LinkSafetyStatus.SAFE,
                emails=["buyer@example.com"],
            )
        ]


class FakeTaskQueue:
    def __init__(self) -> None:
        self.notifications = []
        self.ai_replies = []

    def notify_reviewers(self, opportunity_id) -> None:
        self.notifications.append(opportunity_id)

    def enqueue_ai_reply(self, opportunity_id) -> None:
        self.ai_replies.append(opportunity_id)

    def enqueue_agent_analysis(
        self,
        message_id,
        *,
        force=False,
        usage_ledger_id=None,
        delay_seconds: int = 0,
    ) -> bool:
        raise AssertionError(
            f"must not recursively enqueue "
            f"{message_id}/{force}/{usage_ledger_id}/{delay_seconds}"
        )


async def test_pi_agent_can_promote_rule_miss_but_only_to_human_review() -> None:
    message = Message(
        id=uuid4(),
        channel=IMChannel.TELEGRAM,
        external_message_id="external-1",
        conversation_id="chat-1",
        sender_external_id="customer-1",
        sender_display_name="采购负责人",
        direction=MessageDirection.INCOMING,
        text="需要采购 500 套，详情 https://public.example/rfp",
        source_type="group",
        group_name="采购群",
        raw_message_links=[],
    )
    message_repo = FakeMessageRepository(message)
    opportunity_repo = FakeOpportunityRepository()
    queue = FakeTaskQueue()
    use_case = AnalyzeMessageUseCase(
        message_repo=message_repo,  # type: ignore[arg-type]
        opportunity_repo=opportunity_repo,  # type: ignore[arg-type]
        agent=FakeAgent(),
        link_inspector=FakeLinkInspector(),
        task_queue=queue,
        execution=AgentExecutionMetadata(
            executed_by=AnalysisRunExecutor.SERVER,
            run_id=uuid4(),
            runtime_version="node-pi-0.80.6",
            schema_version=1,
            model_version="test-model",
            policy_version="agent-policy-v1",
        ),
        min_opportunity_confidence=0.75,
        max_links=5,
    )

    opportunity = await use_case.execute(message.id)

    assert opportunity is not None
    assert opportunity_repo.create_status == OpportunityStatus.PENDING_HUMAN
    assert queue.notifications == [opportunity.id]
    assert queue.ai_replies == []
    assert message_repo.completed is True
    assert message_repo.failed is None
    assert message_repo.projection.actions[0]["requires_approval"] is True
    assert message_repo.execution.executed_by == AnalysisRunExecutor.SERVER


async def test_existing_auto_reply_candidate_is_queued_only_after_agent_completion() -> None:
    owner_id = uuid4()
    message = Message(
        id=uuid4(),
        owner_user_id=owner_id,
        channel=IMChannel.TELEGRAM,
        external_message_id="external-business-1",
        conversation_id="business-chat-1",
        sender_external_id="customer-1",
        sender_display_name="采购负责人",
        direction=MessageDirection.INCOMING,
        text="需要采购 500 套，详情 https://public.example/rfp",
        source_type="private",
        raw_message_links=[],
    )
    opportunity = Opportunity(
        id=uuid4(),
        owner_user_id=owner_id,
        source_message_id=message.id,
        channel=IMChannel.TELEGRAM,
        conversation_id=message.conversation_id,
        source_type="private",
        title="批量采购",
        status=OpportunityStatus.AI_AUTO_REPLY,
    )
    message.opportunity_id = opportunity.id
    message_repo = FakeMessageRepository(message)
    opportunity_repo = FakeOpportunityRepository()
    opportunity_repo.opportunity = opportunity
    queue = FakeTaskQueue()
    use_case = AnalyzeMessageUseCase(
        message_repo=message_repo,  # type: ignore[arg-type]
        opportunity_repo=opportunity_repo,  # type: ignore[arg-type]
        agent=FakeAgent(),
        link_inspector=FakeLinkInspector(),
        task_queue=queue,
        min_opportunity_confidence=0.75,
        max_links=5,
    )

    result = await use_case.execute(message.id)

    assert result is opportunity
    assert message_repo.completed is True
    assert queue.ai_replies == [opportunity.id]


def test_message_links_deduplicates_and_limits_urls() -> None:
    assert message_links(
        "see https://a.example/x and https://b.example/y.",
        ["https://a.example/x"],
        limit=2,
    ) == ["https://a.example/x", "https://b.example/y"]


def test_source_profile_samples_are_bounded_and_redacted() -> None:
    sample = redact_source_sample(
        "联系 recruiter@example.com，电话 +65 8123 4567，@example_recruiter，详情 https://jobs.example.com/1"
    )
    assert "recruiter@example.com" not in sample
    assert "8123 4567" not in sample
    assert "@example_recruiter" not in sample
    assert "jobs.example.com" not in sample
    assert sample == "联系 [email]，电话 [phone]，[handle]，详情 [url]"
