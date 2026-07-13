"""add opportunity archiving

Revision ID: 202607130001
Revises: 202607120006
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "202607130001"
down_revision: str | None = "202607120006"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column("opportunities", sa.Column("archived_at", sa.DateTime(timezone=True), nullable=True))
    op.add_column("opportunities", sa.Column("archived_by_user_id", sa.Uuid(), nullable=True))
    op.add_column("opportunities", sa.Column("archive_reason", sa.String(length=500), nullable=True))
    op.create_foreign_key(
        "fk_opportunities_archived_by_user_id_users",
        "opportunities",
        "users",
        ["archived_by_user_id"],
        ["id"],
    )
    op.create_index("ix_opportunities_archived_at", "opportunities", ["archived_at"])
    op.create_index(
        "ix_opportunities_archived_by_user_id",
        "opportunities",
        ["archived_by_user_id"],
    )
    op.create_index(
        "ix_opportunities_owner_archived_last_message",
        "opportunities",
        ["owner_user_id", "archived_at", "last_message_at"],
    )

    op.create_table(
        "opportunity_archive_events",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("action", sa.String(length=8), nullable=False),
        sa.Column("reason", sa.String(length=500), nullable=True),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index(
        "ix_opportunity_archive_events_opportunity_id",
        "opportunity_archive_events",
        ["opportunity_id"],
    )
    op.create_index(
        "ix_opportunity_archive_events_owner_user_id",
        "opportunity_archive_events",
        ["owner_user_id"],
    )
    op.create_index(
        "ix_opportunity_archive_events_action",
        "opportunity_archive_events",
        ["action"],
    )
    op.create_index(
        "ix_opportunity_archive_events_owner_created",
        "opportunity_archive_events",
        ["owner_user_id", "created_at"],
    )


def downgrade() -> None:
    op.drop_index("ix_opportunity_archive_events_owner_created", table_name="opportunity_archive_events")
    op.drop_index("ix_opportunity_archive_events_action", table_name="opportunity_archive_events")
    op.drop_index("ix_opportunity_archive_events_owner_user_id", table_name="opportunity_archive_events")
    op.drop_index("ix_opportunity_archive_events_opportunity_id", table_name="opportunity_archive_events")
    op.drop_table("opportunity_archive_events")
    op.drop_index("ix_opportunities_owner_archived_last_message", table_name="opportunities")
    op.drop_index("ix_opportunities_archived_by_user_id", table_name="opportunities")
    op.drop_index("ix_opportunities_archived_at", table_name="opportunities")
    op.drop_constraint("fk_opportunities_archived_by_user_id_users", "opportunities", type_="foreignkey")
    op.drop_column("opportunities", "archive_reason")
    op.drop_column("opportunities", "archived_by_user_id")
    op.drop_column("opportunities", "archived_at")
