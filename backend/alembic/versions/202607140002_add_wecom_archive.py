"""add enterprise WeCom conversation archive storage

Revision ID: 202607140002
Revises: 202607140001
Create Date: 2026-07-14
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "202607140002"
down_revision: str | None = "202607140001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    status_enum = sa.Enum(
        "PENDING", "ACTIVE", "DISABLED", "ERROR",
        name="wecomconnectionstatus", native_enum=False,
    )
    event_enum = sa.Enum(
        "RECEIVED", "QUEUED", "PROCESSING", "COMPLETED", "FAILED", "IGNORED",
        name="wecomeventstatus", native_enum=False,
    )
    op.create_table(
        "wecom_archive_connections",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column("corp_id", sa.String(length=128), nullable=False),
        sa.Column("secret_encrypted", sa.String(), nullable=False),
        sa.Column("private_key_encrypted", sa.String(), nullable=False),
        sa.Column("public_key_version", sa.Integer(), nullable=False),
        sa.Column("status", status_enum, nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("last_verified_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_polled_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id", "corp_id", name="uq_wecom_archive_connections_owner_corp"
        ),
    )
    for column in ("owner_user_id", "corp_id", "status", "enabled", "last_polled_at"):
        op.create_index(
            f"ix_wecom_archive_connections_{column}", "wecom_archive_connections", [column]
        )
    op.create_index(
        "ix_wecom_archive_connections_status_enabled",
        "wecom_archive_connections", ["status", "enabled"],
    )

    op.alter_column("wecom_sources", "connection_id", existing_type=sa.Uuid(), nullable=True)
    op.add_column(
        "wecom_sources", sa.Column("archive_connection_id", sa.Uuid(), nullable=True)
    )
    op.create_foreign_key(
        "fk_wecom_sources_archive_connection_id",
        "wecom_sources", "wecom_archive_connections",
        ["archive_connection_id"], ["id"],
    )
    op.create_index(
        "ix_wecom_sources_archive_connection_id", "wecom_sources", ["archive_connection_id"]
    )
    op.create_unique_constraint(
        "uq_wecom_sources_archive_owner_conversation",
        "wecom_sources",
        ["archive_connection_id", "owner_user_id", "external_conversation_id"],
    )
    op.create_check_constraint(
        "ck_wecom_sources_exactly_one_connection",
        "wecom_sources",
        "(connection_id IS NOT NULL) <> (archive_connection_id IS NOT NULL)",
    )

    op.create_table(
        "wecom_archive_member_bindings",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("wecom_user_id", sa.String(length=128), nullable=False),
        sa.Column("display_name", sa.String(length=255), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("last_matched_at", sa.DateTime(timezone=True), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_archive_connections.id"]),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "connection_id", "user_id", name="uq_wecom_archive_bindings_connection_user"
        ),
        sa.UniqueConstraint(
            "connection_id", "wecom_user_id",
            name="uq_wecom_archive_bindings_connection_wecom_user",
        ),
    )
    for column in ("connection_id", "user_id", "wecom_user_id", "enabled"):
        op.create_index(
            f"ix_wecom_archive_member_bindings_{column}",
            "wecom_archive_member_bindings", [column],
        )
    op.create_index(
        "ix_wecom_archive_bindings_user_enabled",
        "wecom_archive_member_bindings", ["user_id", "enabled"],
    )

    op.create_table(
        "wecom_archive_cursors",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("last_seq", sa.BigInteger(), nullable=False),
        sa.Column("lease_expires_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_batch_size", sa.Integer(), nullable=False),
        sa.CheckConstraint(
            "last_seq >= 0", name="ck_wecom_archive_cursors_last_seq_nonnegative"
        ),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_archive_connections.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("connection_id", name="uq_wecom_archive_cursors_connection"),
    )
    op.create_index(
        "ix_wecom_archive_cursors_connection_id", "wecom_archive_cursors", ["connection_id"]
    )
    op.create_index(
        "ix_wecom_archive_cursors_lease_expires_at",
        "wecom_archive_cursors", ["lease_expires_at"],
    )

    op.create_table(
        "wecom_archive_events",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("provider_message_id", sa.String(length=255), nullable=False),
        sa.Column("sequence", sa.BigInteger(), nullable=False),
        sa.Column("message_type", sa.String(length=64), nullable=False),
        sa.Column("payload_hash", sa.String(length=64), nullable=False),
        sa.Column("status", event_enum, nullable=False),
        sa.Column("matched_user_count", sa.Integer(), nullable=False),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("processing_error", sa.String(length=1000), nullable=True),
        sa.ForeignKeyConstraint(["connection_id"], ["wecom_archive_connections.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "connection_id", "provider_message_id",
            name="uq_wecom_archive_events_connection_message",
        ),
    )
    for column in ("connection_id", "sequence", "message_type", "status"):
        op.create_index(
            f"ix_wecom_archive_events_{column}", "wecom_archive_events", [column]
        )
    op.create_index(
        "ix_wecom_archive_events_status_created",
        "wecom_archive_events", ["status", "created_at"],
    )


def downgrade() -> None:
    op.drop_table("wecom_archive_events")
    op.drop_table("wecom_archive_cursors")
    op.drop_table("wecom_archive_member_bindings")
    op.execute("DELETE FROM wecom_sources WHERE archive_connection_id IS NOT NULL")
    op.drop_constraint(
        "ck_wecom_sources_exactly_one_connection", "wecom_sources", type_="check"
    )
    op.drop_constraint(
        "uq_wecom_sources_archive_owner_conversation", "wecom_sources", type_="unique"
    )
    op.drop_index("ix_wecom_sources_archive_connection_id", table_name="wecom_sources")
    op.drop_constraint(
        "fk_wecom_sources_archive_connection_id", "wecom_sources", type_="foreignkey"
    )
    op.drop_column("wecom_sources", "archive_connection_id")
    op.alter_column("wecom_sources", "connection_id", existing_type=sa.Uuid(), nullable=False)
    op.drop_table("wecom_archive_connections")
