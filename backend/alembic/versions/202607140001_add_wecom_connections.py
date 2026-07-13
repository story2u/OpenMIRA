"""add user-owned WeCom connections and event delivery state

Revision ID: 202607140001
Revises: 202607130002
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607140001"
down_revision: str | None = "202607130002"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "wecom_connections",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column(
            "connection_type",
            sa.Enum(
                "INTERNAL_APP",
                "MESSAGE_ARCHIVE",
                "CUSTOMER_SERVICE",
                name="wecomconnectiontype",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column(
            "status",
            sa.Enum(
                "PENDING",
                "ACTIVE",
                "DISABLED",
                "ERROR",
                name="wecomconnectionstatus",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column("corp_id", sa.String(length=128), nullable=False),
        sa.Column("agent_id", sa.String(length=64), nullable=False),
        sa.Column("secret_encrypted", sa.String(), nullable=False),
        sa.Column("token_encrypted", sa.String(), nullable=False),
        sa.Column("aes_key_encrypted", sa.String(), nullable=False),
        sa.Column("last_verified_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id", "corp_id", "agent_id", name="uq_wecom_connections_owner_corp_agent"
        ),
    )
    op.create_index("ix_wecom_connections_owner_user_id", "wecom_connections", ["owner_user_id"])
    op.create_index("ix_wecom_connections_corp_id", "wecom_connections", ["corp_id"])
    op.create_index("ix_wecom_connections_enabled", "wecom_connections", ["enabled"])
    op.create_index(
        "ix_wecom_connections_connection_type", "wecom_connections", ["connection_type"]
    )
    op.create_index("ix_wecom_connections_status", "wecom_connections", ["status"])
    op.create_index(
        "ix_wecom_connections_owner_status", "wecom_connections", ["owner_user_id", "status"]
    )

    op.create_table(
        "wecom_sources",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("external_conversation_id", sa.String(length=255), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column(
            "source_type",
            sa.Enum(
                "PRIVATE",
                "INTERNAL_GROUP",
                "EXTERNAL_GROUP",
                "CUSTOMER_SERVICE",
                name="wecomsourcetype",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column(
            "receive_capability",
            sa.Enum(
                "APP_CALLBACK",
                "MESSAGE_ARCHIVE",
                "CUSTOMER_SERVICE",
                name="wecomreceivecapability",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column(
            "send_capability",
            sa.Enum(
                "APP_MESSAGE",
                "CUSTOMER_SERVICE",
                "MANUAL_ONLY",
                name="wecomsendcapability",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("quota_paused", sa.Boolean(), nullable=False),
        sa.Column("quota_reason", sa.String(length=500), nullable=True),
        sa.Column("last_message_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_connections.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "connection_id",
            "external_conversation_id",
            name="uq_wecom_sources_connection_conversation",
        ),
    )
    for column in (
        "owner_user_id",
        "connection_id",
        "external_conversation_id",
        "source_type",
        "enabled",
        "quota_paused",
        "last_message_at",
    ):
        op.create_index(f"ix_wecom_sources_{column}", "wecom_sources", [column])
    op.create_index(
        "ix_wecom_sources_owner_enabled",
        "wecom_sources",
        ["owner_user_id", "enabled", "quota_paused"],
    )

    op.create_table(
        "wecom_webhook_events",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("provider_event_id", sa.String(length=255), nullable=False),
        sa.Column("event_type", sa.String(length=64), nullable=False),
        sa.Column("payload_hash", sa.String(length=64), nullable=False),
        sa.Column("normalized_payload_encrypted", sa.String(), nullable=True),
        sa.Column(
            "status",
            sa.Enum(
                "RECEIVED",
                "QUEUED",
                "PROCESSING",
                "COMPLETED",
                "FAILED",
                "IGNORED",
                name="wecomeventstatus",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("queued_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("processing_error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_connections.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "connection_id", "provider_event_id", name="uq_wecom_webhook_events_connection_provider"
        ),
    )
    for column in ("connection_id", "owner_user_id", "event_type", "status"):
        op.create_index(f"ix_wecom_webhook_events_{column}", "wecom_webhook_events", [column])
    op.create_index(
        "ix_wecom_webhook_events_status_created", "wecom_webhook_events", ["status", "created_at"]
    )

    op.create_table(
        "wecom_outbound_deliveries",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("source_id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("content_hash", sa.String(length=64), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "PENDING",
                "SENDING",
                "SENT",
                "FAILED",
                name="wecomdeliverystatus",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column("provider_message_id", sa.String(length=255), nullable=True),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("sent_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_connections.id"]),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(["source_id"], ["wecom_sources.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_wecom_outbound_deliveries_owner_idempotency",
        ),
    )
    for column in ("owner_user_id", "connection_id", "source_id", "opportunity_id", "status"):
        op.create_index(
            f"ix_wecom_outbound_deliveries_{column}", "wecom_outbound_deliveries", [column]
        )
    op.create_index(
        "ix_wecom_outbound_deliveries_status_created",
        "wecom_outbound_deliveries",
        ["status", "created_at"],
    )


def downgrade() -> None:
    op.drop_table("wecom_outbound_deliveries")
    op.drop_table("wecom_webhook_events")
    op.drop_table("wecom_sources")
    op.drop_table("wecom_connections")
