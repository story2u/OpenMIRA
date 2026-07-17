"""add analysis shadow observations

Revision ID: 202607170009
Revises: 202607170008
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607170009"
down_revision: str | None = "202607170008"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "messages",
        sa.Column(
            "agent_execution",
            postgresql.JSONB(astext_type=sa.Text(), none_as_null=True),
            nullable=True,
        ),
    )
    op.create_check_constraint(
        "ck_messages_agent_execution_bounded_object",
        "messages",
        "agent_execution IS NULL OR (jsonb_typeof(agent_execution) = 'object' "
        "AND octet_length(agent_execution::text) <= 4096)",
    )
    op.add_column(
        "analysis_runs",
        sa.Column(
            "mode",
            sa.Enum(
                "PRIMARY",
                "SHADOW",
                name="analysisrunmode",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            server_default="PRIMARY",
        ),
    )
    op.add_column(
        "analysis_runs",
        sa.Column("shadow_match", sa.Boolean(), nullable=True),
    )
    op.add_column(
        "analysis_runs",
        sa.Column("shadow_difference_count", sa.Integer(), nullable=True),
    )
    op.create_check_constraint(
        "ck_analysis_runs_shadow_observation",
        "analysis_runs",
        "(mode = 'PRIMARY' AND shadow_match IS NULL "
        "AND shadow_difference_count IS NULL) OR "
        "(mode = 'SHADOW' AND status != 'COMPLETED' "
        "AND shadow_match IS NULL AND shadow_difference_count IS NULL) OR "
        "(mode = 'SHADOW' AND status = 'COMPLETED' "
        "AND shadow_match IS NOT NULL AND shadow_difference_count IS NOT NULL "
        "AND shadow_difference_count >= 0)",
    )
    op.create_index(
        "uq_analysis_runs_message_shadow",
        "analysis_runs",
        ["message_id"],
        unique=True,
        postgresql_where=sa.text("mode = 'SHADOW'"),
    )
    op.alter_column("analysis_runs", "mode", server_default=None)


def downgrade() -> None:
    op.drop_index("uq_analysis_runs_message_shadow", table_name="analysis_runs")
    op.drop_constraint(
        "ck_analysis_runs_shadow_observation",
        "analysis_runs",
        type_="check",
    )
    op.drop_column("analysis_runs", "shadow_difference_count")
    op.drop_column("analysis_runs", "shadow_match")
    op.drop_column("analysis_runs", "mode")
    op.drop_constraint(
        "ck_messages_agent_execution_bounded_object",
        "messages",
        type_="check",
    )
    op.drop_column("messages", "agent_execution")
