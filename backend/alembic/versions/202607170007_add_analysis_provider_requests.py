"""add content-free analysis provider request audit

Revision ID: 202607170007
Revises: 202607170006
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "202607170007"
down_revision: str | None = "202607170006"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "analysis_provider_requests",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("run_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "STARTED",
                "COMPLETED",
                "FAILED",
                "CANCELLED",
                name="analysisproviderrequeststatus",
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
            "prompt_tokens IS NULL OR prompt_tokens >= 0",
            name="ck_analysis_provider_requests_prompt_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "completion_tokens IS NULL OR completion_tokens >= 0",
            name="ck_analysis_provider_requests_completion_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "total_tokens IS NULL OR total_tokens >= 0",
            name="ck_analysis_provider_requests_total_tokens_nonnegative",
        ),
        sa.CheckConstraint(
            "estimated_cost_micros IS NULL OR estimated_cost_micros >= 0",
            name="ck_analysis_provider_requests_cost_nonnegative",
        ),
        sa.CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_analysis_provider_requests_latency_nonnegative",
        ),
        sa.CheckConstraint(
            "provider_request_id_hash IS NULL OR "
            "provider_request_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_analysis_provider_requests_id_hash_sha256",
        ),
        sa.CheckConstraint(
            "(status = 'STARTED' AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'CANCELLED') AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL)",
            name="ck_analysis_provider_requests_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "run_id"],
            ["analysis_runs.owner_user_id", "analysis_runs.id"],
            name="fk_analysis_provider_requests_owner_run",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_analysis_provider_requests_owner_device",
        ),
        sa.PrimaryKeyConstraint("id"),
    )
    for column in ("owner_user_id", "run_id", "device_id", "status"):
        op.create_index(
            f"ix_analysis_provider_requests_{column}",
            "analysis_provider_requests",
            [column],
        )
    op.create_index(
        "ix_analysis_provider_requests_owner_created",
        "analysis_provider_requests",
        ["owner_user_id", "created_at"],
    )
    op.create_index(
        "uq_analysis_provider_requests_run_active",
        "analysis_provider_requests",
        ["run_id"],
        unique=True,
        postgresql_where=sa.text("status = 'STARTED'"),
    )


def downgrade() -> None:
    op.drop_table("analysis_provider_requests")
