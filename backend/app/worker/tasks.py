import asyncio
from uuid import UUID, uuid4

import structlog

from app.application.use_cases.ai_reply import AIAutoReplyUseCase, transition_pending_to_ai
from app.application.use_cases.analysis_run import (
    AnalysisRunService,
    DeviceAgentRoutingService,
)
from app.application.use_cases.analyze_message import AnalyzeMessageUseCase
from app.application.use_cases.dispatch_push_hints import dispatch_pending_push_hints
from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnService,
)
from app.application.use_cases.sync_revenuecat_customer import SyncRevenueCatCustomer
from app.application.use_cases.sync_wecom_archive import SyncWeComArchive
from app.core.config import get_settings
from app.core.password_reset import (
    generate_reset_code,
    generate_reset_token,
    reset_credential_digest,
)
from app.core.security import decrypt_secret
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.enums import AgentAnalysisStatus, AnalysisRunExecutor
from app.domain.ports import AgentExecutionMetadata, InboundMessage
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.agent.link_inspector import SafeLinkInspector
from app.infrastructure.agent.pi_client import PiAgentClient
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier, LiteLLMReplyGenerator
from app.infrastructure.billing.revenuecat_client import RevenueCatClient
from app.infrastructure.db.analysis_run_repository import AnalysisRunRepository
from app.infrastructure.db.interactive_agent_repository import (
    InteractiveAgentTurnRepository,
)
from app.infrastructure.db.repositories import (
    BillingEventRepository,
    AutoReplyDeliveryRepository,
    ConfigRepository,
    JobMessageAuditRepository,
    JobOpportunityRepository,
    JobOpportunityMatchRepository,
    JobSearchProfileRepository,
    MessageRepository,
    OpportunityRepository,
    PushRegistrationRepository,
    RuleRepository,
    SourceFunctionalProfileRepository,
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
    UserRepository,
    UserSettingsRepository,
    WeComConnectionRepository,
    WeComArchiveRepository,
    WeComEventRepository,
)
from app.infrastructure.db.models import utc_now
from app.infrastructure.email.smtp import SMTPEmailSender
from app.infrastructure.db.session import AsyncSessionLocal
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.telegram import TelegramAdapter
from app.infrastructure.im.wecom import WeComAdapter
from app.infrastructure.im.wecom_archive import (
    CtypesWeComFinanceProvider,
    WeComArchiveCredentials,
    WeComArchiveProviderError,
)
from app.worker.celery_app import celery_app
from app.worker.queue import CeleryTaskQueue

logger = structlog.get_logger(__name__)


@celery_app.task(
    name="auth.prepare_password_reset",
    queue="default",
    autoretry_for=(Exception,),
    retry_backoff=True,
    retry_kwargs={"max_retries": 3},
)
def prepare_password_reset(email: str) -> None:
    asyncio.run(_prepare_password_reset(email))


async def _prepare_password_reset(email: str) -> None:
    settings = get_settings()
    if not settings.password_reset_email_configured:
        raise RuntimeError("password reset email is not configured")
    async with AsyncSessionLocal() as session:
        user = await UserRepository(session).get_by_email(email)
        if not user or not user.is_active:
            return
        token = generate_reset_token()
        code = generate_reset_code()
        reset_repo = PasswordResetRepository(session)
        challenge = await reset_repo.create(
            user_id=user.id,
            token_digest=reset_credential_digest(token, settings),
            code_digest=reset_credential_digest(code, settings),
            expires_at=utc_now() + timedelta(minutes=settings.password_reset_ttl_minutes),
        )
        try:
            await SMTPEmailSender(settings).send_password_reset(
                recipient=user.email,
                token=token,
                code=code,
            )
        except Exception:
            await reset_repo.invalidate(challenge)
            raise
        logger.info("auth.password_reset_email_sent", challenge_id=str(challenge.id))


