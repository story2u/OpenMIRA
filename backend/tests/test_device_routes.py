import os
from uuid import UUID, uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.deps import (
    DevicePrincipal,
    get_device_session_service,
    get_push_registration_service,
    require_device_principal,
    require_user,
)
from app.api.v1.routes import devices as devices_route
from app.application.use_cases.device_session import DeviceSessionIssue
from app.core.config import Settings, get_settings
from app.domain.enums import (
    DevicePlatform,
    PushEnvironment,
    PushProvider,
    PushRegistrationStatus,
)
from app.domain.services.device_session import DeviceCredentialRejectedError, DeviceNotFoundError
from app.infrastructure.db.models import Device, User, utc_now
from app.infrastructure.db.models import PushRegistration


class FakePushRegistrationService:
    def __init__(self, user: User, device: Device) -> None:
        self.user = user
        self.device = device
        self.request = None
        self.revoked = None

    async def register(self, *, user, device, request):
        assert user.id == self.user.id
        assert device.id == self.device.id
        self.request = request
        now = utc_now()
        return PushRegistration(
            owner_user_id=user.id,
            device_id=device.id,
            provider=request.provider,
            environment=request.environment,
            token_hash="a" * 52 + "0123456789ab",
            token_encrypted="encrypted-token-material-that-is-long-enough",
            status=PushRegistrationStatus.ACTIVE,
            last_registered_at=now,
            created_at=now,
            updated_at=now,
        )

    async def revoke(self, *, user, device, provider, environment):
        self.revoked = (user.id, device.id, provider, environment)
        return True


class FakeDeviceSessionService:
    def __init__(self, user: User) -> None:
        now = utc_now()
        self.user = user
        self.device = Device(
            id=uuid4(),
            owner_user_id=user.id,
            installation_id_hash="a" * 64,
            platform=DevicePlatform.IOS,
            app_variant="production",
            app_version="1.0.0",
            app_build="1",
            capabilities={"sync.schema": 1},
            created_at=now,
            updated_at=now,
            last_seen_at=now,
        )
        self.registered_request = None
        self.rotated_token: str | None = None
        self.revoked: tuple[UUID, UUID] | None = None

    def issue(self) -> DeviceSessionIssue:
        return DeviceSessionIssue(
            access_token="access-token-long-enough",
            refresh_token="radar_device_1_" + "a" * 43,
            refresh_token_expires_at=utc_now(),
            device=self.device,
            user=self.user,
        )

    async def register(self, *, user, request):
        assert user.id == self.user.id
        self.registered_request = request
        return self.issue()

    async def list_devices(self, owner_user_id):
        assert owner_user_id == self.user.id
        return [self.device]

    async def rotate(self, refresh_token):
        self.rotated_token = refresh_token
        if refresh_token.endswith("reject"):
            raise DeviceCredentialRejectedError
        return self.issue()

    async def revoke(self, *, owner_user_id, device_id):
        if device_id != self.device.id:
            raise DeviceNotFoundError
        self.revoked = (owner_user_id, device_id)
        return self.device


def build_client(
    settings: Settings | None = None,
) -> tuple[TestClient, FakeDeviceSessionService, FakePushRegistrationService, User]:
    user = User(id=uuid4(), email="device-route@example.test", display_name="Device Route")
    service = FakeDeviceSessionService(user)
    push_service = FakePushRegistrationService(user, service.device)
    app = FastAPI()
    app.include_router(devices_route.router, prefix="/devices")
    app.dependency_overrides[require_user] = lambda: user
    app.dependency_overrides[require_device_principal] = lambda: DevicePrincipal(
        user=user,
        device=service.device,
    )
    app.dependency_overrides[get_settings] = lambda: settings or Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_sync_rollout_enabled=True,
    )
    app.dependency_overrides[get_device_session_service] = lambda: service
    app.dependency_overrides[get_push_registration_service] = lambda: push_service
    return TestClient(app), service, push_service, user


def registration_body() -> dict:
    return {
        "installationId": str(uuid4()),
        "platform": "ios",
        "displayName": "Test iPhone",
        "appVariant": "production",
        "appVersion": "1.0.0",
        "appBuild": "1",
        "osVersion": "26.0",
        "locale": "en-US",
        "timezone": "Asia/Shanghai",
        "capabilities": {"sync.schema": 1},
    }


