import asyncio
from uuid import UUID

import structlog

from app.application.use_cases.ai_reply import AIAutoReplyUseCase, transition_pending_to_ai
from app.application.use_cases.analyze_message import AnalyzeMessageUseCase
from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.application.use_cases.sync_revenuecat_customer import SyncRevenueCatCustomer
from app.core.config import get_settings
from app.core.security import decrypt_secret
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.ports import InboundMessage
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.agent.link_inspector import SafeLinkInspector
from app.infrastructure.agent.pi_client import PiAgentClient
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier, LiteLLMReplyGenerator
from app.infrastructure.billing.revenuecat_client import RevenueCatClient
from app.infrastructure.db.repositories import (
    BillingEventRepository,
    ConfigRepository,
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
    UserRepository,
    UserSettingsRepository,
    WeComConnectionRepository,
    WeComEventRepository,
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
                asyncio.run(
                    _release_agent_usage(ledger_id, "pi agent analysis failed after retries")
                )
            raise
        raise task.retry(exc=exc, countdown=min(2 ** (task.request.retries + 1), 30)) from exc


@celery_app.task(name="opportunity.sweep_pending_for_ai", queue="default")
def sweep_pending_for_ai() -> None:
    asyncio.run(_sweep_pending_for_ai())


@celery_app.task(
    bind=True,
    name="billing.process_revenuecat_event",
    queue="default",
    max_retries=3,
)
def process_revenuecat_event(task, event_id: str) -> None:
    try:
        asyncio.run(_process_revenuecat_event(UUID(event_id)))
    except Exception as exc:
        if task.request.retries >= task.max_retries:
            raise
        raise task.retry(exc=exc, countdown=min(2 ** (task.request.retries + 1), 30)) from exc


@celery_app.task(name="billing.sync_revenuecat_customer", queue="default")
def sync_revenuecat_customer(user_id: str) -> None:
    asyncio.run(_sync_revenuecat_user(UUID(user_id)))


@celery_app.task(name="billing.reconcile_revenuecat_subscriptions", queue="default")
def reconcile_revenuecat_subscriptions() -> None:
    asyncio.run(_enqueue_revenuecat_reconciliation())


@celery_app.task(
    bind=True,
    name="wecom.process_webhook_event",
    queue="im",
    max_retries=3,
)
def process_wecom_event(task, event_id: str) -> None:
    event_uuid = UUID(event_id)
    try:
        asyncio.run(_process_wecom_event(event_uuid))
    except Exception as exc:
        final = task.request.retries >= task.max_retries
        asyncio.run(_fail_wecom_event(event_uuid, exc.__class__.__name__, final=final))
        if final:
            raise
        raise task.retry(exc=exc, countdown=min(2 ** (task.request.retries + 1), 30)) from exc


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


async def _process_wecom_event(event_id: UUID) -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        event_repo = WeComEventRepository(session)
        event = await event_repo.begin_processing(event_id)
        if not event:
            return
        connection_repo = WeComConnectionRepository(session)
        connection = await connection_repo.get(event.connection_id)
        if (
            not connection
            or not connection.enabled
            or connection.owner_user_id != event.owner_user_id
        ):
            raise RuntimeError("wecom connection is unavailable")
        assert event.normalized_payload_encrypted
        inbound = InboundMessage.model_validate_json(
            decrypt_secret(event.normalized_payload_encrypted, settings)
        )
        if inbound.owner_user_id != connection.owner_user_id or not inbound.sender_external_id:
            raise RuntimeError("wecom event owner context is invalid")
        await connection_repo.ensure_private_source(
            connection=connection,
            external_conversation_id=inbound.sender_external_id,
            display_name=inbound.sender_display_name or inbound.sender_external_id,
        )

        config_repo = ConfigRepository(session)
        raw_config = await config_repo.get_value("working_hours")
        work_config = (
            WorkTimeConfig.model_validate(raw_config)
            if raw_config
            else WorkTimeConfig.from_settings(settings)
        )
        await IngestMessageUseCase(
            message_repo=MessageRepository(session),
            opportunity_repo=OpportunityRepository(session),
            rule_repo=RuleRepository(session),
            detector=OpportunityDetector(ai_classifier=LiteLLMOpportunityClassifier(settings)),
            work_time=WorkTimeService(work_config),
            task_queue=CeleryTaskQueue(),
            subscription_repo=SubscriptionRepository(session),
            user_settings_repo=UserSettingsRepository(session),
        ).execute(inbound)
        await event_repo.finish(event.id)


async def _fail_wecom_event(event_id: UUID, error: str, *, final: bool) -> None:
    async with AsyncSessionLocal() as session:
        await WeComEventRepository(session).fail(
            event_id,
            f"WeCom event processing failed: {error}",
            final=final,
        )


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


async def _sync_revenuecat_user(user_id: UUID) -> None:
    settings = get_settings()
    if not settings.revenuecat_server_available:
        raise RuntimeError("RevenueCat server integration is not configured")
    client = RevenueCatClient(
        secret_api_key=settings.revenuecat_secret_api_key,
        timeout_seconds=settings.revenuecat_sync_timeout_seconds,
    )
    try:
        async with AsyncSessionLocal() as session:
            await SyncRevenueCatCustomer(
                settings=settings,
                provider=client,
                user_repo=UserRepository(session),
                subscription_repo=SubscriptionRepository(session),
                telegram_repo=TelegramUserConfigRepository(session),
                telegram_connection_repo=TelegramConnectionRepository(session),
            ).execute(user_id)
    finally:
        await client.aclose()


async def _process_revenuecat_event(event_id: UUID) -> None:
    async with AsyncSessionLocal() as session:
        event_repo = BillingEventRepository(session)
        event = await event_repo.begin_processing(event_id)
        if not event:
            return
        raw_user_ids = event.app_user_id.split(",") if event.app_user_id else []
        local_user_ids: list[UUID] = []
        user_repo = UserRepository(session)
        for raw_user_id in raw_user_ids:
            try:
                user_id = UUID(raw_user_id)
            except ValueError:
                continue
            user = await user_repo.get(user_id)
            if user and user.is_active:
                local_user_ids.append(user_id)
        if not local_user_ids:
            await event_repo.finish(event_id, orphaned=True)
            return
    try:
        for user_id in local_user_ids:
            await _sync_revenuecat_user(user_id)
    except Exception as exc:
        async with AsyncSessionLocal() as session:
            await BillingEventRepository(session).fail(
                event_id,
                f"RevenueCat synchronization failed: {exc.__class__.__name__}",
            )
        raise
    async with AsyncSessionLocal() as session:
        await BillingEventRepository(session).finish(event_id)


async def _enqueue_revenuecat_reconciliation() -> None:
    settings = get_settings()
    if not settings.revenuecat_reconcile_enabled or not settings.revenuecat_server_available:
        return
    async with AsyncSessionLocal() as session:
        user_ids = await BillingEventRepository(session).reconciliation_user_ids(
            limit=settings.revenuecat_reconcile_batch_size
        )
    for index, user_id in enumerate(user_ids):
        celery_app.send_task(
            "billing.sync_revenuecat_customer",
            args=[str(user_id)],
            countdown=index,
        )


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
