"""Match the pending MTProto QR index to SQLAlchemy enum storage.

Revision ID: 202607120003
Revises: 202607120002
Create Date: 2026-07-12
"""

from alembic import op


revision = "202607120003"
down_revision = "202607120002"
branch_labels = None
depends_on = None

INDEX_NAME = "uq_telegram_connection_attempts_owner_pending_mtproto_qr"


def upgrade() -> None:
    op.drop_index(INDEX_NAME, table_name="telegram_connection_attempts")
    op.execute(
        """
        WITH ranked AS (
            SELECT id,
                   row_number() OVER (
                       PARTITION BY owner_user_id
                       ORDER BY created_at DESC, id DESC
                   ) AS row_number
            FROM telegram_connection_attempts
            WHERE connection_type = 'MTPROTO_QR' AND status = 'PENDING'
        )
        UPDATE telegram_connection_attempts AS attempt
        SET status = 'EXPIRED', updated_at = now()
        FROM ranked
        WHERE attempt.id = ranked.id
          AND (ranked.row_number > 1 OR attempt.expires_at <= now())
        """
    )
    op.execute(
        f"""
        CREATE UNIQUE INDEX {INDEX_NAME}
        ON telegram_connection_attempts (owner_user_id)
        WHERE connection_type = 'MTPROTO_QR' AND status = 'PENDING'
        """
    )


def downgrade() -> None:
    op.drop_index(INDEX_NAME, table_name="telegram_connection_attempts")
    op.execute(
        f"""
        CREATE UNIQUE INDEX {INDEX_NAME}
        ON telegram_connection_attempts (owner_user_id)
        WHERE connection_type = 'mtproto_qr' AND status = 'pending'
        """
    )
