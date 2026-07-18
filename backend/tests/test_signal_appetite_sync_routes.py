import os
from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.deps import (
    DevicePrincipal,
    get_signal_appetite_sync_service,
    require_device_principal,
)
from app.api.v1.routes import sync as sync_route
from app.application.dto import (
    SignalAppetiteEventRead,
    SignalAppetiteEventsAppendRead,
    SignalAppetiteEventsPageRead,
)
from app.core.config import Settings, get_settings
from app.domain.enums import DevicePlatform
from app.infrastructure.db.models import Device, User, utc_now
from app.infrastructure.db.signal_appetite_repository import SignalAppetiteEventConflictError


class FakeSignalAppetiteService:
    appended = None
    conflict = False

    async def append(self, *, owner_user_id, device_id, events):
        if self.conflict:
            raise SignalAppetiteEventConflictError
        self.appended = (owner_user_id, device_id, events)
        now = utc_now()
        return SignalAppetiteEventsAppendRead(
            events=[
                SignalAppetiteEventRead(
                    **event.model_dump(),
                    ownerId=owner_user_id,
                    deviceId=device_id,
                    cursor=index,
                    serverReceivedAt=now,
                )
                for index, event in enumerate(events, 1)
            ],
            serverCursor=len(events),
        )

    async def list_events(self, *, owner_user_id, after, limit):
        del owner_user_id, limit
        return SignalAppetiteEventsPageRead(
            events=[], nextCursor=after, serverCursor=after, hasMore=False
        )


def build_client(*, enabled: bool = True):
    user = User(id=uuid4(), email="appetite-sync@example.test")
    device = Device(
        id=uuid4(),
        owner_user_id=user.id,
        installation_id_hash="a" * 64,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={"client.reactNative": True, "sqlite.schema": 6},
    )
    principal = DevicePrincipal(user=user, device=device)
    service = FakeSignalAppetiteService()
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        signal_appetite_sync_enabled=enabled,
    )
    app = FastAPI()
    app.include_router(sync_route.router, prefix="/sync")
    app.dependency_overrides[require_device_principal] = lambda: principal
    app.dependency_overrides[get_signal_appetite_sync_service] = lambda: service
    app.dependency_overrides[get_settings] = lambda: settings
    return TestClient(app), service, principal


def event_payload():
    return {
        "events": [
            {
                "eventId": str(uuid4()),
                "eventType": "TeachingSessionStarted",
                "aggregateId": str(uuid4()),
                "aggregateVersion": 1,
                "schemaVersion": 1,
                "occurredAt": "2026-07-18T12:00:00Z",
                "payload": {"sessionId": str(uuid4()), "targetCount": 8},
            }
        ]
    }


def test_append_binds_owner_and_device_and_is_content_free() -> None:
    client, service, principal = build_client()

    response = client.post("/sync/signal-appetite/events", json=event_payload())

    assert response.status_code == 200
    assert service.appended[0:2] == (principal.user.id, principal.device.id)
    assert response.json()["events"][0]["ownerId"] == str(principal.user.id)

    with_body = event_payload()
    with_body["events"][0]["payload"]["messageBody"] = "private message"
    assert client.post("/sync/signal-appetite/events", json=with_body).status_code == 422


def test_rollout_gate_and_conflicting_event_id_fail_closed() -> None:
    disabled, _, _ = build_client(enabled=False)
    assert disabled.get("/sync/signal-appetite/events").status_code == 404

    client, service, _ = build_client()
    service.conflict = True
    response = client.post("/sync/signal-appetite/events", json=event_payload())
    assert response.status_code == 409
