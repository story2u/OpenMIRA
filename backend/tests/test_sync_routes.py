import os
from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.deps import DevicePrincipal, get_sync_feed_service, require_device_principal
from app.api.v1.routes import sync as sync_route
from app.application.dto import (
    SyncBootstrapRead,
    SyncChangeRead,
    SyncChangesRead,
    SyncSnapshotItemRead,
)
from app.application.use_cases.sync_feed import InvalidSyncPageToken
from app.domain.enums import DevicePlatform, SyncAggregateType, SyncOperation
from app.infrastructure.db.models import Device, User, utc_now
from app.infrastructure.db.sync_repository import SyncCursorAheadError


class FakeSyncFeedService:
    def __init__(self, device: Device) -> None:
        self.device = device
        self.raise_invalid_page = False
        self.raise_cursor_ahead = False
        self.acknowledged = None

    async def bootstrap(self, *, owner_user_id, limit, page_token):
        if self.raise_invalid_page:
            raise InvalidSyncPageToken
        return SyncBootstrapRead(
            watermarkCursor=9,
            items=[
                SyncSnapshotItemRead(
                    aggregateType=SyncAggregateType.USER_NOTIFICATION_PREFERENCE,
                    aggregateId=owner_user_id,
                    aggregateVersion=0,
                    payload={
                        "newOpportunityEnabled": True,
                        "aiRepliedEnabled": True,
                        "dailyDigestEnabled": False,
                        "urgentOnly": False,
                    },
                )
            ][:limit],
            hasMore=bool(page_token),
            nextPageToken="next-token" if page_token else None,
        )

    async def changes(self, *, owner_user_id, after, limit):
        del owner_user_id, limit
        if after == 99:
            return SyncChangesRead(
                changes=[],
                nextCursor=after,
                serverCursor=9,
                hasMore=False,
                resetRequired=True,
                resetReason="cursor_ahead",
            )
        return SyncChangesRead(
            changes=[
                SyncChangeRead(
                    eventId=uuid4(),
                    cursor=after + 1,
                    aggregateType=SyncAggregateType.MESSAGE,
                    aggregateId=uuid4(),
                    aggregateVersion=1,
                    operation=SyncOperation.UPSERT,
                    schemaVersion=1,
                    createdAt=utc_now(),
                    payload={"content": "hello"},
                )
            ],
            nextCursor=after + 1,
            serverCursor=after + 1,
            hasMore=False,
        )

    async def acknowledge(
        self,
        *,
        owner_user_id,
        device_id,
        cursor,
        error_code,
    ):
        if self.raise_cursor_ahead:
            raise SyncCursorAheadError
        self.acknowledged = (owner_user_id, device_id, cursor, error_code)
        self.device.last_sync_cursor = max(self.device.last_sync_cursor, cursor)
        self.device.last_sync_at = utc_now()
        self.device.last_sync_error_code = error_code
        return self.device


def build_client() -> tuple[TestClient, FakeSyncFeedService, DevicePrincipal]:
    user = User(id=uuid4(), email="sync-route@example.test")
    device = Device(
        owner_user_id=user.id,
        installation_id_hash="a" * 64,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={"sync.schema": 1},
    )
    principal = DevicePrincipal(user=user, device=device)
    service = FakeSyncFeedService(device)
    app = FastAPI()
    app.include_router(sync_route.router, prefix="/sync")
    app.dependency_overrides[require_device_principal] = lambda: principal
    app.dependency_overrides[get_sync_feed_service] = lambda: service
    return TestClient(app), service, principal


def test_bootstrap_and_changes_keep_contract_bounded_and_explicit() -> None:
    client, _, _ = build_client()

    bootstrap = client.get("/sync/bootstrap?limit=1")
    changes = client.get("/sync/changes?after=4&limit=1")
    reset = client.get("/sync/changes?after=99")
    invalid_limit = client.get("/sync/changes?after=0&limit=501")

    assert bootstrap.status_code == 200
    assert bootstrap.json()["watermarkCursor"] == 9
    assert bootstrap.json()["items"][0]["aggregateType"] == "user_notification_preference"
    assert changes.status_code == 200
    assert changes.json()["changes"][0]["operation"] == "upsert"
    assert reset.json()["resetRequired"] is True
    assert reset.json()["resetReason"] == "cursor_ahead"
    assert invalid_limit.status_code == 422


def test_invalid_bootstrap_token_has_stable_nonsecret_error() -> None:
    client, service, _ = build_client()
    service.raise_invalid_page = True

    response = client.get("/sync/bootstrap?pageToken=not-a-valid-token")

    assert response.status_code == 422
    assert response.json() == {"detail": "invalid sync bootstrap page token"}
    assert "not-a-valid-token" not in response.text


def test_ack_uses_authenticated_device_and_rejects_cursor_ahead() -> None:
    client, service, principal = build_client()

    acknowledged = client.post(
        "/sync/ack",
        json={"cursor": 7, "errorCode": "projection.retry"},
    )
    service.raise_cursor_ahead = True
    ahead = client.post("/sync/ack", json={"cursor": 99})
    invalid_error = client.post(
        "/sync/ack",
        json={"cursor": 7, "errorCode": "Unsafe Error With Spaces"},
    )

    assert acknowledged.status_code == 200
    assert acknowledged.json()["deviceId"] == str(principal.device.id)
    assert acknowledged.json()["acknowledgedCursor"] == 7
    assert service.acknowledged == (
        principal.user.id,
        principal.device.id,
        7,
        "projection.retry",
    )
    assert ahead.status_code == 409
    assert ahead.json() == {"detail": "sync cursor exceeds stream head"}
    assert invalid_error.status_code == 422
