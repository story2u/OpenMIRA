"""Store the provider management URL encrypted on the entitlement projection.

Revision ID: 202607120005
Revises: 202607120004
Create Date: 2026-07-12
"""

import sqlalchemy as sa
from alembic import op


revision = "202607120005"
down_revision = "202607120004"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("subscription_accounts", sa.Column("management_url_encrypted", sa.Text(), nullable=True))


def downgrade() -> None:
    op.drop_column("subscription_accounts", "management_url_encrypted")
