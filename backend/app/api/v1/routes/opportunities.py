from uuid import UUID, uuid4

from fastapi import APIRouter, Depends, Header, HTTPException, Query, status

from app.api.deps import (
    get_adapter_registry,
    get_message_repo,
    get_opportunity_or_404,
    get_opportunity_repo,
    get_reply_generator,
    get_subscription_repo,
    get_task_queue,
    require_user,
)
from app.application.dto import (
    AgentAnalysisEnqueueRead,
    AIDraftResponse,
    ManualReplyRequest,
    OpportunityDetailRead,
    OpportunityRead,
    OpportunityStatusUpdate,
)
from app.application.mappers import to_opportunity_detail, to_opportunity_read
from app.application.use_cases.ai_reply import AIDraftUseCase
from app.application.use_cases.manual_reply import ManualReplyUseCase
from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.domain.enums import AgentAnalysisStatus, FrontendOpportunityStatus, IMChannel
from app.domain.services.opportunity_state import (
    InvalidOpportunityTransition,
    ensure_transition_allowed,
)
from app.infrastructure.ai.litellm_client import LiteLLMReplyGenerator
from app.infrastructure.db.models import Opportunity, User
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    SubscriptionRepository,
)
from app.infrastructure.im.base import AdapterRegistry
from app.worker.queue import CeleryTaskQueue

router = APIRouter()


@router.get("", response_model=list[OpportunityRead])
async def list_opportunities(
    status_filter: FrontendOpportunityStatus | None = Query(default=None, alias="status"),
    platform: IMChannel | None = None,
    limit: int = Query(default=100, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> list[OpportunityRead]:
    opportunities = await repo.list(
        frontend_status=status_filter,
        channel=platform,
        owner_user_id=current_user.id,
        limit=limit,
        offset=offset,
    )
    return [to_opportunity_read(opportunity) for opportunity in opportunities]


@router.get("/{opportunity_id}", response_model=OpportunityDetailRead)
async def get_opportunity(
    opportunity: Opportunity = Depends(get_opportunity_or_404),
) -> OpportunityDetailRead:
    return to_opportunity_detail(opportunity)


@router.post("/{opportunity_id}/manual-reply", response_model=OpportunityDetailRead)
async def manual_reply(
    payload: ManualReplyRequest,
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    message_repo: MessageRepository = Depends(get_message_repo),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
) -> OpportunityDetailRead:
    use_case = ManualReplyUseCase(
        opportunity_repo=opportunity_repo,
        message_repo=message_repo,
        adapters=adapters,
    )
    try:
        updated = await use_case.execute(
            opportunity=opportunity,
            text=payload.text,
            operator_id=payload.operator_id,
            mark_following=payload.mark_following,
        )
    except InvalidOpportunityTransition as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    return to_opportunity_detail(updated)


@router.post("/{opportunity_id}/ai-draft", response_model=AIDraftResponse)
async def generate_ai_draft(
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    reply_generator: LiteLLMReplyGenerator = Depends(get_reply_generator),
) -> AIDraftResponse:
    use_case = AIDraftUseCase(
        opportunity_repo=opportunity_repo,
        reply_generator=reply_generator,
    )
    draft = await use_case.execute(opportunity)
    return AIDraftResponse(opportunity_id=opportunity.id, draft=draft)


@router.post(
    "/{opportunity_id}/agent-analysis",
    response_model=AgentAnalysisEnqueueRead,
    status_code=status.HTTP_202_ACCEPTED,
)
async def enqueue_agent_analysis(
    current_user: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    message_repo: MessageRepository = Depends(get_message_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
    idempotency_key: str | None = Header(default=None, alias="Idempotency-Key"),
) -> AgentAnalysisEnqueueRead:
    if not opportunity.source_message_id:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="opportunity has no source message to analyze",
        )
    source_message = await message_repo.get(opportunity.source_message_id)
    if not source_message or source_message.owner_user_id != current_user.id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="source message not found")
    if source_message.agent_analysis_status == AgentAnalysisStatus.RUNNING:
        return AgentAnalysisEnqueueRead(
            messageId=source_message.id,
            status=AgentAnalysisStatus.RUNNING,
        )
    request_key = (idempotency_key or str(uuid4())).strip()
    if not request_key or len(request_key) > 200:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail="Idempotency-Key must contain between 1 and 200 characters",
        )
    result = await ScheduleAgentAnalysisUseCase(
        message_repo=message_repo,
        subscription_repo=subscription_repo,
        task_queue=task_queue,
    ).execute(
        source_message,
        idempotency_key=f"manual:{request_key}",
        force=True,
    )
    if result.status == AgentAnalysisStatus.QUOTA_EXCEEDED:
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail=(
                f"monthly pi agent quota exceeded ({result.quota_allocated}/{result.quota_limit})"
            ),
        )
    if result.status == AgentAnalysisStatus.FAILED:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="pi agent is disabled or the analysis queue is unavailable",
        )
    return AgentAnalysisEnqueueRead(
        messageId=result.message_id,
        status=result.status,
    )


@router.patch("/{opportunity_id}/status", response_model=OpportunityDetailRead)
async def update_status(
    payload: OpportunityStatusUpdate,
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    try:
        ensure_transition_allowed(opportunity.status, payload.status)
    except InvalidOpportunityTransition as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    updated = await repo.update_status(opportunity, payload.status)
    return to_opportunity_detail(updated)


@router.post("/{opportunity_id}/claim", response_model=OpportunityDetailRead)
async def claim_opportunity(
    opportunity_id: UUID,
    operator_id: str = Query(min_length=1, max_length=128),
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    if opportunity.id != opportunity_id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="id mismatch")
    updated = await repo.update_status(opportunity, opportunity.status, assigned_to=operator_id)
    return to_opportunity_detail(updated)
