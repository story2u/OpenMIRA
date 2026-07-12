"""Read-only listener for QR-authorized Telegram user accounts."""

import asyncio
import signal
from contextlib import suppress
from datetime import datetime
from uuid import UUID

import structlog

from app.core.config import Settings, get_settings
from app.core.security import decrypt_secret
from app.infrastructure.db.repositories import TelegramConnectionRepository
from app.infrastructure.db.session import AsyncSessionLocal
from app.infrastructure.im.telegram_user import TelegramUserClient, TelegramUserClientConfig
from app.worker.telegram_listener import ingest

logger = structlog.get_logger(__name__)
POLL_SECONDS = 15


async def load_configs(settings: Settings) -> dict[UUID, tuple[datetime, TelegramUserClientConfig]]:
    if not settings.telegram_mtproto_qr_available:
        return {}
    assert settings.telegram_mtproto_api_id is not None
    configs: dict[UUID, tuple[datetime, TelegramUserClientConfig]] = {}
    async with AsyncSessionLocal() as session:
        repo = TelegramConnectionRepository(session)
        for connection, sources in await repo.list_listenable_mtproto_connections():
            try:
                configs[connection.id] = (
                    connection.updated_at,
                    TelegramUserClientConfig(
                        user_id=connection.owner_user_id,
                        api_id=settings.telegram_mtproto_api_id,
                        api_hash=settings.telegram_mtproto_api_hash,
                        session_string=decrypt_secret(connection.credential_encrypted or "", settings),
                        chats=[source.external_chat_id for source in sources],
                    ),
                )
            except ValueError:
                await repo.record_connection_error(connection.id, "MTProto session is unavailable")
    return configs


async def run_connection_listener(connection_id: UUID, config: TelegramUserClientConfig) -> None:
    client = TelegramUserClient(config)
    try:
        await client.start()
        async with AsyncSessionLocal() as session:
            await TelegramConnectionRepository(session).record_connection_error(connection_id, None)
        async for inbound in client.iter_backfill_messages():
            await ingest(inbound)

        @client.client.on(client.new_message_event())
        async def handle_message(event) -> None:
            inbound = await client.to_inbound_message(event.message)
            if inbound:
                await ingest(inbound)

        await client.client.run_until_disconnected()
    except asyncio.CancelledError:
        raise
    except Exception:
        logger.exception("telegram_mtproto_listener.failed", connection_id=str(connection_id))
        async with AsyncSessionLocal() as session:
            await TelegramConnectionRepository(session).record_connection_error(
                connection_id, "Telegram listener disconnected; retrying automatically"
            )
    finally:
        await client.disconnect()


async def supervise(stop_event: asyncio.Event) -> None:
    settings = get_settings()
    running: dict[UUID, tuple[datetime, asyncio.Task[None]]] = {}
    while not stop_event.is_set():
        desired = await load_configs(settings)
        for connection_id, (_, task) in list(running.items()):
            desired_item = desired.get(connection_id)
            if task.done() or desired_item is None or desired_item[0] != running[connection_id][0]:
                task.cancel()
                with suppress(asyncio.CancelledError):
                    await task
                running.pop(connection_id, None)
        for connection_id, (updated_at, config) in desired.items():
            if connection_id not in running:
                running[connection_id] = (updated_at, asyncio.create_task(run_connection_listener(connection_id, config)))
        with suppress(asyncio.TimeoutError):
            await asyncio.wait_for(stop_event.wait(), timeout=POLL_SECONDS)
    for _, task in running.values():
        task.cancel()
    for _, task in running.values():
        with suppress(asyncio.CancelledError):
            await task


async def run_worker() -> None:
    stop_event = asyncio.Event()
    loop = asyncio.get_running_loop()
    for signame in {"SIGINT", "SIGTERM"}:
        with suppress(NotImplementedError):
            loop.add_signal_handler(getattr(signal, signame), stop_event.set)
    await supervise(stop_event)


def main() -> None:
    asyncio.run(run_worker())


if __name__ == "__main__":
    main()
