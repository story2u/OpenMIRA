import asyncio
import os
from collections.abc import AsyncIterator
from datetime import timedelta
from uuid import uuid4

import pytest
from fastapi import HTTPException
from fastapi.security import HTTPAuthorizationCredentials
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.application.dto import DeviceRegistrationRequest, PushRegistrationRequest
from app.api.deps import _user_from_token, require_device_principal
from app.application.use_cases.device_session import DeviceSessionService
from app.application.use_cases.push_registration import PushRegistrationService
from app.core.config import Settings
from app.core.security import (
    create_access_token,
    hash_device_installation_id,
    hash_device_refresh_token,
)
from app.domain.enums import (
    DeviceCredentialStatus,
    DeviceStatus,
    PushRegistrationStatus,
    SyncAggregateType,
    SyncOperation,
)
from app.domain.services.device_session import (
    DeviceCredentialRejectedError,
    DeviceCredentialReuseDetectedError,
    DeviceLimitReachedError,
    DeviceNotFoundError,
)
from app.infrastructure.db.models import (
    Device,
    DeviceCredential,
    PushRegistration,
    SyncChange,
    User,
    utc_now,
)
from app.infrastructure.db.repositories import DeviceRepository, PushRegistrationRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL device session tests",
)


def device_settings(*, max_devices: int = 20) -> Settings:
    assert TEST_DATABASE_URL
    return Settings(
        database_url=TEST_DATABASE_URL,
        admin_api_token="test-admin-token",
        jwt_secret_key="device-session-test-secret",
        device_refresh_token_days=30,
        device_max_active_per_user=max_devices,
    )


def registration(installation_id=None) -> DeviceRegistrationRequest:
    return DeviceRegistrationRequest(
        installationId=installation_id or uuid4(),
        platform="ios",
        displayName="Test iPhone",
        appVariant="production",
        appVersion="1.0.0",
        appBuild="1",
        osVersion="26.0",
        locale="en-US",
        timezone="Asia/Shanghai",
        capabilities={"push.environment": "production", "sync.schema": 1},
    )


@pytest.fixture
async def device_session_subject() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], User, User]
]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"session-owner-{uuid4()}@example.test")
    other_owner = User(email=f"session-other-{uuid4()}@example.test")
    async with factory() as session:
        session.add_all([owner, other_owner])
        await session.commit()

    yield factory, owner, other_owner

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


async def register_with(
    factory: async_sessionmaker[AsyncSession],
    owner: User,
    request: DeviceRegistrationRequest,
    *,
    settings: Settings | None = None,
):
    async with factory() as session:
        return await DeviceSessionService(
            DeviceRepository(session),
            settings or device_settings(),
        ).register(user=owner, request=request)


