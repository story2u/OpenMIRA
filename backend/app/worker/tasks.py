import asyncio
from uuid import UUID

import structlog

from app.application.use_cases.ai_reply import AIAutoReplyUseCase, transition_pending_to_ai
from app.application.use_cases.analyze_message import AnalyzeMessageUseCase
from app.core.config import get_settings
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.infrastructure.agent.link_inspector import SafeLinkInspector
from app.infrastructure.agent.pi_client import PiAgentClient
from app.infrastructure.ai.litellm_client import LiteLLMReplyGenerator
from app.infrastructure.db.repositories import (
    ConfigRepository,
    MessageRepository,
    OpportunityRepository,
    SubscriptionRepository,
)
from app.infrastructure.db.session import AsyncSessionLocal
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.telegram import TelegramAdapter
from app.infrastructure.im.wecom import WeComAdapter
from app.worker.celery_app import celery_app
from app.worker.queue import CeleryTaskQueue

logger = structlog.get_logger(__name__)


@celery_app.task(
    name="ai.generate_and_send_reply",
    queue="ai",
    autoretry_for=(Exception,),
    retry_backoff=True,
    retry_kwargs={"max_retries": 3},
)
def generate_and_send_reply(opportunity_id: str) -> None:
    asyncio.run(_generate_and_send_reply(UUID(opportunity_id)))


@celery_app.task(
    bind=True,
    name="agent.analyze_message",
    queue="agent",
    max_retries=3,
    soft_time_limit=135,
    time_limit=150,
)
def analyze_message(
    task,
    message_id: str,
    force: bool = False,
    usage_ledger_id: str | None = None,
) -> None:
    ledger_id = UUID(usage_ledger_id) if usage_ledger_id else None
    try:
        asyncio.run(_analyze_message(UUID(message_id), force=force, usage_ledger_id=ledger_id))
    except Exception as exc:
        if task.request.retries >= task.max_retries:
            if ledger_id:
                asyncio.run(_release_agent_usage(ledger_id, "pi agent analysis failed after retries"))
            raise
        raise task.retry(exc=exc, countdown=min(2 ** (task.request.retries + 1), 30)) from exc


@celery_app.task(name="opportunity.sweep_pending_for_ai", queue="default")
def sweep_pending_for_ai() -> None:
    asyncio.run(_sweep_pending_for_ai())


async def _generate_and_send_reply(opportunity_id: UUID) -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        opportunity_repo = OpportunityRepository(session)
        message_repo = MessageRepository(session)
        opportunity = await opportunity_repo.get(opportunity_id)
        if not opportunity:
            logger.warning("opportunity.not_found", opportunity_id=str(opportunity_id))
            return

        reply_generator = LiteLLMReplyGenerator(
            settings=settings,
            opportunity_repo=opportunity_repo,
            message_repo=message_repo,
        )
        adapters = AdapterRegistry(
            [
                TelegramAdapter(settings),
                WeComAdapter(settings),
            ]
        )
        use_case = AIAutoReplyUseCase(
            opportunity_repo=opportunity_repo,
            message_repo=message_repo,
            adapters=adapters,
            reply_generator=reply_generator,
        )
        await use_case.execute(opportunity)


async def _analyze_message(
    message_id: UUID,
    *,
    force: bool,
    usage_ledger_id: UUID | None,
) -> None:
    settings = get_settings()
    if not settings.pi_agent_enabled:
        logger.info("agent.analysis_disabled", message_id=str(message_id))
        async with AsyncSessionLocal() as session:
            await MessageRepository(session).fail_agent_analysis(
                message_id,
                "pi agent was disabled before execution",
            )
            if usage_ledger_id:
                await SubscriptionRepository(session).release_usage(
                    usage_ledger_id,
                    "pi agent was disabled before execution",
                )
        return

    async with AsyncSessionLocal() as session:
        message_repo = MessageRepository(session)
        opportunity_repo = OpportunityRepository(session)
        use_case = AnalyzeMessageUseCase(
            message_repo=message_repo,
            opportunity_repo=opportunity_repo,
            agent=PiAgentClient(
                node_binary=settings.pi_agent_node_binary,
                runner_path=settings.pi_agent_runner_path,
                provider=settings.pi_agent_provider,
                model=settings.pi_agent_model,
                api_key=settings.effective_pi_agent_api_key,
                timeout_seconds=settings.pi_agent_timeout_seconds,
            ),
            link_inspector=SafeLinkInspector(
                max_links=settings.pi_agent_max_links,
                max_content_bytes=settings.pi_agent_max_content_bytes,
                max_text_chars=settings.pi_agent_max_link_text_chars,
                timeout_seconds=settings.pi_agent_link_timeout_seconds,
            ),
            task_queue=CeleryTaskQueue(),
            min_opportunity_confidence=settings.pi_agent_min_opportunity_confidence,
            max_links=settings.pi_agent_max_links,
        )
        opportunity = await use_case.execute(message_id, force=force)
        if usage_ledger_id:
            await SubscriptionRepository(session).consume_usage(usage_ledger_id)
        logger.info(
            "agent.analysis_completed",
            message_id=str(message_id),
            opportunity_id=str(opportunity.id) if opportunity else None,
        )


async def _release_agent_usage(ledger_id: UUID, reason: str) -> None:
    async with AsyncSessionLocal() as session:
        await SubscriptionRepository(session).release_usage(ledger_id, reason)


async def _sweep_pending_for_ai() -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        config_repo = ConfigRepository(session)
        raw_config = await config_repo.get_value("working_hours")
        config = (
            WorkTimeConfig.model_validate(raw_config)
            if raw_config
            else WorkTimeConfig.from_settings(settings)
        )
        work_time = WorkTimeService(config)
        if work_time.is_working_time() or not config.auto_reply_after_hours:
            return

        opportunity_repo = OpportunityRepository(session)
        stale = await opportunity_repo.pending_human_older_than(settings.pending_human_sla_minutes)
        for opportunity in stale:
            updated = await transition_pending_to_ai(opportunity_repo, opportunity.id)
            if updated:
                celery_app.send_task("ai.generate_and_send_reply", args=[str(updated.id)])
