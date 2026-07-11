"""add subscriptions and usage ledger

Revision ID: 202607110001
Revises: 202607100003
Create Date: 2026-07-11 12:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
import sqlmodel

from alembic import op

revision: str = "202607110001"
down_revision: str | None = "202607100003"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


PLAN_CODE = sa.Enum("FREE", "PLUS", "PRO", "MAX", native_enum=False)
SUBSCRIPTION_STATUS = sa.Enum(
    "ACTIVE",
    "TRIALING",
    "PAST_DUE",
    "CANCELED",
    "INACTIVE",
    native_enum=False,
)
USAGE_FEATURE = sa.Enum("PI_AGENT_ANALYSIS", native_enum=False)
USAGE_STATUS = sa.Enum("RESERVED", "CONSUMED", "RELEASED", native_enum=False)


def upgrade() -> None:
    # The previous longest AgentAnalysisStatus name was 13 characters. The quota status
    # is persisted by SQLAlchemy using the enum member name and needs one more character.
    op.alter_column(
        "messages",
        "agent_analysis_status",
        existing_type=sa.String(length=13),
        type_=sa.String(length=14),
        existing_nullable=False,
    )
    op.alter_column(
        "opportunities",
        "agent_analysis_status",
        existing_type=sa.String(length=13),
        type_=sa.String(length=14),
        existing_nullable=False,
    )

    op.create_table(
        "subscription_accounts",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("plan_code", PLAN_CODE, nullable=False),
        sa.Column("status", SUBSCRIPTION_STATUS, nullable=False),
        sa.Column("billing_provider", sqlmodel.sql.sqltypes.AutoString(length=32), nullable=True),
        sa.Column(
            "provider_customer_id",
            sqlmodel.sql.sqltypes.AutoString(length=255),
            nullable=True,
        ),
        sa.Column(
            "provider_subscription_id",
            sqlmodel.sql.sqltypes.AutoString(length=255),
            nullable=True,
        ),
        sa.Column("current_period_start", sa.DateTime(timezone=True), nullable=True),
        sa.Column("current_period_end", sa.DateTime(timezone=True), nullable=True),
        sa.Column("cancel_at_period_end", sa.Boolean(), nullable=False),
        sa.Column(
            "provider_event_version",
            sqlmodel.sql.sqltypes.AutoString(length=255),
            nullable=True,
        ),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("user_id", name="uq_subscription_accounts_user_id"),
    )
    op.create_index(
        "ix_subscription_accounts_plan_code",
        "subscription_accounts",
        ["plan_code"],
        unique=False,
    )
    op.create_index(
        "ix_subscription_accounts_provider_customer_id",
        "subscription_accounts",
        ["provider_customer_id"],
        unique=False,
    )
    op.create_index(
        "ix_subscription_accounts_provider_subscription_id",
        "subscription_accounts",
        ["provider_subscription_id"],
        unique=False,
    )
    op.create_index(
        "ix_subscription_accounts_status",
        "subscription_accounts",
        ["status"],
        unique=False,
    )
    op.create_index(
        "ix_subscription_accounts_status_period_end",
        "subscription_accounts",
        ["status", "current_period_end"],
        unique=False,
    )
    op.create_index(
        "ix_subscription_accounts_user_id",
        "subscription_accounts",
        ["user_id"],
        unique=False,
    )

    op.create_table(
        "usage_ledger",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("feature", USAGE_FEATURE, nullable=False),
        sa.Column("quantity", sa.Integer(), nullable=False),
        sa.Column("period_start", sa.DateTime(timezone=True), nullable=False),
        sa.Column("period_end", sa.DateTime(timezone=True), nullable=False),
        sa.Column("idempotency_key", sqlmodel.sql.sqltypes.AutoString(length=255), nullable=False),
        sa.Column("source_message_id", sa.Uuid(), nullable=True),
        sa.Column("status", USAGE_STATUS, nullable=False),
        sa.Column("consumed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("released_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failure_reason", sqlmodel.sql.sqltypes.AutoString(length=500), nullable=True),
        sa.CheckConstraint("quantity > 0", name="ck_usage_ledger_quantity_positive"),
        sa.ForeignKeyConstraint(["source_message_id"], ["messages.id"]),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "user_id",
            "feature",
            "idempotency_key",
            name="uq_usage_ledger_user_feature_idempotency",
        ),
    )
    op.create_index("ix_usage_ledger_feature", "usage_ledger", ["feature"], unique=False)
    op.create_index(
        "ix_usage_ledger_source_message_id",
        "usage_ledger",
        ["source_message_id"],
        unique=False,
    )
    op.create_index("ix_usage_ledger_status", "usage_ledger", ["status"], unique=False)
    op.create_index("ix_usage_ledger_user_id", "usage_ledger", ["user_id"], unique=False)
    op.create_index(
        "ix_usage_ledger_user_feature_period_status",
        "usage_ledger",
        ["user_id", "feature", "period_start", "period_end", "status"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index("ix_usage_ledger_user_feature_period_status", table_name="usage_ledger")
    op.drop_index("ix_usage_ledger_user_id", table_name="usage_ledger")
    op.drop_index("ix_usage_ledger_status", table_name="usage_ledger")
    op.drop_index("ix_usage_ledger_source_message_id", table_name="usage_ledger")
    op.drop_index("ix_usage_ledger_feature", table_name="usage_ledger")
    op.drop_table("usage_ledger")

    op.drop_index("ix_subscription_accounts_user_id", table_name="subscription_accounts")
    op.drop_index("ix_subscription_accounts_status_period_end", table_name="subscription_accounts")
    op.drop_index("ix_subscription_accounts_status", table_name="subscription_accounts")
    op.drop_index(
        "ix_subscription_accounts_provider_subscription_id",
        table_name="subscription_accounts",
    )
    op.drop_index(
        "ix_subscription_accounts_provider_customer_id",
        table_name="subscription_accounts",
    )
    op.drop_index("ix_subscription_accounts_plan_code", table_name="subscription_accounts")
    op.drop_table("subscription_accounts")

    op.execute(
        "UPDATE messages SET agent_analysis_status = 'NOT_REQUESTED' "
        "WHERE agent_analysis_status = 'QUOTA_EXCEEDED'"
    )
    op.execute(
        "UPDATE opportunities SET agent_analysis_status = 'NOT_REQUESTED' "
        "WHERE agent_analysis_status = 'QUOTA_EXCEEDED'"
    )
    op.alter_column(
        "opportunities",
        "agent_analysis_status",
        existing_type=sa.String(length=14),
        type_=sa.String(length=13),
        existing_nullable=False,
    )
    op.alter_column(
        "messages",
        "agent_analysis_status",
        existing_type=sa.String(length=14),
        type_=sa.String(length=13),
        existing_nullable=False,
    )
