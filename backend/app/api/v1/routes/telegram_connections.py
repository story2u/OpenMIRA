from datetime import datetime, timedelta
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Response, status
from pydantic import BaseModel
from sqlalchemy.exc import IntegrityError
from telethon import TelegramClient
from telethon.sessions import StringSession
from telethon.tl.types import Channel, Chat

from app.api.deps import (
    get_subscription_repo,
    get_telegram_connection_repo,
    get_telegram_user_config_repo,
    require_user,
)
from app.application.dto import (
    TelegramConnectionAttemptRead,
    TelegramConnectionHealthRead,
    TelegramConnectionRead,
    TelegramMtprotoDialogRead,
    TelegramMtprotoSourceCreate,
)
from app.application.mappers import (
    to_telegram_connection_attempt_read,
    to_telegram_connection_read,
)
from app.core.config import Settings, get_settings
from app.core.security import decrypt_secret
from app.core.telegram_connection_tokens import (
    create_chat_request_ids,
    create_connection_token,
    hash_connection_token,
)
from app.domain.enums import TelegramConnectionType, TelegramSourceType
from app.domain.services.subscription_policy import GroupQuotaExceeded, telegram_group_capacity
from app.infrastructure.db.models import TelegramConnectionAttempt, User, utc_now
from app.infrastructure.db.repositories import (
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
)

router = APIRouter()


class TelegramConnectionUpdate(BaseModel):
    enabled: bool


def is_local_mock(settings: Settings) -> bool:
    return settings.app_env != "prod" and settings.telegram_integration_mode == "mock"


def bot_deep_link(settings: Settings, raw_token: str, prefix: str) -> str:
    username = settings.telegram_bot_username.lstrip("@")
    return f"https://t.me/{username}?start={prefix}_{raw_token}"


async def create_connection_attempt(
    *,
    connection_repo: TelegramConnectionRepository,
    owner_user_id: UUID,
    connection_type: TelegramConnectionType,
    expires_at: datetime,
    with_chat_picker: bool,
) -> tuple[TelegramConnectionAttempt, str]:
    """Retry the extremely unlikely token/request-ID collision without retaining raw tokens."""
    for _ in range(3):
        raw_token = create_connection_token()
        group_request_id, channel_request_id = (
            create_chat_request_ids() if with_chat_picker else (None, None)
        )
        try:
            attempt = await connection_repo.create_attempt(
                owner_user_id=owner_user_id,
                connection_type=connection_type,
                token_hash=hash_connection_token(raw_token),
                expires_at=expires_at,
                group_request_id=group_request_id,
                channel_request_id=channel_request_id,
            )
        except IntegrityError:
            continue
        return attempt, raw_token
    raise HTTPException(
        status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
        detail="could not create a Telegram connection attempt; retry shortly",
    )


async def list_connection_reads(
    *,
    current_user: User,
    connection_repo: TelegramConnectionRepository,
    legacy_repo: TelegramUserConfigRepository,
    subscription_repo: SubscriptionRepository,
) -> list[TelegramConnectionRead]:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    legacy_active_count = await legacy_repo.count_active_monitors_by_user(current_user.id)
    await connection_repo.reconcile_source_quota_for_user(
        owner_user_id=current_user.id,
        capacity=telegram_group_capacity(snapshot.entitlements),
        legacy_active_count=legacy_active_count,
    )
    connections = await connection_repo.list_connections_for_owner(current_user.id)
    sources_by_connection: dict[UUID, list] = {connection.id: [] for connection in connections}
    for source in await connection_repo.list_sources_for_owner(current_user.id):
        sources_by_connection.setdefault(source.connection_id, []).append(source)
    return [
        to_telegram_connection_read(connection, sources_by_connection.get(connection.id, []))
        for connection in connections
    ]


