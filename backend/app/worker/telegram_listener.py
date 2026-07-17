import asyncio
import signal
from contextlib import suppress
from datetime import datetime
from uuid import UUID

import structlog

from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.application.use_cases.analysis_run import DeviceAgentRoutingService
from app.core.config import Settings, get_settings
from app.core.security import decrypt_secret
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.services.detection_policy import OpportunityDetector
from app.domain.services.subscription_policy import telegram_group_capacity
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier
from app.infrastructure.db.repositories import (
    ConfigRepository,
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
    TelegramUserConfigRepository,
)
from app.infrastructure.db.session import AsyncSessionLocal
from app.infrastructure.db.analysis_run_repository import AnalysisRunRepository
from app.infrastructure.im.telegram_user import TelegramUserClient, TelegramUserClientConfig
from app.worker.queue import CeleryTaskQueue

logger = structlog.get_logger(__name__)
POLL_SECONDS = 30


async def ingest(inbound) -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        config_repo = ConfigRepository(session)
        raw_config = await config_repo.get_value("working_hours")
        work_time_config = (
            WorkTimeConfig.model_validate(raw_config)
            if raw_config
            else WorkTimeConfig.from_settings(settings)
        )
        use_case = IngestMessageUseCase(
            message_repo=MessageRepository(session),
            opportunity_repo=OpportunityRepository(session),
            rule_repo=RuleRepository(session),
            detector=OpportunityDetector(ai_classifier=LiteLLMOpportunityClassifier(settings)),
            work_time=WorkTimeService(work_time_config),
            task_queue=CeleryTaskQueue(),
            subscription_repo=SubscriptionRepository(session),
            device_routing=DeviceAgentRoutingService(
                run_repo=AnalysisRunRepository(session),
                settings=settings,
            ),
        )
        result = await use_case.execute(inbound)
        logger.info(
            "telegram_user.message_ingested",
            owner_user_id=str(inbound.owner_user_id) if inbound.owner_user_id else None,
            external_message_id=inbound.external_message_id,
            result_type=result.__class__.__name__,
        )


async def record_monitor_error(monitor_id: UUID, error: str | None) -> None:
    async with AsyncSessionLocal() as session:
        await TelegramUserConfigRepository(session).record_monitor_error(monitor_id, error)


async def load_enabled_client_configs(
    settings: Settings,
) -> dict[UUID, tuple[datetime, TelegramUserClientConfig]]:
    configs: dict[UUID, tuple[datetime, TelegramUserClientConfig]] = {}
    async with AsyncSessionLocal() as session:
        repo = TelegramUserConfigRepository(session)
        subscription_repo = SubscriptionRepository(session)
        for user_id in await repo.list_enabled_user_ids():
            snapshot = await subscription_repo.get_snapshot(user_id)
            active_monitors = await repo.reconcile_monitor_quota_for_user(
                user_id=user_id,
                capacity=telegram_group_capacity(snapshot.entitlements),
            )
            for db_config, monitor in active_monitors:
                if (
                    not db_config.api_id
                    or not db_config.api_hash_encrypted
                    or not db_config.session_encrypted
                    or not monitor.chat_id
                ):
                    await repo.record_monitor_error(
                        monitor.id,
                        "enabled monitor config is incomplete",
                    )
                    continue
                try:
                    configs[monitor.id] = (
                        max(db_config.updated_at, monitor.updated_at),
                        TelegramUserClientConfig(
                            user_id=db_config.user_id,
                            api_id=db_config.api_id,
                            api_hash=decrypt_secret(db_config.api_hash_encrypted, settings),
                            session_string=decrypt_secret(
                                db_config.session_encrypted,
                                settings,
                            ),
                            chats=[monitor.chat_id],
                            backfill_limit=monitor.backfill_limit,
                        ),
                    )
                except ValueError as exc:
                    await repo.record_monitor_error(monitor.id, str(exc))
    return configs


async def run_config_listener(monitor_id: UUID, client_config: TelegramUserClientConfig) -> None:
    user_client = TelegramUserClient(client_config)
    try:
        await user_client.start()
        await record_monitor_error(monitor_id, None)
        logger.info(
            "telegram_user.started",
            monitor_id=str(monitor_id),
            owner_user_id=str(client_config.user_id),
            chats=user_client.normalized_chats(),
        )

        async for inbound in user_client.iter_backfill_messages():
            await ingest(inbound)

        @user_client.client.on(user_client.new_message_event())
        async def handle_new_message(event) -> None:
            inbound = await user_client.to_inbound_message(event.message)
            if inbound:
                await ingest(inbound)

        await user_client.client.run_until_disconnected()
    except asyncio.CancelledError:
        raise
    except Exception as exc:
        logger.exception("telegram_user.listener_failed", monitor_id=str(monitor_id))
        await record_monitor_error(monitor_id, str(exc))
    finally:
        with suppress(Exception):
            await user_client.disconnect()


async def supervise_listeners(stop_event: asyncio.Event) -> None:
    settings = get_settings()
    running: dict[UUID, tuple[datetime, asyncio.Task[None]]] = {}
    while not stop_event.is_set():
        desired = await load_enabled_client_configs(settings)
        for monitor_id, (_, task) in list(running.items()):
            if task.done():
                running.pop(monitor_id, None)
                continue
            desired_item = desired.get(monitor_id)
            if desired_item is None or desired_item[0] != running[monitor_id][0]:
                task.cancel()
                with suppress(asyncio.CancelledError):
                    await task
                running.pop(monitor_id, None)

        for monitor_id, (updated_at, client_config) in desired.items():
            if monitor_id in running:
                continue
            running[monitor_id] = (
                updated_at,
                asyncio.create_task(run_config_listener(monitor_id, client_config)),
            )

        if not running:
            logger.info("telegram_user.no_enabled_configs")

        with suppress(asyncio.TimeoutError):
            await asyncio.wait_for(stop_event.wait(), timeout=POLL_SECONDS)

    for _, task in running.values():
        task.cancel()
    for _, task in running.values():
        with suppress(asyncio.CancelledError):
            await task


async def run_listener() -> None:
    stop_event = asyncio.Event()
    loop = asyncio.get_running_loop()
    for signame in {"SIGINT", "SIGTERM"}:
        with suppress(NotImplementedError):
            loop.add_signal_handler(getattr(signal, signame), stop_event.set)
    await supervise_listeners(stop_event)


def main() -> None:
    asyncio.run(run_listener())


if __name__ == "__main__":
    main()
