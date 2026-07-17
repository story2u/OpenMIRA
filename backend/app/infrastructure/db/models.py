from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID, uuid4

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    Column,
    DateTime,
    ForeignKeyConstraint,
    Identity,
    Index,
    Numeric,
    Text,
    UniqueConstraint,
    text,
)
from sqlalchemy import Enum as SAEnum
from sqlalchemy.dialects.postgresql import JSONB
from sqlmodel import Field, SQLModel

from app.domain.enums import (
    AgentAnalysisStatus,
    AnalysisProviderRequestStatus,
    AnalysisRunExecutor,
    AnalysisRunMode,
    AnalysisRunStatus,
    BillingEventStatus,
    BillingInterval,
    BillingProvider,
    BillingStore,
    BillingSubscriptionStatus,
    DeviceCredentialStatus,
    DevicePlatform,
    DeviceStatus,
    IMChannel,
    InteractiveAgentApprovalStatus,
    InteractiveAgentProviderRequestStatus,
    InteractiveAgentTurnStatus,
    ManualReplyDeliveryStatus,
    MessageDirection,
    MessageSource,
    OpportunityArchiveAction,
    OpportunityStatus,
    OpportunityType,
    PlanCode,
    Priority,
    PushEnvironment,
    PushProvider,
    PushRegistrationStatus,
    RuleType,
    SalaryPeriod,
    SourcePrimaryFunction,
    SubscriptionStatus,
    SyncAggregateType,
    SyncOperation,
    TelegramConnectionAttemptStatus,
    TelegramConnectionStatus,
    TelegramConnectionType,
    TelegramSourceType,
    UsageFeature,
    UsageStatus,
    WeComConnectionStatus,
    WeComConnectionType,
    WeComDeliveryStatus,
    WeComEventStatus,
    WeComReceiveCapability,
    WeComSendCapability,
    WeComSourceType,
)


def utc_now() -> datetime:
    return datetime.now(UTC)


class TimestampMixin(SQLModel):
    created_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )


