import asyncio
import signal

import structlog

from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.core.config import get_settings
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier
from app.infrastructure.db.repositories import ConfigRepository, MessageRepository, OpportunityRepository, RuleRepository
from app.infrastructure.db.session import AsyncSessionLocal
from app.infrastructure.im.telegram_user import TelegramUserClient
from app.worker.queue import CeleryTaskQueue

logger = structlog.get_logger(__name__)


def missing_telegram_user_settings() -> list[str]:
    settings = get_settings()
    missing = []
    if not settings.telegram_user_api_id:
        missing.append("TELEGRAM_USER_API_ID")
    if not settings.telegram_user_api_hash or settings.telegram_user_api_hash == "change-me":
        missing.append("TELEGRAM_USER_API_HASH")
    if not settings.telegram_user_session:
        missing.append("TELEGRAM_USER_SESSION")
    if not settings.telegram_user_chats:
        missing.append("TELEGRAM_USER_CHATS")
    return missing


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
        )
        result = await use_case.execute(inbound)
        logger.info(
            "telegram_user.message_ingested",
            external_message_id=inbound.external_message_id,
            result_type=result.__class__.__name__,
        )


async def run_listener() -> None:
    settings = get_settings()
    if not settings.telegram_user_enabled:
        logger.info("telegram_user.disabled")
        await asyncio.Event().wait()

    missing = missing_telegram_user_settings()
    if missing:
        raise RuntimeError(f"missing Telegram user settings: {', '.join(missing)}")

    user_client = TelegramUserClient(settings)
    await user_client.start()

    for signame in {"SIGINT", "SIGTERM"}:
        asyncio.get_running_loop().add_signal_handler(
            getattr(signal, signame),
            lambda: asyncio.create_task(user_client.disconnect()),
        )

    logger.info("telegram_user.started", chats=user_client.normalized_chats())

    async for inbound in user_client.iter_backfill_messages():
        await ingest(inbound)

    @user_client.client.on(user_client.new_message_event())
    async def handle_new_message(event) -> None:
        inbound = await user_client.to_inbound_message(event.message)
        if inbound:
            await ingest(inbound)

    await user_client.client.run_until_disconnected()


def main() -> None:
    asyncio.run(run_listener())


if __name__ == "__main__":
    main()
