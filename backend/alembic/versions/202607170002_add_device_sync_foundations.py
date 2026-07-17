"""add device identity and sync foundation tables

Revision ID: 202607170002
Revises: 202607170001
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "202607170002"
down_revision: str | None = "202607170001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "devices",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("installation_id_hash", sa.String(length=64), nullable=False),
        sa.Column(
            "platform",
            sa.Enum(
                "IOS",
                "ANDROID",
                name="deviceplatform",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column(
            "status",
            sa.Enum(
                "ACTIVE",
                "REVOKED",
                name="devicestatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("display_name", sa.String(length=100), nullable=False),
        sa.Column("app_variant", sa.String(length=32), nullable=False),
        sa.Column("app_version", sa.String(length=32), nullable=False),
        sa.Column("app_build", sa.String(length=32), nullable=False),
        sa.Column("os_version", sa.String(length=64), nullable=True),
        sa.Column("locale", sa.String(length=35), nullable=True),
        sa.Column("timezone", sa.String(length=64), nullable=True),
        sa.Column("capabilities", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("last_seen_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("revoked_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("revocation_reason", sa.String(length=255), nullable=True),
        sa.CheckConstraint(
            "installation_id_hash ~ '^[0-9a-f]{64}$'",
            name="ck_devices_installation_hash_sha256",
        ),
        sa.CheckConstraint(
            "jsonb_typeof(capabilities) = 'object' "
            "AND octet_length(capabilities::text) <= 16384",
            name="ck_devices_capabilities_bounded_object",
        ),
        sa.CheckConstraint(
            "(status = 'ACTIVE' AND revoked_at IS NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL)",
            name="ck_devices_revocation_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "owner_user_id",
            "installation_id_hash",
            name="uq_devices_owner_installation_hash",
        ),
        sa.UniqueConstraint("owner_user_id", "id", name="uq_devices_owner_id"),
    )
    for column in ("owner_user_id", "platform", "status", "last_seen_at"):
        op.create_index(f"ix_devices_{column}", "devices", [column])
    op.create_index(
        "ix_devices_owner_status_last_seen",
        "devices",
        ["owner_user_id", "status", "last_seen_at"],
    )

    op.create_table(
        "device_credentials",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column("token_hash", sa.String(length=64), nullable=False),
        sa.Column("token_family_id", sa.Uuid(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "PENDING",
                "ACTIVE",
                "ROTATED",
                "REVOKED",
                "REUSE_DETECTED",
                name="devicecredentialstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("last_used_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("rotated_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("revoked_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("reuse_detected_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("replaced_by_credential_id", sa.Uuid(), nullable=True),
        sa.CheckConstraint(
            "token_hash ~ '^[0-9a-f]{64}$'",
            name="ck_device_credentials_token_hash_sha256",
        ),
        sa.CheckConstraint(
            "expires_at > created_at",
            name="ck_device_credentials_expiry_after_creation",
        ),
        sa.CheckConstraint(
            "(status IN ('PENDING', 'ACTIVE') AND rotated_at IS NULL AND revoked_at IS NULL "
            "AND reuse_detected_at IS NULL AND replaced_by_credential_id IS NULL) "
            "OR (status = 'ROTATED' AND rotated_at IS NOT NULL "
            "AND replaced_by_credential_id IS NOT NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL) "
            "OR (status = 'REUSE_DETECTED' AND revoked_at IS NOT NULL "
            "AND reuse_detected_at IS NOT NULL)",
            name="ck_device_credentials_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_device_credentials_owner_device",
        ),
        sa.ForeignKeyConstraint(
            ["replaced_by_credential_id"],
            ["device_credentials.id"],
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("token_hash", name="uq_device_credentials_token_hash"),
    )
    for column in (
        "owner_user_id",
        "device_id",
        "token_family_id",
        "status",
        "expires_at",
        "replaced_by_credential_id",
    ):
        op.create_index(f"ix_device_credentials_{column}", "device_credentials", [column])
    op.create_index(
        "uq_device_credentials_device_active",
        "device_credentials",
        ["device_id"],
        unique=True,
        postgresql_where=sa.text("status = 'ACTIVE'"),
    )
    op.create_index(
        "ix_device_credentials_owner_status_expires",
        "device_credentials",
        ["owner_user_id", "status", "expires_at"],
    )

    op.create_table(
        "push_registrations",
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column("device_id", sa.Uuid(), nullable=False),
        sa.Column(
            "provider",
            sa.Enum(
                "APNS",
                "FCM",
                name="pushprovider",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column(
            "environment",
            sa.Enum(
                "SANDBOX",
                "PRODUCTION",
                name="pushenvironment",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("token_hash", sa.String(length=64), nullable=False),
        sa.Column("token_encrypted", sa.Text(), nullable=False),
        sa.Column(
            "status",
            sa.Enum(
                "ACTIVE",
                "INVALIDATED",
                "REVOKED",
                name="pushregistrationstatus",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("last_registered_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("last_attempt_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("last_success_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("invalidated_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("revoked_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("failure_count", sa.Integer(), nullable=False),
        sa.Column("last_error_code", sa.String(length=64), nullable=True),
        sa.CheckConstraint(
            "token_hash ~ '^[0-9a-f]{64}$'",
            name="ck_push_registrations_token_hash_sha256",
        ),
        sa.CheckConstraint(
            "length(token_encrypted) >= 32",
            name="ck_push_registrations_encrypted_token_present",
        ),
        sa.CheckConstraint(
            "failure_count >= 0",
            name="ck_push_registrations_failure_count_nonnegative",
        ),
        sa.CheckConstraint(
            "(status = 'ACTIVE' AND invalidated_at IS NULL AND revoked_at IS NULL) "
            "OR (status = 'INVALIDATED' AND invalidated_at IS NOT NULL "
            "AND revoked_at IS NULL) "
            "OR (status = 'REVOKED' AND revoked_at IS NOT NULL)",
            name="ck_push_registrations_lifecycle_state",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.ForeignKeyConstraint(
            ["owner_user_id", "device_id"],
            ["devices.owner_user_id", "devices.id"],
            name="fk_push_registrations_owner_device",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("token_hash", name="uq_push_registrations_token_hash"),
    )
    for column in ("owner_user_id", "device_id", "provider", "status"):
        op.create_index(f"ix_push_registrations_{column}", "push_registrations", [column])
    op.create_index(
        "uq_push_registrations_device_provider_environment_active",
        "push_registrations",
        ["device_id", "provider", "environment"],
        unique=True,
        postgresql_where=sa.text("status = 'ACTIVE'"),
    )
    op.create_index(
        "ix_push_registrations_owner_status",
        "push_registrations",
        ["owner_user_id", "status"],
    )

    op.create_table(
        "sync_changes",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column(
            "cursor",
            sa.BigInteger(),
            sa.Identity(start=1),
            nullable=False,
        ),
        sa.Column("owner_user_id", sa.Uuid(), nullable=False),
        sa.Column(
            "aggregate_type",
            sa.Enum(
                "OPPORTUNITY",
                "MESSAGE",
                "USER_DETECTION_PREFERENCE",
                "USER_WORK_SCHEDULE",
                "USER_NOTIFICATION_PREFERENCE",
                "REPLY_TEMPLATE",
                name="syncaggregatetype",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("aggregate_id", sa.Uuid(), nullable=False),
        sa.Column("aggregate_version", sa.BigInteger(), nullable=False),
        sa.Column(
            "operation",
            sa.Enum(
                "UPSERT",
                "DELETE",
                name="syncoperation",
                native_enum=False,
                create_constraint=True,
            ),
            nullable=False,
        ),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column(
            "payload",
            postgresql.JSONB(astext_type=sa.Text(), none_as_null=True),
            nullable=True,
        ),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "aggregate_version > 0",
            name="ck_sync_changes_aggregate_version_positive",
        ),
        sa.CheckConstraint(
            "schema_version > 0",
            name="ck_sync_changes_schema_version_positive",
        ),
        sa.CheckConstraint(
            "(operation = 'UPSERT' AND payload IS NOT NULL) "
            "OR (operation = 'DELETE' AND payload IS NULL)",
            name="ck_sync_changes_payload_matches_operation",
        ),
        sa.ForeignKeyConstraint(["owner_user_id"], ["users.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("cursor", name="uq_sync_changes_cursor"),
        sa.UniqueConstraint(
            "owner_user_id",
            "aggregate_type",
            "aggregate_id",
            "aggregate_version",
            name="uq_sync_changes_owner_aggregate_version",
        ),
    )
    for column in ("owner_user_id", "aggregate_type", "aggregate_id", "operation"):
        op.create_index(f"ix_sync_changes_{column}", "sync_changes", [column])
    op.create_index(
        "ix_sync_changes_owner_cursor",
        "sync_changes",
        ["owner_user_id", "cursor"],
    )
    op.create_index(
        "ix_sync_changes_owner_aggregate_version",
        "sync_changes",
        ["owner_user_id", "aggregate_type", "aggregate_id", "aggregate_version"],
    )
    op.create_index("ix_sync_changes_created_at", "sync_changes", ["created_at"])


def downgrade() -> None:
    op.drop_table("sync_changes")
    op.drop_table("push_registrations")
    op.drop_table("device_credentials")
    op.drop_table("devices")
