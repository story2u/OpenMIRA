from datetime import datetime, timezone
from typing import Any
from uuid import UUID, uuid4

from sqlalchemy import Column, DateTime, Enum as SAEnum, Index, UniqueConstraint
from sqlalchemy.dialects.postgresql import JSONB
from sqlmodel import Field, SQLModel

from app.domain.enums import (
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityStatus,
    Priority,
    RuleType,
)


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


class TimestampMixin(SQLModel):
    created_at: datetime = Field(default_factory=utc_now)
    updated_at: datetime = Field(default_factory=utc_now)


class Opportunity(TimestampMixin, table=True):
    __tablename__ = "opportunities"
    __table_args__ = (
        Index("ix_opportunities_channel_conversation", "channel", "conversation_id"),
        Index("ix_opportunities_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    conversation_id: str = Field(index=True)
    customer_external_id: str | None = Field(default=None, index=True)
    contact_name: str = Field(default="未知联系人")
    contact_avatar: str = Field(default="")

    source_message_id: UUID | None = Field(default=None, index=True)
    title: str
    summary: str | None = None
    matched_keywords: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
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
    raw_payload: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    opportunity_id: UUID | None = Field(default=None, foreign_key="opportunities.id", index=True)
    sent_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


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


class ReplyTemplate(TimestampMixin, table=True):
    __tablename__ = "reply_templates"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    title: str = Field(index=True)
    content: str
    category: str = Field(default="通用", index=True)
    enabled: bool = Field(default=True, index=True)
