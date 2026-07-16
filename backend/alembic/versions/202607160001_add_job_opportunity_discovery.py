"""add job opportunity discovery domain

Revision ID: 202607160001
Revises: 202607150002
Create Date: 2026-07-16
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607160001"
down_revision: str | None = "202607150002"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

IM_CHANNEL = sa.Enum("TELEGRAM", "WECOM", native_enum=False)
OPPORTUNITY_TYPE = sa.Enum("BUSINESS", "JOB", native_enum=False)
SOURCE_FUNCTION = sa.Enum(
    "RECRUITMENT",
    "JOB_REFERRAL",
    "CAREER_NETWORKING",
    "TECHNICAL_DISCUSSION",
    "INDUSTRY_COMMUNITY",
    "COMPANY_OFFICIAL",
    "ALUMNI_COMMUNITY",
    "GENERAL_CHAT",
    "EDUCATION_TRAINING",
    "MARKETPLACE",
    "INVESTMENT_CRYPTO",
    "ADVERTISING",
    "UNKNOWN",
    native_enum=False,
)
JOB_CLASSIFICATION = sa.Enum(
    "JOB_POST",
    "JOB_REPOST",
    "CANDIDATE_SELF_PROMOTION",
    "JOB_SEEKING_REQUEST",
    "JOB_DISCUSSION",
    "RECRUITER_CHATTER",
    "REFERRAL_REQUEST",
    "TRAINING_AD",
    "PAID_COURSE_AD",
    "GENERIC_AD",
    "SPAM",
    "SCAM",
    "UNRELATED_CHAT",
    "UNKNOWN",
    native_enum=False,
)
WORK_MODE = sa.Enum("REMOTE", "HYBRID", "ON_SITE", "FLEXIBLE", "UNKNOWN", native_enum=False)
EMPLOYMENT_TYPE = sa.Enum(
    "FULL_TIME",
    "PART_TIME",
    "CONTRACT",
    "INTERNSHIP",
    "FREELANCE",
    "TEMPORARY",
    "UNKNOWN",
    native_enum=False,
)
SENIORITY = sa.Enum(
    "INTERN",
    "JUNIOR",
    "MID",
    "SENIOR",
    "LEAD",
    "MANAGER",
    "DIRECTOR",
    "EXECUTIVE",
    "UNKNOWN",
    native_enum=False,
)
SALARY_PERIOD = sa.Enum(
    "HOURLY", "DAILY", "MONTHLY", "ANNUAL", "PROJECT", "UNKNOWN", native_enum=False
)
ELIGIBILITY = sa.Enum("ELIGIBLE", "NOT_ELIGIBLE", "UNKNOWN", native_enum=False)
FEEDBACK_TYPE = sa.Enum(
    "RELEVANT",
    "NOT_RELEVANT",
    "NOT_A_JOB",
    "DUPLICATE",
    "EXPIRED",
    "SCAM",
    "WRONG_EXTRACTION",
    native_enum=False,
)


def timestamps() -> list[sa.Column]:
    return [
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
    ]


def upgrade() -> None:
    op.add_column(
        "opportunities",
        sa.Column("opportunity_type", OPPORTUNITY_TYPE, nullable=False, server_default="BUSINESS"),
    )
    op.create_index("ix_opportunities_opportunity_type", "opportunities", ["opportunity_type"])
    op.alter_column("opportunities", "opportunity_type", server_default=None)

    op.create_table(
        "source_functional_profiles",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("channel", IM_CHANNEL, nullable=False),
        sa.Column("external_source_id", sa.String(255), nullable=False),
        sa.Column("source_display_name", sa.String(500), nullable=False),
        sa.Column("source_description", sa.String(2000), nullable=True),
        sa.Column("source_username", sa.String(255), nullable=True),
        sa.Column("primary_function", SOURCE_FUNCTION, nullable=False),
        sa.Column("secondary_functions", postgresql.JSONB(), nullable=False),
        sa.Column("industry_tags", postgresql.JSONB(), nullable=False),
        sa.Column("region_tags", postgresql.JSONB(), nullable=False),
        sa.Column("language_tags", postgresql.JSONB(), nullable=False),
        sa.Column("job_signal_prior", sa.Float(), nullable=False),
        sa.Column("estimated_noise_level", sa.Float(), nullable=False),
        sa.Column("reliability_score", sa.Float(), nullable=False),
        sa.Column("confidence", sa.Float(), nullable=False),
        sa.Column("evidence", postgresql.JSONB(), nullable=False),
        sa.Column("manual_override", SOURCE_FUNCTION, nullable=True),
        sa.Column("sampled_message_count", sa.Integer(), nullable=False),
        sa.Column("source_fingerprint", sa.String(64), nullable=False),
        sa.Column("profiled_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "channel",
            "external_source_id",
            name="uq_source_functional_profiles_owner_source",
        ),
    )
    for column in ("owner_user_id", "channel", "external_source_id", "expires_at"):
        op.create_index(
            f"ix_source_functional_profiles_{column}", "source_functional_profiles", [column]
        )
    op.create_index(
        "ix_source_functional_profiles_owner_function",
        "source_functional_profiles",
        ["owner_user_id", "primary_function"],
    )

    op.create_table(
        "job_message_audits",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("message_id", sa.Uuid(), nullable=False),
        sa.Column("source_profile_id", sa.Uuid(), nullable=True),
        sa.Column("classification", JOB_CLASSIFICATION, nullable=False),
        sa.Column("confidence", sa.Float(), nullable=False),
        sa.Column("filter_reason", sa.String(1000), nullable=True),
        sa.Column("prefilter_score", sa.Float(), nullable=False),
        sa.Column("agent_required", sa.Boolean(), nullable=False),
        sa.Column("manually_corrected", sa.Boolean(), nullable=False),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(["message_id"], ["messages.id"]),
        sa.ForeignKeyConstraint(["source_profile_id"], ["source_functional_profiles.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("message_id", name="uq_job_message_audits_message"),
    )
    for column in ("owner_user_id", "message_id", "source_profile_id", "classification"):
        op.create_index(f"ix_job_message_audits_{column}", "job_message_audits", [column])
    op.create_index(
        "ix_job_message_audits_owner_classification",
        "job_message_audits",
        ["owner_user_id", "classification"],
    )

    op.create_table(
        "job_opportunity_details",
        *timestamps(),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("source_channel", IM_CHANNEL, nullable=False),
        sa.Column("source_chat_id", sa.String(255), nullable=False),
        sa.Column("source_chat_name", sa.String(500), nullable=True),
        sa.Column("source_message_id", sa.String(255), nullable=False),
        sa.Column("source_message_url", sa.String(2000), nullable=True),
        sa.Column("source_author_name", sa.String(500), nullable=True),
        sa.Column("source_author_username", sa.String(255), nullable=True),
        sa.Column("source_reliability_score", sa.Float(), nullable=False),
        sa.Column("posted_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("captured_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("job_title", sa.String(500), nullable=False),
        sa.Column("normalized_job_title", sa.String(500), nullable=True),
        sa.Column("company_name", sa.String(500), nullable=True),
        sa.Column("department", sa.String(500), nullable=True),
        sa.Column("company_industry", sa.String(255), nullable=True),
        sa.Column("company_stage", sa.String(255), nullable=True),
        sa.Column("location_text", sa.String(500), nullable=True),
        sa.Column("country_code", sa.String(2), nullable=True),
        sa.Column("city", sa.String(255), nullable=True),
        sa.Column("timezone", sa.String(100), nullable=True),
        sa.Column("work_mode", WORK_MODE, nullable=False),
        sa.Column("employment_type", EMPLOYMENT_TYPE, nullable=False),
        sa.Column("seniority", SENIORITY, nullable=False),
        sa.Column("salary_raw", sa.String(500), nullable=True),
        sa.Column("salary_min", sa.Numeric(18, 2), nullable=True),
        sa.Column("salary_max", sa.Numeric(18, 2), nullable=True),
        sa.Column("salary_currency", sa.String(3), nullable=True),
        sa.Column("salary_period", SALARY_PERIOD, nullable=False),
        sa.Column("salary_negotiable", sa.Boolean(), nullable=True),
        sa.Column("equity_mentioned", sa.Boolean(), nullable=True),
        sa.Column("requirements_summary", sa.String(4000), nullable=True),
        sa.Column("required_skills", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_skills", postgresql.JSONB(), nullable=False),
        sa.Column("minimum_years_experience", sa.Float(), nullable=True),
        sa.Column("maximum_years_experience", sa.Float(), nullable=True),
        sa.Column("degree_required", sa.Boolean(), nullable=True),
        sa.Column("degree_level", sa.String(100), nullable=True),
        sa.Column("degree_field", sa.String(255), nullable=True),
        sa.Column("english_level", sa.String(100), nullable=True),
        sa.Column("other_language_requirements", postgresql.JSONB(), nullable=False),
        sa.Column("visa_sponsorship", sa.Boolean(), nullable=True),
        sa.Column("work_authorization_text", sa.String(1000), nullable=True),
        sa.Column("relocation_support", sa.Boolean(), nullable=True),
        sa.Column("age_requirement_text", sa.String(500), nullable=True),
        sa.Column("age_requirement_present", sa.Boolean(), nullable=False),
        sa.Column("application_deadline", sa.DateTime(timezone=True), nullable=True),
        sa.Column("application_url", sa.String(2000), nullable=True),
        sa.Column("contact_methods", postgresql.JSONB(), nullable=False),
        sa.Column("compliance_flags", postgresql.JSONB(), nullable=False),
        sa.Column("extraction_confidence", sa.Float(), nullable=False),
        sa.Column("missing_fields", postgresql.JSONB(), nullable=False),
        sa.Column("field_evidence", postgresql.JSONB(), nullable=False),
        sa.Column("raw_excerpt", sa.String(4000), nullable=False),
        sa.Column("content_fingerprint", sa.String(64), nullable=False),
        sa.Column("duplicate_group_id", sa.Uuid(), nullable=True),
        sa.Column("conflicting_source_data", sa.Boolean(), nullable=False),
        sa.Column("is_expired", sa.Boolean(), nullable=False),
        sa.Column("expired_reason", sa.String(500), nullable=True),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.PrimaryKeyConstraint("opportunity_id"),
    )
    for column in (
        "source_channel",
        "source_chat_id",
        "source_message_id",
        "posted_at",
        "normalized_job_title",
        "company_name",
        "country_code",
        "city",
        "work_mode",
        "employment_type",
        "seniority",
        "salary_currency",
        "degree_level",
        "english_level",
        "visa_sponsorship",
        "age_requirement_present",
        "application_deadline",
        "content_fingerprint",
        "duplicate_group_id",
        "is_expired",
    ):
        op.create_index(f"ix_job_opportunity_details_{column}", "job_opportunity_details", [column])
    op.create_index(
        "ix_job_details_company_title_location",
        "job_opportunity_details",
        ["company_name", "normalized_job_title", "city"],
    )
    op.create_index("ix_job_details_posted_at", "job_opportunity_details", ["posted_at"])
    op.create_index(
        "ix_job_details_duplicate_group", "job_opportunity_details", ["duplicate_group_id"]
    )

    op.create_table(
        "job_opportunity_sources",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("message_id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("source_channel", IM_CHANNEL, nullable=False),
        sa.Column("source_message_url", sa.String(2000), nullable=True),
        sa.Column("source_chat_name", sa.String(500), nullable=True),
        sa.Column("source_author_name", sa.String(500), nullable=True),
        sa.Column("posted_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("source_reliability_score", sa.Float(), nullable=False),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["message_id"], ["messages.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "opportunity_id", "message_id", name="uq_job_sources_opportunity_message"
        ),
    )
    for column in ("opportunity_id", "message_id", "owner_user_id", "source_channel"):
        op.create_index(f"ix_job_opportunity_sources_{column}", "job_opportunity_sources", [column])

    op.create_table(
        "job_search_profiles",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("name", sa.String(120), nullable=False),
        sa.Column("is_default", sa.Boolean(), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("target_roles", postgresql.JSONB(), nullable=False),
        sa.Column("excluded_roles", postgresql.JSONB(), nullable=False),
        sa.Column("target_industries", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_seniority", postgresql.JSONB(), nullable=False),
        sa.Column("candidate_skills", postgresql.JSONB(), nullable=False),
        sa.Column("years_experience", sa.Float(), nullable=True),
        sa.Column("education_level", sa.String(100), nullable=True),
        sa.Column("english_level", sa.String(100), nullable=True),
        sa.Column("other_languages", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_countries", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_cities", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_timezones", postgresql.JSONB(), nullable=False),
        sa.Column("work_modes", postgresql.JSONB(), nullable=False),
        sa.Column("employment_types", postgresql.JSONB(), nullable=False),
        sa.Column("minimum_salary", sa.Numeric(18, 2), nullable=True),
        sa.Column("salary_currency", sa.String(3), nullable=True),
        sa.Column("salary_period", SALARY_PERIOD, nullable=True),
        sa.Column("visa_sponsorship_required", sa.Boolean(), nullable=True),
        sa.Column("relocation_acceptable", sa.Boolean(), nullable=True),
        sa.Column("required_keywords", postgresql.JSONB(), nullable=False),
        sa.Column("preferred_keywords", postgresql.JSONB(), nullable=False),
        sa.Column("excluded_keywords", postgresql.JSONB(), nullable=False),
        sa.Column("require_salary_disclosed", sa.Boolean(), nullable=False),
        sa.Column("minimum_match_score", sa.Integer(), nullable=False),
        sa.Column("notification_enabled", sa.Boolean(), nullable=False),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_job_search_profiles_user_id", "job_search_profiles", ["user_id"])
    op.create_index("ix_job_search_profiles_enabled", "job_search_profiles", ["enabled"])
    op.create_index(
        "ix_job_search_profiles_user_enabled", "job_search_profiles", ["user_id", "enabled"]
    )
    op.create_index(
        "uq_job_search_profiles_default",
        "job_search_profiles",
        ["user_id"],
        unique=True,
        postgresql_where=sa.text("is_default"),
    )

    op.create_table(
        "job_opportunity_matches",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("job_search_profile_id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("eligibility", ELIGIBILITY, nullable=False),
        sa.Column("match_score", sa.Integer(), nullable=False),
        sa.Column("matched_reasons", postgresql.JSONB(), nullable=False),
        sa.Column("mismatch_reasons", postgresql.JSONB(), nullable=False),
        sa.Column("unknown_constraints", postgresql.JSONB(), nullable=False),
        sa.Column("score_breakdown", postgresql.JSONB(), nullable=False),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["job_search_profile_id"], ["job_search_profiles.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "opportunity_id", "job_search_profile_id", name="uq_job_matches_opportunity_profile"
        ),
    )
    for column in (
        "opportunity_id",
        "job_search_profile_id",
        "owner_user_id",
        "eligibility",
        "match_score",
    ):
        op.create_index(f"ix_job_opportunity_matches_{column}", "job_opportunity_matches", [column])
    op.create_index(
        "ix_job_matches_profile_score",
        "job_opportunity_matches",
        ["job_search_profile_id", "match_score"],
    )

    op.create_table(
        "job_opportunity_feedback",
        *timestamps(),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("opportunity_id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("feedback_type", FEEDBACK_TYPE, nullable=False),
        sa.Column("note", sa.String(1000), nullable=True),
        sa.ForeignKeyConstraint(["opportunity_id"], ["opportunities.id"]),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "opportunity_id", "owner_user_id", name="uq_job_feedback_owner_opportunity"
        ),
    )
    for column in ("opportunity_id", "owner_user_id", "feedback_type"):
        op.create_index(
            f"ix_job_opportunity_feedback_{column}", "job_opportunity_feedback", [column]
        )


def downgrade() -> None:
    op.drop_table("job_opportunity_feedback")
    op.drop_table("job_opportunity_matches")
    op.drop_table("job_search_profiles")
    op.drop_table("job_opportunity_sources")
    op.drop_table("job_opportunity_details")
    op.drop_table("job_message_audits")
    op.drop_table("source_functional_profiles")
    op.drop_index("ix_opportunities_opportunity_type", table_name="opportunities")
    op.drop_column("opportunities", "opportunity_type")
