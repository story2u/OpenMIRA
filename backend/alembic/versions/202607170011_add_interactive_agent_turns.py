"""add content-free interactive Agent turn leases and provider audit

Revision ID: 202607170011
Revises: 202607170010
Create Date: 2026-07-17
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607170011"
down_revision: str | None = "202607170010"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    # SQLAlchemy stores this non-native enum by member name. The new feature name is
    # longer than PI_AGENT_ANALYSIS, so widen the existing varchar before reservations.
    op.alter_column(
        "usage_ledger",
        "feature",
        existing_type=sa.String(length=17),
        type_=sa.String(length=22),
        existing_nullable=False,
    )
    op.create_table(
        "interactive_agent_turns",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("local_session_id", sa.Uuid(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("usage_ledger_id", sa.Uuid(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "CLAIMED",
                "RUNNING",
                "COMPLETED",
                "FAILED",
                "EXPIRED",
                name="interactiveagentturnstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("runtime_version", sa.String(length=64), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column("model_alias", sa.String(length=64), nullable=False),
        sa.Column("policy_version", sa.String(length=64), nullable=False),
        sa.Column("lock_version", sa.Integer(), nullable=False),
        sa.Column("request_count", sa.Integer(), nullable=False),
        sa.Column("token_nonce_hash", sa.String(length=64), nullable=False),
        sa.Column("lease_expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("claimed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("heartbeat_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("expired_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failure_code", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "schema_version > 0 AND lock_version > 0 AND request_count >= 0",
            name="ck_interactive_agent_turns_versions_and_count",
        ),
        sa.CheckConstraint(
            "token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_interactive_agent_turns_nonce_hash_sha256",
        ),
        sa.CheckConstraint(
            "lease_expires_at > claimed_at",
            name="ck_interactive_agent_turns_lease_after_claim",
        ),
        sa.CheckConstraint(
            "(status IN ('CLAIMED', 'RUNNING') AND completed_at IS NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND completed_at IS NOT NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'FAILED' AND failed_at IS NOT NULL "
            "AND completed_at IS NULL AND expired_at IS NULL AND failure_code IS NOT NULL) "
            "OR (status = 'EXPIRED' AND expired_at IS NOT NULL "
            "AND completed_at IS NULL AND failed_at IS NULL AND failure_code IS NULL)",
            name="ck_interactive_agent_turns_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_interactive_agent_turns_owner_device",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "usage_ledger_id"],
            ["usage_ledger.user_id", "usage_ledger.id"],
            name="fk_interactive_agent_turns_owner_usage_ledger",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "id",
            name="uq_interactive_agent_turns_owner_id",
        ),
        sa.UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_interactive_agent_turns_owner_idempotency",
        ),
        sa.UniqueConstraint(
            "usage_ledger_id",
            name="uq_interactive_agent_turns_usage_ledger",
        ),
    )
    for column in (
        "owner_user_id",
        "device_id",
        "local_session_id",
        "usage_ledger_id",
        "status",
        "lease_expires_at",
    ):
        op.create_index(
            f"ix_interactive_agent_turns_{column}",
            "interactive_agent_turns",
            [column],
        )
    op.create_index(
        "ix_interactive_agent_turns_owner_status_lease",
        "interactive_agent_turns",
        ["owner_user_id", "status", "lease_expires_at"],
    )
    op.create_index(
        "uq_interactive_agent_turns_session_active",
        "interactive_agent_turns",
        ["owner_user_id", "local_session_id"],
        unique=True,
        postgresql_where=sa.text("status IN ('CLAIMED', 'RUNNING')"),
    )

    op.create_table(
        "interactive_agent_provider_requests",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("turn_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("request_sequence", sa.Integer(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "STARTED",
                "COMPLETED",
                "FAILED",
                "CANCELLED",
                name="interactiveagentproviderrequeststatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("provider", sa.String(length=32), nullable=False),
        sa.Column("provider_model", sa.String(length=128), nullable=False),
        sa.Column("model_alias", sa.String(length=64), nullable=False),
        sa.Column("provider_request_id_hash", sa.String(length=64), nullable=True),
        sa.Column("prompt_tokens", sa.Integer(), nullable=True),
        sa.Column("completion_tokens", sa.Integer(), nullable=True),
        sa.Column("total_tokens", sa.Integer(), nullable=True),
        sa.Column("estimated_cost_micros", sa.BigInteger(), nullable=True),
        sa.Column("latency_ms", sa.BigInteger(), nullable=True),
        sa.Column("failure_code", sa.String(length=64), nullable=True),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("finished_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint(
            "request_sequence > 0",
            name="ck_iapr_sequence_positive",
        ),
        sa.CheckConstraint(
            "prompt_tokens IS NULL OR prompt_tokens >= 0",
            name="ck_iapr_prompt_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "completion_tokens IS NULL OR completion_tokens >= 0",
            name="ck_iapr_completion_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "total_tokens IS NULL OR total_tokens >= 0",
            name="ck_iapr_total_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "estimated_cost_micros IS NULL OR estimated_cost_micros >= 0",
            name="ck_iapr_cost_nonnegative",
        ),
        sa.CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_iapr_latency_nonnegative",
        ),
        sa.CheckConstraint(
            "provider_request_id_hash IS NULL OR provider_request_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iapr_id_hash_sha256",
        ),
        sa.CheckConstraint(
            "(status = 'STARTED' AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'CANCELLED') AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL)",
            name="ck_iapr_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "turn_id"],
            ["interactive_agent_turns.owner_user_id", "interactive_agent_turns.id"],
            name="fk_interactive_agent_provider_requests_owner_turn",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_interactive_agent_provider_requests_owner_device",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "turn_id",
            "request_sequence",
            name="uq_interactive_agent_provider_requests_turn_sequence",
        ),
    )
    for column in ("owner_user_id", "turn_id", "device_id", "status"):
        op.create_index(
            f"ix_interactive_agent_provider_requests_{column}",
            "interactive_agent_provider_requests",
            [column],
        )
    op.create_index(
        "ix_interactive_agent_provider_requests_owner_created",
        "interactive_agent_provider_requests",
        ["owner_user_id", "created_at"],
    )
    op.create_index(
        "uq_interactive_agent_provider_requests_turn_active",
        "interactive_agent_provider_requests",
        ["turn_id"],
        unique=True,
        postgresql_where=sa.text("status = 'STARTED'"),
    )


def downgrade() -> None:
    op.drop_table("interactive_agent_provider_requests")
    op.drop_table("interactive_agent_turns")
    op.execute(
        sa.text(
            "DELETE FROM usage_ledger WHERE feature = 'INTERACTIVE_AGENT_TURN'"
        )
    )
    op.alter_column(
        "usage_ledger",
        "feature",
        existing_type=sa.String(length=22),
        type_=sa.String(length=17),
        existing_nullable=False,
    )
