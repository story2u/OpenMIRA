from dataclasses import dataclass
from enum import StrEnum
from uuid import UUID

from app.domain.enums import DevicePlatform, PushEnvironment, PushProvider


class PushRegistrationConflictError(Exception):
    """The supplied provider does not belong to the authenticated device."""


class PushRegistrationUnavailableError(Exception):
    """The requested provider is intentionally disabled or not configured."""


class PushDeliveryStatus(StrEnum):
    SUCCESS = "success"
    INVALID_TOKEN = "invalid_token"
    RETRY = "retry"


@dataclass(frozen=True, slots=True)
class PendingPushDelivery:
    registration_id: UUID
    provider: PushProvider
    environment: PushEnvironment
    token_encrypted: str
    cursor: int


@dataclass(frozen=True, slots=True)
class PushDeliveryResult:
    status: PushDeliveryStatus
    error_code: str | None = None


def provider_for_platform(platform: DevicePlatform) -> PushProvider:
    if platform == DevicePlatform.IOS:
        return PushProvider.APNS
    return PushProvider.FCM
