import hashlib
import json

from fastapi import APIRouter, Depends, Request

from app.api.deps import (
    get_adapter_registry,
    get_detector,
    get_message_repo,
    get_opportunity_repo,
    get_rule_repo,
    get_subscription_repo,
    get_telegram_connection_repo,
    get_telegram_user_config_repo,
    get_task_queue,
    get_work_time_service,
)
from app.application.mappers import to_opportunity_read
from app.application.use_cases.telegram_connection_workflow import (
    TelegramConnectionWorkflow,
    TelegramConnectionWorkflowError,
)
from app.core.config import Settings, get_settings
from app.core.security import require_secret
from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.domain.enums import IMChannel
from app.domain.services.detection_policy import OpportunityDetector
from app.domain.services.subscription_policy import telegram_group_capacity
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
)
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.telegram_connector import TelegramBotConnector
from app.worker.queue import CeleryTaskQueue

router = APIRouter()


def telegram_event_type(payload: dict) -> str:
    if "business_connection" in payload:
        return "business_connection"
    if "business_message" in payload or "edited_business_message" in payload:
        return "business_message"
    message = payload.get("message") or {}
    if "chat_shared" in message:
        return "chat_shared"
    if str(message.get("text") or "").startswith("/start"):
        return "start"
    if "channel_post" in payload or "edited_channel_post" in payload:
        return "channel_post"
    return "message"


def telegram_payload_hash(payload: dict) -> str:
    serialized = json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


@router.post("")
async def telegram_webhook(
    request: Request,
    settings: Settings = Depends(get_settings),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
    message_repo: MessageRepository = Depends(get_message_repo),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    rule_repo: RuleRepository = Depends(get_rule_repo),
    detector: OpportunityDetector = Depends(get_detector),
    work_time=Depends(get_work_time_service),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
    legacy_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
) -> dict:
    # Reject unauthenticated requests before parsing or persisting their JSON body.
    require_secret(
        request.headers.get("x-telegram-bot-api-secret-token", ""),
        settings.telegram_webhook_secret,
        "invalid telegram secret",
    )
    payload = await request.json()
    update_id = payload.get("update_id")
    if not isinstance(update_id, int):
        return {"ok": True, "ignored": True, "reason": "missing_update_id"}
    event, should_process = await connection_repo.reserve_webhook_event(
        update_id=update_id,
        payload_hash=telegram_payload_hash(payload),
        event_type=telegram_event_type(payload),
    )
    if not should_process:
        return {"ok": True, "duplicate": True}

    workflow = TelegramConnectionWorkflow(
        connection_repo=connection_repo,
        legacy_repo=legacy_repo,
        subscription_repo=subscription_repo,
        bot=TelegramBotConnector(settings),
    )
    try:
        for handler in (
            workflow.handle_private_start,
            workflow.handle_chat_shared,
            workflow.handle_business_connection,
        ):
            workflow_result = await handler(payload)
            if workflow_result.handled:
                await connection_repo.finish_webhook_event(
                    event=event,
                    connection_id=workflow_result.connection_id,
                )
                return {"ok": True, "connection_event": True}
    except TelegramConnectionWorkflowError as exc:
        await connection_repo.finish_webhook_event(event=event, error=str(exc))
        return {"ok": True, "connection_event": True, "error": "connection could not be completed"}

    adapter = adapters.get(IMChannel.TELEGRAM)
    use_case = IngestMessageUseCase(
        message_repo=message_repo,
        opportunity_repo=opportunity_repo,
        rule_repo=rule_repo,
        detector=detector,
        work_time=work_time,
        task_queue=task_queue,
        subscription_repo=subscription_repo,
    )
    business_message = payload.get("business_message") or payload.get("edited_business_message")
    if isinstance(business_message, dict):
        business_connection_id = business_message.get("business_connection_id")
        if not business_connection_id:
            await connection_repo.finish_webhook_event(event=event)
            return {"ok": True, "ignored": True}
        connection = await connection_repo.get_connection_by_provider_connection_id(
            str(business_connection_id)
        )
        if not connection or not connection.enabled:
            await connection_repo.finish_webhook_event(event=event)
            return {"ok": True, "ignored": True}
        inbound = await adapter.parse_webhook(
            payload,
            dict(request.headers),
            dict(request.query_params),
            owner_user_id=connection.owner_user_id,
            external_id_prefix=f"business:{connection.id}",
        )
        if not inbound:
            await connection_repo.finish_webhook_event(event=event, connection_id=connection.id)
            return {"ok": True, "ignored": True}
        result = await use_case.execute(inbound)
        await connection_repo.finish_webhook_event(event=event, connection_id=connection.id)
        response = {"ok": True, "id": str(result.id), "type": result.__class__.__name__}
        if isinstance(result, Opportunity):
            response["opportunity"] = to_opportunity_read(result).model_dump(mode="json")
        return response

    message = (
        payload.get("message")
        or payload.get("edited_message")
        or payload.get("channel_post")
        or payload.get("edited_channel_post")
        or {}
    )
    chat_id = (message.get("chat") or {}).get("id")
    if chat_id is None:
        await connection_repo.finish_webhook_event(event=event)
        return {"ok": True, "ignored": True, "reason": "missing_chat"}
    external_chat_id = str(chat_id)
    candidate_sources = await connection_repo.list_enabled_sources_for_chat(external_chat_id)
    for owner_user_id in {source.owner_user_id for source, _ in candidate_sources}:
        snapshot = await subscription_repo.get_snapshot(owner_user_id)
        legacy_active_count = await legacy_repo.count_active_monitors_by_user(owner_user_id)
        await connection_repo.reconcile_source_quota_for_user(
            owner_user_id=owner_user_id,
            capacity=telegram_group_capacity(snapshot.entitlements),
            legacy_active_count=legacy_active_count,
        )
    active_sources = await connection_repo.list_active_sources_for_chat(external_chat_id)
    if active_sources:
        results = []
        for source, connection in active_sources:
            inbound = await adapter.parse_webhook(
                payload,
                dict(request.headers),
                dict(request.query_params),
                owner_user_id=source.owner_user_id,
                external_id_prefix=f"connection:{connection.id}",
            )
            if inbound:
                result = await use_case.execute(inbound)
                results.append({"id": str(result.id), "type": result.__class__.__name__})
        await connection_repo.finish_webhook_event(
            event=event,
            connection_id=active_sources[0][1].id if len(active_sources) == 1 else None,
        )
        return {"ok": True, "results": results, "connection_message": True}

    # The unified Bot path is fail-closed: an unselected chat must not become an ownerless
    # message merely because the shared platform Bot happens to be a member of it.
    await connection_repo.finish_webhook_event(event=event)
    return {"ok": True, "ignored": True, "reason": "unconfigured_source"}
