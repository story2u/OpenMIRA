from datetime import datetime, timezone
from typing import Any
from uuid import UUID, uuid4

from sqlalchemy import BigInteger, CheckConstraint, Column, DateTime, Index, UniqueConstraint
from sqlalchemy import Enum as SAEnum
from sqlalchemy.dialects.postgresql import JSONB
from sqlmodel import Field, SQLModel

from app.domain.enums import (
    AgentAnalysisStatus,
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityStatus,
    PlanCode,
    Priority,
    RuleType,
    SubscriptionStatus,
    TelegramConnectionAttemptStatus,
    TelegramConnectionStatus,
    TelegramConnectionType,
    TelegramSourceType,
    UsageFeature,
    UsageStatus,
)


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


class TimestampMixin(SQLModel):
    created_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )


class User(TimestampMixin, table=True):
    __tablename__ = "users"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    email: str = Field(index=True, unique=True)
    display_name: str = Field(default="")
    avatar_url: str = Field(default="")
    password_hash: str | None = None
    is_active: bool = Field(default=True, index=True)
    is_admin: bool = Field(default=False, index=True)
    last_login_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class AuthAccount(TimestampMixin, table=True):
    __tablename__ = "auth_accounts"
    __table_args__ = (
        UniqueConstraint("provider", "provider_subject", name="uq_auth_accounts_provider_subject"),
        Index("ix_auth_accounts_user_provider", "user_id", "provider"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    provider: str = Field(index=True)
    provider_subject: str = Field(index=True)
    email: str | None = Field(default=None, index=True)


class SubscriptionAccount(TimestampMixin, table=True):
    __tablename__ = "subscription_accounts"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_subscription_accounts_user_id"),
        Index("ix_subscription_accounts_status_period_end", "status", "current_period_end"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    plan_code: PlanCode = Field(
        default=PlanCode.FREE,
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True),
    )
    status: SubscriptionStatus = Field(
        default=SubscriptionStatus.INACTIVE,
        sa_column=Column(SAEnum(SubscriptionStatus, native_enum=False), nullable=False, index=True),
    )
    billing_provider: str | None = Field(default=None, max_length=32)
    provider_customer_id: str | None = Field(default=None, max_length=255, index=True)
    provider_subscription_id: str | None = Field(default=None, max_length=255, index=True)
    current_period_start: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    current_period_end: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    cancel_at_period_end: bool = Field(default=False)
    provider_event_version: str | None = Field(default=None, max_length=255)


class Opportunity(TimestampMixin, table=True):
    __tablename__ = "opportunities"
    __table_args__ = (
        Index("ix_opportunities_channel_conversation", "channel", "conversation_id"),
        Index("ix_opportunities_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    conversation_id: str = Field(index=True)
    customer_external_id: str | None = Field(default=None, index=True)
    contact_name: str = Field(default="未知联系人")
    contact_avatar: str = Field(default="")
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None

    source_message_id: UUID | None = Field(default=None, index=True, unique=True)
    title: str
    summary: str | None = None
    matched_keywords: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    raw_message_links: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    trust_score: int = Field(default=70, ge=0, le=100)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    priority: Priority = Field(
        default=Priority.NORMAL,
        sa_column=Column(SAEnum(Priority, native_enum=False), nullable=False, index=True),
    )
    status: OpportunityStatus = Field(
        default=OpportunityStatus.PENDING_HUMAN,
        sa_column=Column(SAEnum(OpportunityStatus, native_enum=False), nullable=False, index=True),
    )
    detection_reason: str | None = None

    link_verification: dict[str, Any] = Field(
        default_factory=lambda: {
            "status": "unverified",
            "verifiedAt": None,
            "riskReasons": [],
            "resolvedInfo": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    extracted_contacts: dict[str, Any] = Field(
        default_factory=lambda: {
            "phone": None,
            "email": None,
            "telegramHandle": None,
            "wecomId": None,
            "extractionSource": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    friend_request_status: str = Field(default="n/a")
    sop_stage: str = Field(default="detected")
    agent_actions: list[dict[str, Any]] = Field(
        default_factory=list,
        sa_column=Column(JSONB, nullable=False),
    )
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True),
    )
    agent_analysis_error: str | None = None
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    attention_required: bool = Field(default=False, index=True)

    ai_reply_draft: str | None = None
    final_reply: str | None = None
    assigned_to: str | None = None
    follow_up_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_message_preview: str = Field(default="")
    last_message_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )


class Message(TimestampMixin, table=True):
    __tablename__ = "messages"
    __table_args__ = (
        UniqueConstraint("channel", "external_message_id", name="uq_message_channel_external"),
        Index("ix_messages_conversation_created", "conversation_id", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    external_message_id: str = Field(index=True)
    conversation_id: str = Field(index=True)
    sender_external_id: str | None = Field(default=None, index=True)
    sender_display_name: str | None = None
    direction: MessageDirection = Field(
        sa_column=Column(SAEnum(MessageDirection, native_enum=False), nullable=False, index=True)
    )
    source: MessageSource | None = Field(
        default=None,
        sa_column=Column(SAEnum(MessageSource, native_enum=False), nullable=True),
    )
    text: str | None = None
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None
    raw_message_links: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    raw_payload: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    opportunity_id: UUID | None = Field(default=None, foreign_key="opportunities.id", index=True)
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True),
    )
    agent_result: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    agent_error: str | None = None
    agent_started_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    sent_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class UsageLedger(TimestampMixin, table=True):
    __tablename__ = "usage_ledger"
    __table_args__ = (
        UniqueConstraint(
            "user_id",
            "feature",
            "idempotency_key",
            name="uq_usage_ledger_user_feature_idempotency",
        ),
        CheckConstraint("quantity > 0", name="ck_usage_ledger_quantity_positive"),
        Index(
            "ix_usage_ledger_user_feature_period_status",
            "user_id",
            "feature",
            "period_start",
            "period_end",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    feature: UsageFeature = Field(
        sa_column=Column(SAEnum(UsageFeature, native_enum=False), nullable=False, index=True)
    )
    quantity: int = Field(default=1, ge=1)
    period_start: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    period_end: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    idempotency_key: str = Field(max_length=255)
    source_message_id: UUID | None = Field(default=None, foreign_key="messages.id", index=True)
    status: UsageStatus = Field(
        default=UsageStatus.RESERVED,
        sa_column=Column(SAEnum(UsageStatus, native_enum=False), nullable=False, index=True),
    )
    consumed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    released_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_reason: str | None = Field(default=None, max_length=500)


class Rule(TimestampMixin, table=True):
    __tablename__ = "rules"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    name: str = Field(index=True)
    enabled: bool = Field(default=True, index=True)
    priority: int = Field(default=100, index=True)
    rule_type: RuleType = Field(
        sa_column=Column(SAEnum(RuleType, native_enum=False), nullable=False, index=True)
    )
    pattern: str
    score: float = Field(default=0.5, ge=0.0, le=1.0)
    extra_data: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))


class AppConfig(SQLModel, table=True):
    __tablename__ = "app_configs"

    key: str = Field(primary_key=True)
    value: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    description: str | None = None
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )


class TelegramUserConfig(TimestampMixin, table=True):
    __tablename__ = "telegram_user_configs"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_telegram_user_configs_user_id"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    enabled: bool = Field(default=False, index=True)
    api_id: int | None = Field(default=None)
    api_hash_encrypted: str | None = None
    session_encrypted: str | None = None
    retention_limit: int | None = Field(default=None, ge=0)
    retention_selected_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramMonitor(TimestampMixin, table=True):
    __tablename__ = "telegram_monitors"
    __table_args__ = (
        UniqueConstraint("user_id", "chat_id", name="uq_telegram_monitors_user_chat"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    telegram_config_id: UUID = Field(foreign_key="telegram_user_configs.id", index=True)
    enabled: bool = Field(default=True, index=True)
    name: str = Field(default="Telegram 群监控")
    chat_id: str = Field(index=True)
    chat_title: str | None = None
    backfill_limit: int = Field(default=30, ge=0, le=500)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = None


class TelegramConnection(TimestampMixin, table=True):
    """A user-owned Telegram identity or integration, never a plaintext secret store."""

    __tablename__ = "telegram_connections"
    __table_args__ = (
        UniqueConstraint(
            "provider_connection_id",
            name="uq_telegram_connections_provider_connection_id",
        ),
        Index(
            "ix_telegram_connections_owner_type_status",
            "owner_user_id",
            "connection_type",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionStatus = Field(
        default=TelegramConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    enabled: bool = Field(default=True, index=True)
    label: str = Field(default="Telegram 连接", max_length=255)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    provider_connection_id: str | None = Field(default=None, max_length=255, index=True)
    credential_encrypted: str | None = None
    connection_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    capabilities: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    last_error: str | None = Field(default=None, max_length=1000)
    last_checked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramSource(TimestampMixin, table=True):
    """A group, channel, or private conversation selected through a connection."""

    __tablename__ = "telegram_sources"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "external_chat_id",
            name="uq_telegram_sources_connection_chat",
        ),
        Index(
            "ix_telegram_sources_owner_enabled",
            "owner_user_id",
            "enabled",
            "quota_paused",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID = Field(foreign_key="telegram_connections.id", index=True)
    source_type: TelegramSourceType = Field(
        sa_column=Column(
            SAEnum(TelegramSourceType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    external_chat_id: str = Field(index=True, max_length=128)
    display_name: str = Field(default="Telegram 来源", max_length=255)
    username: str | None = Field(default=None, max_length=255)
    enabled: bool = Field(default=True, index=True)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = Field(default=None, max_length=1000)


class TelegramConnectionAttempt(TimestampMixin, table=True):
    """A short-lived, owner-bound handshake. The random token itself is never stored."""

    __tablename__ = "telegram_connection_attempts"
    __table_args__ = (
        UniqueConstraint("token_hash", name="uq_telegram_connection_attempts_token_hash"),
        UniqueConstraint(
            "group_request_id",
            name="uq_telegram_connection_attempts_group_request_id",
        ),
        UniqueConstraint(
            "channel_request_id",
            name="uq_telegram_connection_attempts_channel_request_id",
        ),
        Index(
            "ix_telegram_connection_attempts_owner_status_expires",
            "owner_user_id",
            "status",
            "expires_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionAttemptStatus = Field(
        default=TelegramConnectionAttemptStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionAttemptStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    token_hash: str = Field(max_length=128)
    group_request_id: int | None = Field(default=None, index=True)
    channel_request_id: int | None = Field(default=None, index=True)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    attempt_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class TelegramWebhookEvent(TimestampMixin, table=True):
    """Minimal webhook audit and idempotency record; raw payload stays out of persistence."""

    __tablename__ = "telegram_webhook_events"
    __table_args__ = (
        UniqueConstraint("update_id", name="uq_telegram_webhook_events_update_id"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    update_id: int = Field(sa_column=Column(BigInteger, nullable=False, index=True))
    payload_hash: str = Field(max_length=128)
    event_type: str = Field(default="unknown", max_length=64, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class ReplyTemplate(TimestampMixin, table=True):
    __tablename__ = "reply_templates"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    title: str = Field(index=True)
    content: str
    category: str = Field(default="通用", index=True)
    enabled: bool = Field(default=True, index=True)
