"""Add user-level settings tables: detection prefs, work schedule, notifications.

Revision ID: 202607130001
Revises: 202607120006
Create Date: 2026-07-13
"""

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects.postgresql import JSONB


revision = "202607130001"
down_revision = "202607120006"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "user_detection_preferences",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("keywords", JSONB(), nullable=False),
        sa.Column("ai_semantics_enabled", sa.Boolean(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("user_id", name="uq_user_detection_preferences_user_id"),
    )
    op.create_index(
        "ix_user_detection_preferences_user_id",
        "user_detection_preferences",
        ["user_id"],
    )

    op.create_table(
        "user_work_schedules",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("timezone", sa.String(length=64), nullable=False),
        sa.Column("slots", JSONB(), nullable=False),
        sa.Column("auto_reply_outside_hours", sa.Boolean(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("user_id", name="uq_user_work_schedules_user_id"),
    )
    op.create_index(
        "ix_user_work_schedules_user_id",
        "user_work_schedules",
        ["user_id"],
    )

    op.create_table(
        "user_notification_preferences",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("new_opportunity_enabled", sa.Boolean(), nullable=False),
        sa.Column("ai_replied_enabled", sa.Boolean(), nullable=False),
        sa.Column("daily_digest_enabled", sa.Boolean(), nullable=False),
        sa.Column("urgent_only", sa.Boolean(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("user_id", name="uq_user_notification_preferences_user_id"),
    )
    op.create_index(
        "ix_user_notification_preferences_user_id",
        "user_notification_preferences",
        ["user_id"],
    )


def downgrade() -> None:
    op.drop_index("ix_user_notification_preferences_user_id", "user_notification_preferences")
    op.drop_table("user_notification_preferences")
    op.drop_index("ix_user_work_schedules_user_id", "user_work_schedules")
    op.drop_table("user_work_schedules")
    op.drop_index("ix_user_detection_preferences_user_id", "user_detection_preferences")
    op.drop_table("user_detection_preferences")
