from datetime import datetime
from typing import Annotated, Any, Protocol
from uuid import UUID

from pydantic import BaseModel, Field

from app.domain.enums import (
    AgentActionType,
    IMChannel,
    LinkSafetyStatus,
    MessageDirection,
    Priority,
    RuleType,
)


class InboundMessage(BaseModel):
    owner_user_id: UUID | None = None
    channel: IMChannel
    external_message_id: str
    conversation_id: str
    sender_external_id: str | None = None
    sender_display_name: str | None = None
    text: str | None = None
    source_type: str = "private"
    group_name: str | None = None
    raw_message_links: list[str] = Field(default_factory=list)
    raw_payload: dict[str, Any] = Field(default_factory=dict)
    force_human_review: bool = False


class SendReceipt(BaseModel):
    provider_message_id: str | None = None
    raw_response: dict[str, Any] = Field(default_factory=dict)


class DetectionRule(BaseModel):
    id: UUID
    name: str
    rule_type: RuleType
    pattern: str
    score: float
    priority: int


class DetectionResult(BaseModel):
    is_opportunity: bool
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    title: str | None = Field(default=None, max_length=200)
    summary: str | None = Field(default=None, max_length=2000)
    reason: str | None = Field(default=None, max_length=1000)
    matched_keywords: list[Annotated[str, Field(max_length=100)]] = Field(
        default_factory=list,
        max_length=20,
    )
    priority: Priority = Priority.NORMAL
    requires_human_review: bool = False


class ConversationTurn(BaseModel):
    sender_display_name: str | None = Field(default=None, max_length=256)
    direction: MessageDirection
    text: str = Field(max_length=1000)


class OpportunityClassificationRequest(BaseModel):
    text: str = Field(max_length=20000)
    rule_score: float = Field(ge=0.0, le=1.0)
    matched_keywords: list[Annotated[str, Field(max_length=100)]] = Field(
        default_factory=list,
        max_length=20,
    )
    ai_hints: list[str] = Field(default_factory=list, max_length=20)
    conversation: list[ConversationTurn] = Field(default_factory=list, max_length=10)
    source_type: str = Field(default="private", max_length=32)
    group_name: str | None = Field(default=None, max_length=256)


class LinkInspection(BaseModel):
    url: str = Field(max_length=2048)
    final_url: str | None = Field(default=None, max_length=2048)
    status: LinkSafetyStatus
    http_status: int | None = None
    content_type: str | None = Field(default=None, max_length=128)
    title: str | None = Field(default=None, max_length=500)
    text: str = Field(default="", max_length=20000)
    emails: list[str] = Field(default_factory=list, max_length=20)
    risk_reasons: list[str] = Field(default_factory=list, max_length=20)


class AgentAnalysisRequest(BaseModel):
    message_id: UUID
    channel: IMChannel
    sender_display_name: str | None = Field(default=None, max_length=256)
    source_type: str = Field(default="private", max_length=32)
    group_name: str | None = Field(default=None, max_length=256)
    text: str = Field(default="", max_length=20000)
    links: list[LinkInspection] = Field(default_factory=list, max_length=10)


class AgentContactExtraction(BaseModel):
    email: str | None = Field(default=None, max_length=320)
    phone: str | None = Field(default=None, max_length=64)
    telegram_handle: str | None = Field(default=None, max_length=128)
    wecom_id: str | None = Field(default=None, max_length=128)
    extraction_source: str | None = Field(default=None, max_length=32)


class AgentActionRecommendation(BaseModel):
    action_type: AgentActionType
    reason: str = Field(min_length=1, max_length=1000)
    target: str | None = Field(default=None, max_length=320)
    draft: str | None = Field(default=None, max_length=4000)
    requires_approval: bool = True


class AgentAnalysisResult(BaseModel):
    is_opportunity: bool
    confidence: float = Field(ge=0.0, le=1.0)
    title: str = Field(min_length=1, max_length=200)
    summary: str = Field(min_length=1, max_length=2000)
    priority: Priority = Priority.NORMAL
    trust_score: int = Field(default=70, ge=0, le=100)
    attention_required: bool = False
    link_status: LinkSafetyStatus = LinkSafetyStatus.UNVERIFIED
    link_summary: str | None = Field(default=None, max_length=2000)
    risk_flags: list[str] = Field(default_factory=list, max_length=20)
    contacts: AgentContactExtraction = Field(default_factory=AgentContactExtraction)
    actions: list[AgentActionRecommendation] = Field(default_factory=list, max_length=8)


class AgentAnalysisProjection(BaseModel):
    result: AgentAnalysisResult
    link_verification: dict[str, Any]
    extracted_contacts: dict[str, Any]
    actions: list[dict[str, Any]]
    attention_required: bool
    analyzed_at: datetime


class IMAdapter(Protocol):
    channel: IMChannel

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
        query: dict[str, str] | None = None,
    ) -> InboundMessage | None: ...

    async def send_message(
        self,
        conversation_id: str,
        text: str,
        *,
        idempotency_key: str | None = None,
        opportunity_id: UUID | None = None,
        owner_user_id: UUID | None = None,
    ) -> SendReceipt: ...


class OpportunityAIClassifier(Protocol):
    async def classify(
        self,
        request: OpportunityClassificationRequest,
    ) -> DetectionResult | None: ...


class ReplyGenerator(Protocol):
    async def generate_reply(self, opportunity_id: UUID) -> str: ...


class MessageAgent(Protocol):
    async def analyze(self, request: AgentAnalysisRequest) -> AgentAnalysisResult: ...


class LinkInspector(Protocol):
    async def inspect_many(self, urls: list[str]) -> list[LinkInspection]: ...


class TaskQueue(Protocol):
    def enqueue_ai_reply(self, opportunity_id: UUID) -> None: ...

    def notify_reviewers(self, opportunity_id: UUID) -> None: ...

    def enqueue_agent_analysis(
        self,
        message_id: UUID,
        *,
        force: bool = False,
        usage_ledger_id: UUID | None = None,
    ) -> bool: ...