@router.get("/health", response_model=TelegramConnectionHealthRead)
async def get_health(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    legacy_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
) -> TelegramConnectionHealthRead:
    production_mock = settings.app_env == "prod" and settings.telegram_integration_mode == "mock"
    legacy_config = await legacy_repo.get_by_user(current_user.id)
    legacy_active_count = await legacy_repo.count_active_monitors_by_user(current_user.id)
    return TelegramConnectionHealthRead(
        mode=settings.telegram_integration_mode,
        botConfigured=settings.telegram_bot_configured and not production_mock,
        botUsername=settings.telegram_bot_username or None,
        businessAvailable=settings.telegram_bot_configured and not production_mock,
        mtprotoQrAvailable=settings.telegram_mtproto_qr_available,
        listenerMode="vps-long-running",
        legacyMonitoringActive=bool(legacy_config and legacy_config.enabled),
        legacyActiveSourceCount=legacy_active_count,
        message=(
            "生产环境禁止使用 Telegram mock adapter"
            if production_mock
            else (
                None
                if settings.telegram_mtproto_qr_available
                else "普通账号 QR 尚未由管理员配置；不会收集用户 API Hash、手机号、验证码或 Session。"
            )
        ),
    )


@router.get("/connections", response_model=list[TelegramConnectionRead])
async def list_connections(
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
    legacy_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> list[TelegramConnectionRead]:
    return await list_connection_reads(
        current_user=current_user,
        connection_repo=connection_repo,
        legacy_repo=legacy_repo,
        subscription_repo=subscription_repo,
    )


@router.post("/connect/bot-chat", response_model=TelegramConnectionAttemptRead)
async def start_bot_chat_connection(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
    legacy_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramConnectionAttemptRead:
    local_mock = is_local_mock(settings)
    if not local_mock:
        if settings.app_env == "prod" and settings.telegram_integration_mode != "live":
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail="Telegram mock is disabled in production",
            )
        if not settings.telegram_bot_configured:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail="Telegram Bot is not configured by the administrator",
            )
    expires_at = utc_now() + timedelta(seconds=settings.telegram_connect_ttl_seconds)
    attempt, raw_token = await create_connection_attempt(
        connection_repo=connection_repo,
        owner_user_id=current_user.id,
        connection_type=TelegramConnectionType.BOT_CHAT,
        expires_at=expires_at,
        with_chat_picker=True,
    )
    if local_mock:
        snapshot = await subscription_repo.get_snapshot(current_user.id)
        legacy_active_count = await legacy_repo.count_active_monitors_by_user(current_user.id)
        try:
            await connection_repo.complete_bot_chat(
                attempt=attempt,
                external_chat_id="mock-bot-chat",
                source_type=TelegramSourceType.GROUP,
                display_name="本地测试群（Mock）",
                username=None,
                entitlements=snapshot.entitlements,
                legacy_active_count=legacy_active_count,
            )
        except GroupQuotaExceeded as exc:
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(exc)) from exc
        completed = await connection_repo.get_attempt_for_owner(
            owner_user_id=current_user.id,
            attempt_id=attempt.id,
        )
        assert completed is not None
        return to_telegram_connection_attempt_read(
            completed,
            instructions=["本地 mock adapter 已创建测试来源；它不会连接或接收真实 Telegram 消息。"],
            local_mock=True,
        )
    return to_telegram_connection_attempt_read(
        attempt,
        telegram_url=bot_deep_link(settings, raw_token, "connect"),
        instructions=[
            "在 Telegram 中打开机器人并点击开始。",
            "将机器人添加到目标群或频道后，在机器人私聊中选择该会话。",
            "完成后本页面会自动刷新连接状态。",
        ],
    )


@router.post("/connect/business", response_model=TelegramConnectionAttemptRead)
async def start_business_connection(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> TelegramConnectionAttemptRead:
    if is_local_mock(settings):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Telegram Business cannot be verified by the local mock adapter",
        )
    if settings.app_env == "prod" and settings.telegram_integration_mode != "live":
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Telegram mock is disabled in production",
        )
    if not settings.telegram_bot_configured:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Telegram Bot is not configured by the administrator",
        )
    attempt, raw_token = await create_connection_attempt(
        connection_repo=connection_repo,
        owner_user_id=current_user.id,
        connection_type=TelegramConnectionType.BUSINESS,
        expires_at=utc_now() + timedelta(seconds=settings.telegram_connect_ttl_seconds),
        with_chat_picker=False,
    )
    return to_telegram_connection_attempt_read(
        attempt,
        telegram_url=bot_deep_link(settings, raw_token, "business"),
        instructions=[
            "先在 Telegram 中打开机器人并点击开始，以确认你的 Business 账号。",
            "然后在 Telegram 的 Business 设置中添加该机器人。",
            "收到 Telegram 的 Business connection 回调后，本页面会显示已连接。",
        ],
    )