async def test_registration_persists_only_hashes_and_reissues_atomically(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    request = registration()
    first = await register_with(factory, owner, request)
    second = await register_with(factory, owner, request)

    assert second.device.id == first.device.id
    assert second.refresh_token != first.refresh_token
    assert first.access_token != ""

    async with factory() as session:
        device = await session.get(Device, first.device.id)
        credentials = (
            await session.exec(
                select(DeviceCredential)
                .where(DeviceCredential.device_id == first.device.id)
                .order_by(DeviceCredential.created_at)
            )
        ).all()

    assert device is not None
    assert device.installation_id_hash == hash_device_installation_id(
        request.installationId,
        device_settings(),
    )
    assert str(request.installationId) not in device.installation_id_hash
    assert [item.status for item in credentials] == [
        DeviceCredentialStatus.REVOKED,
        DeviceCredentialStatus.ACTIVE,
    ]
    assert credentials[1].token_hash == hash_device_refresh_token(second.refresh_token)
    assert all(second.refresh_token != item.token_hash for item in credentials)


async def test_rotation_is_single_use_and_reuse_revokes_the_family_and_device(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    registered = await register_with(factory, owner, registration())

    async with factory() as session:
        rotated = await DeviceSessionService(DeviceRepository(session), device_settings()).rotate(
            registered.refresh_token
        )

    assert rotated.refresh_token != registered.refresh_token
    assert rotated.access_token

    async with factory() as session:
        service = DeviceSessionService(DeviceRepository(session), device_settings())
        with pytest.raises(DeviceCredentialReuseDetectedError):
            await service.rotate(registered.refresh_token)

    async with factory() as session:
        device = await session.get(Device, registered.device.id)
        credentials = (
            await session.exec(
                select(DeviceCredential).where(
                    DeviceCredential.device_id == registered.device.id
                )
            )
        ).all()

    assert device is not None
    assert device.status == DeviceStatus.REVOKED
    assert device.revocation_reason == "credential_reuse_detected"
    assert {item.status for item in credentials} == {DeviceCredentialStatus.REUSE_DETECTED}

    async with factory() as session:
        with pytest.raises(DeviceCredentialRejectedError):
            await DeviceSessionService(DeviceRepository(session), device_settings()).rotate(
                rotated.refresh_token
            )


async def test_owner_scoped_revoke_clears_active_credential(device_session_subject) -> None:
    factory, owner, other_owner = device_session_subject
    registered = await register_with(factory, owner, registration())

    async with factory() as session:
        service = DeviceSessionService(DeviceRepository(session), device_settings())
        with pytest.raises(DeviceNotFoundError):
            await service.revoke(owner_user_id=other_owner.id, device_id=registered.device.id)
        revoked = await service.revoke(owner_user_id=owner.id, device_id=registered.device.id)
        repeated = await service.revoke(owner_user_id=owner.id, device_id=registered.device.id)

    assert revoked.status == DeviceStatus.REVOKED
    assert repeated.status == DeviceStatus.REVOKED

    async with factory() as session:
        credential = (
            await session.exec(
                select(DeviceCredential).where(
                    DeviceCredential.device_id == registered.device.id
                )
            )
        ).one()
    assert credential.status == DeviceCredentialStatus.REVOKED


async def test_device_bound_access_token_stops_authorizing_after_revoke(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    settings = device_settings()
    registered = await register_with(factory, owner, registration(), settings=settings)

    async with factory() as session:
        authenticated = await _user_from_token(registered.access_token, settings, session)
        assert authenticated.id == owner.id
        await DeviceRepository(session).revoke_owned(
            owner_user_id=owner.id,
            device_id=registered.device.id,
            reason="test_revoked",
        )

    async with factory() as session:
        with pytest.raises(HTTPException) as raised:
            await _user_from_token(registered.access_token, settings, session)
    assert raised.value.status_code == 401
    assert raised.value.detail == "inactive device"


async def test_sync_dependency_requires_device_bound_access_token(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    settings = device_settings()
    registered = await register_with(factory, owner, registration(), settings=settings)
    legacy_access = create_access_token(subject=owner.id, settings=settings)

    async with factory() as session:
        principal = await require_device_principal(
            credentials=HTTPAuthorizationCredentials(
                scheme="Bearer",
                credentials=registered.access_token,
            ),
            settings=settings,
            session=session,
        )
        assert principal.user.id == owner.id
        assert principal.device is not None
        assert principal.device.id == registered.device.id
        with pytest.raises(HTTPException) as raised:
            await require_device_principal(
                credentials=HTTPAuthorizationCredentials(
                    scheme="Bearer",
                    credentials=legacy_access,
                ),
                settings=settings,
                session=session,
            )
    assert raised.value.status_code == 401
    assert raised.value.detail == "device session required"


async def test_concurrent_rotation_detects_reuse_and_revokes_both_results(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    registered = await register_with(factory, owner, registration())

    async def rotate_once():
        async with factory() as session:
            return await DeviceSessionService(DeviceRepository(session), device_settings()).rotate(
                registered.refresh_token
            )

    results = await asyncio.gather(rotate_once(), rotate_once(), return_exceptions=True)

    assert sum(not isinstance(result, Exception) for result in results) == 1
    assert sum(isinstance(result, DeviceCredentialReuseDetectedError) for result in results) == 1
    async with factory() as session:
        device = await session.get(Device, registered.device.id)
        assert device is not None
        assert device.status == DeviceStatus.REVOKED


async def test_device_limit_and_expired_credentials_fail_closed(device_session_subject) -> None:
    factory, owner, _ = device_session_subject
    settings = device_settings(max_devices=1)
    registered = await register_with(factory, owner, registration(), settings=settings)
    with pytest.raises(DeviceLimitReachedError):
        await register_with(factory, owner, registration(), settings=settings)

    async with factory() as session:
        credential = (
            await session.exec(
                select(DeviceCredential).where(
                    DeviceCredential.device_id == registered.device.id,
                    DeviceCredential.status == DeviceCredentialStatus.ACTIVE,
                )
            )
        ).one()
        with pytest.raises(DeviceCredentialRejectedError):
            await DeviceRepository(session).rotate_credential(
                credential_hash=credential.token_hash,
                replacement_hash="9" * 64,
                replacement_expires_at=utc_now() + timedelta(days=30),
                now=credential.expires_at + timedelta(seconds=1),
            )

    async with factory() as session:
        credential = await session.get(DeviceCredential, credential.id)
        assert credential is not None
        assert credential.status == DeviceCredentialStatus.REVOKED


async def test_push_token_rotation_claim_and_cursor_advance_are_durable(
    device_session_subject,
) -> None:
    factory, owner, _ = device_session_subject
    registered = await register_with(factory, owner, registration())
    settings = device_settings().model_copy(
        update={
            "rn_sync_rollout_enabled": True,
            "rn_push_rollout_enabled": True,
            "push_dispatch_enabled": True,
            "apns_team_id": "TEAM123",
            "apns_key_id": "KEY123",
            "apns_private_key": "configured-for-registration-only",
            "apns_topic": "com.codeiy.im",
        }
    )
    first_token = "a" * 64
    second_token = "b" * 64

    async with factory() as session:
        service = PushRegistrationService(PushRegistrationRepository(session), settings)
        first = await service.register(
            user=owner,
            device=registered.device,
            request=PushRegistrationRequest(
                provider="apns",
                environment="production",
                token=first_token,
            ),
        )
        second = await service.register(
            user=owner,
            device=registered.device,
            request=PushRegistrationRequest(
                provider="apns",
                environment="production",
                token=second_token,
            ),
        )
        session.add(
            SyncChange(
                owner_user_id=owner.id,
                aggregate_type=SyncAggregateType.OPPORTUNITY,
                aggregate_id=uuid4(),
                aggregate_version=1,
                operation=SyncOperation.UPSERT,
                schema_version=1,
                payload={"id": "cursor-only-trigger"},
            )
        )
        await session.commit()

        repository = PushRegistrationRepository(session)
        claimed = await repository.claim_pending(limit=10, lease_seconds=120)
        leased_again = await repository.claim_pending(limit=10, lease_seconds=120)
        await repository.mark_success(second.id, claimed[0].cursor)

    async with factory() as session:
        first_stored = await session.get(PushRegistration, first.id)
        second_stored = await session.get(PushRegistration, second.id)

    assert first_stored is not None
    assert first_stored.status == PushRegistrationStatus.INVALIDATED
    assert first_token not in first_stored.token_encrypted
    assert second_stored is not None
    assert second_token not in second_stored.token_encrypted
    assert second_stored.last_notified_cursor == claimed[0].cursor
    assert claimed[0].registration_id == second.id
    assert claimed[0].token_encrypted == second.token_encrypted
    assert leased_again == []
