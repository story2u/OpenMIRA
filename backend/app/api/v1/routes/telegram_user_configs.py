import json
import secrets
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, status
from redis.asyncio import Redis
from telethon import TelegramClient
from telethon.errors import SessionPasswordNeededError
from telethon.sessions import StringSession

from app.api.deps import (
    get_redis_client,
    get_subscription_repo,
    get_telegram_user_config_repo,
    require_user,
)
from app.application.dto import (
    TelegramDialogRead,
    TelegramMonitorRetentionUpdate,
    TelegramSendCodeRead,
    TelegramSendCodeRequest,
    TelegramUserConfigRead,
    TelegramUserConfigUpdate,
    TelegramVerifyCodeRead,
    TelegramVerifyCodeRequest,
)
from app.application.mappers import to_telegram_user_config_read
from app.core.config import Settings, get_settings
from app.core.security import decrypt_secret, encrypt_secret
from app.domain.services.subscription_policy import GroupQuotaExceeded, telegram_group_capacity
from app.infrastructure.db.models import TelegramUserConfig, User
from app.infrastructure.db.repositories import SubscriptionRepository, TelegramUserConfigRepository

router = APIRouter()

LOGIN_TTL_SECONDS = 600


def login_key(user_id: object, login_id: str) -> str:
    return f"telegram-login:{user_id}:{login_id}"


def normalize_chats(chats: list[str | int]) -> list[str | int]:
    normalized: list[str | int] = []
    seen: set[str] = set()
    for chat in chats:
        if isinstance(chat, int):
            value: str | int = chat
            key = str(chat)
        else:
            stripped = chat.strip()
            if not stripped:
                continue
            value = int(stripped) if stripped.lstrip("-").isdigit() else stripped
            key = str(value)
        if key in seen:
            continue
        seen.add(key)
        normalized.append(value)
    return normalized


def ensure_enabled_config_is_complete(
    *,
    enabled: bool,
    api_id: int | None,
    has_api_hash: bool,
    has_session: bool,
    chats: list[str | int],
) -> None:
    if not enabled:
        return
    missing = []
    if not api_id:
        missing.append("apiId")
    if not has_api_hash:
        missing.append("apiHash")
    if not has_session:
        missing.append("sessionString")
    if not chats:
        missing.append("chats")
    if missing:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=f"missing required Telegram settings: {', '.join(missing)}",
        )


def decrypted_telegram_credentials(
    config: TelegramUserConfig | None,
    settings: Settings,
) -> tuple[int, str, str]:
    if not config or not config.api_id or not config.api_hash_encrypted or not config.session_encrypted:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="Telegram account is not connected")
    try:
        return (
            config.api_id,
            decrypt_secret(config.api_hash_encrypted, settings),
            decrypt_secret(config.session_encrypted, settings),
        )
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc


@router.get("/config", response_model=TelegramUserConfigRead)
async def get_config(
    current_user: User = Depends(require_user),
    repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramUserConfigRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    monitor_limit = telegram_group_capacity(snapshot.entitlements)
    await repo.reconcile_monitor_quota_for_user(
        user_id=current_user.id,
        capacity=monitor_limit,
    )
    return to_telegram_user_config_read(
        await repo.get_by_user(current_user.id),
        await repo.list_monitors_by_user(current_user.id),
        monitor_limit=monitor_limit,
    )


@router.put("/config", response_model=TelegramUserConfigRead)
async def update_config(
    payload: TelegramUserConfigUpdate,
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramUserConfigRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    monitor_limit = telegram_group_capacity(snapshot.entitlements)
    existing = await repo.get_by_user(current_user.id)
    api_id = payload.apiId if payload.apiId is not None else existing.api_id if existing else None
    api_hash_encrypted = (
        encrypt_secret(payload.apiHash.strip(), settings) if payload.apiHash and payload.apiHash.strip() else None
    )
    session_encrypted = (
        encrypt_secret(payload.sessionString.strip(), settings)
        if payload.sessionString and payload.sessionString.strip()
        else None
    )
    chats = normalize_chats(payload.chats)
    if len(chats) > monitor_limit:
        suffix = "" if monitor_limit == 1 else "s"
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=f"current plan allows {monitor_limit} Telegram monitor{suffix}",
        )
    has_api_hash = bool(api_hash_encrypted or (existing and existing.api_hash_encrypted))
    has_session = bool(session_encrypted or (existing and existing.session_encrypted))

    ensure_enabled_config_is_complete(
        enabled=payload.enabled,
        api_id=api_id,
        has_api_hash=has_api_hash,
        has_session=has_session,
        chats=chats,
    )
    config = await repo.save_account_for_user(
        user_id=current_user.id,
        api_id=api_id,
        api_hash_encrypted=api_hash_encrypted,
        session_encrypted=session_encrypted,
    )
    try:
        await repo.replace_monitors_for_user(
            user_id=current_user.id,
            telegram_config_id=config.id,
            chats=chats,
            enabled=payload.enabled,
            backfill_limit=payload.backfillLimit,
            entitlements=snapshot.entitlements,
        )
    except GroupQuotaExceeded as exc:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(exc)) from exc
    return to_telegram_user_config_read(
        config,
        await repo.list_monitors_by_user(current_user.id),
        monitor_limit=monitor_limit,
    )


