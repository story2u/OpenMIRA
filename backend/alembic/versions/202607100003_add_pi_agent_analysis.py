"""add pi agent analysis projections

Revision ID: 202607100003
Revises: 202607100002
Create Date: 2026-07-10 23:30:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
import sqlmodel
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607100003"
down_revision: str | None = "202607100002"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


AGENT_STATUS = sa.Enum(
    "NOT_REQUESTED",
    "QUEUED",
    "RUNNING",
    "COMPLETED",
    "FAILED",
    native_enum=False,
)


def upgrade() -> None:
    op.add_column(
        "messages",
        sa.Column(
            "source_type",
            sqlmodel.sql.sqltypes.AutoString(),
            nullable=False,
            server_default="private",
        ),
    )
    op.add_column(
        "messages",
        sa.Column("group_name", sqlmodel.sql.sqltypes.AutoString(), nullable=True),
    )
    op.add_column(
        "messages",
        sa.Column(
            "raw_message_links",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text("'[]'::jsonb"),
        ),
    )
    op.add_column(
        "messages",
        sa.Column(
            "agent_analysis_status",
            AGENT_STATUS,
            nullable=False,
            server_default="NOT_REQUESTED",
        ),
    )
    op.add_column(
        "messages",
        sa.Column(
            "agent_result",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text("'{}'::jsonb"),
        ),
    )
    op.add_column("messages", sa.Column("agent_error", sqlmodel.sql.sqltypes.AutoString(), nullable=True))
    op.add_column("messages", sa.Column("agent_started_at", sa.DateTime(timezone=True), nullable=True))
    op.add_column("messages", sa.Column("agent_analyzed_at", sa.DateTime(timezone=True), nullable=True))
    op.create_index("ix_messages_source_type", "messages", ["source_type"], unique=False)
    op.create_index(
        "ix_messages_agent_analysis_status",
        "messages",
        ["agent_analysis_status"],
        unique=False,
    )

    op.add_column(
        "opportunities",
        sa.Column(
            "link_verification",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text(
                "jsonb_build_object("
                "'status', 'unverified', "
                "'verifiedAt', NULL, "
                "'riskReasons', '[]'::jsonb, "
                "'resolvedInfo', NULL)"
            ),
        ),
    )
    op.add_column(
        "opportunities",
        sa.Column(
            "extracted_contacts",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text(
                "jsonb_build_object("
                "'phone', NULL, "
                "'email', NULL, "
                "'telegramHandle', NULL, "
                "'wecomId', NULL, "
                "'extractionSource', NULL)"
            ),
        ),
    )
    op.add_column(
        "opportunities",
        sa.Column(
            "friend_request_status",
            sqlmodel.sql.sqltypes.AutoString(),
            nullable=False,
            server_default="n/a",
        ),
    )
    op.execute(
        "UPDATE opportunities SET friend_request_status = 'not_sent' "
        "WHERE source_type = 'group'"
    )
    op.add_column(
        "opportunities",
        sa.Column(
            "sop_stage",
            sqlmodel.sql.sqltypes.AutoString(),
            nullable=False,
            server_default="detected",
        ),
    )
    op.add_column(
        "opportunities",
        sa.Column(
            "agent_actions",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text("'[]'::jsonb"),
        ),
    )
    op.add_column(
        "opportunities",
        sa.Column(
            "agent_analysis_status",
            AGENT_STATUS,
            nullable=False,
            server_default="NOT_REQUESTED",
        ),
    )
    op.add_column(
        "opportunities",
        sa.Column("agent_analysis_error", sqlmodel.sql.sqltypes.AutoString(), nullable=True),
    )
    op.add_column(
        "opportunities",
        sa.Column("agent_analyzed_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.add_column(
        "opportunities",
        sa.Column("attention_required", sa.Boolean(), nullable=False, server_default=sa.false()),
    )
    op.create_index(
        "ix_opportunities_agent_analysis_status",
        "opportunities",
        ["agent_analysis_status"],
        unique=False,
    )
    op.create_index(
        "ix_opportunities_attention_required",
        "opportunities",
        ["attention_required"],
        unique=False,
    )
    op.drop_index("ix_opportunities_source_message_id", table_name="opportunities")
    op.create_index(
        "ix_opportunities_source_message_id",
        "opportunities",
        ["source_message_id"],
        unique=True,
    )


def downgrade() -> None:
    op.drop_index("ix_opportunities_source_message_id", table_name="opportunities")
    op.create_index(
        "ix_opportunities_source_message_id",
        "opportunities",
        ["source_message_id"],
        unique=False,
    )
    op.drop_index("ix_opportunities_attention_required", table_name="opportunities")
    op.drop_index("ix_opportunities_agent_analysis_status", table_name="opportunities")
    op.drop_column("opportunities", "attention_required")
    op.drop_column("opportunities", "agent_analyzed_at")
    op.drop_column("opportunities", "agent_analysis_error")
    op.drop_column("opportunities", "agent_analysis_status")
    op.drop_column("opportunities", "agent_actions")
    op.drop_column("opportunities", "sop_stage")
    op.drop_column("opportunities", "friend_request_status")
    op.drop_column("opportunities", "extracted_contacts")
    op.drop_column("opportunities", "link_verification")

    op.drop_index("ix_messages_agent_analysis_status", table_name="messages")
    op.drop_index("ix_messages_source_type", table_name="messages")
    op.drop_column("messages", "agent_analyzed_at")
    op.drop_column("messages", "agent_started_at")
    op.drop_column("messages", "agent_error")
    op.drop_column("messages", "agent_result")
    op.drop_column("messages", "agent_analysis_status")
    op.drop_column("messages", "raw_message_links")
    op.drop_column("messages", "group_name")
    op.drop_column("messages", "source_type")
