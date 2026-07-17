"""Add bounded cursor-hint push delivery state.

Revision ID: 202607170005
Revises: 202607170004
Create Date: 2026-07-17
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "202607170005"
down_revision: str | None = "202607170004"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "push_registrations",
        sa.Column("last_notified_cursor", sa.BigInteger(), nullable=False, server_default="0"),
    )
    op.add_column(
        "push_registrations",
        sa.Column("next_attempt_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_check_constraint(
        "ck_push_registrations_last_notified_cursor_nonnegative",
        "push_registrations",
        "last_notified_cursor >= 0",
    )
    op.create_index(
        "ix_push_registrations_next_attempt_at",
        "push_registrations",
        ["next_attempt_at"],
    )
    op.alter_column("push_registrations", "last_notified_cursor", server_default=None)


def downgrade() -> None:
    op.drop_index("ix_push_registrations_next_attempt_at", table_name="push_registrations")
    op.drop_constraint(
        "ck_push_registrations_last_notified_cursor_nonnegative",
        "push_registrations",
        type_="check",
    )
    op.drop_column("push_registrations", "next_attempt_at")
    op.drop_column("push_registrations", "last_notified_cursor")
