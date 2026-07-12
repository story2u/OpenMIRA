"""Add unified billing records and expand the entitlement projection.

Revision ID: 202607120004
Revises: 202607120003
Create Date: 2026-07-12
"""

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op


revision = "202607120004"
down_revision = "202607120003"
branch_labels = None
depends_on = None

PLAN_CODE = sa.Enum("FREE", "PLUS", "PRO", "MAX", native_enum=False)
BILLING_PROVIDER = sa.Enum("REVENUECAT", native_enum=False)
BILLING_STORE = sa.Enum("APP_STORE", "PLAY_STORE", "PADDLE", "TEST_STORE", "UNKNOWN", native_enum=False)
BILLING_INTERVAL = sa.Enum("MONTHLY", "ANNUAL", "UNKNOWN", native_enum=False)
BILLING_STATUS = sa.Enum(
    "TRIALING",
    "ACTIVE",
    "GRACE_PERIOD",
    "BILLING_RETRY",
    "CANCELED",
    "PAUSED",
    "ON_HOLD",
    "EXPIRED",
    "REFUNDED",
    "REVOKED",
    "INACTIVE",
    "UNKNOWN",
    native_enum=False,
)
BILLING_EVENT_STATUS = sa.Enum(
    "RECEIVED", "QUEUED", "PROCESSING", "COMPLETED", "FAILED", "ORPHANED", native_enum=False
)


