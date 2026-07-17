class DeviceLimitReachedError(Exception):
    """The owner already has the configured maximum number of active devices."""


class DeviceNotFoundError(Exception):
    """The requested owner-scoped device does not exist."""


class DeviceRevokedError(Exception):
    """A revoked installation cannot silently register or refresh again."""


class DeviceCredentialRejectedError(Exception):
    """The refresh credential is missing, malformed, expired, revoked, or unknown."""


class DeviceCredentialReuseDetectedError(DeviceCredentialRejectedError):
    """A rotated credential was presented and its entire family was revoked."""
