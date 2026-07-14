from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Response, status
from redis.asyncio import Redis

from app.api.deps import get_redis_client, get_wecom_archive_repo, require_user
from app.application.dto import (
    WeComArchiveConnectionCreate,
    WeComArchiveConnectionRead,
    WeComArchiveSyncAccepted,
)
from app.application.mappers import to_wecom_archive_connection_read
from app.core.config import Settings, get_settings
from app.core.security import encrypt_secret
from app.infrastructure.db.models import User, WeComArchiveConnection
from app.infrastructure.db.repositories import WeComArchiveRepository
from app.infrastructure.im.wecom_archive import validate_wecom_archive_private_key
from app.worker.queue import CeleryTaskQueue

router = APIRouter()


async def _read_connection(
    connection: WeComArchiveConnection,
    *,
    owner: User,
    repository: WeComArchiveRepository,
    settings: Settings,
) -> WeComArchiveConnectionRead:
    binding = await repository.binding_for_user(connection.id, owner.id)
    cursor = await repository.cursor_for_connection(connection.id)
    if not binding or not cursor:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="WeCom archive connection is incomplete",
        )
    sources = [
        source
        for source in await repository.list_sources_for_owner(owner.id)
        if source.archive_connection_id == connection.id
    ]
    return to_wecom_archive_connection_read(
        connection,
        binding=binding,
        cursor=cursor,
        sdk_configured=settings.wecom_archive_sdk_configured,
        sources=sources,
    )


@router.get("/archive-connections", response_model=list[WeComArchiveConnectionRead])
async def list_archive_connections(
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repository: WeComArchiveRepository = Depends(get_wecom_archive_repo),
) -> list[WeComArchiveConnectionRead]:
    return [
        await _read_connection(
            connection, owner=current_user, repository=repository, settings=settings
        )
        for connection in await repository.list_for_owner(current_user.id)
    ]


@router.post(
    "/archive-connections",
    response_model=WeComArchiveConnectionRead,
    status_code=status.HTTP_201_CREATED,
)
async def create_archive_connection(
    payload: WeComArchiveConnectionCreate,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repository: WeComArchiveRepository = Depends(get_wecom_archive_repo),
) -> WeComArchiveConnectionRead:
    if (
        await repository.count_enabled_for_owner(current_user.id)
        >= settings.wecom_archive_connection_limit
    ):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=(
                "current account allows "
                f"{settings.wecom_archive_connection_limit} WeCom archive connection"
            ),
        )
    try:
        validate_wecom_archive_private_key(payload.privateKeyPem)
        connection, _, _ = await repository.create_with_owner_binding(
            connection=WeComArchiveConnection(
                owner_user_id=current_user.id,
                display_name=payload.displayName.strip(),
                corp_id=payload.corpId.strip(),
                secret_encrypted=encrypt_secret(payload.archiveSecret, settings),
                private_key_encrypted=encrypt_secret(payload.privateKeyPem, settings),
                public_key_version=payload.publicKeyVersion,
            ),
            wecom_user_id=payload.wecomUserId.strip(),
            member_display_name=payload.memberDisplayName.strip(),
        )
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    return await _read_connection(
        connection, owner=current_user, repository=repository, settings=settings
    )


async def _enqueue_sync(
    connection_id: UUID,
    *,
    verifying: bool,
    current_user: User,
    settings: Settings,
    repository: WeComArchiveRepository,
    redis: Redis,
) -> WeComArchiveSyncAccepted:
    connection = await repository.get_for_owner(connection_id, current_user.id)
    if not connection or not connection.enabled:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="WeCom archive connection not found",
        )
    if not settings.wecom_archive_sdk_configured:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="WeCom archive SDK is not configured on the server",
        )
    rate_key = f"wecom:archive:manual-sync:{current_user.id}:{connection.id}"
    if not await redis.set(
        rate_key, "1", ex=settings.wecom_archive_sync_rate_limit_seconds, nx=True
    ):
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail="WeCom archive sync was requested recently",
        )
    if not CeleryTaskQueue().enqueue_wecom_archive_sync(
        connection.id, verifying=verifying
    ):
        await redis.delete(rate_key)
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="WeCom archive queue is unavailable",
        )
    return WeComArchiveSyncAccepted()


@router.post(
    "/archive-connections/{connection_id}/verify",
    response_model=WeComArchiveSyncAccepted,
    status_code=status.HTTP_202_ACCEPTED,
)
async def verify_archive_connection(
    connection_id: UUID,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repository: WeComArchiveRepository = Depends(get_wecom_archive_repo),
    redis: Redis = Depends(get_redis_client),
) -> WeComArchiveSyncAccepted:
    return await _enqueue_sync(
        connection_id,
        verifying=True,
        current_user=current_user,
        settings=settings,
        repository=repository,
        redis=redis,
    )


@router.post(
    "/archive-connections/{connection_id}/sync",
    response_model=WeComArchiveSyncAccepted,
    status_code=status.HTTP_202_ACCEPTED,
)
async def sync_archive_connection(
    connection_id: UUID,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repository: WeComArchiveRepository = Depends(get_wecom_archive_repo),
    redis: Redis = Depends(get_redis_client),
) -> WeComArchiveSyncAccepted:
    return await _enqueue_sync(
        connection_id,
        verifying=False,
        current_user=current_user,
        settings=settings,
        repository=repository,
        redis=redis,
    )


@router.delete(
    "/archive-connections/{connection_id}", status_code=status.HTTP_204_NO_CONTENT
)
async def delete_archive_connection(
    connection_id: UUID,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repository: WeComArchiveRepository = Depends(get_wecom_archive_repo),
) -> Response:
    connection = await repository.get_for_owner(connection_id, current_user.id)
    if not connection:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="WeCom archive connection not found",
        )
    await repository.disable_and_clear_secrets(
        connection, cleared_secret=encrypt_secret("", settings)
    )
    return Response(status_code=status.HTTP_204_NO_CONTENT)