def upgrade() -> None:
    op.create_table(
        "billing_products",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("plan_code", PLAN_CODE, nullable=False),
        sa.Column("billing_interval", BILLING_INTERVAL, nullable=False),
        sa.Column("store", BILLING_STORE, nullable=False),
        sa.Column("external_product_id", sa.String(length=255), nullable=False),
        sa.Column("external_base_plan_id", sa.String(length=255), nullable=True),
        sa.Column("revenuecat_entitlement_id", sa.String(length=64), nullable=False),
        sa.Column("revenuecat_package_id", sa.String(length=64), nullable=False),
        sa.Column("active", sa.Boolean(), nullable=False),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_billing_products_plan_code", "billing_products", ["plan_code"])
    op.create_index("ix_billing_products_billing_interval", "billing_products", ["billing_interval"])
    op.create_index("ix_billing_products_store", "billing_products", ["store"])
    op.create_index("ix_billing_products_external_product_id", "billing_products", ["external_product_id"])
    op.create_index("ix_billing_products_revenuecat_entitlement_id", "billing_products", ["revenuecat_entitlement_id"])
    op.create_index("ix_billing_products_revenuecat_package_id", "billing_products", ["revenuecat_package_id"])
    op.create_index("ix_billing_products_active", "billing_products", ["active"])
    op.execute(
        """
        CREATE UNIQUE INDEX uq_billing_products_store_product_base_plan
        ON billing_products (store, external_product_id, COALESCE(external_base_plan_id, ''))
        """
    )

    op.create_table(
        "billing_subscriptions",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("user_id", sa.Uuid(), nullable=False),
        sa.Column("provider", BILLING_PROVIDER, nullable=False),
        sa.Column("store", BILLING_STORE, nullable=False),
        sa.Column("environment", sa.String(length=32), nullable=False),
        sa.Column("external_key", sa.String(length=512), nullable=False),
        sa.Column("external_product_id", sa.String(length=255), nullable=False),
        sa.Column("external_transaction_id", sa.String(length=255), nullable=True),
        sa.Column("external_original_transaction_id", sa.String(length=255), nullable=True),
        sa.Column("external_subscription_id", sa.String(length=255), nullable=True),
        sa.Column("revenuecat_entitlement_id", sa.String(length=64), nullable=True),
        sa.Column("plan_code", PLAN_CODE, nullable=False),
        sa.Column("billing_interval", BILLING_INTERVAL, nullable=False),
        sa.Column("status", BILLING_STATUS, nullable=False),
        sa.Column("current_period_start", sa.DateTime(timezone=True), nullable=True),
        sa.Column("current_period_end", sa.DateTime(timezone=True), nullable=True),
        sa.Column("grace_period_end", sa.DateTime(timezone=True), nullable=True),
        sa.Column("will_renew", sa.Boolean(), nullable=False),
        sa.Column("cancel_at_period_end", sa.Boolean(), nullable=False),
        sa.Column("billing_issue_detected_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_provider_event_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_synced_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("provider", "external_key", name="uq_billing_subscriptions_provider_external_key"),
        sa.CheckConstraint("environment IN ('sandbox', 'production')", name="ck_billing_subscriptions_environment"),
        sa.CheckConstraint("length(external_key) > 0", name="ck_billing_subscriptions_external_key_nonempty"),
    )
    for column in ("user_id", "provider", "store", "environment", "external_product_id", "revenuecat_entitlement_id", "plan_code", "status", "last_synced_at"):
        op.create_index(f"ix_billing_subscriptions_{column}", "billing_subscriptions", [column])
    op.create_index(
        "ix_billing_subscriptions_user_status_end",
        "billing_subscriptions",
        ["user_id", "status", "current_period_end"],
    )

    op.create_table(
        "billing_events",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("provider", BILLING_PROVIDER, nullable=False),
        sa.Column("provider_event_id", sa.String(length=255), nullable=False),
        sa.Column("event_type", sa.String(length=128), nullable=False),
        sa.Column("app_user_id", sa.String(length=255), nullable=True),
        sa.Column("environment", sa.String(length=32), nullable=True),
        sa.Column("payload_hash", sa.String(length=64), nullable=False),
        sa.Column("status", BILLING_EVENT_STATUS, nullable=False),
        sa.Column("attempt_count", sa.Integer(), nullable=False),
        sa.Column("received_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("queued_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("processing_error", sa.String(length=1000), nullable=True),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("provider", "provider_event_id", name="uq_billing_events_provider_event_id"),
        sa.CheckConstraint("attempt_count >= 0", name="ck_billing_events_attempt_count_nonnegative"),
        sa.CheckConstraint("length(payload_hash) = 64", name="ck_billing_events_payload_hash_length"),
    )
    for column in ("provider", "event_type", "app_user_id", "status"):
        op.create_index(f"ix_billing_events_{column}", "billing_events", [column])
    op.create_index("ix_billing_events_status_received", "billing_events", ["status", "received_at"])

    op.add_column("subscription_accounts", sa.Column("effective_store", BILLING_STORE, nullable=True))
    op.add_column("subscription_accounts", sa.Column("billing_interval", BILLING_INTERVAL, nullable=True))
    op.add_column("subscription_accounts", sa.Column("entitlement_started_at", sa.DateTime(timezone=True), nullable=True))
    op.add_column("subscription_accounts", sa.Column("entitlement_expires_at", sa.DateTime(timezone=True), nullable=True))
    op.add_column("subscription_accounts", sa.Column("will_renew", sa.Boolean(), nullable=False, server_default=sa.false()))
    op.add_column("subscription_accounts", sa.Column("billing_issue", sa.Boolean(), nullable=False, server_default=sa.false()))
    op.add_column(
        "subscription_accounts",
        sa.Column("multiple_active_subscriptions", sa.Boolean(), nullable=False, server_default=sa.false()),
    )
    op.add_column("subscription_accounts", sa.Column("last_synced_at", sa.DateTime(timezone=True), nullable=True))
    op.create_index("ix_subscription_accounts_effective_store", "subscription_accounts", ["effective_store"])
    op.create_index("ix_subscription_accounts_entitlement_expires_at", "subscription_accounts", ["entitlement_expires_at"])
    op.create_index("ix_subscription_accounts_last_synced_at", "subscription_accounts", ["last_synced_at"])


def downgrade() -> None:
    op.drop_index("ix_subscription_accounts_last_synced_at", table_name="subscription_accounts")
    op.drop_index("ix_subscription_accounts_entitlement_expires_at", table_name="subscription_accounts")
    op.drop_index("ix_subscription_accounts_effective_store", table_name="subscription_accounts")
    for column in (
        "last_synced_at",
        "multiple_active_subscriptions",
        "billing_issue",
        "will_renew",
        "entitlement_expires_at",
        "entitlement_started_at",
        "billing_interval",
        "effective_store",
    ):
        op.drop_column("subscription_accounts", column)

    op.drop_index("ix_billing_events_status_received", table_name="billing_events")
    for column in ("status", "app_user_id", "event_type", "provider"):
        op.drop_index(f"ix_billing_events_{column}", table_name="billing_events")
    op.drop_table("billing_events")

    op.drop_index("ix_billing_subscriptions_user_status_end", table_name="billing_subscriptions")
    for column in ("last_synced_at", "status", "plan_code", "revenuecat_entitlement_id", "external_product_id", "environment", "store", "provider", "user_id"):
        op.drop_index(f"ix_billing_subscriptions_{column}", table_name="billing_subscriptions")
    op.drop_table("billing_subscriptions")

    op.drop_index("uq_billing_products_store_product_base_plan", table_name="billing_products")
    for column in ("active", "revenuecat_package_id", "revenuecat_entitlement_id", "external_product_id", "store", "billing_interval", "plan_code"):
        op.drop_index(f"ix_billing_products_{column}", table_name="billing_products")
    op.drop_table("billing_products")