@celery_app.task(
    name="ai.generate_and_send_reply",
    queue="ai",
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


@celery_app.task(name="agent.expire_device_analysis_runs", queue="default")
def expire_device_analysis_runs() -> None:
    asyncio.run(_expire_device_analysis_runs())


@celery_app.task(name="agent.expire_interactive_turns", queue="default")
def expire_interactive_turns() -> None:
    asyncio.run(_expire_interactive_turns())


@celery_app.task(name="push.dispatch_cursor_hints", queue="default")
def dispatch_push_cursor_hints() -> None:
    asyncio.run(_dispatch_push_cursor_hints())


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


@celery_app.task(name="wecom.enqueue_archive_syncs", queue="default")
def enqueue_wecom_archive_syncs() -> None:
    asyncio.run(_enqueue_wecom_archive_syncs())


@celery_app.task(
    bind=True,
    name="wecom.sync_archive_connection",
    queue="wecom_archive",
    max_retries=3,
    soft_time_limit=90,
    time_limit=105,
)
def sync_wecom_archive_connection(task, connection_id: str, verifying: bool = False) -> None:
    try:
        asyncio.run(_sync_wecom_archive_connection(UUID(connection_id), verifying=verifying))
    except Exception as exc:
        if task.request.retries >= task.max_retries:
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
        telegram_repo = TelegramConnectionRepository(session)
        adapters = AdapterRegistry(
            [
                TelegramAdapter(settings, connection_repo=telegram_repo),
                WeComAdapter(settings),
            ]
        )
        use_case = AIAutoReplyUseCase(
            settings=settings,
            opportunity_repo=opportunity_repo,
            message_repo=message_repo,
            delivery_repo=AutoReplyDeliveryRepository(session),
            telegram_repo=telegram_repo,
            user_settings_repo=UserSettingsRepository(session),
            adapters=adapters,
            reply_generator=reply_generator,
            task_queue=CeleryTaskQueue(),
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
        message_repo = MessageRepository(session)
        await IngestMessageUseCase(
            message_repo=message_repo,
            opportunity_repo=OpportunityRepository(session),
            rule_repo=RuleRepository(session),
            detector=OpportunityDetector(ai_classifier=LiteLLMOpportunityClassifier(settings)),
            work_time=WorkTimeService(work_config),
            task_queue=CeleryTaskQueue(),
            subscription_repo=SubscriptionRepository(session),
            user_settings_repo=UserSettingsRepository(session),
            device_routing=DeviceAgentRoutingService(
                run_repo=AnalysisRunRepository(session),
                settings=settings,
            ),
        ).execute(inbound)
        await event_repo.finish(event.id)


async def _fail_wecom_event(event_id: UUID, error: str, *, final: bool) -> None:
    async with AsyncSessionLocal() as session:
        await WeComEventRepository(session).fail(
            event_id,
            f"WeCom event processing failed: {error}",
            final=final,
        )


async def _enqueue_wecom_archive_syncs() -> None:
    settings = get_settings()
    if not settings.wecom_archive_enabled:
        return
    async with AsyncSessionLocal() as session:
        connection_ids = await WeComArchiveRepository(session).active_connection_ids(limit=100)
    queue = CeleryTaskQueue()
    for connection_id in connection_ids:
        queue.enqueue_wecom_archive_sync(connection_id)


async def _sync_wecom_archive_connection(connection_id: UUID, *, verifying: bool) -> None:
    settings = get_settings()
    if not settings.wecom_archive_enabled:
        raise RuntimeError("WeCom archive integration is disabled")
    async with AsyncSessionLocal() as session:
        repository = WeComArchiveRepository(session)
        connection = await repository.get(connection_id)
        if not connection or not connection.enabled:
            return
        try:
            provider = CtypesWeComFinanceProvider(settings.wecom_archive_sdk_path)
        except WeComArchiveProviderError as exc:
            await repository.mark_connection_error(connection, str(exc))
            raise
        credentials = WeComArchiveCredentials(
            corp_id=connection.corp_id,
            archive_secret=decrypt_secret(connection.secret_encrypted, settings),
            private_key_pem=decrypt_secret(connection.private_key_encrypted, settings),
            public_key_version=connection.public_key_version,
        )
        config_repo = ConfigRepository(session)
        raw_config = await config_repo.get_value("working_hours")
        work_config = (
            WorkTimeConfig.model_validate(raw_config)
            if raw_config
            else WorkTimeConfig.from_settings(settings)
        )
        message_repository = MessageRepository(session)
        subscription_repository = SubscriptionRepository(session)
        ingest = IngestMessageUseCase(
            message_repo=message_repository,
            opportunity_repo=OpportunityRepository(session),
            rule_repo=RuleRepository(session),
            detector=OpportunityDetector(ai_classifier=LiteLLMOpportunityClassifier(settings)),
            work_time=WorkTimeService(work_config),
            task_queue=CeleryTaskQueue(),
            subscription_repo=subscription_repository,
            user_settings_repo=UserSettingsRepository(session),
            job_discovery=PrepareJobDiscoveryUseCase(
                message_repo=message_repository,
                profile_repo=SourceFunctionalProfileRepository(session),
                audit_repo=JobMessageAuditRepository(session),
            ),
        )
        result = await SyncWeComArchive(
            repository=repository,
            provider=provider,
            ingest_message=ingest,
            message_repository=message_repository,
            subscription_repository=subscription_repository,
            batch_size=settings.wecom_archive_batch_size,
            timeout_seconds=settings.wecom_archive_sdk_timeout_seconds,
            lease_seconds=settings.wecom_archive_lease_seconds,
        ).execute(connection, credentials, verifying=verifying)
        if result:
            logger.info(
                "wecom.archive_sync_completed",
                connection_id=str(connection.id),
                fetched=result.fetched,
                processed=result.processed,
                ignored=result.ignored,
                projected_users=result.projected_users,
                last_sequence=result.last_sequence,
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
            execution=AgentExecutionMetadata(
                executed_by=AnalysisRunExecutor.SERVER,
                run_id=usage_ledger_id or uuid4(),
                runtime_version=f"node-{settings.device_agent_runtime_version}",
                schema_version=settings.device_agent_schema_version,
                model_version=settings.pi_agent_model,
                policy_version=settings.device_agent_policy_version,
            ),
            min_opportunity_confidence=settings.pi_agent_min_opportunity_confidence,
            max_links=settings.pi_agent_max_links,
            job_audit_repo=JobMessageAuditRepository(session),
            source_profile_repo=SourceFunctionalProfileRepository(session),
            job_opportunity_repo=JobOpportunityRepository(session),
            job_search_profile_repo=JobSearchProfileRepository(session),
            job_match_repo=JobOpportunityMatchRepository(session),
        )
        opportunity = await use_case.execute(message_id, force=force)
        final_message = await message_repo.get(message_id)
        if final_message is None:
            if usage_ledger_id:
                await SubscriptionRepository(session).release_usage(
                    usage_ledger_id,
                    "source message no longer exists",
                )
            logger.info("agent.analysis_source_missing", message_id=str(message_id))
            return
        if final_message.agent_analysis_status != AgentAnalysisStatus.COMPLETED:
            logger.info(
                "agent.analysis_execution_deferred",
                message_id=str(message_id),
                status=final_message.agent_analysis_status.value,
            )
            return
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


async def _expire_device_analysis_runs() -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        expired = await AnalysisRunService(
            run_repo=AnalysisRunRepository(session),
            subscription_repo=SubscriptionRepository(session),
            message_repo=MessageRepository(session),
            opportunity_repo=OpportunityRepository(session),
            task_queue=CeleryTaskQueue(),
            settings=settings,
        ).expire_stale()
    if expired:
        logger.info("agent.device_runs_expired", count=expired)


async def _expire_interactive_turns() -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        expired = await InteractiveAgentTurnService(
            turn_repo=InteractiveAgentTurnRepository(session),
            subscription_repo=SubscriptionRepository(session),
            settings=settings,
            routing_service=InteractiveAgentRoutingService(settings=settings),
        ).expire_stale()
    if expired:
        logger.info("agent.interactive_turns_expired", count=expired)


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
                wecom_archive_repo=WeComArchiveRepository(session),
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


async def _dispatch_push_cursor_hints() -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        await dispatch_pending_push_hints(PushRegistrationRepository(session), settings)
