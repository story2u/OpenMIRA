from uuid import uuid4

import pytest
from pydantic import ValidationError

from app.application.dto import DeviceRegistrationRequest
from app.core.config import Settings
from app.core.security import (
    DEVICE_REFRESH_TOKEN_PATTERN,
    create_device_refresh_token,
    hash_device_installation_id,
    hash_device_refresh_token,
)


def settings(secret: str = "device-session-secret") -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        jwt_secret_key=secret,
    )


def valid_registration(**overrides) -> DeviceRegistrationRequest:
    values = {
        "installationId": uuid4(),
        "platform": "ios",
        "displayName": "Bruce's iPhone",
        "appVariant": "production",
        "appVersion": "1.2.3",
        "appBuild": "42",
        "osVersion": "26.0",
        "locale": "zh-CN",
        "timezone": "Asia/Shanghai",
        "capabilities": {"sync.schema": 1, "push": True},
    }
    values.update(overrides)
    return DeviceRegistrationRequest.model_validate(values)


def test_device_refresh_token_is_high_entropy_versioned_and_only_hashable_when_well_formed() -> None:
    token = create_device_refresh_token()

    assert DEVICE_REFRESH_TOKEN_PATTERN.fullmatch(token)
    assert len(hash_device_refresh_token(token)) == 64
    assert token not in hash_device_refresh_token(token)
    with pytest.raises(ValueError):
        hash_device_refresh_token("radar_device_1_short")
    with pytest.raises(ValueError):
        hash_device_refresh_token(f"{token}\r\nX-Injected: value")


def test_installation_hash_is_stable_per_environment_and_does_not_reveal_uuid() -> None:
    installation_id = uuid4()
    first = hash_device_installation_id(installation_id, settings("secret-a"))
    repeated = hash_device_installation_id(installation_id, settings("secret-a"))
    other_environment = hash_device_installation_id(installation_id, settings("secret-b"))

    assert first == repeated
    assert first != other_environment
    assert str(installation_id) not in first


def test_device_registration_bounds_capabilities_and_runtime_metadata() -> None:
    assert valid_registration().capabilities == {"sync.schema": 1, "push": True}

    with pytest.raises(ValidationError):
        valid_registration(capabilities={f"cap-{index}": True for index in range(65)})
    with pytest.raises(ValidationError):
        valid_registration(capabilities={"nested": {"unsafe": True}})
    with pytest.raises(ValidationError):
        valid_registration(capabilities={"oversized": "x" * 257})
    with pytest.raises(ValidationError):
        valid_registration(timezone="Not/A_Real_Timezone")
    with pytest.raises(ValidationError):
        valid_registration(appVersion="latest")
