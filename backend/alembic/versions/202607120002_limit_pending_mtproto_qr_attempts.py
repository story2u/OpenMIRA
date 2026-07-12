"""Limit each user to one pending MTProto QR attempt.

Revision ID: 202607120002
Revises: 202607120001
Create Date: 2026-07-12
"""

from alembic import op


revision = "202607120002"
down_revision = "202607120001"
branch_labels = None
depends_on = None

INDEX_NAME = "uq_telegram_connection_attempts_owner_pending_mtproto_qr"


def upgrade() -> None:
    op.execute(
        """
        WITH ranked AS (
            SELECT id,
                   row_number() OVER (
                       PARTITION BY owner_user_id
                       ORDER BY created_at DESC, id DESC
                   ) AS row_number
            FROM telegram_connection_attempts
            WHERE connection_type = 'mtproto_qr' AND status = 'pending'
        )
        UPDATE telegram_connection_attempts AS attempt
        SET status = 'expired', updated_at = now()
        FROM ranked
        WHERE attempt.id = ranked.id
          AND (ranked.row_number > 1 OR attempt.expires_at <= now())
        """
    )
    op.execute(
        f"""
        CREATE UNIQUE INDEX {INDEX_NAME}
        ON telegram_connection_attempts (owner_user_id)
        WHERE connection_type = 'mtproto_qr' AND status = 'pending'
        """
    )


def downgrade() -> None:
    op.drop_index(INDEX_NAME, table_name="telegram_connection_attempts")
