"""Allow transfer webhooks to retain every valid local user UUID.

Revision ID: 202607120006
Revises: 202607120005
Create Date: 2026-07-12
"""

import sqlalchemy as sa
from alembic import op


revision = "202607120006"
down_revision = "202607120005"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.alter_column("billing_events", "app_user_id", existing_type=sa.String(length=255), type_=sa.Text(), existing_nullable=True)


def downgrade() -> None:
    op.execute("UPDATE billing_events SET app_user_id = left(app_user_id, 255) WHERE length(app_user_id) > 255")
    op.alter_column("billing_events", "app_user_id", existing_type=sa.Text(), type_=sa.String(length=255), existing_nullable=True)
