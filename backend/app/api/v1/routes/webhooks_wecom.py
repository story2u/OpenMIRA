import hashlib
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Request, Response, status

from app.api.deps import (
    get_adapter_registry,
    get_detector,
    get_message_repo,
    get_opportunity_repo,
    get_rule_repo,
    get_subscription_repo,
    get_task_queue,
    get_user_settings_repo,
    get_wecom_connection_repo,
    get_wecom_event_repo,
    get_work_time_service,
)
from app.application.mappers import to_opportunity_read
from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.core.config import Settings, get_settings
from app.core.security import encrypt_secret
from app.domain.enums import IMChannel
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
    UserSettingsRepository,
    WeComConnectionRepository,
    WeComEventRepository,
)
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.wecom import (
    WeComAdapter,
    WeComCredentials,
    WeComCryptoError,
    parse_xml_envelope,
    serialize_inbound,
)
from app.worker.queue import CeleryTaskQueue

router = APIRouter()


@router.get("")
async def wecom_verify_url(
    request: Request,
    adapters: AdapterRegistry = Depends(get_adapter_registry),
) -> Response:
    adapter = adapters.get(IMChannel.WECOM)
    if not isinstance(adapter, WeComAdapter):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="wecom adapter is not registered",
        )
    plain = await adapter.verify_url(dict(request.query_params))
    return Response(content=plain, media_type="text/plain")


@router.post("")
async def wecom_webhook(
    request: Request,
    adapters: AdapterRegistry = Depends(get_adapter_registry),
    message_repo: MessageRepository = Depends(get_message_repo),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    rule_repo: RuleRepository = Depends(get_rule_repo),
    detector: OpportunityDetector = Depends(get_detector),
    work_time=Depends(get_work_time_service),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    user_settings_repo: UserSettingsRepository = Depends(get_user_settings_repo),
) -> dict:
    body = await request.body()
    payload = parse_xml_envelope(body)
    adapter = adapters.get(IMChannel.WECOM)
    inbound = await adapter.parse_webhook(
        {"xml": payload},
        dict(request.headers),
        dict(request.query_params),
    )
    if not inbound:
        return {"ok": True, "ignored": True}

    use_case = IngestMessageUseCase(
        message_repo=message_repo,
        opportunity_repo=opportunity_repo,
        rule_repo=rule_repo,
        detector=detector,
        work_time=work_time,
        task_queue=task_queue,
        subscription_repo=subscription_repo,
        user_settings_repo=user_settings_repo,
    )
    result = await use_case.execute(inbound)
    response = {"ok": True, "id": str(result.id), "type": result.__class__.__name__}
    if isinstance(result, Opportunity):
        response["opportunity"] = to_opportunity_read(result).model_dump(mode="json")
    return response


@router.get("/{connection_id}", name="wecom_connection_verify_url")
async def wecom_connection_verify_url(
    connection_id: UUID,
    request: Request,
    adapters: AdapterRegistry = Depends(get_adapter_registry),
    connection_repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
) -> Response:
    connection = await connection_repo.get(connection_id)
    if not connection or not connection.enabled:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="wecom connection not found"
        )
    adapter = adapters.get(IMChannel.WECOM)
    if not isinstance(adapter, WeComAdapter):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR, detail="wecom adapter unavailable"
        )
    try:
        plain = await adapter.verify_url(
            dict(request.query_params),
            credentials=WeComCredentials.from_connection(connection, adapter.settings),
        )
    except (WeComCryptoError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid wecom callback"
        ) from exc
    await connection_repo.mark_verified(connection)
    return Response(content=plain, media_type="text/plain")


@router.post("/{connection_id}")
async def wecom_connection_webhook(
    connection_id: UUID,
    request: Request,
    settings: Settings = Depends(get_settings),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
    connection_repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
    event_repo: WeComEventRepository = Depends(get_wecom_event_repo),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
) -> Response:
    body = await request.body()
    if len(body) > settings.wecom_webhook_max_body_bytes:
        raise HTTPException(
            status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE, detail="wecom payload too large"
        )
    connection = await connection_repo.get(connection_id)
    if not connection or not connection.enabled:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="wecom connection not found"
        )
    adapter = adapters.get(IMChannel.WECOM)
    if not isinstance(adapter, WeComAdapter):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR, detail="wecom adapter unavailable"
        )
    try:
        payload = parse_xml_envelope(body)
        inbound = await adapter.parse_webhook(
            {"xml": payload},
            dict(request.headers),
            dict(request.query_params),
            credentials=WeComCredentials.from_connection(connection, settings),
            connection=connection,
        )
    except (WeComCryptoError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid wecom callback"
        ) from exc

    payload_hash = hashlib.sha256(body).hexdigest()
    provider_event_id = inbound.external_message_id if inbound else f"ignored:{payload_hash}"
    event, should_queue = await event_repo.reserve(
        connection=connection,
        provider_event_id=provider_event_id[:255],
        event_type="text" if inbound else "ignored",
        payload_hash=payload_hash,
        normalized_payload_encrypted=(
            encrypt_secret(serialize_inbound(inbound), settings) if inbound else None
        ),
        ignored=inbound is None,
    )
    if not should_queue:
        return Response(content="success", media_type="text/plain")
    if not task_queue.enqueue_wecom_event(event.id):
        await event_repo.fail(event.id, "WeCom event enqueue failed", final=False)
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="wecom event queue unavailable"
        )
    await event_repo.mark_queued(event)
    if connection.status.value != "active":
        await connection_repo.mark_verified(connection)
    return Response(content="success", media_type="text/plain")
