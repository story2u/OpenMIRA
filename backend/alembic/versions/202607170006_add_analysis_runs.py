"""add device analysis run leases

Revision ID: 202607170006
Revises: 202607170005
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607170006"
down_revision: str | None = "202607170005"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_unique_constraint(
        "uq_messages_owner_id",
        "messages",
        ["owner_user_id", "id"],
    )
    op.create_unique_constraint(
        "uq_usage_ledger_user_id",
        "usage_ledger",
        ["user_id", "id"],
    )
    op.create_table(
        "analysis_runs",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("message_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("usage_ledger_id", sa.Uuid(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "CLAIMED",
                "RUNNING",
                "COMPLETED",
                "FAILED",
                "EXPIRED",
                name="analysisrunstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column(
            "executor",
            sa.Enum(
                "DEVICE",
                "SERVER",
                name="analysisrunexecutor",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("runtime_version", sa.String(length=64), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column("model_alias", sa.String(length=64), nullable=False),
        sa.Column("policy_version", sa.String(length=64), nullable=False),
        sa.Column("source_message_version", sa.BigInteger(), nullable=False),
        sa.Column("lock_version", sa.Integer(), nullable=False),
        sa.Column("token_nonce_hash", sa.String(length=64), nullable=False),
        sa.Column("lease_expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("claimed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("heartbeat_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("expired_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failure_code", sa.String(length=64), nullable=True),
        sa.Column(
            "result",
            postgresql.JSONB(astext_type=sa.Text(), none_as_null=True),
            nullable=True,
        ),
        sa.CheckConstraint(
            "source_message_version > 0 AND schema_version > 0 AND lock_version > 0",
            name="ck_analysis_runs_positive_versions",
        ),
        sa.CheckConstraint(
            "token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_analysis_runs_nonce_hash_sha256",
        ),
        sa.CheckConstraint(
            "lease_expires_at > claimed_at",
            name="ck_analysis_runs_lease_after_claim",
        ),
        sa.CheckConstraint(
            "result IS NULL OR (jsonb_typeof(result) = 'object' "
            "AND octet_length(result::text) <= 65536)",
            name="ck_analysis_runs_result_bounded_object",
        ),
        sa.CheckConstraint(
            "(status IN ('CLAIMED', 'RUNNING') AND completed_at IS NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL "
            "AND result IS NULL) "
            "OR (status = 'COMPLETED' AND completed_at IS NOT NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL "
            "AND result IS NOT NULL) "
            "OR (status = 'FAILED' AND failed_at IS NOT NULL "
            "AND completed_at IS NULL AND expired_at IS NULL AND failure_code IS NOT NULL "
            "AND result IS NULL) "
            "OR (status = 'EXPIRED' AND expired_at IS NOT NULL "
            "AND completed_at IS NULL AND failed_at IS NULL AND failure_code IS NULL "
            "AND result IS NULL)",
            name="ck_analysis_runs_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_analysis_runs_owner_device",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "message_id"],
            ["messages.owner_user_id", "messages.id"],
            name="fk_analysis_runs_owner_message",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "usage_ledger_id"],
            ["usage_ledger.user_id", "usage_ledger.id"],
            name="fk_analysis_runs_owner_usage_ledger",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("owner_user_id", "id", name="uq_analysis_runs_owner_id"),
        sa.UniqueConstraint("usage_ledger_id", name="uq_analysis_runs_usage_ledger"),
    )
    for column in (
        "owner_user_id",
        "message_id",
        "device_id",
        "usage_ledger_id",
        "status",
        "lease_expires_at",
    ):
        op.create_index(f"ix_analysis_runs_{column}", "analysis_runs", [column])
    op.create_index(
        "ix_analysis_runs_owner_status_lease",
        "analysis_runs",
        ["owner_user_id", "status", "lease_expires_at"],
    )
    op.create_index(
        "uq_analysis_runs_message_active",
        "analysis_runs",
        ["message_id"],
        unique=True,
        postgresql_where=sa.text("status IN ('CLAIMED', 'RUNNING')"),
    )


def downgrade() -> None:
    op.drop_table("analysis_runs")
    op.drop_constraint("uq_usage_ledger_user_id", "usage_ledger", type_="unique")
    op.drop_constraint("uq_messages_owner_id", "messages", type_="unique")
