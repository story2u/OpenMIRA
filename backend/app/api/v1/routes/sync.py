from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.api.deps import DevicePrincipal, get_sync_feed_service, require_device_principal
from app.application.dto import (
    SyncAckRead,
    SyncAckRequest,
    SyncBootstrapRead,
    SyncChangesRead,
)
from app.application.use_cases.sync_feed import InvalidSyncPageToken, SyncFeedService
from app.infrastructure.db.sync_repository import (
    SyncCursorAheadError,
    SyncDeviceUnavailableError,
)

router = APIRouter()


@router.get("/bootstrap", response_model=SyncBootstrapRead)
async def bootstrap_sync(
    principal: DevicePrincipal = Depends(require_device_principal),
    service: SyncFeedService = Depends(get_sync_feed_service),
    limit: Annotated[int, Query(ge=1, le=500)] = 200,
    page_token: Annotated[str | None, Query(alias="pageToken", max_length=2048)] = None,
) -> SyncBootstrapRead:
    try:
        return await service.bootstrap(
            owner_user_id=principal.user.id,
            limit=limit,
            page_token=page_token,
        )
    except InvalidSyncPageToken as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail="invalid sync bootstrap page token",
        ) from exc


@router.get("/changes", response_model=SyncChangesRead)
async def list_sync_changes(
    principal: DevicePrincipal = Depends(require_device_principal),
    service: SyncFeedService = Depends(get_sync_feed_service),
    after: Annotated[int, Query(ge=0)] = 0,
    limit: Annotated[int, Query(ge=1, le=500)] = 200,
) -> SyncChangesRead:
    return await service.changes(
        owner_user_id=principal.user.id,
        after=after,
        limit=limit,
    )


@router.post("/ack", response_model=SyncAckRead)
async def acknowledge_sync(
    payload: SyncAckRequest,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: SyncFeedService = Depends(get_sync_feed_service),
) -> SyncAckRead:
    assert principal.device is not None
    try:
        device = await service.acknowledge(
            owner_user_id=principal.user.id,
            device_id=principal.device.id,
            cursor=payload.cursor,
            error_code=payload.errorCode,
        )
    except SyncCursorAheadError as exc:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="sync cursor exceeds stream head",
        ) from exc
    except SyncDeviceUnavailableError as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="inactive device",
        ) from exc
    assert device.last_sync_at is not None
    return SyncAckRead(
        deviceId=device.id,
        acknowledgedCursor=device.last_sync_cursor,
        acknowledgedAt=device.last_sync_at,
        errorCode=device.last_sync_error_code,
    )