def test_registration_returns_one_time_credentials_without_echoing_installation_id() -> None:
    client, service, _, _ = build_client()
    payload = registration_body()

    response = client.post(
        "/devices/register",
        json=payload,
        headers={"Authorization": "Bearer access-token"},
    )

    assert response.status_code == 201
    assert response.headers["cache-control"] == "no-store"
    assert response.headers["pragma"] == "no-cache"
    assert response.json()["deviceRefreshToken"].startswith("radar_device_1_")
    assert str(payload["installationId"]) not in response.text
    assert "installationId" not in response.json()["device"]
    assert service.registered_request.installationId == UUID(payload["installationId"])


def test_registration_rejects_unbounded_or_nested_capabilities_before_service() -> None:
    client, service, _, _ = build_client()
    payload = registration_body()
    payload["capabilities"] = {"nested": {"not": "allowed"}}

    response = client.post("/devices/register", json=payload)

    assert response.status_code == 422
    assert service.registered_request is None


def test_rotation_requires_explicit_bearer_and_never_echoes_rejected_value() -> None:
    client, service, _, _ = build_client()

    missing = client.post("/devices/credentials/rotate")
    rejected_token = "radar_device_1_" + "reject".rjust(43, "x")
    rejected = client.post(
        "/devices/credentials/rotate",
        headers={"Authorization": f"Bearer {rejected_token}"},
    )
    accepted_token = "radar_device_1_" + "b" * 43
    accepted = client.post(
        "/devices/credentials/rotate",
        headers={"Authorization": f"Bearer {accepted_token}"},
    )

    assert missing.status_code == 401
    assert rejected.status_code == 401
    assert rejected.json() == {"detail": "invalid device credential"}
    assert rejected_token not in rejected.text
    assert accepted.status_code == 200
    assert accepted.headers["cache-control"] == "no-store"
    assert service.rotated_token == accepted_token


def test_list_and_revoke_are_bound_to_authenticated_owner() -> None:
    client, service, _, user = build_client()

    listed = client.get("/devices")
    revoked = client.post(f"/devices/{service.device.id}/revoke")
    foreign = client.post(f"/devices/{uuid4()}/revoke")

    assert listed.status_code == 200
    assert [item["id"] for item in listed.json()] == [str(service.device.id)]
    assert revoked.status_code == 200
    assert service.revoked == (user.id, service.device.id)
    assert foreign.status_code == 404


def test_client_capabilities_require_server_rollout_and_supported_device_schema() -> None:
    client, service, _, _ = build_client()
    service.device.capabilities = {
        "client.reactNative": True,
        "sqlite.schema": 2,
    }

    supported = client.get("/devices/current/capabilities")
    service.device.capabilities["sqlite.schema"] = 1
    old_schema = client.get("/devices/current/capabilities")

    assert supported.status_code == 200
    assert supported.json() == {
        "agentToolsAvailable": False,
        "deviceAgentAvailable": False,
        "e2eeAvailable": False,
        "hostedFallbackAvailable": False,
        "pushAvailable": False,
        "rnClientSupported": True,
        "syncAvailable": True,
        "signalAppetiteSyncAvailable": False,
    }
    assert old_schema.status_code == 200
    assert old_schema.json()["rnClientSupported"] is True
    assert old_schema.json()["syncAvailable"] is False


def test_signal_appetite_sync_capability_requires_v6_and_independent_rollout() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_sync_rollout_enabled=True,
        signal_appetite_sync_enabled=True,
    )
    client, service, _, _ = build_client(settings)
    service.device.capabilities = {
        "client.reactNative": True,
        "sqlite.schema": 6,
    }
    supported = client.get("/devices/current/capabilities")
    service.device.capabilities["sqlite.schema"] = 5
    old_schema = client.get("/devices/current/capabilities")

    assert supported.json()["signalAppetiteSyncAvailable"] is True
    assert old_schema.json()["syncAvailable"] is True
    assert old_schema.json()["signalAppetiteSyncAvailable"] is False


