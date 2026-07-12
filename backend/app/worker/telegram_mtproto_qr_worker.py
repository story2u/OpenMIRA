"""Own the Telegram QR-login lifecycle without exposing user credentials to the web app."""

import asyncio
import signal
from contextlib import suppress
from dataclasses import dataclass
from uuid import UUID

import structlog
from telethon import TelegramClient
from telethon.errors import SessionPasswordNeededError
from telethon.sessions import StringSession

from app.core.config import Settings, get_settings
from app.core.security import encrypt_secret
from app.infrastructure.db.repositories import TelegramConnectionRepository
from app.infrastructure.db.session import AsyncSessionLocal

logger = structlog.get_logger(__name__)
POLL_SECONDS = 2


@dataclass
class PendingQrLogin:
    client: TelegramClient
    task: asyncio.Task[None]


async def complete_qr_login(
    *,
    attempt_id: UUID,
    client: TelegramClient,
    qr_login: object,
    settings: Settings,
) -> None:
    try:
        await qr_login.wait()  # type: ignore[attr-defined]
        account = await client.get_me()
        if not account:
            raise ValueError("Telegram did not return an account")
        label = "Telegram 普通账号"
        if getattr(account, "first_name", None):
            label = f"Telegram · {account.first_name}"
        async with AsyncSessionLocal() as session:
            await TelegramConnectionRepository(session).complete_mtproto_qr_attempt(
                attempt_id=attempt_id,
                telegram_account_id=str(account.id),
                label=label,
                credential_encrypted=encrypt_secret(client.session.save(), settings),
            )
        logger.info("telegram_mtproto_qr.completed", attempt_id=str(attempt_id))
    except SessionPasswordNeededError:
        async with AsyncSessionLocal() as session:
            repo = TelegramConnectionRepository(session)
            attempt = await repo.get_attempt(attempt_id)
            if attempt:
                await repo.fail_attempt(
                    attempt=attempt,
                    error="此账号启用了两步验证；当前 QR 连接不会采集密码。",
                )
        logger.info("telegram_mtproto_qr.password_required", attempt_id=str(attempt_id))
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("telegram_mtproto_qr.failed", attempt_id=str(attempt_id))
        async with AsyncSessionLocal() as session:
            repo = TelegramConnectionRepository(session)
            attempts = await repo.list_pending_mtproto_qr_attempts()
            for attempt in attempts:
                if attempt.id == attempt_id:
                    await repo.fail_attempt(attempt=attempt, error="Telegram QR 登录失败，请重新生成二维码。")
                    break
    finally:
        await client.disconnect()


async def start_pending_login(attempt_id: UUID, settings: Settings) -> PendingQrLogin | None:
    if not settings.telegram_mtproto_qr_available:
        return None
    assert settings.telegram_mtproto_api_id is not None
    client = TelegramClient(
        StringSession(),
        settings.telegram_mtproto_api_id,
        settings.telegram_mtproto_api_hash,
    )
    try:
        await client.connect()
        qr_login = await client.qr_login()
        async with AsyncSessionLocal() as session:
            stored = await TelegramConnectionRepository(session).set_qr_url_encrypted(
                attempt_id=attempt_id,
                qr_url_encrypted=encrypt_secret(qr_login.url, settings),
            )
        if not stored:
            await client.disconnect()
            return None
        return PendingQrLogin(
            client=client,
            task=asyncio.create_task(
                complete_qr_login(
                    attempt_id=attempt_id,
                    client=client,
                    qr_login=qr_login,
                    settings=settings,
                )
            ),
        )
    except Exception:
        await client.disconnect()
        raise


async def supervise_qr_logins(stop_event: asyncio.Event) -> None:
    settings = get_settings()
    pending: dict[UUID, PendingQrLogin] = {}
    while not stop_event.is_set():
        async with AsyncSessionLocal() as session:
            desired = await TelegramConnectionRepository(session).list_pending_mtproto_qr_attempts()
        desired_ids = {attempt.id for attempt in desired}
        for attempt_id, active in list(pending.items()):
            if active.task.done() or attempt_id not in desired_ids:
                if not active.task.done():
                    active.task.cancel()
                    with suppress(asyncio.CancelledError):
                        await active.task
                pending.pop(attempt_id, None)
        for attempt in desired:
            if attempt.id in pending:
                continue
            try:
                started = await start_pending_login(attempt.id, settings)
                if started:
                    pending[attempt.id] = started
            except Exception:
                logger.exception("telegram_mtproto_qr.start_failed", attempt_id=str(attempt.id))
        with suppress(asyncio.TimeoutError):
            await asyncio.wait_for(stop_event.wait(), timeout=POLL_SECONDS)
    for active in pending.values():
        active.task.cancel()
    for active in pending.values():
        with suppress(asyncio.CancelledError):
            await active.task


async def run_worker() -> None:
    stop_event = asyncio.Event()
    loop = asyncio.get_running_loop()
    for signame in {"SIGINT", "SIGTERM"}:
        with suppress(NotImplementedError):
            loop.add_signal_handler(getattr(signal, signame), stop_event.set)
    await supervise_qr_logins(stop_event)


def main() -> None:
    asyncio.run(run_worker())


if __name__ == "__main__":
    main()
