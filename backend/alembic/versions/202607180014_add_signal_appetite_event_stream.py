"""add append-only Signal Appetite event stream

Revision ID: 202607180014
Revises: 202607180013
Create Date: 2026-07-18
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607180014"
down_revision: str | None = "202607180013"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "signal_appetite_events",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column(
            "cursor",
            sa.BigInteger(),
            sa.Identity(always=False, start=1),
            nullable=False,
        ),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("event_id", sa.Uuid(), nullable=False),
        sa.Column("event_type", sa.String(length=64), nullable=False),
        sa.Column("aggregate_id", sa.Uuid(), nullable=False),
        sa.Column("aggregate_version", sa.BigInteger(), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column("payload", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("occurred_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "aggregate_version > 0",
            name="ck_signal_appetite_events_aggregate_version_positive",
        ),
        sa.CheckConstraint(
            "schema_version = 1",
            name="ck_signal_appetite_events_schema_v1",
        ),
        sa.CheckConstraint(
            "jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536",
            name="ck_signal_appetite_events_payload_bounded_object",
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id"], ["users.id"], ondelete="CASCADE"
        ),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_signal_appetite_events_owner_device",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("cursor", name="uq_signal_appetite_events_cursor"),
        sa.UniqueConstraint(
            "owner_user_id",
            "event_id",
            name="uq_signal_appetite_events_owner_event",
        ),
    )
    for column in (
        "owner_user_id",
        "device_id",
        "event_id",
        "event_type",
        "aggregate_id",
    ):
        op.create_index(
            f"ix_signal_appetite_events_{column}",
            "signal_appetite_events",
            [column],
        )
    op.create_index(
        "ix_signal_appetite_events_owner_cursor",
        "signal_appetite_events",
        ["owner_user_id", "cursor"],
    )
    op.create_index(
        "ix_signal_appetite_events_owner_aggregate",
        "signal_appetite_events",
        ["owner_user_id", "aggregate_id", "aggregate_version"],
    )


def downgrade() -> None:
    op.drop_table("signal_appetite_events")
