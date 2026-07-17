"""add aggregate sync versions and device acknowledgement state

Revision ID: 202607170003
Revises: 202607170002
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607170003"
down_revision: str | None = "202607170002"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


VERSIONED_TABLES = (
    "opportunities",
    "messages",
    "user_detection_preferences",
    "user_work_schedules",
    "user_notification_preferences",
)


def upgrade() -> None:
    for table_name in VERSIONED_TABLES:
        op.add_column(
            table_name,
            sa.Column(
                "aggregate_version",
                sa.BigInteger(),
                server_default=sa.text("1"),
                nullable=False,
            ),
        )
        op.create_check_constraint(
            f"ck_{table_name}_aggregate_version_positive",
            table_name,
            "aggregate_version > 0",
        )

    op.add_column(
        "devices",
        sa.Column(
            "last_sync_cursor",
            sa.BigInteger(),
            server_default=sa.text("0"),
            nullable=False,
        ),
    )
    op.add_column(
        "devices",
        sa.Column("last_sync_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.add_column(
        "devices",
        sa.Column("last_sync_error_code", sa.String(length=64), nullable=True),
    )
    op.create_check_constraint(
        "ck_devices_last_sync_cursor_nonnegative",
        "devices",
        "last_sync_cursor >= 0",
    )
    op.drop_constraint(
        "sync_changes_owner_user_id_fkey",
        "sync_changes",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "sync_changes_owner_user_id_fkey",
        "sync_changes",
        "users",
        ["owner_user_id"],
        ["id"],
        ondelete="CASCADE",
    )


def downgrade() -> None:
    op.drop_constraint(
        "sync_changes_owner_user_id_fkey",
        "sync_changes",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "sync_changes_owner_user_id_fkey",
        "sync_changes",
        "users",
        ["owner_user_id"],
        ["id"],
    )
    op.drop_constraint(
        "ck_devices_last_sync_cursor_nonnegative",
        "devices",
        type_="check",
    )
    op.drop_column("devices", "last_sync_error_code")
    op.drop_column("devices", "last_sync_at")
    op.drop_column("devices", "last_sync_cursor")

    for table_name in reversed(VERSIONED_TABLES):
        op.drop_constraint(
            f"ck_{table_name}_aggregate_version_positive",
            table_name,
            type_="check",
        )
        op.drop_column(table_name, "aggregate_version")
