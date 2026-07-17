from sqlalchemy import CheckConstraint, Identity, Index, UniqueConstraint

from app.infrastructure.db.models import (
    Device,
    DeviceCredential,
    Message,
    Opportunity,
    PushRegistration,
    SyncChange,
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
)


def constraint_names(model) -> set[str]:
    return {
        constraint.name
        for constraint in model.__table__.constraints
        if isinstance(constraint, (CheckConstraint, UniqueConstraint)) and constraint.name
    }


def index_names(model) -> set[str]:
    return {index.name for index in model.__table__.indexes if isinstance(index, Index)}


def test_device_identity_and_owner_constraints_are_declared() -> None:
    assert {
        "uq_devices_owner_installation_hash",
        "uq_devices_owner_id",
        "ck_devices_installation_hash_sha256",
        "ck_devices_capabilities_bounded_object",
        "ck_devices_revocation_state",
        "ck_devices_last_sync_cursor_nonnegative",
    }.issubset(constraint_names(Device))
    assert "ix_devices_owner_status_last_seen" in index_names(Device)


def test_device_credential_never_has_a_plaintext_token_column() -> None:
    columns = set(DeviceCredential.__table__.columns.keys())

    assert "token_hash" in columns
    assert "token" not in columns
    assert "token_encrypted" not in columns
    assert {
        "uq_device_credentials_token_hash",
        "ck_device_credentials_token_hash_sha256",
        "ck_device_credentials_expiry_after_creation",
        "ck_device_credentials_lifecycle_state",
    }.issubset(constraint_names(DeviceCredential))
    assert "uq_device_credentials_device_active" in index_names(DeviceCredential)


def test_push_registration_keeps_only_encrypted_minimum_and_hash() -> None:
    columns = set(PushRegistration.__table__.columns.keys())

    assert {"token_hash", "token_encrypted"}.issubset(columns)
    assert "token" not in columns
    assert {
        "uq_push_registrations_token_hash",
        "ck_push_registrations_encrypted_token_present",
        "ck_push_registrations_lifecycle_state",
    }.issubset(constraint_names(PushRegistration))
    assert "uq_push_registrations_device_provider_environment_active" in index_names(
        PushRegistration
    )


def test_sync_change_uses_database_identity_and_immutable_envelope_constraints() -> None:
    cursor = SyncChange.__table__.columns["cursor"]
    owner_foreign_key = next(iter(SyncChange.__table__.columns["owner_user_id"].foreign_keys))

    assert isinstance(cursor.server_default, Identity)
    assert owner_foreign_key.ondelete == "CASCADE"
    assert {
        "uq_sync_changes_cursor",
        "uq_sync_changes_owner_aggregate_version",
        "ck_sync_changes_aggregate_version_positive",
        "ck_sync_changes_schema_version_positive",
        "ck_sync_changes_payload_matches_operation",
    }.issubset(constraint_names(SyncChange))
    assert {
        "ix_sync_changes_owner_cursor",
        "ix_sync_changes_owner_aggregate_version",
    }.issubset(index_names(SyncChange))


def test_sync_source_aggregates_have_positive_versions() -> None:
    versioned_models = {
        Opportunity: "ck_opportunities_aggregate_version_positive",
        Message: "ck_messages_aggregate_version_positive",
        UserDetectionPreference: "ck_user_detection_preferences_aggregate_version_positive",
        UserWorkSchedule: "ck_user_work_schedules_aggregate_version_positive",
        UserNotificationPreference: "ck_user_notification_preferences_aggregate_version_positive",
    }

    for model, constraint in versioned_models.items():
        assert "aggregate_version" in model.__table__.columns
        assert constraint in constraint_names(model)
