"""add monitor retention selection

Revision ID: 202607110002
Revises: 202607110001
Create Date: 2026-07-11 16:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607110002"
down_revision: str | None = "202607110001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "telegram_user_configs",
        sa.Column("retention_limit", sa.Integer(), nullable=True),
    )
    op.add_column(
        "telegram_user_configs",
        sa.Column("retention_selected_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.add_column(
        "telegram_monitors",
        sa.Column("quota_paused", sa.Boolean(), nullable=False, server_default=sa.false()),
    )
    op.add_column(
        "telegram_monitors",
        sa.Column("quota_reason", sa.String(length=500), nullable=True),
    )
    op.add_column(
        "telegram_monitors",
        sa.Column("retention_priority", sa.Integer(), nullable=False, server_default="0"),
    )
    op.create_index(
        "ix_telegram_monitors_quota_paused",
        "telegram_monitors",
        ["quota_paused"],
        unique=False,
    )
    op.create_check_constraint(
        "ck_telegram_user_configs_retention_limit_nonnegative",
        "telegram_user_configs",
        "retention_limit IS NULL OR retention_limit >= 0",
    )
    op.create_check_constraint(
        "ck_telegram_monitors_retention_priority_nonnegative",
        "telegram_monitors",
        "retention_priority >= 0",
    )


def downgrade() -> None:
    op.drop_constraint(
        "ck_telegram_monitors_retention_priority_nonnegative",
        "telegram_monitors",
        type_="check",
    )
    op.drop_constraint(
        "ck_telegram_user_configs_retention_limit_nonnegative",
        "telegram_user_configs",
        type_="check",
    )
    op.drop_index("ix_telegram_monitors_quota_paused", table_name="telegram_monitors")
    op.drop_column("telegram_monitors", "retention_priority")
    op.drop_column("telegram_monitors", "quota_reason")
    op.drop_column("telegram_monitors", "quota_paused")
    op.drop_column("telegram_user_configs", "retention_selected_at")
    op.drop_column("telegram_user_configs", "retention_limit")