@router.post("/connect/mtproto-qr", response_model=TelegramConnectionAttemptRead)
async def start_mtproto_qr_connection(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> TelegramConnectionAttemptRead:
    if not settings.telegram_mtproto_qr_available:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="MTProto QR worker is not configured by the administrator",
        )
    attempt = await connection_repo.get_pending_attempt_for_owner(
        owner_user_id=current_user.id,
        connection_type=TelegramConnectionType.MTPROTO_QR,
    )
    if attempt is None:
        try:
            attempt, _ = await create_connection_attempt(
                connection_repo=connection_repo,
                owner_user_id=current_user.id,
                connection_type=TelegramConnectionType.MTPROTO_QR,
                expires_at=utc_now() + timedelta(seconds=settings.telegram_connect_ttl_seconds),
                with_chat_picker=False,
            )
        except HTTPException as exc:
            # A concurrent request may have won the partial unique-index race.
            attempt = await connection_repo.get_pending_attempt_for_owner(
                owner_user_id=current_user.id,
                connection_type=TelegramConnectionType.MTPROTO_QR,
            )
            if attempt is None:
                raise exc
    return to_telegram_connection_attempt_read(
        attempt,
        instructions=["正在生成二维码。请使用已登录的 Telegram 客户端扫码确认。", "二维码过期后请重新开始。"],
    )


@router.get("/connect/attempts/{attempt_id}", response_model=TelegramConnectionAttemptRead)
async def get_connection_attempt(
    attempt_id: UUID,
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> TelegramConnectionAttemptRead:
    attempt = await connection_repo.get_attempt_for_owner(
        owner_user_id=current_user.id,
        attempt_id=attempt_id,
    )
    if not attempt:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="connection attempt not found"
        )
    qr_code_url = None
    if attempt.connection_type == TelegramConnectionType.MTPROTO_QR and attempt.qr_url_encrypted:
        try:
            qr_code_url = decrypt_secret(attempt.qr_url_encrypted, get_settings())
        except ValueError:
            await connection_repo.fail_attempt(attempt=attempt, error="QR login state could not be recovered")
    return to_telegram_connection_attempt_read(attempt, qr_code_url=qr_code_url)


async def mtproto_client_for_connection(
    *,
    connection_repo: TelegramConnectionRepository,
    owner_user_id: UUID,
    connection_id: UUID,
    settings: Settings,
) -> TelegramClient:
    connection = await connection_repo.get_connection_for_owner(
        owner_user_id=owner_user_id, connection_id=connection_id
    )
    if (
        not connection
        or connection.connection_type != TelegramConnectionType.MTPROTO_QR
        or not connection.credential_encrypted
        or not settings.telegram_mtproto_qr_available
    ):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="MTProto connection not found")
    try:
        session_string = decrypt_secret(connection.credential_encrypted, settings)
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="MTProto session is unavailable") from exc
    return TelegramClient(
        StringSession(session_string),
        settings.telegram_mtproto_api_id,
        settings.telegram_mtproto_api_hash,
    )


