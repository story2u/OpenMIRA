from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Request, Response, status

from app.api.deps import get_adapter_registry, get_wecom_connection_repo, require_user
from app.application.dto import WeComConnectionCreate, WeComConnectionRead, WeComSourceRead
from app.application.mappers import to_wecom_connection_read, to_wecom_source_read
from app.core.config import Settings, get_settings
from app.core.security import encrypt_secret
from app.domain.enums import IMChannel, WeComConnectionType
from app.infrastructure.db.models import User, WeComConnection
from app.infrastructure.db.repositories import WeComConnectionRepository
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.wecom import WeComAdapter, WeComCrypto, WeComProviderError

router = APIRouter()


def _callback_url(request: Request, connection_id: UUID) -> str:
    return str(
        request.url_for(
            "wecom_connection_verify_url",
            connection_id=str(connection_id),
        )
    )


@router.get("/connections", response_model=list[WeComConnectionRead])
async def list_connections(
    request: Request,
    current_user: User = Depends(require_user),
    repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
) -> list[WeComConnectionRead]:
    connections = await repo.list_for_owner(current_user.id)
    sources = await repo.list_sources_for_owner(current_user.id)
    by_connection: dict[UUID, list] = {}
    for source in sources:
        by_connection.setdefault(source.connection_id, []).append(source)
    return [
        to_wecom_connection_read(
            connection,
            callback_url=_callback_url(request, connection.id),
            sources=by_connection.get(connection.id, []),
        )
        for connection in connections
    ]


@router.get("/sources", response_model=list[WeComSourceRead])
async def list_sources(
    current_user: User = Depends(require_user),
    repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
) -> list[WeComSourceRead]:
    return [
        to_wecom_source_read(source)
        for source in await repo.list_sources_for_owner(current_user.id)
    ]


@router.post(
    "/connections",
    response_model=WeComConnectionRead,
    status_code=status.HTTP_201_CREATED,
)
async def create_connection(
    payload: WeComConnectionCreate,
    request: Request,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
) -> WeComConnectionRead:
    if await repo.count_enabled_for_owner(current_user.id) >= settings.wecom_connection_limit:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"current account allows {settings.wecom_connection_limit} WeCom connection",
        )
    try:
        WeComCrypto(payload.token, payload.encodingAesKey, payload.corpId)
        connection = await repo.create(
            WeComConnection(
                owner_user_id=current_user.id,
                connection_type=WeComConnectionType.INTERNAL_APP,
                display_name=payload.displayName.strip(),
                corp_id=payload.corpId.strip(),
                agent_id=payload.agentId,
                secret_encrypted=encrypt_secret(payload.secret, settings),
                token_encrypted=encrypt_secret(payload.token, settings),
                aes_key_encrypted=encrypt_secret(payload.encodingAesKey, settings),
            )
        )
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    return to_wecom_connection_read(
        connection,
        callback_url=_callback_url(request, connection.id),
        sources=[],
    )


@router.post("/connections/{connection_id}/verify", response_model=WeComConnectionRead)
async def verify_connection(
    connection_id: UUID,
    request: Request,
    current_user: User = Depends(require_user),
    repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
) -> WeComConnectionRead:
    connection = await repo.get_for_owner(connection_id, current_user.id)
    if not connection or not connection.enabled:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="wecom connection not found"
        )
    adapter = adapters.get(IMChannel.WECOM)
    if not isinstance(adapter, WeComAdapter):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR, detail="wecom adapter unavailable"
        )
    try:
        await adapter.verify_credentials(connection)
        connection = await repo.mark_verified(connection)
    except (WeComProviderError, ValueError) as exc:
        await repo.mark_error(connection, exc.__class__.__name__)
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="WeCom credential verification failed",
        ) from exc
    sources = [
        source
        for source in await repo.list_sources_for_owner(current_user.id)
        if source.connection_id == connection.id
    ]
    return to_wecom_connection_read(
        connection,
        callback_url=_callback_url(request, connection.id),
        sources=sources,
    )


@router.delete("/connections/{connection_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_connection(
    connection_id: UUID,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
) -> Response:
    connection = await repo.get_for_owner(connection_id, current_user.id)
    if not connection:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="wecom connection not found"
        )
    await repo.disable_and_clear_secrets(
        connection,
        cleared_secret=encrypt_secret("", settings),
    )
    return Response(status_code=status.HTTP_204_NO_CONTENT)
