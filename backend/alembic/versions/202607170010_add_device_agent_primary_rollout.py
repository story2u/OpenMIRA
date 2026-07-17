"""add device Agent primary rollout reservation invariant

Revision ID: 202607170010
Revises: 202607170009
Create Date: 2026-07-17
"""

from collections.abc import Sequence

from alembic import op
import sqlalchemy as sa


revision: str = "202607170010"
down_revision: str | None = "202607170009"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    # Older callers could reserve the same message under different idempotency keys.
    # Keep the oldest reservation and release only redundant, still-unconsumed rows.
    op.execute(
        sa.text(
            """
            WITH duplicates AS (
                SELECT id,
                       row_number() OVER (
                           PARTITION BY source_message_id
                           ORDER BY created_at, id
                       ) AS position
                FROM usage_ledger
                WHERE feature = 'PI_AGENT_ANALYSIS'
                  AND status = 'RESERVED'
                  AND source_message_id IS NOT NULL
            )
            UPDATE usage_ledger AS ledger
            SET status = 'RELEASED',
                released_at = now(),
                updated_at = now(),
                failure_reason = 'superseded duplicate message reservation'
            FROM duplicates
            WHERE ledger.id = duplicates.id
              AND duplicates.position > 1
            """
        )
    )
    op.create_index(
        "uq_usage_ledger_message_reserved_agent",
        "usage_ledger",
        ["source_message_id"],
        unique=True,
        postgresql_where=sa.text(
            "feature = 'PI_AGENT_ANALYSIS' AND status = 'RESERVED' "
            "AND source_message_id IS NOT NULL"
        ),
    )
    op.create_index(
        "ix_analysis_runs_mode_status_claimed",
        "analysis_runs",
        ["mode", "status", "claimed_at"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index(
        "ix_analysis_runs_mode_status_claimed",
        table_name="analysis_runs",
    )
    op.drop_index(
        "uq_usage_ledger_message_reserved_agent",
        table_name="usage_ledger",
    )
