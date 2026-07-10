from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

from app.core.time_window import WorkTimeConfig
from app.domain.enums import (
    AgentActionType,
    AgentAnalysisStatus,
    FrontendOpportunityStatus,
    IMChannel,
    MessageSource,
    OpportunityStatus,
    Priority,
    RuleType,
)


class AgentActionRead(BaseModel):
    actionType: AgentActionType
    reason: str
    target: str | None = None
    draft: str | None = None
    requiresApproval: bool = True


class OpportunityRead(BaseModel):
    id: UUID
    platform: IMChannel
    contactName: str
    contactAvatar: str
    summary: str
    matchedKeywords: list[str]
    confidenceScore: float
    status: FrontendOpportunityStatus
    internalStatus: OpportunityStatus
    priority: Priority
    lastMessagePreview: str
    createdAt: datetime
    updatedAt: datetime
    sourceType: str = "private"
    groupName: str | None = None
    groupMemberRole: str = "member"
    rawMessageLinks: list[str] = Field(default_factory=list)
    linkVerification: dict = Field(default_factory=dict)
    extractedContacts: dict = Field(default_factory=dict)
    friendRequestStatus: str = "not_sent"
    sopStage: str = "detected"
    trustScore: int = 70
    agentActions: list[AgentActionRead] = Field(default_factory=list)
    agentAnalysisStatus: AgentAnalysisStatus = AgentAnalysisStatus.NOT_REQUESTED
    agentAnalysisError: str | None = None
    agentAnalyzedAt: datetime | None = None
    attentionRequired: bool = False


class OpportunityDetailRead(OpportunityRead):
    aiReplyDraft: str | None = None
    finalReply: str | None = None
    detectionReason: str | None = None


class ChatMessageRead(BaseModel):
    id: UUID
    senderName: str
    content: str
    isFromContact: bool
    sentAt: datetime
    source: MessageSource | None


class ManualReplyRequest(BaseModel):
    text: str = Field(min_length=1, max_length=4000)
    operator_id: str = Field(default="operator", min_length=1, max_length=128)
    mark_following: bool = True


class AIDraftResponse(BaseModel):
    opportunity_id: UUID
    draft: str


class AgentAnalysisEnqueueRead(BaseModel):
    messageId: UUID
    status: AgentAnalysisStatus


class OpportunityStatusUpdate(BaseModel):
    status: OpportunityStatus


class RuleCreate(BaseModel):
    name: str = Field(min_length=1, max_length=128)
    rule_type: RuleType
    pattern: str = Field(min_length=1, max_length=500)
    score: float = Field(default=0.5, ge=0.0, le=1.0)
    priority: int = Field(default=100, ge=1, le=1000)
    enabled: bool = True


class RuleUpdate(BaseModel):
    name: str | None = Field(default=None, min_length=1, max_length=128)
    rule_type: RuleType | None = None
    pattern: str | None = Field(default=None, min_length=1, max_length=500)
    score: float | None = Field(default=None, ge=0.0, le=1.0)
    priority: int | None = Field(default=None, ge=1, le=1000)
    enabled: bool | None = None


class RuleRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: UUID
    name: str
    enabled: bool
    priority: int
    rule_type: RuleType
    pattern: str
    score: float
    created_at: datetime
    updated_at: datetime


class ConfigRead(BaseModel):
    key: str
    value: dict
    description: str | None = None
    updated_at: datetime | None = None


class ConfigUpdate(BaseModel):
    value: dict
    description: str | None = None


class WorkModeRead(BaseModel):
    mode: str
    is_working_time: bool
    work_time: WorkTimeConfig


class ReplyTemplateCreate(BaseModel):
    title: str = Field(min_length=1, max_length=128)
    content: str = Field(min_length=1, max_length=4000)
    category: str = Field(default="通用", max_length=64)


class ReplyTemplateUpdate(BaseModel):
    title: str | None = Field(default=None, min_length=1, max_length=128)
    content: str | None = Field(default=None, min_length=1, max_length=4000)
    category: str | None = Field(default=None, max_length=64)
    enabled: bool | None = None


class ReplyTemplateRead(BaseModel):
    id: UUID
    title: str
    content: str
    category: str


class StatsSummaryRead(BaseModel):
    total: int
    pending: int
    replied: int
    ignored: int
    avgConfidence: float


class AuthUserRead(BaseModel):
    id: UUID
    email: str
    displayName: str
    avatarUrl: str = ""
    isAdmin: bool = False


class AuthTokenRead(BaseModel):
    accessToken: str
    tokenType: str = "bearer"
    user: AuthUserRead


class OAuthAuthorizeRead(BaseModel):
    authorizationUrl: str


class TelegramMonitorRead(BaseModel):
    id: UUID
    enabled: bool
    name: str
    chatId: str
    chatTitle: str | None = None
    backfillLimit: int = 30
    lastError: str | None = None
    updatedAt: datetime | None = None


class TelegramUserConfigRead(BaseModel):
    apiId: int | None = None
    apiHashConfigured: bool = False
    sessionConfigured: bool = False
    monitors: list[TelegramMonitorRead] = Field(default_factory=list)
    monitorLimit: int = 1
    canCreateMore: bool = False
    updatedAt: datetime | None = None


class TelegramUserConfigUpdate(BaseModel):
    enabled: bool = False
    apiId: int | None = Field(default=None, ge=1)
    apiHash: str | None = Field(default=None, max_length=512)
    sessionString: str | None = Field(default=None, max_length=10000)
    chats: list[str | int] = Field(default_factory=list)
    backfillLimit: int = Field(default=30, ge=0, le=500)


class TelegramSendCodeRequest(BaseModel):
    apiId: int = Field(ge=1)
    apiHash: str = Field(min_length=1, max_length=512)
    phone: str = Field(min_length=5, max_length=64)


class TelegramSendCodeRead(BaseModel):
    loginId: str
    expiresInSeconds: int


class TelegramVerifyCodeRequest(BaseModel):
    loginId: str = Field(min_length=16, max_length=128)
    code: str = Field(min_length=2, max_length=32)
    password: str | None = Field(default=None, max_length=256)


class TelegramVerifyCodeRead(BaseModel):
    status: str
    config: TelegramUserConfigRead | None = None


class TelegramDialogRead(BaseModel):
    id: int
    name: str
    username: str | None = None
