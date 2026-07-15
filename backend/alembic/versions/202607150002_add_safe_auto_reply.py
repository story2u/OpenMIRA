"""add safe AI auto reply policy state and delivery ledger

Revision ID: 202607150002
Revises: 202607150001
Create Date: 2026-07-15
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "202607150002"
down_revision: str | None = "202607150001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

IM_CHANNEL = sa.Enum("TELEGRAM", "WECOM", native_enum=False)
DELIVERY_STATUS = sa.Enum(
    "CANDIDATE",
    "BLOCKED",
    "GENERATING",
    "READY",
    "SENDING",
    "SENT",
    "FAILED",
    "DRY_RUN",
    "CANCELED",
    native_enum=False,
)
DECISION_REASON = sa.Enum(
    "ELIGIBLE",
    "FEATURE_DISABLED",
    "SEND_DISABLED",
    "USER_DISABLED",
    "WORKING_HOURS",
    "SOURCE_NOT_ELIGIBLE",
    "SOURCE_DISABLED",
    "HUMAN_REVIEW_REQUIRED",
    "AGENT_NOT_COMPLETED",
    "LOW_CONFIDENCE",
    "ATTENTION_REQUIRED",
    "UNSAFE_LINK",
    "SENSITIVE_INTENT",
    "COOLDOWN_ACTIVE",
    "WINDOW_LIMIT_REACHED",
    "OPPORTUNITY_INACTIVE",
    "DRAFT_UNSAFE",
    "DUPLICATE",
    "DELIVERY_DRY_RUN",
    "PROVIDER_ERROR",
    native_enum=False,
)


def upgrade() -> None:
    op.add_column(
        "telegram_sources",
        sa.Column("auto_reply_enabled", sa.Boolean(), nullable=False, server_default=sa.false()),
    )
    op.create_index(
        "ix_telegram_sources_auto_reply_enabled",
        "telegram_sources",
        ["auto_reply_enabled"],
    )
    op.alter_column("telegram_sources", "auto_reply_enabled", server_default=None)

    # Existing schedules predate explicit consent and were saved as true by the Web UI.
    # Disable them during migration so deployment cannot silently enable external sending.
    op.execute(sa.text("UPDATE user_work_schedules SET auto_reply_outside_hours = false"))
    op.alter_column(
        "user_work_schedules",
        "auto_reply_outside_hours",
        existing_type=sa.Boolean(),
        server_default=sa.false(),
        existing_nullable=False,
    )

    op.create_table(
        "auto_reply_deliveries",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("source_message_id", sa.Uuid(), nullable=False),
        sa.Column("channel", IM_CHANNEL, nullable=False),
        sa.Column("conversation_id", sa.String(length=255), nullable=False),
        sa.Column("idempotency_key", sa.String(length=255), nullable=False),
        sa.Column("status", DELIVERY_STATUS, nullable=False),
        sa.Column("decision_reason", DECISION_REASON, nullable=True),
        sa.Column("content_hash", sa.String(length=64), nullable=True),
        sa.Column("provider_message_id", sa.String(length=255), nullable=True),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("ready_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("sending_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("sent_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("error", sa.String(length=500), nullable=True),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(["source_message_id"], ["messages.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id", "idempotency_key", name="uq_auto_reply_deliveries_owner_key"
        ),
    )
    op.create_index(
        "ix_auto_reply_deliveries_owner_user_id",
        "auto_reply_deliveries",
        ["owner_user_id"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_opportunity_id",
        "auto_reply_deliveries",
        ["opportunity_id"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_source_message_id",
        "auto_reply_deliveries",
        ["source_message_id"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_channel",
        "auto_reply_deliveries",
        ["channel"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_conversation_id",
        "auto_reply_deliveries",
        ["conversation_id"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_status",
        "auto_reply_deliveries",
        ["status"],
    )
    op.create_index(
        "ix_auto_reply_deliveries_conversation_status_created",
        "auto_reply_deliveries",
        ["owner_user_id", "channel", "conversation_id", "status", "created_at"],
    )


def downgrade() -> None:
    op.drop_table("auto_reply_deliveries")
    op.drop_index("ix_telegram_sources_auto_reply_enabled", table_name="telegram_sources")
    op.drop_column("telegram_sources", "auto_reply_enabled")
    op.alter_column(
        "user_work_schedules",
        "auto_reply_outside_hours",
        existing_type=sa.Boolean(),
        server_default=None,
        existing_nullable=False,
    )
