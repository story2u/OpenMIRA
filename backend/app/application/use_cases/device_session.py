from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID

from app.application.dto import DeviceRegistrationRequest
from app.core.config import Settings
from app.core.security import (
    create_access_token,
    create_device_refresh_token,
    hash_device_installation_id,
    hash_device_refresh_token,
)
from app.domain.services.device_session import DeviceCredentialRejectedError
from app.infrastructure.db.models import Device, User, utc_now
from app.infrastructure.db.repositories import DeviceRepository


@dataclass(frozen=True, slots=True)
class DeviceSessionIssue:
    access_token: str
    refresh_token: str
    refresh_token_expires_at: datetime
    device: Device
    user: User


class DeviceSessionService:
    def __init__(self, repository: DeviceRepository, settings: Settings) -> None:
        self.repository = repository
        self.settings = settings

    def _new_refresh_token(self) -> tuple[str, str, datetime]:
        raw = create_device_refresh_token()
        expires_at = utc_now() + timedelta(days=self.settings.device_refresh_token_days)
        return raw, hash_device_refresh_token(raw), expires_at

    async def register(
        self,
        *,
        user: User,
        request: DeviceRegistrationRequest,
    ) -> DeviceSessionIssue:
        raw_token, token_hash, expires_at = self._new_refresh_token()
        device, _ = await self.repository.register(
            owner_user_id=user.id,
            installation_id_hash=hash_device_installation_id(
                request.installationId,
                self.settings,
            ),
            platform=request.platform,
            display_name=request.displayName,
            app_variant=request.appVariant,
            app_version=request.appVersion,
            app_build=request.appBuild,
            os_version=request.osVersion,
            locale=request.locale,
            timezone_name=request.timezone,
            capabilities=dict(request.capabilities),
            credential_hash=token_hash,
            credential_expires_at=expires_at,
            max_active_devices=self.settings.device_max_active_per_user,
        )
        return DeviceSessionIssue(
            access_token=create_access_token(
                subject=user.id,
                device_id=device.id,
                settings=self.settings,
            ),
            refresh_token=raw_token,
            refresh_token_expires_at=expires_at,
            device=device,
            user=user,
        )

    async def rotate(self, refresh_token: str) -> DeviceSessionIssue:
        try:
            credential_hash = hash_device_refresh_token(refresh_token)
        except ValueError as exc:
            raise DeviceCredentialRejectedError from exc
        replacement_token, replacement_hash, expires_at = self._new_refresh_token()
        user, device, _ = await self.repository.rotate_credential(
            credential_hash=credential_hash,
            replacement_hash=replacement_hash,
            replacement_expires_at=expires_at,
        )
        return DeviceSessionIssue(
            access_token=create_access_token(
                subject=user.id,
                device_id=device.id,
                settings=self.settings,
            ),
            refresh_token=replacement_token,
            refresh_token_expires_at=expires_at,
            device=device,
            user=user,
        )

    async def list_devices(self, owner_user_id: UUID) -> list[Device]:
        return await self.repository.list_owned(owner_user_id)

    async def revoke(self, *, owner_user_id: UUID, device_id: UUID) -> Device:
        return await self.repository.revoke_owned(
            owner_user_id=owner_user_id,
            device_id=device_id,
            reason="user_revoked",
        )