@router.get("/connections/{connection_id}/dialogs", response_model=list[TelegramMtprotoDialogRead])
async def list_mtproto_dialogs(
    connection_id: UUID,
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> list[TelegramMtprotoDialogRead]:
    client = await mtproto_client_for_connection(
        connection_repo=connection_repo,
        owner_user_id=current_user.id,
        connection_id=connection_id,
        settings=settings,
    )
    try:
        await client.connect()
        dialogs: list[TelegramMtprotoDialogRead] = []
        async for dialog in client.iter_dialogs(limit=100):
            entity = dialog.entity
            if not isinstance(entity, (Chat, Channel)):
                continue
            dialogs.append(
                TelegramMtprotoDialogRead(
                    id=str(dialog.id),
                    sourceType=(
                        TelegramSourceType.CHANNEL if getattr(entity, "broadcast", False) else TelegramSourceType.GROUP
                    ),
                    displayName=dialog.name or "Telegram 群组",
                    username=getattr(entity, "username", None),
                )
            )
        return dialogs
    finally:
        await client.disconnect()


@router.post("/connections/{connection_id}/sources", response_model=TelegramConnectionRead)
async def add_mtproto_source(
    connection_id: UUID,
    payload: TelegramMtprotoSourceCreate,
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
    legacy_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramConnectionRead:
    client = await mtproto_client_for_connection(
        connection_repo=connection_repo,
        owner_user_id=current_user.id,
        connection_id=connection_id,
        settings=settings,
    )
    try:
        await client.connect()
        entity = None
        async for dialog in client.iter_dialogs(limit=100):
            if str(dialog.id) == payload.chatId:
                entity = dialog.entity
                break
        if not isinstance(entity, (Chat, Channel)):
            raise HTTPException(
                status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
                detail="only groups and channels can be monitored",
            )
        source_type = TelegramSourceType.CHANNEL if getattr(entity, "broadcast", False) else TelegramSourceType.GROUP
        snapshot = await subscription_repo.get_snapshot(current_user.id)
        await connection_repo.add_mtproto_source(
            owner_user_id=current_user.id,
            connection_id=connection_id,
            external_chat_id=payload.chatId,
            source_type=source_type,
            display_name=getattr(entity, "title", None) or "Telegram 群组",
            username=getattr(entity, "username", None),
            entitlements=snapshot.entitlements,
            legacy_active_count=await legacy_repo.count_active_monitors_by_user(current_user.id),
        )
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="invalid Telegram group") from exc
    finally:
        await client.disconnect()
    connection = await connection_repo.get_connection_for_owner(
        owner_user_id=current_user.id, connection_id=connection_id
    )
    assert connection is not None
    sources = [
        item
        for item in await connection_repo.list_sources_for_owner(current_user.id)
        if item.connection_id == connection.id
    ]
    return to_telegram_connection_read(connection, sources)


@router.post("/connect/attempts/{attempt_id}/cancel", response_model=TelegramConnectionAttemptRead)
async def cancel_connection_attempt(
    attempt_id: UUID,
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> TelegramConnectionAttemptRead:
    attempt = await connection_repo.cancel_attempt_for_owner(
        owner_user_id=current_user.id,
        attempt_id=attempt_id,
    )
    if not attempt:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="connection attempt not found"
        )
    return to_telegram_connection_attempt_read(attempt)


@router.patch("/connections/{connection_id}", response_model=TelegramConnectionRead)
async def update_connection(
    connection_id: UUID,
    payload: TelegramConnectionUpdate,
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> TelegramConnectionRead:
    connection = await connection_repo.set_connection_enabled(
        owner_user_id=current_user.id,
        connection_id=connection_id,
        enabled=payload.enabled,
    )
    if not connection:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="connection not found")
    sources = [
        source
        for source in await connection_repo.list_sources_for_owner(current_user.id)
        if source.connection_id == connection.id
    ]
    return to_telegram_connection_read(connection, sources)


@router.delete("/connections/{connection_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_connection(
    connection_id: UUID,
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> Response:
    deleted = await connection_repo.delete_connection(
        owner_user_id=current_user.id,
        connection_id=connection_id,
    )
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="connection not found")
    return Response(status_code=status.HTTP_204_NO_CONTENT)


@router.delete("/sources/{source_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_source(
    source_id: UUID,
    current_user: User = Depends(require_user),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> Response:
    deleted = await connection_repo.delete_source(
        owner_user_id=current_user.id, source_id=source_id
    )
    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="source not found")
    return Response(status_code=status.HTTP_204_NO_CONTENT)
