"""add idempotency receipts for internal offline commands

Revision ID: 202607170004
Revises: 202607170003
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607170004"
down_revision: str | None = "202607170003"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "internal_command_receipts",
        sa.Column("id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("owner_user_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("opportunity_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("idempotency_key", sa.String(length=128), nullable=False),
        sa.Column("command_type", sa.String(length=64), nullable=False),
        sa.Column("expected_version", sa.BigInteger(), nullable=False),
        sa.Column("payload_hash", sa.String(length=64), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "command_type = 'opportunity_status'",
            name="ck_internal_command_receipts_supported_type",
        ),
        sa.CheckConstraint(
            "expected_version > 0",
            name="ck_internal_command_receipts_expected_version_positive",
        ),
        sa.CheckConstraint(
            "char_length(payload_hash) = 64",
            name="ck_internal_command_receipts_payload_hash_length",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id"],
            ["users.id"],
            ondelete="CASCADE",
        ),
        sa.ForeignKeyConstraint(
            ["opportunity_id"],
            ["opportunities.id"],
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_internal_command_receipts_owner_idempotency",
        ),
    )
    op.create_index(
        "ix_internal_command_receipts_owner_created",
        "internal_command_receipts",
        ["owner_user_id", "created_at"],
    )
    op.create_index(
        op.f("ix_internal_command_receipts_owner_user_id"),
        "internal_command_receipts",
        ["owner_user_id"],
    )
    op.create_index(
        op.f("ix_internal_command_receipts_opportunity_id"),
        "internal_command_receipts",
        ["opportunity_id"],
    )
    op.create_index(
        op.f("ix_internal_command_receipts_expires_at"),
        "internal_command_receipts",
        ["expires_at"],
    )


def downgrade() -> None:
    op.drop_index(
        op.f("ix_internal_command_receipts_expires_at"),
        table_name="internal_command_receipts",
    )
    op.drop_index(
        op.f("ix_internal_command_receipts_opportunity_id"),
        table_name="internal_command_receipts",
    )
    op.drop_index(
        op.f("ix_internal_command_receipts_owner_user_id"),
        table_name="internal_command_receipts",
    )
    op.drop_index(
        "ix_internal_command_receipts_owner_created",
        table_name="internal_command_receipts",
    )
    op.drop_table("internal_command_receipts")
