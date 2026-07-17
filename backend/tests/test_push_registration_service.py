import hashlib
from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.dto import PushRegistrationRequest
from app.application.use_cases.push_registration import PushRegistrationService
from app.core.config import Settings
from app.domain.enums import DevicePlatform
from app.domain.services.push_delivery import (
    PushRegistrationConflictError,
    PushRegistrationUnavailableError,
)
from app.infrastructure.db.models import Device, User, utc_now


class CapturingRepository:
    def __init__(self) -> None:
        self.values = None

    async def register(self, **values):
        self.values = values
        return SimpleNamespace(**values)


def service_settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_sync_rollout_enabled=True,
        rn_push_rollout_enabled=True,
        push_dispatch_enabled=True,
        apns_team_id="TEAM123",
        apns_key_id="KEY123",
        apns_private_key="private-key",
        apns_topic="com.codeiy.im",
    )


def ios_subject() -> tuple[User, Device]:
    user = User(id=uuid4(), email="push-service@example.test")
    now = utc_now()
    device = Device(
        owner_user_id=user.id,
        installation_id_hash="a" * 64,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={"push.environment": "production"},
        created_at=now,
        updated_at=now,
        last_seen_at=now,
    )
    return user, device


async def test_registration_hashes_and_encrypts_token_before_repository() -> None:
    user, device = ios_subject()
    repository = CapturingRepository()
    native_token = "native-token-value-that-is-high-entropy"

    await PushRegistrationService(repository, service_settings()).register(
        user=user,
        device=device,
        request=PushRegistrationRequest(
            provider="apns",
            environment="production",
            token=native_token,
        ),
    )

    assert repository.values["token_hash"] == hashlib.sha256(native_token.encode()).hexdigest()
    assert native_token not in repository.values["token_encrypted"]


async def test_registration_rejects_platform_mismatch_and_unconfigured_environment() -> None:
    user, device = ios_subject()
    repository = CapturingRepository()
    service = PushRegistrationService(repository, service_settings())

    with pytest.raises(PushRegistrationConflictError):
        await service.register(
            user=user,
            device=device,
            request=PushRegistrationRequest(
                provider="fcm",
                environment="production",
                token="fcm-native-token-value-long-enough",
            ),
        )
    with pytest.raises(PushRegistrationConflictError):
        await service.register(
            user=user,
            device=device,
            request=PushRegistrationRequest(
                provider="apns",
                environment="sandbox",
                token="apns-native-token-value-long-enough",
            ),
        )
    with pytest.raises(PushRegistrationUnavailableError):
        device.capabilities["push.environment"] = "sandbox"
        await service.register(
            user=user,
            device=device,
            request=PushRegistrationRequest(
                provider="apns",
                environment="sandbox",
                token="apns-native-token-value-long-enough",
            ),
        )
    assert repository.values is None
