"""add channel-agnostic manual reply delivery ledger

Revision ID: 202607170001
Revises: 202607140001
"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "202607170001"
down_revision: str | None = "202607140001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "manual_reply_deliveries",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("content_hash", sa.String(length=64), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "PENDING",
                "SENDING",
                "DELIVERED",
                "COMPLETED",
                "FAILED",
                "UNCERTAIN",
                name="manualreplydeliverystatus",
                native_enum=False,
            ),
            nullable=False,
        ),
        sa.Column("provider_message_id", sa.String(length=255), nullable=True),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("delivered_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("completed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("error_class", sa.String(length=255), nullable=True),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_manual_reply_deliveries_owner_idempotency",
        ),
    )
    for column in ("owner_user_id", "opportunity_id", "status"):
        op.create_index(
            f"ix_manual_reply_deliveries_{column}",
            "manual_reply_deliveries",
            [column],
        )
    op.create_index(
        "ix_manual_reply_deliveries_status_created",
        "manual_reply_deliveries",
        ["status", "created_at"],
    )


def downgrade() -> None:
    op.drop_table("manual_reply_deliveries")
