from datetime import datetime
from decimal import Decimal
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from app.core.time_window import WorkTimeConfig
from app.domain.enums import (
    AgentActionType,
    AgentAnalysisStatus,
    BillingInterval,
    BillingStore,
    FrontendOpportunityStatus,
    IMChannel,
    JobEligibility,
    JobEmploymentType,
    JobFeedbackType,
    JobMessageClassification,
    JobSeniority,
    JobWorkMode,
    MessageSource,
    OpportunityStatus,
    PlanCode,
    Priority,
    RuleType,
    SalaryPeriod,
    SourcePrimaryFunction,
    SubscriptionStatus,
    TelegramConnectionAttemptStatus,
    TelegramConnectionStatus,
    TelegramConnectionType,
    TelegramSourceType,
    WeComConnectionStatus,
    WeComConnectionType,
    WeComReceiveCapability,
    WeComSendCapability,
    WeComSourceType,
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
    archivedAt: datetime | None = None
    archivedByUserId: UUID | None = None
    archiveReason: str | None = None


class JobSearchProfileWrite(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str = Field(min_length=1, max_length=120)
    isDefault: bool = False
    enabled: bool = True
    targetRoles: list[str] = Field(default_factory=list, max_length=30)
    excludedRoles: list[str] = Field(default_factory=list, max_length=30)
    targetIndustries: list[str] = Field(default_factory=list, max_length=30)
    preferredSeniority: list[JobSeniority] = Field(default_factory=list)
    candidateSkills: list[str] = Field(default_factory=list, max_length=100)
    yearsExperience: float | None = Field(default=None, ge=0, le=80)
    educationLevel: str | None = Field(default=None, max_length=100)
    englishLevel: str | None = Field(default=None, max_length=100)
    otherLanguages: list[str] = Field(default_factory=list, max_length=30)
    preferredCountries: list[str] = Field(default_factory=list, max_length=50)
    preferredCities: list[str] = Field(default_factory=list, max_length=50)
    preferredTimezones: list[str] = Field(default_factory=list, max_length=50)
    workModes: list[JobWorkMode] = Field(default_factory=list)
    employmentTypes: list[JobEmploymentType] = Field(default_factory=list)
    minimumSalary: Decimal | None = Field(default=None, ge=0)
    salaryCurrency: str | None = Field(default=None, min_length=3, max_length=3)
    salaryPeriod: SalaryPeriod | None = None
    visaSponsorshipRequired: bool | None = None
    relocationAcceptable: bool | None = None
    requiredKeywords: list[str] = Field(default_factory=list, max_length=50)
    preferredKeywords: list[str] = Field(default_factory=list, max_length=50)
    excludedKeywords: list[str] = Field(default_factory=list, max_length=50)
    requireSalaryDisclosed: bool = False
    minimumMatchScore: int = Field(default=0, ge=0, le=100)
    notificationEnabled: bool = False


class JobSearchProfileUpdate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str | None = Field(default=None, min_length=1, max_length=120)
    isDefault: bool | None = None
    enabled: bool | None = None
    targetRoles: list[str] | None = Field(default=None, max_length=30)
    excludedRoles: list[str] | None = Field(default=None, max_length=30)
    targetIndustries: list[str] | None = Field(default=None, max_length=30)
    preferredSeniority: list[JobSeniority] | None = None
    candidateSkills: list[str] | None = Field(default=None, max_length=100)
    yearsExperience: float | None = Field(default=None, ge=0, le=80)
    educationLevel: str | None = Field(default=None, max_length=100)
    englishLevel: str | None = Field(default=None, max_length=100)
    otherLanguages: list[str] | None = Field(default=None, max_length=30)
    preferredCountries: list[str] | None = Field(default=None, max_length=50)
    preferredCities: list[str] | None = Field(default=None, max_length=50)
    preferredTimezones: list[str] | None = Field(default=None, max_length=50)
    workModes: list[JobWorkMode] | None = None
    employmentTypes: list[JobEmploymentType] | None = None
    minimumSalary: Decimal | None = Field(default=None, ge=0)
    salaryCurrency: str | None = Field(default=None, min_length=3, max_length=3)
    salaryPeriod: SalaryPeriod | None = None
    visaSponsorshipRequired: bool | None = None
    relocationAcceptable: bool | None = None
    requiredKeywords: list[str] | None = Field(default=None, max_length=50)
    preferredKeywords: list[str] | None = Field(default=None, max_length=50)
    excludedKeywords: list[str] | None = Field(default=None, max_length=50)
    requireSalaryDisclosed: bool | None = None
    minimumMatchScore: int | None = Field(default=None, ge=0, le=100)
    notificationEnabled: bool | None = None

    @model_validator(mode="after")
    def reject_null_for_required_fields(self):
        required = {
            "name",
            "isDefault",
            "enabled",
            "targetRoles",
            "excludedRoles",
            "targetIndustries",
            "preferredSeniority",
            "candidateSkills",
            "otherLanguages",
            "preferredCountries",
            "preferredCities",
            "preferredTimezones",
            "workModes",
            "employmentTypes",
            "requiredKeywords",
            "preferredKeywords",
            "excludedKeywords",
            "requireSalaryDisclosed",
            "minimumMatchScore",
            "notificationEnabled",
        }
        null_fields = [name for name in required & self.model_fields_set if getattr(self, name) is None]
        if null_fields:
            raise ValueError(f"fields cannot be null: {', '.join(sorted(null_fields))}")
        return self


class JobSearchProfileRead(JobSearchProfileWrite):
    id: UUID
    createdAt: datetime
    updatedAt: datetime


class JobProfileParseRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    text: str = Field(min_length=5, max_length=4000)


class JobProfileParseRead(JobSearchProfileWrite):
    requiresConfirmation: bool = True


class JobMatchRead(BaseModel):
    eligibility: JobEligibility
    matchScore: int = Field(ge=0, le=100)
    matchedReasons: list[str] = Field(default_factory=list)
    mismatchReasons: list[str] = Field(default_factory=list)
    unknownConstraints: list[str] = Field(default_factory=list)
    scoreBreakdown: dict[str, int] = Field(default_factory=dict)


class JobSourceRead(BaseModel):
    id: UUID
    channel: IMChannel
    chatName: str | None = None
    authorName: str | None = None
    postedAt: datetime
    sourceMessageUrl: str | None = None
    reliabilityScore: float


class JobOpportunityRead(BaseModel):
    opportunityId: UUID
    jobTitle: str
    companyName: str | None = None
    sourceChannel: IMChannel
    sourceChatName: str | None = None
    postedAt: datetime
    locationText: str | None = None
    countryCode: str | None = None
    city: str | None = None
    workMode: JobWorkMode
    employmentType: JobEmploymentType
    seniority: JobSeniority
    salaryRaw: str | None = None
    salaryMin: Decimal | None = None
    salaryMax: Decimal | None = None
    salaryCurrency: str | None = None
    salaryPeriod: SalaryPeriod
    requiredSkills: list[str] = Field(default_factory=list)
    degreeLevel: str | None = None
    englishLevel: str | None = None
    visaSponsorship: bool | None = None
    applicationDeadline: datetime | None = None
    sourceReliabilityScore: float
    extractionConfidence: float
    sourceCount: int = 1
    conflictingSourceData: bool = False
    complianceFlags: list[str] = Field(default_factory=list)
    isExpired: bool = False
    match: JobMatchRead | None = None


class JobOpportunityDetailRead(JobOpportunityRead):
    sourceMessageUrl: str | None = None
    sourceAuthorName: str | None = None
    department: str | None = None
    companyIndustry: str | None = None
    companyStage: str | None = None
    timezone: str | None = None
    salaryNegotiable: bool | None = None
    equityMentioned: bool | None = None
    requirementsSummary: str | None = None
    preferredSkills: list[str] = Field(default_factory=list)
    minimumYearsExperience: float | None = None
    maximumYearsExperience: float | None = None
    degreeRequired: bool | None = None
    degreeField: str | None = None
    otherLanguageRequirements: list[str] = Field(default_factory=list)
    workAuthorizationText: str | None = None
    relocationSupport: bool | None = None
    ageRequirementText: str | None = None
    ageRequirementPresent: bool = False
    applicationUrl: str | None = None
    contactMethods: list[dict] = Field(default_factory=list)
    missingFields: list[str] = Field(default_factory=list)
    fieldEvidence: dict[str, str] = Field(default_factory=dict)
    rawExcerpt: str
    expiredReason: str | None = None
    sources: list[JobSourceRead] = Field(default_factory=list)


class JobsPageRead(BaseModel):
    items: list[JobOpportunityRead]
    total: int
    limit: int
    offset: int
    filterSummary: dict
    profile: JobSearchProfileRead | None = None


class JobFeedbackRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    feedbackType: JobFeedbackType
    note: str | None = Field(default=None, max_length=1000)


class JobFeedbackRead(BaseModel):
    id: UUID
    feedbackType: JobFeedbackType
    note: str | None = None
    updatedAt: datetime


class JobMessageAuditRead(BaseModel):
    id: UUID
    messageId: UUID
    channel: IMChannel
    sourceName: str | None = None
    messageExcerpt: str
    classification: JobMessageClassification
    confidence: float
    filterReason: str | None = None
    prefilterScore: float
    agentRequired: bool
    manuallyCorrected: bool
    sentAt: datetime
    updatedAt: datetime


class JobMessageAuditPageRead(BaseModel):
    items: list[JobMessageAuditRead]
    total: int
    limit: int
    offset: int


class JobMessageAuditCorrectionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    isJob: bool
    note: str | None = Field(default=None, max_length=500)


class SourceFunctionalProfileRead(BaseModel):
    id: UUID
    channel: IMChannel
    externalSourceId: str
    sourceDisplayName: str
    sourceDescription: str | None = None
    primaryFunction: SourcePrimaryFunction
    effectiveFunction: SourcePrimaryFunction
    secondaryFunctions: list[str]
    industryTags: list[str]
    regionTags: list[str]
    languageTags: list[str]
    jobSignalPrior: float
    estimatedNoiseLevel: float
    reliabilityScore: float
    confidence: float
    evidence: list[str]
    manualOverride: SourcePrimaryFunction | None = None
    sampledMessageCount: int
    profiledAt: datetime
    expiresAt: datetime


class SourceFunctionOverrideRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    override: SourcePrimaryFunction | None


class OpportunityDetailRead(OpportunityRead):
    aiReplyDraft: str | None = None
    finalReply: str | None = None
    detectionReason: str | None = None


class DashboardRead(BaseModel):
    """商机看板聚合响应：分页结果 + 不受分页影响的总量/重大商机/关键词选项。"""

    items: list[OpportunityRead]
    total: int
    limit: int
    offset: int
    pendingCount: int
    attentionItems: list[OpportunityRead] = Field(default_factory=list)
    keywordOptions: list[str] = Field(default_factory=list)


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


class FriendRequestUpdate(BaseModel):
    """好友申请状态流转（operator 手动驱动：发送/确认通过/确认被拒/重试）。

    平台侧没有自动发好友申请的 IM 能力；本端点只持久化操作员声明的真实进度，
    对方是否通过由操作员在 IM 客户端确认后回填，禁止任何自动伪造"已通过"。
    """

    status: Literal["not_sent", "pending", "accepted", "rejected"]


class OpportunityArchiveRequest(BaseModel):
    reason: str | None = Field(default=None, max_length=500)


class OpportunityBulkArchiveRequest(OpportunityArchiveRequest):
    opportunityIds: list[UUID] = Field(min_length=1, max_length=100)

    @field_validator("opportunityIds")
    @classmethod
    def require_unique_opportunity_ids(cls, value: list[UUID]) -> list[UUID]:
        if len(set(value)) != len(value):
            raise ValueError("opportunityIds must be unique")
        return value


class OpportunityBulkArchiveRead(BaseModel):
    archivedCount: int
    opportunities: list[OpportunityRead]


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
    hasPassword: bool = False


class AuthTokenRead(BaseModel):
    accessToken: str
    tokenType: str = "bearer"
    user: AuthUserRead


class PlanEntitlementsRead(BaseModel):
    planCode: PlanCode
    telegramGroupLimit: int | None
    wecomGroupLimit: int | None
    combinedGroupLimit: int
    piAgentAnalysisMonthlyLimit: int


class SubscriptionUsageRead(BaseModel):
    planCode: PlanCode
    subscriptionStatus: SubscriptionStatus
    periodStart: datetime
    periodEnd: datetime
    cancelAtPeriodEnd: bool = False
    entitlements: PlanEntitlementsRead
    telegramGroupsUsed: int
    wecomGroupsUsed: int
    combinedGroupsUsed: int
    aiAnalysesConsumed: int
    aiAnalysesReserved: int
    aiAnalysesRemaining: int
    effectiveStore: BillingStore | None = None
    billingInterval: BillingInterval | None = None
    billingPeriodStart: datetime | None = None
    billingPeriodEnd: datetime | None = None
    usagePeriodStart: datetime
    usagePeriodEnd: datetime
    entitlementExpiresAt: datetime | None = None
    willRenew: bool = False
    billingIssue: bool = False
    multipleActiveSubscriptions: bool = False
    managementUrl: str | None = None
    lastSyncedAt: datetime | None = None


class SubscriptionCatalogPlanRead(BaseModel):
    planCode: PlanCode
    displayName: str
    rank: int
    entitlements: PlanEntitlementsRead
    availableIntervals: list[BillingInterval]
    revenuecatPackageIdentifiers: list[str]


class SubscriptionManagementRead(BaseModel):
    store: BillingStore | None = None
    managementUrl: str | None = None
    instruction: str
    canOpenInCurrentClient: bool


class OAuthAuthorizeRead(BaseModel):
    authorizationUrl: str


class NativeLoginRequest(BaseModel):
    """移动端原生登录：App 用系统 SDK 取得 provider id_token 后换取本服务 JWT。"""

    idToken: str = Field(min_length=16, max_length=8192)


class PasswordLoginRequest(BaseModel):
    """已有账户使用邮箱和密码换取访问令牌。"""

    email: str = Field(
        min_length=3,
        max_length=320,
        pattern=r"^[^@\s]+@[^@\s]+\.[^@\s]+$",
    )
    password: str = Field(min_length=1, max_length=128)

    @field_validator("email", mode="before")
    @classmethod
    def normalize_email(cls, value: object) -> object:
        return value.strip().lower() if isinstance(value, str) else value


class PasswordChangeRequest(BaseModel):
    currentPassword: str = Field(min_length=1, max_length=128)
    newPassword: str = Field(min_length=10, max_length=128)


class PasswordResetRequest(BaseModel):
    email: str = Field(
        min_length=3,
        max_length=320,
        pattern=r"^[^@\s]+@[^@\s]+\.[^@\s]+$",
    )

    @field_validator("email", mode="before")
    @classmethod
    def normalize_email(cls, value: object) -> object:
        return value.strip().lower() if isinstance(value, str) else value


class PasswordResetConfirmRequest(BaseModel):
    newPassword: str = Field(min_length=10, max_length=128)
    token: str | None = Field(default=None, min_length=32, max_length=256)
    email: str | None = Field(
        default=None,
        min_length=3,
        max_length=320,
        pattern=r"^[^@\s]+@[^@\s]+\.[^@\s]+$",
    )
    code: str | None = Field(default=None, min_length=10, max_length=10, pattern=r"^[A-Z2-9]+$")

    @field_validator("email", mode="before")
    @classmethod
    def normalize_optional_email(cls, value: object) -> object:
        return value.strip().lower() if isinstance(value, str) else value

    @field_validator("code", mode="before")
    @classmethod
    def normalize_code(cls, value: object) -> object:
        return value.replace(" ", "").upper() if isinstance(value, str) else value

    @model_validator(mode="after")
    def validate_reset_credential(self) -> "PasswordResetConfirmRequest":
        has_token = bool(self.token)
        has_code = bool(self.email and self.code)
        if has_token == has_code:
            raise ValueError("provide either token or email and code")
        return self


class PasswordActionRead(BaseModel):
    message: str


class TelegramMonitorRead(BaseModel):
    id: UUID
    enabled: bool
    name: str
    chatId: str
    chatTitle: str | None = None
    backfillLimit: int = 30
    quotaPaused: bool = False
    quotaReason: str | None = None
    lastError: str | None = None
    updatedAt: datetime | None = None


class TelegramUserConfigRead(BaseModel):
    apiId: int | None = None
    apiHashConfigured: bool = False
    sessionConfigured: bool = False
    monitors: list[TelegramMonitorRead] = Field(default_factory=list)
    monitorLimit: int = 1
    canCreateMore: bool = False
    activeMonitorCount: int = 0
    storedMonitorCount: int = 0
    retentionSelectionRequired: bool = False
    retentionSelectedAt: datetime | None = None
    updatedAt: datetime | None = None


class TelegramUserConfigUpdate(BaseModel):
    enabled: bool = False
    apiId: int | None = Field(default=None, ge=1)
    apiHash: str | None = Field(default=None, max_length=512)
    sessionString: str | None = Field(default=None, max_length=10000)
    chats: list[str | int] = Field(default_factory=list)
    backfillLimit: int = Field(default=30, ge=0, le=500)


class TelegramMonitorRetentionUpdate(BaseModel):
    monitorIds: list[UUID] = Field(default_factory=list, max_length=100)


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


class TelegramSourceRead(BaseModel):
    id: UUID
    connectionId: UUID
    sourceType: TelegramSourceType
    externalChatId: str
    displayName: str
    username: str | None = None
    enabled: bool
    autoReplyEnabled: bool = False
    autoReplyEligible: bool = False
    quotaPaused: bool
    quotaReason: str | None = None
    lastError: str | None = None
    updatedAt: datetime


class TelegramConnectionRead(BaseModel):
    id: UUID
    connectionType: TelegramConnectionType
    status: TelegramConnectionStatus
    enabled: bool
    label: str
    capabilities: dict = Field(default_factory=dict)
    lastError: str | None = None
    lastCheckedAt: datetime | None = None
    updatedAt: datetime
    sources: list[TelegramSourceRead] = Field(default_factory=list)


class TelegramConnectionAttemptRead(BaseModel):
    id: UUID
    connectionType: TelegramConnectionType
    status: TelegramConnectionAttemptStatus
    expiresAt: datetime
    connectionId: UUID | None = None
    error: str | None = None
    telegramUrl: str | None = None
    qrCodeUrl: str | None = None
    instructions: list[str] = Field(default_factory=list)
    localMock: bool = False


class TelegramConnectionHealthRead(BaseModel):
    mode: str
    botConfigured: bool
    botUsername: str | None = None
    businessAvailable: bool
    mtprotoQrAvailable: bool
    listenerMode: str
    legacyMonitoringActive: bool = False
    legacyActiveSourceCount: int = 0
    message: str | None = None


class TelegramMtprotoDialogRead(BaseModel):
    id: str
    sourceType: TelegramSourceType
    displayName: str
    username: str | None = None


class TelegramMtprotoSourceCreate(BaseModel):
    chatId: str = Field(min_length=1, max_length=128)


class WeComConnectionCreate(BaseModel):
    displayName: str = Field(default="企业微信自建应用", min_length=1, max_length=255)
    corpId: str = Field(min_length=2, max_length=128)
    agentId: str = Field(pattern=r"^\d{1,20}$")
    secret: str = Field(min_length=8, max_length=512)
    token: str = Field(min_length=3, max_length=128)
    encodingAesKey: str = Field(min_length=43, max_length=43)


class WeComSourceRead(BaseModel):
    id: UUID
    connectionId: UUID | None = None
    archiveConnectionId: UUID | None = None
    sourceType: WeComSourceType
    externalConversationId: str
    displayName: str
    receiveCapability: WeComReceiveCapability
    sendCapability: WeComSendCapability
    enabled: bool
    quotaPaused: bool
    quotaReason: str | None = None
    lastMessageAt: datetime | None = None
    lastError: str | None = None


class WeComConnectionRead(BaseModel):
    id: UUID
    connectionType: WeComConnectionType
    status: WeComConnectionStatus
    enabled: bool
    displayName: str
    corpId: str
    agentId: str
    callbackUrl: str
    credentialConfigured: bool = True
    lastVerifiedAt: datetime | None = None
    lastError: str | None = None
    updatedAt: datetime
    sources: list[WeComSourceRead] = Field(default_factory=list)


class WeComArchiveConnectionCreate(BaseModel):
    displayName: str = Field(default="企业微信会话存档", min_length=1, max_length=255)
    corpId: str = Field(min_length=2, max_length=128)
    archiveSecret: str = Field(min_length=8, max_length=512)
    privateKeyPem: str = Field(min_length=100, max_length=16_384)
    publicKeyVersion: int = Field(ge=1)
    wecomUserId: str = Field(min_length=1, max_length=128, pattern=r"^[^\s]+$")
    memberDisplayName: str = Field(default="企业微信成员", min_length=1, max_length=255)


class WeComArchiveMemberBindingRead(BaseModel):
    id: UUID
    wecomUserId: str
    displayName: str
    enabled: bool
    lastMatchedAt: datetime | None = None


class WeComArchiveConnectionRead(BaseModel):
    id: UUID
    status: WeComConnectionStatus
    enabled: bool
    displayName: str
    corpId: str
    publicKeyVersion: int
    credentialConfigured: bool = True
    sdkConfigured: bool
    lastSequence: int
    lastVerifiedAt: datetime | None = None
    lastPolledAt: datetime | None = None
    lastError: str | None = None
    updatedAt: datetime
    member: WeComArchiveMemberBindingRead
    sources: list[WeComSourceRead] = Field(default_factory=list)


class WeComArchiveSyncAccepted(BaseModel):
    accepted: bool = True


# MARK: 用户级设置（detection / work-schedule / notifications）

MAX_DETECTION_KEYWORDS = 50
MAX_DETECTION_KEYWORD_LEN = 64


def _normalize_keywords(values: list[str]) -> list[str]:
    """去空格、丢空串、按首次出现顺序去重，并限制单个长度。"""
    seen: set[str] = set()
    result: list[str] = []
    for raw in values:
        keyword = raw.strip()
        if not keyword:
            continue
        if len(keyword) > MAX_DETECTION_KEYWORD_LEN:
            raise ValueError(f"关键词长度不能超过 {MAX_DETECTION_KEYWORD_LEN} 个字符")
        lowered = keyword.lower()
        if lowered in seen:
            continue
        seen.add(lowered)
        result.append(keyword)
    if len(result) > MAX_DETECTION_KEYWORDS:
        raise ValueError(f"关键词数量不能超过 {MAX_DETECTION_KEYWORDS} 个")
    return result


class DetectionSettingsRead(BaseModel):
    keywords: list[str] = Field(default_factory=list)
    aiSemanticsEnabled: bool = True


class DetectionSettingsUpdate(BaseModel):
    keywords: list[str] = Field(default_factory=list, max_length=200)
    aiSemanticsEnabled: bool = True

    @field_validator("keywords")
    @classmethod
    def validate_keywords(cls, value: list[str]) -> list[str]:
        return _normalize_keywords(value)


class WorkScheduleSlot(BaseModel):
    """一个连续人工审核时段：weekday 为 ISO 星期(1=周一..7=周日)，端点为 HH:MM。"""

    weekday: int = Field(ge=1, le=7)
    start: str = Field(pattern=r"^([01]\d|2[0-3]):[0-5]\d$")
    end: str = Field(pattern=r"^([01]\d|2[0-3]):[0-5]\d$")

    @field_validator("end")
    @classmethod
    def end_after_start(cls, value: str, info) -> str:
        start = info.data.get("start")
        if start and value <= start:
            raise ValueError("结束时间必须晚于开始时间")
        return value


class WorkScheduleRead(BaseModel):
    timezone: str = "Asia/Shanghai"
    slots: list[WorkScheduleSlot] = Field(default_factory=list)
    autoReplyOutsideHours: bool = False
    isDefault: bool = False


class WorkScheduleUpdate(BaseModel):
    timezone: str = Field(min_length=1, max_length=64)
    slots: list[WorkScheduleSlot] = Field(default_factory=list, max_length=168)
    autoReplyOutsideHours: bool = False

    @field_validator("timezone")
    @classmethod
    def validate_timezone(cls, value: str) -> str:
        from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

        try:
            ZoneInfo(value)
        except (ZoneInfoNotFoundError, ValueError) as exc:
            raise ValueError("无效的 IANA 时区标识") from exc
        return value


class NotificationSettingsRead(BaseModel):
    newOpportunityEnabled: bool = True
    aiRepliedEnabled: bool = True
    dailyDigestEnabled: bool = False
    urgentOnly: bool = False


class NotificationSettingsUpdate(NotificationSettingsRead):
    pass


class SettingsCapabilitiesRead(BaseModel):
    """能力位：诚实告诉客户端哪些下游能力尚未开放，避免伪装可用。"""

    pushAvailable: bool = False
    wecomUserBindingAvailable: bool = False


class SettingsBundleRead(BaseModel):
    detection: DetectionSettingsRead
    workSchedule: WorkScheduleRead
    notifications: NotificationSettingsRead
    capabilities: SettingsCapabilitiesRead