def test_device_agent_capability_requires_exact_runtime_schema_and_rollout() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_sync_rollout_enabled=True,
        rn_device_agent_rollout_enabled=True,
        rn_device_agent_rollout_percentage=100,
        device_agent_rollout_require_shadow_ready=False,
        device_agent_fallback_enabled=True,
        device_agent_gateway_enabled=True,
        device_agent_gateway_api_key="test-provider-key",
        pi_agent_api_key="test-server-key",
    )
    client, service, _, _ = build_client(settings)
    service.device.capabilities = {
        "client.reactNative": True,
        "agent.submitAnalysis": True,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.schema": 1,
    }

    supported = client.get("/devices/current/capabilities")
    service.device.capabilities["agent.runtime"] = "pi-0.80.7"
    wrong_runtime = client.get("/devices/current/capabilities")
    service.device.capabilities["agent.runtime"] = "pi-0.80.6"
    service.device.capabilities["agent.schema"] = True
    boolean_schema = client.get("/devices/current/capabilities")

    assert supported.status_code == 200
    assert supported.json()["deviceAgentAvailable"] is True
    assert wrong_runtime.json()["deviceAgentAvailable"] is False
    assert boolean_schema.json()["deviceAgentAvailable"] is False


def test_interactive_agent_capability_requires_all_gates_and_allowlisted_device() -> None:
    base = {
        "database_url": "postgresql+asyncpg://user:password@localhost/test",
        "admin_api_token": "test-admin-token",
        "interactive_agent_beta_enabled": True,
        "interactive_agent_gateway_enabled": True,
        "interactive_agent_beta_monthly_turn_limit": 10,
        "device_agent_gateway_api_key": "test-provider-key",
    }
    client, service, _, _ = build_client()
    service.device.capabilities = {
        "client.reactNative": True,
        "sqlite.schema": 5,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.interactive": True,
        "agent.interactiveSchema": 1,
    }
    settings = Settings(
        **base,
        interactive_agent_device_allowlist=str(service.device.id),
    )
    client.app.dependency_overrides[get_settings] = lambda: settings

    supported = client.get("/devices/current/capabilities")
    service.device.capabilities["agent.interactiveSchema"] = 2
    newer_client = client.get("/devices/current/capabilities")
    service.device.capabilities["agent.interactiveSchema"] = 0
    unsupported_client = client.get("/devices/current/capabilities")
    service.device.capabilities["agent.interactiveSchema"] = 1
    settings.interactive_agent_gateway_enabled = False
    killed = client.get("/devices/current/capabilities")

    assert supported.status_code == 200
    assert supported.json()["agentToolsAvailable"] is True
    assert newer_client.json()["agentToolsAvailable"] is True
    assert unsupported_client.json()["agentToolsAvailable"] is False
    assert killed.json()["agentToolsAvailable"] is False


def test_shadow_capability_is_independent_from_primary_rollout() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_device_agent_rollout_enabled=False,
        device_agent_shadow_enabled=True,
        device_agent_gateway_enabled=True,
        device_agent_gateway_api_key="test-provider-key",
    )
    client, service, _, _ = build_client(settings)
    service.device.capabilities = {
        "client.reactNative": True,
        "agent.submitAnalysis": True,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.schema": 1,
    }

    response = client.get("/devices/current/capabilities")

    assert response.status_code == 200
    assert response.json()["deviceAgentAvailable"] is True


def test_push_capability_requires_rollout_sync_schema_and_platform_credentials() -> None:
    settings = Settings(
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
    client, service, _, _ = build_client(settings)
    service.device.capabilities = {
        "client.reactNative": True,
        "push.environment": "production",
        "sqlite.schema": 3,
    }

    response = client.get("/devices/current/capabilities")

    assert response.status_code == 200
    assert response.json()["syncAvailable"] is True
    assert response.json()["pushAvailable"] is True

    service.device.capabilities["sqlite.schema"] = 2
    old_push_schema = client.get("/devices/current/capabilities")
    assert old_push_schema.json()["syncAvailable"] is True
    assert old_push_schema.json()["pushAvailable"] is False

    service.device.capabilities["sqlite.schema"] = 3
    service.device.capabilities["push.environment"] = "sandbox"
    wrong_environment = client.get("/devices/current/capabilities")
    assert wrong_environment.json()["pushAvailable"] is False


def test_push_registration_never_echoes_the_native_token_and_revoke_is_scoped() -> None:
    client, service, push_service, user = build_client()
    native_token = "native-token-value-that-must-never-be-echoed"

    registered = client.put(
        "/devices/current/push-registration",
        json={"provider": "apns", "environment": "production", "token": native_token},
    )
    revoked = client.delete("/devices/current/push-registration/apns/production")

    assert registered.status_code == 200
    assert native_token not in registered.text
    assert registered.json()["tokenFingerprint"] == "0123456789ab"
    assert push_service.request.token == native_token
    assert revoked.status_code == 204
    assert push_service.revoked == (
        user.id,
        service.device.id,
        PushProvider.APNS,
        PushEnvironment.PRODUCTION,
    )