class User(TimestampMixin, table=True):
    __tablename__ = "users"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    email: str = Field(index=True, unique=True)
    display_name: str = Field(default="")
    avatar_url: str = Field(default="")
    password_hash: str | None = None
    auth_version: int = Field(default=0, nullable=False)
    is_active: bool = Field(default=True, index=True)
    is_admin: bool = Field(default=False, index=True)
    last_login_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class PasswordResetChallenge(TimestampMixin, table=True):
    __tablename__ = "password_reset_challenges"
    __table_args__ = (
        CheckConstraint("failed_attempts >= 0", name="ck_password_reset_failed_attempts"),
        Index("ix_password_reset_user_expires", "user_id", "expires_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    token_digest: str = Field(max_length=64, unique=True)
    code_digest: str = Field(max_length=64)
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    used_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    failed_attempts: int = Field(default=0, nullable=False)


class AuthAccount(TimestampMixin, table=True):
    __tablename__ = "auth_accounts"
    __table_args__ = (
        UniqueConstraint("provider", "provider_subject", name="uq_auth_accounts_provider_subject"),
        Index("ix_auth_accounts_user_provider", "user_id", "provider"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    provider: str = Field(index=True)
    provider_subject: str = Field(index=True)
    email: str | None = Field(default=None, index=True)


class Device(TimestampMixin, table=True):
    """A revocable, owner-bound mobile installation; never a bearer credential store."""

    __tablename__ = "devices"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "installation_id_hash",
            name="uq_devices_owner_installation_hash",
        ),
        UniqueConstraint("owner_user_id", "id", name="uq_devices_owner_id"),
        CheckConstraint(
            "installation_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_devices_installation_hash_sha256",
        ),
        CheckConstraint(
            "jsonb_typeof(capabilities) = 'object' AND octet_length(capabilities::text) <= 16384",
            name="ck_devices_capabilities_bounded_object",
        ),
        CheckConstraint(
            "(status = 'ACTIVE' AND revoked_at IS NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL)",
            name="ck_devices_revocation_state",
        ),
        CheckConstraint(
            "last_sync_cursor >= 0",
            name="ck_devices_last_sync_cursor_nonnegative",
        ),
        Index(
            "ix_devices_owner_status_last_seen",
            "owner_user_id",
            "status",
            "last_seen_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    installation_id_hash: str = Field(min_length=64, max_length=64)
    platform: DevicePlatform = Field(
        sa_column=Column(
            SAEnum(
                DevicePlatform,
                name="deviceplatform",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        )
    )
    status: DeviceStatus = Field(
        default=DeviceStatus.ACTIVE,
        sa_column=Column(
            SAEnum(
                DeviceStatus,
                name="devicestatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    display_name: str = Field(default="", max_length=100)
    app_variant: str = Field(default="production", min_length=1, max_length=32)
    app_version: str = Field(min_length=1, max_length=32)
    app_build: str = Field(min_length=1, max_length=32)
    os_version: str | None = Field(default=None, max_length=64)
    locale: str | None = Field(default=None, max_length=35)
    timezone: str | None = Field(default=None, max_length=64)
    capabilities: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    last_seen_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    revoked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    revocation_reason: str | None = Field(default=None, max_length=255)
    last_sync_cursor: int = Field(
        default=0,
        ge=0,
        sa_column=Column(BigInteger, nullable=False, server_default="0"),
    )
    last_sync_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_sync_error_code: str | None = Field(default=None, max_length=64)


class DeviceCredential(TimestampMixin, table=True):
    """Hashed rotating device bearer metadata; raw refresh tokens are never persisted."""

    __tablename__ = "device_credentials"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_device_credentials_owner_device",
        ),
        UniqueConstraint("token_hash", name="uq_device_credentials_token_hash"),
        CheckConstraint(
            "token_hash ~ '^[0-9a-f]{64}$'",
            name="ck_device_credentials_token_hash_sha256",
        ),
        CheckConstraint(
            "expires_at > created_at",
            name="ck_device_credentials_expiry_after_creation",
        ),
        CheckConstraint(
            "(status IN ('PENDING', 'ACTIVE') AND rotated_at IS NULL AND revoked_at IS NULL "
            "AND reuse_detected_at IS NULL AND replaced_by_credential_id IS NULL) "
            "OR (status = 'ROTATED' AND rotated_at IS NOT NULL "
            "AND replaced_by_credential_id IS NOT NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL) "
            "OR (status = 'REUSE_DETECTED' AND revoked_at IS NOT NULL "
            "AND reuse_detected_at IS NOT NULL)",
            name="ck_device_credentials_lifecycle_state",
        ),
        Index(
            "uq_device_credentials_device_active",
            "device_id",
            unique=True,
            postgresql_where=text("status = 'ACTIVE'"),
        ),
        Index(
            "ix_device_credentials_owner_status_expires",
            "owner_user_id",
            "status",
            "expires_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    device_id: UUID = Field(index=True)
    token_hash: str = Field(min_length=64, max_length=64)
    token_family_id: UUID = Field(default_factory=uuid4, index=True)
    status: DeviceCredentialStatus = Field(
        default=DeviceCredentialStatus.ACTIVE,
        sa_column=Column(
            SAEnum(
                DeviceCredentialStatus,
                name="devicecredentialstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    last_used_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    rotated_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    revoked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    reuse_detected_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    replaced_by_credential_id: UUID | None = Field(
        default=None,
        foreign_key="device_credentials.id",
        index=True,
    )


class PushRegistration(TimestampMixin, table=True):
    """Encrypted minimum platform token plus a non-secret hash for rotation and deduplication."""

    __tablename__ = "push_registrations"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_push_registrations_owner_device",
        ),
        UniqueConstraint("token_hash", name="uq_push_registrations_token_hash"),
        CheckConstraint(
            "token_hash ~ '^[0-9a-f]{64}$'",
            name="ck_push_registrations_token_hash_sha256",
        ),
        CheckConstraint(
            "length(token_encrypted) >= 32",
            name="ck_push_registrations_encrypted_token_present",
        ),
        CheckConstraint(
            "failure_count >= 0",
            name="ck_push_registrations_failure_count_nonnegative",
        ),
        CheckConstraint(
            "last_notified_cursor >= 0",
            name="ck_push_registrations_last_notified_cursor_nonnegative",
        ),
        CheckConstraint(
            "(status = 'ACTIVE' AND invalidated_at IS NULL AND revoked_at IS NULL) "
            "OR (status = 'INVALIDATED' AND invalidated_at IS NOT NULL "
            "AND revoked_at IS NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL)",
            name="ck_push_registrations_lifecycle_state",
        ),
        Index(
            "uq_push_registrations_device_provider_environment_active",
            "device_id",
            "provider",
            "environment",
            unique=True,
            postgresql_where=text("status = 'ACTIVE'"),
        ),
        Index(
            "ix_push_registrations_owner_status",
            "owner_user_id",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    device_id: UUID = Field(index=True)
    provider: PushProvider = Field(
        sa_column=Column(
            SAEnum(
                PushProvider,
                name="pushprovider",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        )
    )
    environment: PushEnvironment = Field(
        sa_column=Column(
            SAEnum(
                PushEnvironment,
                name="pushenvironment",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        )
    )
    token_hash: str = Field(min_length=64, max_length=64)
    token_encrypted: str = Field(sa_column=Column(Text(), nullable=False))
    status: PushRegistrationStatus = Field(
        default=PushRegistrationStatus.ACTIVE,
        sa_column=Column(
            SAEnum(
                PushRegistrationStatus,
                name="pushregistrationstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    last_registered_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    last_attempt_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_success_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_notified_cursor: int = Field(
        default=0,
        ge=0,
        sa_column=Column(BigInteger(), nullable=False),
    )
    next_attempt_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    invalidated_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    revoked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_count: int = Field(default=0, ge=0)
    last_error_code: str | None = Field(default=None, max_length=64)


class SyncChange(SQLModel, table=True):
    """Immutable owner-scoped aggregate envelope ordered by a database-generated cursor."""

    __tablename__ = "sync_changes"
    __table_args__ = (
        UniqueConstraint("cursor", name="uq_sync_changes_cursor"),
        UniqueConstraint(
            "owner_user_id",
            "aggregate_type",
            "aggregate_id",
            "aggregate_version",
            name="uq_sync_changes_owner_aggregate_version",
        ),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_sync_changes_aggregate_version_positive",
        ),
        CheckConstraint(
            "schema_version > 0",
            name="ck_sync_changes_schema_version_positive",
        ),
        CheckConstraint(
            "(operation = 'UPSERT' AND payload IS NOT NULL) "
            "OR (operation = 'DELETE' AND payload IS NULL)",
            name="ck_sync_changes_payload_matches_operation",
        ),
        Index("ix_sync_changes_owner_cursor", "owner_user_id", "cursor"),
        Index(
            "ix_sync_changes_owner_aggregate_version",
            "owner_user_id",
            "aggregate_type",
            "aggregate_id",
            "aggregate_version",
        ),
        Index("ix_sync_changes_created_at", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    cursor: int | None = Field(
        default=None,
        sa_column=Column(BigInteger, Identity(start=1), nullable=False),
    )
    owner_user_id: UUID = Field(
        foreign_key="users.id",
        ondelete="CASCADE",
        index=True,
    )
    aggregate_type: SyncAggregateType = Field(
        sa_column=Column(
            SAEnum(
                SyncAggregateType,
                name="syncaggregatetype",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        )
    )
    aggregate_id: UUID = Field(index=True)
    aggregate_version: int = Field(sa_column=Column(BigInteger, nullable=False))
    operation: SyncOperation = Field(
        sa_column=Column(
            SAEnum(
                SyncOperation,
                name="syncoperation",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        )
    )
    schema_version: int = Field(default=1, ge=1)
    payload: dict[str, Any] | None = Field(
        default=None,
        sa_column=Column(JSONB(none_as_null=True), nullable=True),
    )
    created_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )


class SubscriptionAccount(TimestampMixin, table=True):
    __tablename__ = "subscription_accounts"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_subscription_accounts_user_id"),
        Index("ix_subscription_accounts_status_period_end", "status", "current_period_end"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    plan_code: PlanCode = Field(
        default=PlanCode.FREE,
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True),
    )
    status: SubscriptionStatus = Field(
        default=SubscriptionStatus.INACTIVE,
        sa_column=Column(SAEnum(SubscriptionStatus, native_enum=False), nullable=False, index=True),
    )
    billing_provider: str | None = Field(default=None, max_length=32)
    effective_store: BillingStore | None = Field(
        default=None,
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=True, index=True),
    )
    billing_interval: BillingInterval | None = Field(
        default=None,
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=True),
    )
    entitlement_started_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    entitlement_expires_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    provider_customer_id: str | None = Field(default=None, max_length=255, index=True)
    provider_subscription_id: str | None = Field(default=None, max_length=255, index=True)
    current_period_start: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    current_period_end: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    cancel_at_period_end: bool = Field(default=False)
    will_renew: bool = Field(default=False)
    billing_issue: bool = Field(default=False)
    multiple_active_subscriptions: bool = Field(default=False)
    last_synced_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    management_url_encrypted: str | None = None
    provider_event_version: str | None = Field(default=None, max_length=255)


class BillingProduct(TimestampMixin, table=True):
    __tablename__ = "billing_products"
    __table_args__ = (
        Index(
            "uq_billing_products_store_product_base_plan",
            "store",
            "external_product_id",
            text("COALESCE(external_base_plan_id, '')"),
            unique=True,
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    plan_code: PlanCode = Field(
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True)
    )
    billing_interval: BillingInterval = Field(
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=False, index=True)
    )
    store: BillingStore = Field(
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=False, index=True)
    )
    external_product_id: str = Field(max_length=255, index=True)
    external_base_plan_id: str | None = Field(default=None, max_length=255)
    revenuecat_entitlement_id: str = Field(max_length=64, index=True)
    revenuecat_package_id: str = Field(max_length=64, index=True)
    active: bool = Field(default=True, index=True)
    metadata_json: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column("metadata", JSONB, nullable=False),
    )


class BillingSubscription(TimestampMixin, table=True):
    __tablename__ = "billing_subscriptions"
    __table_args__ = (
        UniqueConstraint(
            "provider", "external_key", name="uq_billing_subscriptions_provider_external_key"
        ),
        Index(
            "ix_billing_subscriptions_user_status_end", "user_id", "status", "current_period_end"
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    provider: BillingProvider = Field(
        default=BillingProvider.REVENUECAT,
        sa_column=Column(SAEnum(BillingProvider, native_enum=False), nullable=False, index=True),
    )
    store: BillingStore = Field(
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=False, index=True)
    )
    environment: str = Field(default="production", max_length=32, index=True)
    external_key: str = Field(min_length=1, max_length=512)
    external_product_id: str = Field(max_length=255, index=True)
    external_transaction_id: str | None = Field(default=None, max_length=255)
    external_original_transaction_id: str | None = Field(default=None, max_length=255)
    external_subscription_id: str | None = Field(default=None, max_length=255)
    revenuecat_entitlement_id: str | None = Field(default=None, max_length=64, index=True)
    plan_code: PlanCode = Field(
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True)
    )
    billing_interval: BillingInterval = Field(
        default=BillingInterval.UNKNOWN,
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=False),
    )
    status: BillingSubscriptionStatus = Field(
        default=BillingSubscriptionStatus.UNKNOWN,
        sa_column=Column(
            SAEnum(BillingSubscriptionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    current_period_start: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    current_period_end: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    grace_period_end: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    will_renew: bool = Field(default=False)
    cancel_at_period_end: bool = Field(default=False)
    billing_issue_detected_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_provider_event_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_synced_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    metadata_json: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column("metadata", JSONB, nullable=False)
    )


class BillingEvent(TimestampMixin, table=True):
    __tablename__ = "billing_events"
    __table_args__ = (
        UniqueConstraint(
            "provider", "provider_event_id", name="uq_billing_events_provider_event_id"
        ),
        Index("ix_billing_events_status_received", "status", "received_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    provider: BillingProvider = Field(
        default=BillingProvider.REVENUECAT,
        sa_column=Column(SAEnum(BillingProvider, native_enum=False), nullable=False, index=True),
    )
    provider_event_id: str = Field(max_length=255)
    event_type: str = Field(max_length=128, index=True)
    app_user_id: str | None = Field(
        default=None, sa_column=Column(Text(), nullable=True, index=True)
    )
    environment: str | None = Field(default=None, max_length=32)
    payload_hash: str = Field(min_length=64, max_length=64)
    status: BillingEventStatus = Field(
        default=BillingEventStatus.RECEIVED,
        sa_column=Column(SAEnum(BillingEventStatus, native_enum=False), nullable=False, index=True),
    )
    attempt_count: int = Field(default=0, ge=0)
    received_at: datetime = Field(
        default_factory=utc_now, sa_column=Column(DateTime(timezone=True), nullable=False)
    )
    queued_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processed_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class Opportunity(TimestampMixin, table=True):
    __tablename__ = "opportunities"
    __table_args__ = (
        Index("ix_opportunities_channel_conversation", "channel", "conversation_id"),
        Index("ix_opportunities_status_created", "status", "created_at"),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_opportunities_aggregate_version_positive",
        ),
        Index(
            "ix_opportunities_owner_archived_last_message",
            "owner_user_id",
            "archived_at",
            "last_message_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    aggregate_version: int = Field(
        default=1,
        ge=1,
        sa_column=Column(BigInteger, nullable=False, server_default="1"),
    )
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    conversation_id: str = Field(index=True)
    customer_external_id: str | None = Field(default=None, index=True)
    contact_name: str = Field(default="未知联系人")
    contact_avatar: str = Field(default="")
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None

    source_message_id: UUID | None = Field(default=None, index=True, unique=True)
    title: str
    summary: str | None = None
    matched_keywords: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    raw_message_links: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    trust_score: int = Field(default=70, ge=0, le=100)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    priority: Priority = Field(
        default=Priority.NORMAL,
        sa_column=Column(SAEnum(Priority, native_enum=False), nullable=False, index=True),
    )
    status: OpportunityStatus = Field(
        default=OpportunityStatus.PENDING_HUMAN,
        sa_column=Column(SAEnum(OpportunityStatus, native_enum=False), nullable=False, index=True),
    )
    detection_reason: str | None = None

    link_verification: dict[str, Any] = Field(
        default_factory=lambda: {
            "status": "unverified",
            "verifiedAt": None,
            "riskReasons": [],
            "resolvedInfo": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    extracted_contacts: dict[str, Any] = Field(
        default_factory=lambda: {
            "phone": None,
            "email": None,
            "telegramHandle": None,
            "wecomId": None,
            "extractionSource": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    friend_request_status: str = Field(default="n/a")
    sop_stage: str = Field(default="detected")
    agent_actions: list[dict[str, Any]] = Field(
        default_factory=list,
        sa_column=Column(JSONB, nullable=False),
    )
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(
            SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True
        ),
    )
    agent_analysis_error: str | None = None
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    attention_required: bool = Field(default=False, index=True)

    ai_reply_draft: str | None = None
    final_reply: str | None = None
    assigned_to: str | None = None
    follow_up_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_message_preview: str = Field(default="")
    last_message_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    archived_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    archived_by_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    archive_reason: str | None = Field(default=None, max_length=500)


class SourceFunctionalProfile(TimestampMixin, table=True):
    __tablename__ = "source_functional_profiles"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "channel",
            "external_source_id",
            name="uq_source_functional_profiles_owner_source",
        ),
        Index("ix_source_functional_profiles_owner_function", "owner_user_id", "primary_function"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    external_source_id: str = Field(max_length=255, index=True)
    source_display_name: str = Field(default="", max_length=500)
    source_description: str | None = Field(default=None, max_length=2000)
    source_username: str | None = Field(default=None, max_length=255)
    primary_function: SourcePrimaryFunction = Field(
        default=SourcePrimaryFunction.UNKNOWN,
        sa_column=Column(
            SAEnum(SourcePrimaryFunction, native_enum=False), nullable=False, index=True
        ),
    )
    secondary_functions: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    industry_tags: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    region_tags: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    language_tags: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    job_signal_prior: float = Field(default=0.5, ge=0.0, le=1.0)
    estimated_noise_level: float = Field(default=0.5, ge=0.0, le=1.0)
    reliability_score: float = Field(default=0.5, ge=0.0, le=1.0)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    evidence: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    manual_override: SourcePrimaryFunction | None = Field(
        default=None,
        sa_column=Column(SAEnum(SourcePrimaryFunction, native_enum=False), nullable=True),
    )
    sampled_message_count: int = Field(default=0, ge=0)
    source_fingerprint: str = Field(default="", max_length=64)
    profiled_at: datetime = Field(
        default_factory=utc_now, sa_column=Column(DateTime(timezone=True), nullable=False)
    )
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )


class JobMessageAudit(TimestampMixin, table=True):
    __tablename__ = "job_message_audits"
    __table_args__ = (
        UniqueConstraint("message_id", name="uq_job_message_audits_message"),
        Index("ix_job_message_audits_owner_classification", "owner_user_id", "classification"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    message_id: UUID = Field(foreign_key="messages.id", index=True)
    source_profile_id: UUID | None = Field(
        default=None, foreign_key="source_functional_profiles.id", index=True
    )
    classification: JobMessageClassification = Field(
        default=JobMessageClassification.UNKNOWN,
        sa_column=Column(
            SAEnum(JobMessageClassification, native_enum=False), nullable=False, index=True
        ),
    )
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    filter_reason: str | None = Field(default=None, max_length=1000)
    prefilter_score: float = Field(default=0.0, ge=0.0, le=1.0)
    agent_required: bool = Field(default=False)
    manually_corrected: bool = Field(default=False)


class JobOpportunityDetail(TimestampMixin, table=True):
    __tablename__ = "job_opportunity_details"
    __table_args__ = (
        Index(
            "ix_job_details_company_title_location", "company_name", "normalized_job_title", "city"
        ),
        Index("ix_job_details_posted_at", "posted_at"),
        Index("ix_job_details_duplicate_group", "duplicate_group_id"),
    )

    opportunity_id: UUID = Field(primary_key=True, foreign_key="opportunities.id")
    source_channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    source_chat_id: str = Field(max_length=255, index=True)
    source_chat_name: str | None = Field(default=None, max_length=500)
    source_message_id: str = Field(max_length=255, index=True)
    source_message_url: str | None = Field(default=None, max_length=2000)
    source_author_name: str | None = Field(default=None, max_length=500)
    source_author_username: str | None = Field(default=None, max_length=255)
    source_reliability_score: float = Field(default=0.5, ge=0.0, le=1.0)
    posted_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    captured_at: datetime = Field(
        default_factory=utc_now, sa_column=Column(DateTime(timezone=True), nullable=False)
    )
    job_title: str = Field(max_length=500)
    normalized_job_title: str | None = Field(default=None, max_length=500, index=True)
    company_name: str | None = Field(default=None, max_length=500, index=True)
    department: str | None = Field(default=None, max_length=500)
    company_industry: str | None = Field(default=None, max_length=255)
    company_stage: str | None = Field(default=None, max_length=255)
    location_text: str | None = Field(default=None, max_length=500)
    country_code: str | None = Field(default=None, max_length=2, index=True)
    city: str | None = Field(default=None, max_length=255, index=True)
    timezone: str | None = Field(default=None, max_length=100)
    work_mode: JobWorkMode = Field(
        default=JobWorkMode.UNKNOWN,
        sa_column=Column(SAEnum(JobWorkMode, native_enum=False), nullable=False, index=True),
    )
    employment_type: JobEmploymentType = Field(
        default=JobEmploymentType.UNKNOWN,
        sa_column=Column(SAEnum(JobEmploymentType, native_enum=False), nullable=False, index=True),
    )
    seniority: JobSeniority = Field(
        default=JobSeniority.UNKNOWN,
        sa_column=Column(SAEnum(JobSeniority, native_enum=False), nullable=False, index=True),
    )
    salary_raw: str | None = Field(default=None, max_length=500)
    salary_min: Decimal | None = Field(
        default=None, ge=0, sa_column=Column(Numeric(18, 2), nullable=True)
    )
    salary_max: Decimal | None = Field(
        default=None, ge=0, sa_column=Column(Numeric(18, 2), nullable=True)
    )
    salary_currency: str | None = Field(default=None, max_length=3, index=True)
    salary_period: SalaryPeriod = Field(
        default=SalaryPeriod.UNKNOWN,
        sa_column=Column(SAEnum(SalaryPeriod, native_enum=False), nullable=False),
    )
    salary_negotiable: bool | None = None
    equity_mentioned: bool | None = None
    requirements_summary: str | None = Field(default=None, max_length=4000)
    required_skills: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_skills: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    minimum_years_experience: float | None = Field(default=None, ge=0)
    maximum_years_experience: float | None = Field(default=None, ge=0)
    degree_required: bool | None = None
    degree_level: str | None = Field(default=None, max_length=100, index=True)
    degree_field: str | None = Field(default=None, max_length=255)
    english_level: str | None = Field(default=None, max_length=100, index=True)
    other_language_requirements: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    visa_sponsorship: bool | None = Field(default=None, index=True)
    work_authorization_text: str | None = Field(default=None, max_length=1000)
    relocation_support: bool | None = None
    age_requirement_text: str | None = Field(default=None, max_length=500)
    age_requirement_present: bool = Field(default=False, index=True)
    application_deadline: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    application_url: str | None = Field(default=None, max_length=2000)
    contact_methods: list[dict[str, Any]] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    compliance_flags: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    extraction_confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    missing_fields: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    field_evidence: dict[str, str] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )
    raw_excerpt: str = Field(default="", max_length=4000)
    content_fingerprint: str = Field(max_length=64, index=True)
    duplicate_group_id: UUID | None = Field(default=None, index=True)
    conflicting_source_data: bool = Field(default=False)
    is_expired: bool = Field(default=False, index=True)
    expired_reason: str | None = Field(default=None, max_length=500)


class JobOpportunitySource(TimestampMixin, table=True):
    __tablename__ = "job_opportunity_sources"
    __table_args__ = (
        UniqueConstraint("opportunity_id", "message_id", name="uq_job_sources_opportunity_message"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    message_id: UUID = Field(foreign_key="messages.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    source_channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    source_message_url: str | None = Field(default=None, max_length=2000)
    source_chat_name: str | None = Field(default=None, max_length=500)
    source_author_name: str | None = Field(default=None, max_length=500)
    posted_at: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    source_reliability_score: float = Field(default=0.5, ge=0.0, le=1.0)


class JobSearchProfile(TimestampMixin, table=True):
    __tablename__ = "job_search_profiles"
    __table_args__ = (
        Index("ix_job_search_profiles_user_enabled", "user_id", "enabled"),
        Index(
            "uq_job_search_profiles_default",
            "user_id",
            unique=True,
            postgresql_where=text("is_default"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    name: str = Field(max_length=120)
    is_default: bool = Field(default=False)
    enabled: bool = Field(default=True, index=True)
    target_roles: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    excluded_roles: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    target_industries: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_seniority: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    candidate_skills: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    years_experience: float | None = Field(default=None, ge=0)
    education_level: str | None = Field(default=None, max_length=100)
    english_level: str | None = Field(default=None, max_length=100)
    other_languages: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_countries: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_cities: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_timezones: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    work_modes: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    employment_types: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    minimum_salary: Decimal | None = Field(
        default=None, ge=0, sa_column=Column(Numeric(18, 2), nullable=True)
    )
    salary_currency: str | None = Field(default=None, max_length=3)
    salary_period: SalaryPeriod | None = Field(
        default=None, sa_column=Column(SAEnum(SalaryPeriod, native_enum=False), nullable=True)
    )
    visa_sponsorship_required: bool | None = None
    relocation_acceptable: bool | None = None
    required_keywords: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    preferred_keywords: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    excluded_keywords: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    require_salary_disclosed: bool = Field(default=False)
    minimum_match_score: int = Field(default=0, ge=0, le=100)
    notification_enabled: bool = Field(default=False)


class JobOpportunityMatch(TimestampMixin, table=True):
    __tablename__ = "job_opportunity_matches"
    __table_args__ = (
        UniqueConstraint(
            "opportunity_id", "job_search_profile_id", name="uq_job_matches_opportunity_profile"
        ),
        Index("ix_job_matches_profile_score", "job_search_profile_id", "match_score"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    job_search_profile_id: UUID = Field(foreign_key="job_search_profiles.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    eligibility: JobEligibility = Field(
        default=JobEligibility.UNKNOWN,
        sa_column=Column(SAEnum(JobEligibility, native_enum=False), nullable=False, index=True),
    )
    match_score: int = Field(default=0, ge=0, le=100, index=True)
    matched_reasons: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    mismatch_reasons: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    unknown_constraints: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    score_breakdown: dict[str, int] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )


class JobOpportunityFeedback(TimestampMixin, table=True):
    __tablename__ = "job_opportunity_feedback"
    __table_args__ = (
        UniqueConstraint(
            "opportunity_id", "owner_user_id", name="uq_job_feedback_owner_opportunity"
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    feedback_type: JobFeedbackType = Field(
        sa_column=Column(SAEnum(JobFeedbackType, native_enum=False), nullable=False, index=True)
    )
    note: str | None = Field(default=None, max_length=1000)


class OpportunityArchiveEvent(TimestampMixin, table=True):
    __tablename__ = "opportunity_archive_events"
    __table_args__ = (
        Index("ix_opportunity_archive_events_owner_created", "owner_user_id", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    action: OpportunityArchiveAction = Field(
        sa_column=Column(
            SAEnum(OpportunityArchiveAction, native_enum=False), nullable=False, index=True
        )
    )
    reason: str | None = Field(default=None, max_length=500)


class InternalCommandReceipt(TimestampMixin, table=True):
    __tablename__ = "internal_command_receipts"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_internal_command_receipts_owner_idempotency",
        ),
        CheckConstraint(
            "command_type = 'opportunity_status'",
            name="ck_internal_command_receipts_supported_type",
        ),
        CheckConstraint(
            "expected_version > 0",
            name="ck_internal_command_receipts_expected_version_positive",
        ),
        CheckConstraint(
            "char_length(payload_hash) = 64",
            name="ck_internal_command_receipts_payload_hash_length",
        ),
        Index(
            "ix_internal_command_receipts_owner_created",
            "owner_user_id",
            "created_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(
        foreign_key="users.id",
        ondelete="CASCADE",
        index=True,
    )
    opportunity_id: UUID = Field(
        foreign_key="opportunities.id",
        ondelete="CASCADE",
        index=True,
    )
    idempotency_key: str = Field(min_length=8, max_length=128)
    command_type: str = Field(default="opportunity_status", max_length=64)
    expected_version: int = Field(
        ge=1,
        sa_column=Column(BigInteger, nullable=False),
    )
    payload_hash: str = Field(min_length=64, max_length=64)
    expires_at: datetime = Field(
        default_factory=lambda: utc_now() + timedelta(days=30),
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )


class ManualReplyDelivery(TimestampMixin, table=True):
    __tablename__ = "manual_reply_deliveries"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_manual_reply_deliveries_owner_idempotency",
        ),
        Index("ix_manual_reply_deliveries_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    idempotency_key: str = Field(max_length=128)
    content_hash: str = Field(min_length=64, max_length=64)
    status: ManualReplyDeliveryStatus = Field(
        default=ManualReplyDeliveryStatus.PENDING,
        sa_column=Column(
            SAEnum(ManualReplyDeliveryStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    provider_message_id: str | None = Field(default=None, max_length=255)
    attempt_count: int = Field(default=0, ge=0)
    delivered_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error_class: str | None = Field(default=None, max_length=255)


class Message(TimestampMixin, table=True):
    __tablename__ = "messages"
    __table_args__ = (
        UniqueConstraint("channel", "external_message_id", name="uq_message_channel_external"),
        UniqueConstraint("owner_user_id", "id", name="uq_messages_owner_id"),
        Index("ix_messages_conversation_created", "conversation_id", "created_at"),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_messages_aggregate_version_positive",
        ),
        CheckConstraint(
            "agent_execution IS NULL OR (jsonb_typeof(agent_execution) = 'object' "
            "AND octet_length(agent_execution::text) <= 4096)",
            name="ck_messages_agent_execution_bounded_object",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    aggregate_version: int = Field(
        default=1,
        ge=1,
        sa_column=Column(BigInteger, nullable=False, server_default="1"),
    )
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    external_message_id: str = Field(index=True)
    conversation_id: str = Field(index=True)
    sender_external_id: str | None = Field(default=None, index=True)
    sender_display_name: str | None = None
    direction: MessageDirection = Field(
        sa_column=Column(SAEnum(MessageDirection, native_enum=False), nullable=False, index=True)
    )
    source: MessageSource | None = Field(
        default=None,
        sa_column=Column(SAEnum(MessageSource, native_enum=False), nullable=True),
    )
    text: str | None = None
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None
    raw_message_links: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    raw_payload: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )
    opportunity_id: UUID | None = Field(default=None, foreign_key="opportunities.id", index=True)
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(
            SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True
        ),
    )
    agent_result: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )
    agent_execution: dict[str, Any] | None = Field(
        default=None,
        sa_column=Column(JSONB(none_as_null=True), nullable=True),
    )
    agent_error: str | None = None
    agent_started_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    sent_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class UsageLedger(TimestampMixin, table=True):
    __tablename__ = "usage_ledger"
    __table_args__ = (
        UniqueConstraint(
            "user_id",
            "feature",
            "idempotency_key",
            name="uq_usage_ledger_user_feature_idempotency",
        ),
        UniqueConstraint("user_id", "id", name="uq_usage_ledger_user_id"),
        CheckConstraint("quantity > 0", name="ck_usage_ledger_quantity_positive"),
        Index(
            "ix_usage_ledger_user_feature_period_status",
            "user_id",
            "feature",
            "period_start",
            "period_end",
            "status",
        ),
        Index(
            "uq_usage_ledger_message_reserved_agent",
            "source_message_id",
            unique=True,
            postgresql_where=text(
                "feature = 'PI_AGENT_ANALYSIS' AND status = 'RESERVED' "
                "AND source_message_id IS NOT NULL"
            ),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    feature: UsageFeature = Field(
        sa_column=Column(SAEnum(UsageFeature, native_enum=False), nullable=False, index=True)
    )
    quantity: int = Field(default=1, ge=1)
    period_start: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    period_end: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    idempotency_key: str = Field(max_length=255)
    source_message_id: UUID | None = Field(default=None, foreign_key="messages.id", index=True)
    status: UsageStatus = Field(
        default=UsageStatus.RESERVED,
        sa_column=Column(SAEnum(UsageStatus, native_enum=False), nullable=False, index=True),
    )
    consumed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    released_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_reason: str | None = Field(default=None, max_length=500)


class AnalysisRun(TimestampMixin, table=True):
    """Owner- and device-bound lease for one top-level message analysis."""

    __tablename__ = "analysis_runs"
    __table_args__ = (
        UniqueConstraint("owner_user_id", "id", name="uq_analysis_runs_owner_id"),
        UniqueConstraint("usage_ledger_id", name="uq_analysis_runs_usage_ledger"),
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_analysis_runs_owner_device",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "message_id"],
            ["messages.owner_user_id", "messages.id"],
            name="fk_analysis_runs_owner_message",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "usage_ledger_id"],
            ["usage_ledger.user_id", "usage_ledger.id"],
            name="fk_analysis_runs_owner_usage_ledger",
        ),
        CheckConstraint(
            "source_message_version > 0 AND schema_version > 0 AND lock_version > 0",
            name="ck_analysis_runs_positive_versions",
        ),
        CheckConstraint(
            "token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_analysis_runs_nonce_hash_sha256",
        ),
        CheckConstraint(
            "lease_expires_at > claimed_at",
            name="ck_analysis_runs_lease_after_claim",
        ),
        CheckConstraint(
            "result IS NULL OR (jsonb_typeof(result) = 'object' "
            "AND octet_length(result::text) <= 65536)",
            name="ck_analysis_runs_result_bounded_object",
        ),
        CheckConstraint(
            "(link_evidence IS NULL AND link_evidence_fetched_at IS NULL) OR "
            "(jsonb_typeof(link_evidence) = 'array' "
            "AND octet_length(link_evidence::text) <= 262144 "
            "AND link_evidence_fetched_at IS NOT NULL)",
            name="ck_analysis_runs_link_evidence_bounded_array",
        ),
        CheckConstraint(
            "(mode = 'PRIMARY' AND shadow_match IS NULL "
            "AND shadow_difference_count IS NULL) OR "
            "(mode = 'SHADOW' AND status != 'COMPLETED' "
            "AND shadow_match IS NULL AND shadow_difference_count IS NULL) OR "
            "(mode = 'SHADOW' AND status = 'COMPLETED' "
            "AND shadow_match IS NOT NULL AND shadow_difference_count IS NOT NULL "
            "AND shadow_difference_count >= 0)",
            name="ck_analysis_runs_shadow_observation",
        ),
        CheckConstraint(
            "(status IN ('CLAIMED', 'RUNNING') AND completed_at IS NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL "
            "AND result IS NULL) "
            "OR (status = 'COMPLETED' AND completed_at IS NOT NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL "
            "AND result IS NOT NULL) "
            "OR (status = 'FAILED' AND failed_at IS NOT NULL "
            "AND completed_at IS NULL AND expired_at IS NULL AND failure_code IS NOT NULL "
            "AND result IS NULL) "
            "OR (status = 'EXPIRED' AND expired_at IS NOT NULL "
            "AND completed_at IS NULL AND failed_at IS NULL AND failure_code IS NULL "
            "AND result IS NULL)",
            name="ck_analysis_runs_lifecycle_state",
        ),
        Index(
            "ix_analysis_runs_owner_status_lease",
            "owner_user_id",
            "status",
            "lease_expires_at",
        ),
        Index(
            "ix_analysis_runs_mode_status_claimed",
            "mode",
            "status",
            "claimed_at",
        ),
        Index(
            "uq_analysis_runs_message_active",
            "message_id",
            unique=True,
            postgresql_where=text("status IN ('CLAIMED', 'RUNNING')"),
        ),
        Index(
            "uq_analysis_runs_message_shadow",
            "message_id",
            unique=True,
            postgresql_where=text("mode = 'SHADOW'"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    message_id: UUID = Field(index=True)
    device_id: UUID = Field(index=True)
    usage_ledger_id: UUID = Field(index=True)
    status: AnalysisRunStatus = Field(
        default=AnalysisRunStatus.CLAIMED,
        sa_column=Column(
            SAEnum(
                AnalysisRunStatus,
                name="analysisrunstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    executor: AnalysisRunExecutor = Field(
        default=AnalysisRunExecutor.DEVICE,
        sa_column=Column(
            SAEnum(
                AnalysisRunExecutor,
                name="analysisrunexecutor",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
    )
    mode: AnalysisRunMode = Field(
        default=AnalysisRunMode.PRIMARY,
        sa_column=Column(
            SAEnum(
                AnalysisRunMode,
                name="analysisrunmode",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
    )
    runtime_version: str = Field(min_length=1, max_length=64)
    schema_version: int = Field(default=1, ge=1)
    model_alias: str = Field(min_length=1, max_length=64)
    policy_version: str = Field(min_length=1, max_length=64)
    source_message_version: int = Field(
        ge=1,
        sa_column=Column(BigInteger, nullable=False),
    )
    lock_version: int = Field(default=1, ge=1)
    token_nonce_hash: str = Field(min_length=64, max_length=64)
    lease_expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    claimed_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    heartbeat_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    expired_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_code: str | None = Field(default=None, max_length=64)
    result: dict[str, Any] | None = Field(
        default=None,
        sa_column=Column(JSONB(none_as_null=True), nullable=True),
    )
    link_evidence: list[dict[str, Any]] | None = Field(
        default=None,
        sa_column=Column(JSONB(none_as_null=True), nullable=True),
    )
    link_evidence_fetched_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    shadow_match: bool | None = None
    shadow_difference_count: int | None = Field(default=None, ge=0)


class AnalysisProviderRequest(TimestampMixin, table=True):
    """Content-free audit record for one run-bound provider request."""

    __tablename__ = "analysis_provider_requests"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_user_id", "run_id"],
            ["analysis_runs.owner_user_id", "analysis_runs.id"],
            name="fk_analysis_provider_requests_owner_run",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_analysis_provider_requests_owner_device",
        ),
        CheckConstraint(
            "prompt_tokens IS NULL OR prompt_tokens >= 0",
            name="ck_analysis_provider_requests_prompt_tokens_nonnegative",
        ),
        CheckConstraint(
            "completion_tokens IS NULL OR completion_tokens >= 0",
            name="ck_analysis_provider_requests_completion_tokens_nonnegative",
        ),
        CheckConstraint(
            "total_tokens IS NULL OR total_tokens >= 0",
            name="ck_analysis_provider_requests_total_tokens_nonnegative",
        ),
        CheckConstraint(
            "estimated_cost_micros IS NULL OR estimated_cost_micros >= 0",
            name="ck_analysis_provider_requests_cost_nonnegative",
        ),
        CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_analysis_provider_requests_latency_nonnegative",
        ),
        CheckConstraint(
            "provider_request_id_hash IS NULL OR provider_request_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_analysis_provider_requests_id_hash_sha256",
        ),
        CheckConstraint(
            "(status = 'STARTED' AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'CANCELLED') AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL)",
            name="ck_analysis_provider_requests_lifecycle_state",
        ),
        Index(
            "ix_analysis_provider_requests_owner_created",
            "owner_user_id",
            "created_at",
        ),
        Index(
            "uq_analysis_provider_requests_run_active",
            "run_id",
            unique=True,
            postgresql_where=text("status = 'STARTED'"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    run_id: UUID = Field(index=True)
    device_id: UUID = Field(index=True)
    status: AnalysisProviderRequestStatus = Field(
        default=AnalysisProviderRequestStatus.STARTED,
        sa_column=Column(
            SAEnum(
                AnalysisProviderRequestStatus,
                name="analysisproviderrequeststatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    provider: str = Field(min_length=1, max_length=32)
    provider_model: str = Field(min_length=1, max_length=128)
    model_alias: str = Field(min_length=1, max_length=64)
    provider_request_id_hash: str | None = Field(default=None, max_length=64)
    prompt_tokens: int | None = Field(default=None, ge=0)
    completion_tokens: int | None = Field(default=None, ge=0)
    total_tokens: int | None = Field(default=None, ge=0)
    estimated_cost_micros: int | None = Field(
        default=None,
        ge=0,
        sa_column=Column(BigInteger, nullable=True),
    )
    latency_ms: int | None = Field(
        default=None,
        ge=0,
        sa_column=Column(BigInteger, nullable=True),
    )
    failure_code: str | None = Field(default=None, max_length=64)
    started_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    finished_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class InteractiveAgentTurn(TimestampMixin, table=True):
    """Content-free server lease for one local interactive Agent user turn."""

    __tablename__ = "interactive_agent_turns"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "id",
            name="uq_interactive_agent_turns_owner_id",
        ),
        UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_interactive_agent_turns_owner_idempotency",
        ),
        UniqueConstraint(
            "usage_ledger_id",
            name="uq_interactive_agent_turns_usage_ledger",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_interactive_agent_turns_owner_device",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "usage_ledger_id"],
            ["usage_ledger.user_id", "usage_ledger.id"],
            name="fk_interactive_agent_turns_owner_usage_ledger",
        ),
        CheckConstraint(
            "schema_version > 0 AND lock_version > 0 AND request_count >= 0",
            name="ck_interactive_agent_turns_versions_and_count",
        ),
        CheckConstraint(
            "token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_interactive_agent_turns_nonce_hash_sha256",
        ),
        CheckConstraint(
            "lease_expires_at > claimed_at",
            name="ck_interactive_agent_turns_lease_after_claim",
        ),
        CheckConstraint(
            "(status IN ('CLAIMED', 'RUNNING') AND completed_at IS NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND completed_at IS NOT NULL "
            "AND failed_at IS NULL AND expired_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'FAILED' AND failed_at IS NOT NULL "
            "AND completed_at IS NULL AND expired_at IS NULL AND failure_code IS NOT NULL) "
            "OR (status = 'EXPIRED' AND expired_at IS NOT NULL "
            "AND completed_at IS NULL AND failed_at IS NULL AND failure_code IS NULL)",
            name="ck_interactive_agent_turns_lifecycle_state",
        ),
        Index(
            "ix_interactive_agent_turns_owner_status_lease",
            "owner_user_id",
            "status",
            "lease_expires_at",
        ),
        Index(
            "uq_interactive_agent_turns_session_active",
            "owner_user_id",
            "local_session_id",
            unique=True,
            postgresql_where=text("status IN ('CLAIMED', 'RUNNING')"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    device_id: UUID = Field(index=True)
    local_session_id: UUID = Field(index=True)
    idempotency_key: str = Field(min_length=1, max_length=128)
    usage_ledger_id: UUID = Field(index=True)
    status: InteractiveAgentTurnStatus = Field(
        default=InteractiveAgentTurnStatus.CLAIMED,
        sa_column=Column(
            SAEnum(
                InteractiveAgentTurnStatus,
                name="interactiveagentturnstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    runtime_version: str = Field(min_length=1, max_length=64)
    schema_version: int = Field(default=1, ge=1)
    model_alias: str = Field(min_length=1, max_length=64)
    policy_version: str = Field(min_length=1, max_length=64)
    lock_version: int = Field(default=1, ge=1)
    request_count: int = Field(default=0, ge=0)
    token_nonce_hash: str = Field(min_length=64, max_length=64)
    lease_expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    claimed_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    heartbeat_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    expired_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_code: str | None = Field(default=None, max_length=64)


class InteractiveAgentActionApproval(TimestampMixin, table=True):
    """Content-free proof of one explicit decision for one proposed external action."""

    __tablename__ = "interactive_agent_action_approvals"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "turn_id",
            "tool_call_id",
            name="uq_iaaa_owner_turn_tool_call",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_iaaa_owner_device",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "turn_id"],
            ["interactive_agent_turns.owner_user_id", "interactive_agent_turns.id"],
            name="fk_iaaa_owner_turn",
        ),
        CheckConstraint(
            "expected_version > 0",
            name="ck_iaaa_expected_version_positive",
        ),
        CheckConstraint(
            "arguments_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iaaa_arguments_hash_sha256",
        ),
        CheckConstraint(
            "token_nonce_hash IS NULL OR token_nonce_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iaaa_nonce_hash_sha256",
        ),
        CheckConstraint(
            "(status = 'DENIED' AND token_nonce_hash IS NULL AND expires_at IS NULL "
            "AND execution_started_at IS NULL AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'GRANTED' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NULL "
            "AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'EXECUTING' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NOT NULL "
            "AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'CONSUMED' AND token_nonce_hash IS NOT NULL "
            "AND execution_started_at IS NOT NULL AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'UNCERTAIN') AND token_nonce_hash IS NOT NULL "
            "AND execution_started_at IS NOT NULL AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL) "
            "OR (status = 'EXPIRED' AND token_nonce_hash IS NOT NULL "
            "AND expires_at > decided_at AND execution_started_at IS NULL "
            "AND finished_at IS NOT NULL AND failure_code IS NULL)",
            name="ck_iaaa_lifecycle_state",
        ),
        Index(
            "ix_iaaa_owner_status_expires",
            "owner_user_id",
            "status",
            "expires_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    device_id: UUID = Field(index=True)
    turn_id: UUID = Field(index=True)
    tool_call_id: str = Field(min_length=1, max_length=128)
    tool_name: str = Field(default="send_reply", min_length=1, max_length=64)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    expected_version: int = Field(ge=1, sa_column=Column(BigInteger, nullable=False))
    idempotency_key: str = Field(min_length=8, max_length=128)
    arguments_hash: str = Field(min_length=64, max_length=64)
    status: InteractiveAgentApprovalStatus = Field(
        sa_column=Column(
            SAEnum(
                InteractiveAgentApprovalStatus,
                name="interactiveagentapprovalstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        )
    )
    token_nonce_hash: str | None = Field(default=None, min_length=64, max_length=64)
    decided_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    expires_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    execution_started_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    finished_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_code: str | None = Field(default=None, max_length=64)
    manual_reply_delivery_id: UUID | None = Field(
        default=None,
        foreign_key="manual_reply_deliveries.id",
        index=True,
    )


class InteractiveAgentProviderRequest(TimestampMixin, table=True):
    """Content-free provider billing and reliability audit for an interactive turn."""

    __tablename__ = "interactive_agent_provider_requests"
    __table_args__ = (
        ForeignKeyConstraint(
            ["owner_user_id", "turn_id"],
            ["interactive_agent_turns.owner_user_id", "interactive_agent_turns.id"],
            name="fk_interactive_agent_provider_requests_owner_turn",
        ),
        ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_interactive_agent_provider_requests_owner_device",
        ),
        UniqueConstraint(
            "turn_id",
            "request_sequence",
            name="uq_interactive_agent_provider_requests_turn_sequence",
        ),
        CheckConstraint(
            "request_sequence > 0",
            name="ck_iapr_sequence_positive",
        ),
        CheckConstraint(
            "prompt_tokens IS NULL OR prompt_tokens >= 0",
            name="ck_iapr_prompt_tokens_nonnegative",
        ),
        CheckConstraint(
            "completion_tokens IS NULL OR completion_tokens >= 0",
            name="ck_iapr_completion_tokens_nonnegative",
        ),
        CheckConstraint(
            "total_tokens IS NULL OR total_tokens >= 0",
            name="ck_iapr_total_tokens_nonnegative",
        ),
        CheckConstraint(
            "estimated_cost_micros IS NULL OR estimated_cost_micros >= 0",
            name="ck_iapr_cost_nonnegative",
        ),
        CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_iapr_latency_nonnegative",
        ),
        CheckConstraint(
            "provider_request_id_hash IS NULL OR provider_request_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_iapr_id_hash_sha256",
        ),
        CheckConstraint(
            "(status = 'STARTED' AND finished_at IS NULL AND failure_code IS NULL) "
            "OR (status = 'COMPLETED' AND finished_at IS NOT NULL "
            "AND failure_code IS NULL) "
            "OR (status IN ('FAILED', 'CANCELLED') AND finished_at IS NOT NULL "
            "AND failure_code IS NOT NULL)",
            name="ck_iapr_lifecycle_state",
        ),
        Index(
            "ix_interactive_agent_provider_requests_owner_created",
            "owner_user_id",
            "created_at",
        ),
        Index(
            "uq_interactive_agent_provider_requests_turn_active",
            "turn_id",
            unique=True,
            postgresql_where=text("status = 'STARTED'"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    turn_id: UUID = Field(index=True)
    device_id: UUID = Field(index=True)
    request_sequence: int = Field(ge=1)
    status: InteractiveAgentProviderRequestStatus = Field(
        default=InteractiveAgentProviderRequestStatus.STARTED,
        sa_column=Column(
            SAEnum(
                InteractiveAgentProviderRequestStatus,
                name="interactiveagentproviderrequeststatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
            index=True,
        ),
    )
    provider: str = Field(min_length=1, max_length=32)
    provider_model: str = Field(min_length=1, max_length=128)
    model_alias: str = Field(min_length=1, max_length=64)
    provider_request_id_hash: str | None = Field(default=None, max_length=64)
    prompt_tokens: int | None = Field(default=None, ge=0)
    completion_tokens: int | None = Field(default=None, ge=0)
    total_tokens: int | None = Field(default=None, ge=0)
    estimated_cost_micros: int | None = Field(
        default=None,
        ge=0,
        sa_column=Column(BigInteger, nullable=True),
    )
    latency_ms: int | None = Field(
        default=None,
        ge=0,
        sa_column=Column(BigInteger, nullable=True),
    )
    failure_code: str | None = Field(default=None, max_length=64)
    started_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )
    finished_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class Rule(TimestampMixin, table=True):
    __tablename__ = "rules"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    name: str = Field(index=True)
    enabled: bool = Field(default=True, index=True)
    priority: int = Field(default=100, index=True)
    rule_type: RuleType = Field(
        sa_column=Column(SAEnum(RuleType, native_enum=False), nullable=False, index=True)
    )
    pattern: str
    score: float = Field(default=0.5, ge=0.0, le=1.0)
    extra_data: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )


class AppConfig(SQLModel, table=True):
    __tablename__ = "app_configs"

    key: str = Field(primary_key=True)
    value: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    description: str | None = None
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )


class UserDetectionPreference(TimestampMixin, table=True):
    """用户级商机识别偏好：自定义关键词 + AI 语义识别开关（叠加在全局规则之上）。"""

    __tablename__ = "user_detection_preferences"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_user_detection_preferences_user_id"),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_user_detection_preferences_aggregate_version_positive",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    aggregate_version: int = Field(
        default=1,
        ge=1,
        sa_column=Column(BigInteger, nullable=False, server_default="1"),
    )
    user_id: UUID = Field(foreign_key="users.id", index=True)
    keywords: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    ai_semantics_enabled: bool = Field(default=True)


class UserWorkSchedule(TimestampMixin, table=True):
    """用户级工作时间：选中时段为人工审核，其余时段可 AI 自动回复；时区为 IANA 标识。"""

    __tablename__ = "user_work_schedules"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_user_work_schedules_user_id"),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_user_work_schedules_aggregate_version_positive",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    aggregate_version: int = Field(
        default=1,
        ge=1,
        sa_column=Column(BigInteger, nullable=False, server_default="1"),
    )
    user_id: UUID = Field(foreign_key="users.id", index=True)
    timezone: str = Field(default="Asia/Shanghai", max_length=64)
    # 每个元素 {"weekday": 1-7, "start": "HH:MM", "end": "HH:MM"}
    slots: list[dict[str, Any]] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    auto_reply_outside_hours: bool = Field(default=False)


class UserNotificationPreference(TimestampMixin, table=True):
    """用户级通知偏好；推送通道落地前仅持久化，不代表已生效。"""

    __tablename__ = "user_notification_preferences"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_user_notification_preferences_user_id"),
        CheckConstraint(
            "aggregate_version > 0",
            name="ck_user_notification_preferences_aggregate_version_positive",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    aggregate_version: int = Field(
        default=1,
        ge=1,
        sa_column=Column(BigInteger, nullable=False, server_default="1"),
    )
    user_id: UUID = Field(foreign_key="users.id", index=True)
    new_opportunity_enabled: bool = Field(default=True)
    ai_replied_enabled: bool = Field(default=True)
    daily_digest_enabled: bool = Field(default=False)
    urgent_only: bool = Field(default=False)


class TelegramUserConfig(TimestampMixin, table=True):
    __tablename__ = "telegram_user_configs"
    __table_args__ = (UniqueConstraint("user_id", name="uq_telegram_user_configs_user_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    enabled: bool = Field(default=False, index=True)
    api_id: int | None = Field(default=None)
    api_hash_encrypted: str | None = None
    session_encrypted: str | None = None
    retention_limit: int | None = Field(default=None, ge=0)
    retention_selected_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramMonitor(TimestampMixin, table=True):
    __tablename__ = "telegram_monitors"
    __table_args__ = (
        UniqueConstraint("user_id", "chat_id", name="uq_telegram_monitors_user_chat"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    telegram_config_id: UUID = Field(foreign_key="telegram_user_configs.id", index=True)
    enabled: bool = Field(default=True, index=True)
    name: str = Field(default="Telegram 群监控")
    chat_id: str = Field(index=True)
    chat_title: str | None = None
    backfill_limit: int = Field(default=30, ge=0, le=500)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = None


class TelegramConnection(TimestampMixin, table=True):
    """A user-owned Telegram identity or integration, never a plaintext secret store."""

    __tablename__ = "telegram_connections"
    __table_args__ = (
        UniqueConstraint(
            "provider_connection_id",
            name="uq_telegram_connections_provider_connection_id",
        ),
        Index(
            "ix_telegram_connections_owner_type_status",
            "owner_user_id",
            "connection_type",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionStatus = Field(
        default=TelegramConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    enabled: bool = Field(default=True, index=True)
    label: str = Field(default="Telegram 连接", max_length=255)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    provider_connection_id: str | None = Field(default=None, max_length=255, index=True)
    credential_encrypted: str | None = None
    connection_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    capabilities: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    last_error: str | None = Field(default=None, max_length=1000)
    last_checked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramSource(TimestampMixin, table=True):
    """A group, channel, or private conversation selected through a connection."""

    __tablename__ = "telegram_sources"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "external_chat_id",
            name="uq_telegram_sources_connection_chat",
        ),
        Index(
            "ix_telegram_sources_owner_enabled",
            "owner_user_id",
            "enabled",
            "quota_paused",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID = Field(foreign_key="telegram_connections.id", index=True)
    source_type: TelegramSourceType = Field(
        sa_column=Column(
            SAEnum(TelegramSourceType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    external_chat_id: str = Field(index=True, max_length=128)
    display_name: str = Field(default="Telegram 来源", max_length=255)
    username: str | None = Field(default=None, max_length=255)
    enabled: bool = Field(default=True, index=True)
    auto_reply_enabled: bool = Field(default=False, index=True)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = Field(default=None, max_length=1000)


class TelegramConnectionAttempt(TimestampMixin, table=True):
    """A short-lived, owner-bound handshake. The random token itself is never stored."""

    __tablename__ = "telegram_connection_attempts"
    __table_args__ = (
        UniqueConstraint("token_hash", name="uq_telegram_connection_attempts_token_hash"),
        UniqueConstraint(
            "group_request_id",
            name="uq_telegram_connection_attempts_group_request_id",
        ),
        UniqueConstraint(
            "channel_request_id",
            name="uq_telegram_connection_attempts_channel_request_id",
        ),
        Index(
            "ix_telegram_connection_attempts_owner_status_expires",
            "owner_user_id",
            "status",
            "expires_at",
        ),
        Index(
            "uq_telegram_connection_attempts_owner_pending_mtproto_qr",
            "owner_user_id",
            unique=True,
            postgresql_where=text("connection_type = 'MTPROTO_QR' AND status = 'PENDING'"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionAttemptStatus = Field(
        default=TelegramConnectionAttemptStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionAttemptStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    token_hash: str = Field(max_length=128)
    group_request_id: int | None = Field(default=None, index=True)
    channel_request_id: int | None = Field(default=None, index=True)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    attempt_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    # QR URLs are bearer login grants. Keep them encrypted and only reveal them to the owner.
    qr_url_encrypted: str | None = None
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class TelegramWebhookEvent(TimestampMixin, table=True):
    """Minimal webhook audit and idempotency record; raw payload stays out of persistence."""

    __tablename__ = "telegram_webhook_events"
    __table_args__ = (UniqueConstraint("update_id", name="uq_telegram_webhook_events_update_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    update_id: int = Field(sa_column=Column(BigInteger, nullable=False, index=True))
    payload_hash: str = Field(max_length=128)
    event_type: str = Field(default="unknown", max_length=64, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class WeComConnection(TimestampMixin, table=True):
    __tablename__ = "wecom_connections"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "corp_id",
            "agent_id",
            name="uq_wecom_connections_owner_corp_agent",
        ),
        Index("ix_wecom_connections_owner_status", "owner_user_id", "status"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: WeComConnectionType = Field(
        default=WeComConnectionType.INTERNAL_APP,
        sa_column=Column(
            SAEnum(WeComConnectionType, native_enum=False), nullable=False, index=True
        ),
    )
    status: WeComConnectionStatus = Field(
        default=WeComConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComConnectionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    enabled: bool = Field(default=True, index=True)
    display_name: str = Field(default="企业微信自建应用", max_length=255)
    corp_id: str = Field(max_length=128, index=True)
    agent_id: str = Field(max_length=64)
    secret_encrypted: str
    token_encrypted: str
    aes_key_encrypted: str
    last_verified_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComSource(TimestampMixin, table=True):
    __tablename__ = "wecom_sources"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "external_conversation_id",
            name="uq_wecom_sources_connection_conversation",
        ),
        UniqueConstraint(
            "archive_connection_id",
            "owner_user_id",
            "external_conversation_id",
            name="uq_wecom_sources_archive_owner_conversation",
        ),
        CheckConstraint(
            "(connection_id IS NOT NULL) <> (archive_connection_id IS NOT NULL)",
            name="ck_wecom_sources_exactly_one_connection",
        ),
        Index("ix_wecom_sources_owner_enabled", "owner_user_id", "enabled", "quota_paused"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID | None = Field(default=None, foreign_key="wecom_connections.id", index=True)
    archive_connection_id: UUID | None = Field(
        default=None, foreign_key="wecom_archive_connections.id", index=True
    )
    external_conversation_id: str = Field(max_length=255, index=True)
    display_name: str = Field(default="企业微信成员", max_length=255)
    source_type: WeComSourceType = Field(
        default=WeComSourceType.PRIVATE,
        sa_column=Column(SAEnum(WeComSourceType, native_enum=False), nullable=False, index=True),
    )
    receive_capability: WeComReceiveCapability = Field(
        default=WeComReceiveCapability.APP_CALLBACK,
        sa_column=Column(SAEnum(WeComReceiveCapability, native_enum=False), nullable=False),
    )
    send_capability: WeComSendCapability = Field(
        default=WeComSendCapability.APP_MESSAGE,
        sa_column=Column(SAEnum(WeComSendCapability, native_enum=False), nullable=False),
    )
    enabled: bool = Field(default=True, index=True)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    last_message_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComWebhookEvent(TimestampMixin, table=True):
    __tablename__ = "wecom_webhook_events"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "provider_event_id",
            name="uq_wecom_webhook_events_connection_provider",
        ),
        Index("ix_wecom_webhook_events_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_connections.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    provider_event_id: str = Field(max_length=255)
    event_type: str = Field(default="unknown", max_length=64, index=True)
    payload_hash: str = Field(max_length=64)
    normalized_payload_encrypted: str | None = None
    status: WeComEventStatus = Field(
        default=WeComEventStatus.RECEIVED,
        sa_column=Column(SAEnum(WeComEventStatus, native_enum=False), nullable=False, index=True),
    )
    attempt_count: int = Field(default=0, ge=0)
    queued_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class WeComOutboundDelivery(TimestampMixin, table=True):
    __tablename__ = "wecom_outbound_deliveries"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_wecom_outbound_deliveries_owner_idempotency",
        ),
        Index("ix_wecom_outbound_deliveries_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID = Field(foreign_key="wecom_connections.id", index=True)
    source_id: UUID = Field(foreign_key="wecom_sources.id", index=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    idempotency_key: str = Field(max_length=128)
    content_hash: str = Field(max_length=64)
    status: WeComDeliveryStatus = Field(
        default=WeComDeliveryStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComDeliveryStatus, native_enum=False), nullable=False, index=True
        ),
    )
    provider_message_id: str | None = Field(default=None, max_length=255)
    attempt_count: int = Field(default=0, ge=0)
    sent_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    error: str | None = Field(default=None, max_length=1000)


class WeComArchiveConnection(TimestampMixin, table=True):
    """Enterprise Finance SDK credentials managed by one local installer."""

    __tablename__ = "wecom_archive_connections"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id", "corp_id", name="uq_wecom_archive_connections_owner_corp"
        ),
        Index("ix_wecom_archive_connections_status_enabled", "status", "enabled"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    display_name: str = Field(default="企业微信会话存档", max_length=255)
    corp_id: str = Field(max_length=128, index=True)
    secret_encrypted: str
    private_key_encrypted: str
    public_key_version: int = Field(ge=1)
    status: WeComConnectionStatus = Field(
        default=WeComConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComConnectionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    enabled: bool = Field(default=True, index=True)
    last_verified_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_polled_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComArchiveMemberBinding(TimestampMixin, table=True):
    """Maps one local user to the WeCom member whose conversations they may see."""

    __tablename__ = "wecom_archive_member_bindings"
    __table_args__ = (
        UniqueConstraint(
            "connection_id", "user_id", name="uq_wecom_archive_bindings_connection_user"
        ),
        UniqueConstraint(
            "connection_id",
            "wecom_user_id",
            name="uq_wecom_archive_bindings_connection_wecom_user",
        ),
        Index("ix_wecom_archive_bindings_user_enabled", "user_id", "enabled"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    wecom_user_id: str = Field(max_length=128, index=True)
    display_name: str = Field(default="企业微信成员", max_length=255)
    enabled: bool = Field(default=True, index=True)
    last_matched_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )


class WeComArchiveCursor(TimestampMixin, table=True):
    __tablename__ = "wecom_archive_cursors"
    __table_args__ = (
        UniqueConstraint("connection_id", name="uq_wecom_archive_cursors_connection"),
        CheckConstraint("last_seq >= 0", name="ck_wecom_archive_cursors_last_seq_nonnegative"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    last_seq: int = Field(default=0, sa_column=Column(BigInteger, nullable=False))
    lease_expires_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    last_batch_size: int = Field(default=0, ge=0)


class WeComArchiveEvent(TimestampMixin, table=True):
    """Minimal provider audit. Decrypted message bodies are not stored here."""

    __tablename__ = "wecom_archive_events"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "provider_message_id",
            name="uq_wecom_archive_events_connection_message",
        ),
        Index("ix_wecom_archive_events_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    provider_message_id: str = Field(max_length=255)
    sequence: int = Field(sa_column=Column(BigInteger, nullable=False, index=True))
    message_type: str = Field(default="unknown", max_length=64, index=True)
    payload_hash: str = Field(max_length=64)
    status: WeComEventStatus = Field(
        default=WeComEventStatus.RECEIVED,
        sa_column=Column(SAEnum(WeComEventStatus, native_enum=False), nullable=False, index=True),
    )
    matched_user_count: int = Field(default=0, ge=0)
    attempt_count: int = Field(default=0, ge=0)
    processed_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class ReplyTemplate(TimestampMixin, table=True):
    __tablename__ = "reply_templates"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    title: str = Field(index=True)
    content: str
    category: str = Field(default="通用", index=True)
    enabled: bool = Field(default=True, index=True)
