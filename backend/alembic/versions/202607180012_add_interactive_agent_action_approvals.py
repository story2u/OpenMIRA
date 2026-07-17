"""add content-free one-time interactive Agent action approvals

Revision ID: 202607180012
Revises: 202607170011
Create Date: 2026-07-18
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607180012"
down_revision: str | None = "202607170011"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "interactive_agent_action_approvals",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("turn_id", sa.Uuid(), nullable=False),
        sa.Column("tool_call_id", sa.String(length=128), nullable=False),
        sa.Column("tool_name", sa.String(length=64), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("expected_version", sa.BigInteger(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("arguments_hash", sa.String(length=64), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "DENIED",
                "GRANTED",
                "EXECUTING",
                "CONSUMED",
                "FAILED",
                "UNCERTAIN",
                "EXPIRED",
                name="interactiveagentapprovalstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("token_nonce_hash", sa.String(length=64), nullable=True),
        sa.Column("decided_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("execution_started_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("finished_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failure_code", sa.String(length=64), nullable=True),
        sa.Column("manual_reply_delivery_id", sa.Uuid(), nullable=True),
        sa.CheckConstraint(
            "expected_version > 0",
            name="ck_iaaa_expected_version_positive",
        ),
        sa.CheckConstraint(
            "arguments_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iaaa_arguments_hash_sha256",
        ),
        sa.CheckConstraint(
            "token_nonce_hash IS NULL OR token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iaaa_nonce_hash_sha256",
        ),
        sa.CheckConstraint(
            "(status = 'DENIED' AND token_nonce_hash IS NULL AND expires_at IS NULL "
            "AND execution_started_at IS NULL AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'GRANTED' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NULL "
            "AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'EXECUTING' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NOT NULL "
            "AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'CONSUMED' AND token_nonce_hash IS NOT NULL "
            "AND execution_started_at IS NOT NULL AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'UNCERTAIN') AND token_nonce_hash IS NOT NULL "
            "AND execution_started_at IS NOT NULL AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL) "
            "OR (status = 'EXPIRED' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NULL "
            "AND finished_at IS NOT NULL AND failure_code IS NULL)",
            name="ck_iaaa_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_iaaa_owner_device",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "turn_id"],
            ["interactive_agent_turns.owner_user_id", "interactive_agent_turns.id"],
            name="fk_iaaa_owner_turn",
        ),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(
            ["manual_reply_delivery_id"],
            ["manual_reply_deliveries.id"],
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "turn_id",
            "tool_call_id",
            name="uq_iaaa_owner_turn_tool_call",
        ),
    )
    for column in (
        "owner_user_id",
        "device_id",
        "turn_id",
        "opportunity_id",
        "status",
        "expires_at",
        "manual_reply_delivery_id",
    ):
        op.create_index(
            f"ix_interactive_agent_action_approvals_{column}",
            "interactive_agent_action_approvals",
            [column],
        )
    op.create_index(
        "ix_iaaa_owner_status_expires",
        "interactive_agent_action_approvals",
        ["owner_user_id", "status", "expires_at"],
    )


def downgrade() -> None:
    op.drop_table("interactive_agent_action_approvals")
