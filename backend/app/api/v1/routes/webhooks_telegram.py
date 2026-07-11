from fastapi import APIRouter, Depends, Request

from app.api.deps import (
    get_adapter_registry,
    get_detector,
    get_message_repo,
    get_opportunity_repo,
    get_rule_repo,
    get_subscription_repo,
    get_task_queue,
    get_work_time_service,
)
from app.application.mappers import to_opportunity_read
from app.application.use_cases.ingest_message import IngestMessageUseCase
from app.domain.enums import IMChannel
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
)
from app.infrastructure.im.base import AdapterRegistry
from app.worker.queue import CeleryTaskQueue

router = APIRouter()


@router.post("")
async def telegram_webhook(
    request: Request,
    adapters: AdapterRegistry = Depends(get_adapter_registry),
    message_repo: MessageRepository = Depends(get_message_repo),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    rule_repo: RuleRepository = Depends(get_rule_repo),
    detector: OpportunityDetector = Depends(get_detector),
    work_time=Depends(get_work_time_service),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> dict:
    payload = await request.json()
    adapter = adapters.get(IMChannel.TELEGRAM)
    inbound = await adapter.parse_webhook(payload, dict(request.headers), dict(request.query_params))
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
    )
    result = await use_case.execute(inbound)
    response = {"ok": True, "id": str(result.id), "type": result.__class__.__name__}
    if isinstance(result, Opportunity):
        response["opportunity"] = to_opportunity_read(result).model_dump(mode="json")
    return response
