"""add bounded analysis run link evidence

Revision ID: 202607170008
Revises: 202607170007
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607170008"
down_revision: str | None = "202607170007"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "analysis_runs",
        sa.Column(
            "link_evidence",
            postgresql.JSONB(astext_type=sa.Text(), none_as_null=True),
            nullable=True,
        ),
    )
    op.add_column(
        "analysis_runs",
        sa.Column("link_evidence_fetched_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_check_constraint(
        "ck_analysis_runs_link_evidence_bounded_array",
        "analysis_runs",
        "(link_evidence IS NULL AND link_evidence_fetched_at IS NULL) OR "
        "(jsonb_typeof(link_evidence) = 'array' "
        "AND octet_length(link_evidence::text) <= 262144 "
        "AND link_evidence_fetched_at IS NOT NULL)",
    )


def downgrade() -> None:
    op.drop_constraint(
        "ck_analysis_runs_link_evidence_bounded_array",
        "analysis_runs",
        type_="check",
    )
    op.drop_column("analysis_runs", "link_evidence_fetched_at")
    op.drop_column("analysis_runs", "link_evidence")
