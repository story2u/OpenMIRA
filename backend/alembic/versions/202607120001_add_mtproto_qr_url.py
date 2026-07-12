"""Store the short-lived Telegram QR login URL encrypted.

Revision ID: 202607120001
Revises: 202607110003
Create Date: 2026-07-12
"""

from alembic import op
import sqlalchemy as sa


revision = "202607120001"
down_revision = "202607110003"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("telegram_connection_attempts", sa.Column("qr_url_encrypted", sa.Text(), nullable=True))


def downgrade() -> None:
    op.drop_column("telegram_connection_attempts", "qr_url_encrypted")
