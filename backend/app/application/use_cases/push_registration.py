import hashlib

from app.application.dto import PushRegistrationRequest
from app.core.config import Settings
from app.core.security import encrypt_secret
from app.domain.enums import DevicePlatform, PushEnvironment, PushProvider
from app.domain.services.push_delivery import (
    PushRegistrationConflictError,
    PushRegistrationUnavailableError,
    provider_for_platform,
)
from app.infrastructure.db.models import Device, PushRegistration, User
from app.infrastructure.db.repositories import PushRegistrationRepository


class PushRegistrationService:
    def __init__(self, repository: PushRegistrationRepository, settings: Settings) -> None:
        self.repository = repository
        self.settings = settings

    def provider_available(
        self,
        platform: DevicePlatform,
        environment: PushEnvironment,
    ) -> bool:
        if not self.settings.rn_sync_rollout_enabled or not self.settings.rn_push_rollout_enabled:
            return False
        if platform == DevicePlatform.IOS:
            return self.settings.apns_available_for(environment.value)
        return self.settings.fcm_push_available

    async def register(
        self,
        *,
        user: User,
        device: Device,
        request: PushRegistrationRequest,
    ) -> PushRegistration:
        if provider_for_platform(device.platform) != request.provider:
            raise PushRegistrationConflictError
        if device.capabilities.get("push.environment") != request.environment.value:
            raise PushRegistrationConflictError
        if not self.provider_available(device.platform, request.environment):
            raise PushRegistrationUnavailableError
        token = request.token
        if token != token.strip() or any(ord(character) < 33 for character in token):
            raise PushRegistrationConflictError
        return await self.repository.register(
            owner_user_id=user.id,
            device_id=device.id,
            provider=request.provider,
            environment=request.environment,
            token_hash=hashlib.sha256(token.encode("utf-8")).hexdigest(),
            token_encrypted=encrypt_secret(token, self.settings),
        )

    async def revoke(
        self,
        *,
        user: User,
        device: Device,
        provider: PushProvider,
        environment: PushEnvironment,
    ) -> bool:
        if provider_for_platform(device.platform) != provider:
            raise PushRegistrationConflictError
        return await self.repository.revoke(
            owner_user_id=user.id,
            device_id=device.id,
            provider=provider,
            environment=environment,
        )
