import os
from collections.abc import AsyncIterator
from datetime import timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    DeviceCredentialStatus,
    DevicePlatform,
    PushEnvironment,
    PushProvider,
    PushRegistrationStatus,
    SyncAggregateType,
    SyncOperation,
)
from app.infrastructure.db.models import (
    Device,
    DeviceCredential,
    PushRegistration,
    SyncChange,
    User,
    utc_now,
)

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL device/sync tests",
)


def make_device(owner: User, *, installation_hash: str, build: str = "1") -> Device:
    return Device(
        owner_user_id=owner.id,
        installation_id_hash=installation_hash,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build=build,
        capabilities={"sync": True},
    )


@pytest.fixture
async def device_sync_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, User, Device, Device]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"device-owner-{uuid4()}@example.test")
    other_owner = User(email=f"device-other-{uuid4()}@example.test")

    async with factory() as session:
        session.add_all([owner, other_owner])
        await session.commit()
        owned_device = make_device(owner, installation_hash="a" * 64)
        foreign_device = make_device(other_owner, installation_hash="a" * 64)
        session.add_all([owned_device, foreign_device])
        await session.commit()

    yield factory, owner, other_owner, owned_device, foreign_device

    async with factory() as session:
        owner_ids = [owner.id, other_owner.id]
        await session.exec(delete(SyncChange).where(SyncChange.owner_user_id.in_(owner_ids)))
        await session.exec(
            delete(PushRegistration).where(PushRegistration.owner_user_id.in_(owner_ids))
        )
        await session.exec(
            delete(DeviceCredential).where(DeviceCredential.owner_user_id.in_(owner_ids))
        )
        await session.exec(delete(Device).where(Device.owner_user_id.in_(owner_ids)))
        await session.exec(delete(User).where(User.id.in_(owner_ids)))
        await session.commit()
    await engine.dispose()


async def test_installation_hash_is_unique_per_owner_but_not_globally(device_sync_subject) -> None:
    factory, owner, _, _, _ = device_sync_subject
    duplicate = make_device(owner, installation_hash="a" * 64, build="2")

    async with factory() as session:
        session.add(duplicate)
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()


async def test_credential_is_owner_bound_hashed_and_only_one_can_be_active(
    device_sync_subject,
) -> None:
    factory, owner, other_owner, owned_device, _ = device_sync_subject
    expires_at = utc_now() + timedelta(days=30)

    async with factory() as session:
        active = DeviceCredential(
            owner_user_id=owner.id,
            device_id=owned_device.id,
            token_hash="b" * 64,
            status=DeviceCredentialStatus.ACTIVE,
            expires_at=expires_at,
        )
        session.add(active)
        await session.commit()
        active_id = active.id

        session.add(
            DeviceCredential(
                owner_user_id=owner.id,
                device_id=owned_device.id,
                token_hash="c" * 64,
                status=DeviceCredentialStatus.ACTIVE,
                expires_at=expires_at,
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        session.add(
            DeviceCredential(
                owner_user_id=other_owner.id,
                device_id=owned_device.id,
                token_hash="d" * 64,
                status=DeviceCredentialStatus.ACTIVE,
                expires_at=expires_at,
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        stored = await session.get(DeviceCredential, active_id)
        assert stored is not None
        assert stored.token_hash == "b" * 64
        assert "token" not in DeviceCredential.__table__.columns


async def test_push_registration_enforces_rotation_and_lifecycle_constraints(
    device_sync_subject,
) -> None:
    factory, owner, _, owned_device, _ = device_sync_subject

    async with factory() as session:
        registration = PushRegistration(
            owner_user_id=owner.id,
            device_id=owned_device.id,
            provider=PushProvider.APNS,
            environment=PushEnvironment.PRODUCTION,
            token_hash="e" * 64,
            token_encrypted="encrypted:" + "x" * 64,
        )
        session.add(registration)
        await session.commit()

        session.add(
            PushRegistration(
                owner_user_id=owner.id,
                device_id=owned_device.id,
                provider=PushProvider.APNS,
                environment=PushEnvironment.PRODUCTION,
                token_hash="f" * 64,
                token_encrypted="encrypted:" + "y" * 64,
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        session.add(
            PushRegistration(
                owner_user_id=owner.id,
                device_id=owned_device.id,
                provider=PushProvider.FCM,
                environment=PushEnvironment.PRODUCTION,
                token_hash="1" * 64,
                token_encrypted="encrypted:" + "z" * 64,
                status=PushRegistrationStatus.INVALIDATED,
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()


async def test_sync_cursor_is_monotonic_owner_scoped_and_envelope_is_strict(
    device_sync_subject,
) -> None:
    factory, owner, other_owner, _, _ = device_sync_subject
    aggregate_id = uuid4()

    async with factory() as session:
        first = SyncChange(
            owner_user_id=owner.id,
            aggregate_type=SyncAggregateType.OPPORTUNITY,
            aggregate_id=aggregate_id,
            aggregate_version=1,
            operation=SyncOperation.UPSERT,
            payload={"id": str(aggregate_id), "title": "first"},
        )
        foreign = SyncChange(
            owner_user_id=other_owner.id,
            aggregate_type=SyncAggregateType.OPPORTUNITY,
            aggregate_id=uuid4(),
            aggregate_version=1,
            operation=SyncOperation.UPSERT,
            payload={"title": "foreign"},
        )
        second = SyncChange(
            owner_user_id=owner.id,
            aggregate_type=SyncAggregateType.OPPORTUNITY,
            aggregate_id=aggregate_id,
            aggregate_version=2,
            operation=SyncOperation.DELETE,
            payload=None,
        )
        session.add_all([first, foreign, second])
        await session.commit()
        await session.refresh(first)
        await session.refresh(second)

        owned = (
            await session.exec(
                select(SyncChange)
                .where(SyncChange.owner_user_id == owner.id)
                .order_by(SyncChange.cursor)
            )
        ).all()
        assert [change.id for change in owned] == [first.id, second.id]
        assert first.cursor is not None and second.cursor is not None
        assert first.cursor < second.cursor

        session.add(
            SyncChange(
                owner_user_id=owner.id,
                aggregate_type=SyncAggregateType.OPPORTUNITY,
                aggregate_id=aggregate_id,
                aggregate_version=2,
                operation=SyncOperation.DELETE,
                payload=None,
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        session.add(
            SyncChange(
                owner_user_id=owner.id,
                aggregate_type=SyncAggregateType.MESSAGE,
                aggregate_id=uuid4(),
                aggregate_version=1,
                operation=SyncOperation.DELETE,
                payload={"must": "be null"},
            )
        )
        with pytest.raises(IntegrityError):
            await session.commit()
        await session.rollback()

        with pytest.raises(IntegrityError):
            await session.exec(
                text(
                    """
                    INSERT INTO sync_changes (
                        id, owner_user_id, aggregate_type, aggregate_id,
                        aggregate_version, operation, schema_version, payload, created_at
                    ) VALUES (
                        :id, :owner_user_id, 'OPPORTUNITY', :aggregate_id,
                        1, 'BOGUS', 1, CAST(:payload AS jsonb), :created_at
                    )
                    """
                ),
                params={
                    "id": uuid4(),
                    "owner_user_id": owner.id,
                    "aggregate_id": uuid4(),
                    "payload": '{"title":"unknown"}',
                    "created_at": utc_now(),
                },
            )
            await session.commit()
        await session.rollback()