@router.put("/monitors/retention", response_model=TelegramUserConfigRead)
async def update_monitor_retention(
    payload: TelegramMonitorRetentionUpdate,
    current_user: User = Depends(require_user),
    repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramUserConfigRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    monitor_limit = telegram_group_capacity(snapshot.entitlements)
    try:
        monitors = await repo.select_retained_monitors(
            user_id=current_user.id,
            monitor_ids=payload.monitorIds,
            capacity=monitor_limit,
        )
    except ValueError as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(exc),
        ) from exc
    config = await repo.get_by_user(current_user.id)
    assert config is not None
    return to_telegram_user_config_read(
        config,
        monitors,
        monitor_limit=monitor_limit,
    )


@router.post("/send-code", response_model=TelegramSendCodeRead)
async def send_code(
    payload: TelegramSendCodeRequest,
    current_user: User = Depends(require_user),
    redis: Redis = Depends(get_redis_client),
) -> TelegramSendCodeRead:
    client = TelegramClient(StringSession(), payload.apiId, payload.apiHash)
    await client.connect()
    try:
        sent_code = await client.send_code_request(payload.phone)
        login_id = secrets.token_urlsafe(24)
        temp_payload: dict[str, Any] = {
            "api_id": payload.apiId,
            "api_hash": payload.apiHash,
            "phone": payload.phone,
            "phone_code_hash": sent_code.phone_code_hash,
            "session": client.session.save(),
        }
        await redis.set(
            login_key(current_user.id, login_id),
            json.dumps(temp_payload),
            ex=LOGIN_TTL_SECONDS,
        )
        return TelegramSendCodeRead(loginId=login_id, expiresInSeconds=LOGIN_TTL_SECONDS)
    except Exception as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc
    finally:
        await client.disconnect()


@router.post("/verify-code", response_model=TelegramVerifyCodeRead)
async def verify_code(
    payload: TelegramVerifyCodeRequest,
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    redis: Redis = Depends(get_redis_client),
    repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> TelegramVerifyCodeRead:
    key = login_key(current_user.id, payload.loginId)
    raw_payload = await redis.get(key)
    if not raw_payload:
        raise HTTPException(status_code=status.HTTP_410_GONE, detail="login session expired")
    data = json.loads(raw_payload)

    client = TelegramClient(
        StringSession(data["session"]),
        int(data["api_id"]),
        data["api_hash"],
    )
    await client.connect()
    try:
        try:
            await client.sign_in(
                phone=data["phone"],
                code=payload.code,
                phone_code_hash=data["phone_code_hash"],
            )
        except SessionPasswordNeededError:
            if not payload.password:
                return TelegramVerifyCodeRead(status="password_required")
            await client.sign_in(password=payload.password)

        config = await repo.save_account_for_user(
            user_id=current_user.id,
            api_id=int(data["api_id"]),
            api_hash_encrypted=encrypt_secret(data["api_hash"], settings),
            session_encrypted=encrypt_secret(client.session.save(), settings),
        )
        await redis.delete(key)
        return TelegramVerifyCodeRead(
            status="connected",
            config=to_telegram_user_config_read(
                config,
                await repo.list_monitors_by_user(current_user.id),
                monitor_limit=telegram_group_capacity(
                    (await subscription_repo.get_snapshot(current_user.id)).entitlements
                ),
            ),
        )
    except Exception as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc
    finally:
        await client.disconnect()


@router.get("/dialogs", response_model=list[TelegramDialogRead])
async def list_dialogs(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
) -> list[TelegramDialogRead]:
    api_id, api_hash, session_string = decrypted_telegram_credentials(
        await repo.get_by_user(current_user.id),
        settings,
    )
    client = TelegramClient(StringSession(session_string), api_id, api_hash)
    await client.start()
    try:
        dialogs: list[TelegramDialogRead] = []
        async for dialog in client.iter_dialogs(limit=200):
            entity = dialog.entity
            dialogs.append(
                TelegramDialogRead(
                    id=dialog.id,
                    name=dialog.name,
                    username=getattr(entity, "username", None),
                )
            )
        return dialogs
    finally:
        await client.disconnect()
