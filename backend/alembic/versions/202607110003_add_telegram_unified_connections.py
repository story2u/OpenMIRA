"""add unified Telegram connections and webhook idempotency

Revision ID: 202607110003
Revises: 202607110002
Create Date: 2026-07-11 20:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql


revision: str = "202607110003"
down_revision: str | None = "202607110002"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


connection_type = sa.Enum(
    "bot_chat",
    "business",
    "mtproto_qr",
    name="telegramconnectiontype",
    native_enum=False,
)
connection_status = sa.Enum(
    "pending",
    "connected",
    "disabled",
    "error",
    "expired",
    name="telegramconnectionstatus",
    native_enum=False,
)
attempt_status = sa.Enum(
    "pending",
    "completed",
    "cancelled",
    "expired",
    "failed",
    name="telegramconnectionattemptstatus",
    native_enum=False,
)
source_type = sa.Enum(
    "group",
    "channel",
    "private",
    name="telegramsourcetype",
    native_enum=False,
)


def upgrade() -> None:
    op.create_table(
        "telegram_connections",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("owner_user_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("connection_type", connection_type, nullable=False),
        sa.Column("status", connection_status, nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("label", sa.String(length=255), nullable=False),
        sa.Column("telegram_account_id", sa.String(length=128), nullable=True),
        sa.Column("provider_connection_id", sa.String(length=255), nullable=True),
        sa.Column("credential_encrypted", sa.Text(), nullable=True),
        sa.Column("connection_metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("capabilities", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("last_error", sa.String(length=1000), nullable=True),
        sa.Column("last_checked_at", sa.DateTime(timezone=True), nullable=True),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("provider_connection_id", name="uq_telegram_connections_provider_connection_id"),
    )
    op.create_index("ix_telegram_connections_owner_user_id", "telegram_connections", ["owner_user_id"])
    op.create_index("ix_telegram_connections_connection_type", "telegram_connections", ["connection_type"])
    op.create_index("ix_telegram_connections_status", "telegram_connections", ["status"])
    op.create_index("ix_telegram_connections_enabled", "telegram_connections", ["enabled"])
    op.create_index("ix_telegram_connections_telegram_account_id", "telegram_connections", ["telegram_account_id"])
    op.create_index("ix_telegram_connections_provider_connection_id", "telegram_connections", ["provider_connection_id"])
    op.create_index(
        "ix_telegram_connections_owner_type_status",
        "telegram_connections",
        ["owner_user_id", "connection_type", "status"],
    )

    op.create_table(
        "telegram_sources",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("owner_user_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("connection_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("source_type", source_type, nullable=False),
        sa.Column("external_chat_id", sa.String(length=128), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column("username", sa.String(length=255), nullable=True),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default=sa.true()),
        sa.Column("quota_paused", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("quota_reason", sa.String(length=500), nullable=True),
        sa.Column("retention_priority", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("last_error", sa.String(length=1000), nullable=True),
        sa.CheckConstraint("retention_priority >= 0", name="ck_telegram_sources_retention_priority_nonnegative"),
        sa.ForeignKeyConstraint(["connection_id"], ["telegram_connections.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("connection_id", "external_chat_id", name="uq_telegram_sources_connection_chat"),
    )
    op.create_index("ix_telegram_sources_owner_user_id", "telegram_sources", ["owner_user_id"])
    op.create_index("ix_telegram_sources_connection_id", "telegram_sources", ["connection_id"])
    op.create_index("ix_telegram_sources_source_type", "telegram_sources", ["source_type"])
    op.create_index("ix_telegram_sources_external_chat_id", "telegram_sources", ["external_chat_id"])
    op.create_index("ix_telegram_sources_enabled", "telegram_sources", ["enabled"])
    op.create_index("ix_telegram_sources_quota_paused", "telegram_sources", ["quota_paused"])
    op.create_index(
        "ix_telegram_sources_owner_enabled",
        "telegram_sources",
        ["owner_user_id", "enabled", "quota_paused"],
    )

    op.create_table(
        "telegram_connection_attempts",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("owner_user_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("connection_type", connection_type, nullable=False),
        sa.Column("status", attempt_status, nullable=False),
        sa.Column("token_hash", sa.String(length=128), nullable=False),
        sa.Column("group_request_id", sa.Integer(), nullable=True),
        sa.Column("channel_request_id", sa.Integer(), nullable=True),
        sa.Column("telegram_account_id", sa.String(length=128), nullable=True),
        sa.Column("connection_id", postgresql.UUID(as_uuid=True), nullable=True),
        sa.Column("attempt_metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["telegram_connections.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("token_hash", name="uq_telegram_connection_attempts_token_hash"),
        sa.UniqueConstraint("group_request_id", name="uq_telegram_connection_attempts_group_request_id"),
        sa.UniqueConstraint("channel_request_id", name="uq_telegram_connection_attempts_channel_request_id"),
    )
    op.create_index("ix_telegram_connection_attempts_owner_user_id", "telegram_connection_attempts", ["owner_user_id"])
    op.create_index("ix_telegram_connection_attempts_connection_type", "telegram_connection_attempts", ["connection_type"])
    op.create_index("ix_telegram_connection_attempts_status", "telegram_connection_attempts", ["status"])
    op.create_index("ix_telegram_connection_attempts_group_request_id", "telegram_connection_attempts", ["group_request_id"])
    op.create_index("ix_telegram_connection_attempts_channel_request_id", "telegram_connection_attempts", ["channel_request_id"])
    op.create_index("ix_telegram_connection_attempts_telegram_account_id", "telegram_connection_attempts", ["telegram_account_id"])
    op.create_index("ix_telegram_connection_attempts_connection_id", "telegram_connection_attempts", ["connection_id"])
    op.create_index("ix_telegram_connection_attempts_expires_at", "telegram_connection_attempts", ["expires_at"])
    op.create_index(
        "ix_telegram_connection_attempts_owner_status_expires",
        "telegram_connection_attempts",
        ["owner_user_id", "status", "expires_at"],
    )

    op.create_table(
        "telegram_webhook_events",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("update_id", sa.BigInteger(), nullable=False),
        sa.Column("payload_hash", sa.String(length=128), nullable=False),
        sa.Column("event_type", sa.String(length=64), nullable=False),
        sa.Column("connection_id", postgresql.UUID(as_uuid=True), nullable=True),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["telegram_connections.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("update_id", name="uq_telegram_webhook_events_update_id"),
    )
    op.create_index("ix_telegram_webhook_events_update_id", "telegram_webhook_events", ["update_id"])
    op.create_index("ix_telegram_webhook_events_event_type", "telegram_webhook_events", ["event_type"])
    op.create_index("ix_telegram_webhook_events_connection_id", "telegram_webhook_events", ["connection_id"])


def downgrade() -> None:
    op.drop_index("ix_telegram_webhook_events_connection_id", table_name="telegram_webhook_events")
    op.drop_index("ix_telegram_webhook_events_event_type", table_name="telegram_webhook_events")
    op.drop_index("ix_telegram_webhook_events_update_id", table_name="telegram_webhook_events")
    op.drop_table("telegram_webhook_events")

    op.drop_index("ix_telegram_connection_attempts_owner_status_expires", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_expires_at", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_connection_id", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_telegram_account_id", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_channel_request_id", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_group_request_id", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_status", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_connection_type", table_name="telegram_connection_attempts")
    op.drop_index("ix_telegram_connection_attempts_owner_user_id", table_name="telegram_connection_attempts")
    op.drop_table("telegram_connection_attempts")

    op.drop_index("ix_telegram_sources_owner_enabled", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_quota_paused", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_enabled", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_external_chat_id", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_source_type", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_connection_id", table_name="telegram_sources")
    op.drop_index("ix_telegram_sources_owner_user_id", table_name="telegram_sources")
    op.drop_table("telegram_sources")

    op.drop_index("ix_telegram_connections_owner_type_status", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_provider_connection_id", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_telegram_account_id", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_enabled", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_status", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_connection_type", table_name="telegram_connections")
    op.drop_index("ix_telegram_connections_owner_user_id", table_name="telegram_connections")
    op.drop_table("telegram_connections")
