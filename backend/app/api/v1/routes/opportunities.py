from datetime import datetime
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
    DashboardRead,
    FriendRequestUpdate,
    ManualReplyRequest,
    OpportunityArchiveRequest,
    OpportunityBulkArchiveRead,
    OpportunityBulkArchiveRequest,
    OpportunityDetailRead,
    OpportunityRead,
    OpportunityStatusUpdate,
)
from app.application.mappers import to_opportunity_detail, to_opportunity_read
from app.application.use_cases.ai_reply import AIDraftUseCase
from app.application.use_cases.manual_reply import ManualReplyUseCase
from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.domain.enums import (
    AgentAnalysisStatus,
    FrontendOpportunityStatus,
    IMChannel,
    OpportunityArchiveScope,
)
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


# 与 Web frontend/lib/sop.ts 的 trustLevel 边界严格一致。
TRUST_LEVEL_RANGES: dict[str, tuple[int, int]] = {
    "trusted": (80, 100),
    "unverified": (60, 79),
    "suspicious": (40, 59),
    "risky": (0, 39),
}
DASHBOARD_SORTS = {"newest", "oldest", "confidence", "trust"}
SOURCE_TYPES = {"group", "private"}


@router.get("/dashboard", response_model=DashboardRead)
async def dashboard(
    status_filter: FrontendOpportunityStatus | None = Query(default=None, alias="status"),
    platform: IMChannel | None = None,
    source_type: str | None = Query(default=None, alias="source_type"),
    created_from: datetime | None = Query(default=None, alias="created_from"),
    created_to: datetime | None = Query(default=None, alias="created_to"),
    trust_levels: list[str] | None = Query(default=None, alias="trust_levels"),
    sop_stages: list[str] | None = Query(default=None, alias="sop_stages"),
    keywords: list[str] | None = Query(default=None, alias="keywords"),
    sort: str = Query(default="newest"),
    limit: int = Query(default=20, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> DashboardRead:
    """商机看板聚合端点：数据库层完成筛选/排序/分页，所有结果按 owner 隔离。

    数组参数用重复 query（?trust_levels=trusted&trust_levels=risky）。旧的 GET
    /opportunities 保持不变，Web 与旧版 App 不受影响。
    """
    if sort not in DASHBOARD_SORTS:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=f"sort must be one of {sorted(DASHBOARD_SORTS)}",
        )
    if source_type is not None and source_type not in SOURCE_TYPES:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=f"source_type must be one of {sorted(SOURCE_TYPES)}",
        )
    trust_ranges: list[tuple[int, int]] | None = None
    if trust_levels:
        try:
            trust_ranges = [TRUST_LEVEL_RANGES[level] for level in trust_levels]
        except KeyError as exc:
            raise HTTPException(
                status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
                detail=f"unknown trust level: {exc.args[0]}",
            ) from exc
    if created_from and created_to and created_from > created_to:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail="created_from must not be after created_to",
        )

    items, total = await repo.dashboard(
        owner_user_id=current_user.id,
        frontend_status=status_filter,
        channel=platform,
        source_type=source_type,
        created_from=created_from,
        created_to=created_to,
        trust_ranges=trust_ranges,
        sop_stages=sop_stages or None,
        keywords=keywords or None,
        sort=sort,
        limit=limit,
        offset=offset,
    )
    pending_count = await repo.count_pending(current_user.id)
    attention = await repo.list_attention(current_user.id)
    keyword_options = await repo.keyword_options(current_user.id)
    return DashboardRead(
        items=[to_opportunity_read(item) for item in items],
        total=total,
        limit=limit,
        offset=offset,
        pendingCount=pending_count,
        attentionItems=[to_opportunity_read(item) for item in attention],
        keywordOptions=keyword_options,
    )


@router.get("", response_model=list[OpportunityRead])
async def list_opportunities(
    status_filter: FrontendOpportunityStatus | None = Query(default=None, alias="status"),
    platform: IMChannel | None = None,
    archive_scope: OpportunityArchiveScope = Query(
        default=OpportunityArchiveScope.ACTIVE,
        alias="archive",
    ),
    limit: int = Query(default=100, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> list[OpportunityRead]:
    opportunities = await repo.list(
        frontend_status=status_filter,
        channel=platform,
        owner_user_id=current_user.id,
        archive_scope=archive_scope,
        limit=limit,
        offset=offset,
    )
    return [to_opportunity_read(opportunity) for opportunity in opportunities]


@router.post("/bulk-archive", response_model=OpportunityBulkArchiveRead)
async def bulk_archive_opportunities(
    payload: OpportunityBulkArchiveRequest,
    current_user: User = Depends(require_user),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityBulkArchiveRead:
    try:
        opportunities, archived_count = await repo.archive_many(
            owner_user_id=current_user.id,
            opportunity_ids=payload.opportunityIds,
            reason=payload.reason,
        )
    except LookupError as exc:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="one or more opportunities were not found",
        ) from exc
    return OpportunityBulkArchiveRead(
        archivedCount=archived_count,
        opportunities=[to_opportunity_read(opportunity) for opportunity in opportunities],
    )


@router.get("/{opportunity_id}", response_model=OpportunityDetailRead)
async def get_opportunity(
    opportunity: Opportunity = Depends(get_opportunity_or_404),
) -> OpportunityDetailRead:
    return to_opportunity_detail(opportunity)


@router.post("/{opportunity_id}/archive", response_model=OpportunityDetailRead)
async def archive_opportunity(
    payload: OpportunityArchiveRequest,
    current_user: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    updated = await repo.archive(
        opportunity,
        actor_user_id=current_user.id,
        reason=payload.reason,
    )
    return to_opportunity_detail(updated)


@router.post("/{opportunity_id}/restore", response_model=OpportunityDetailRead)
async def restore_opportunity(
    current_user: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    updated = await repo.restore(opportunity, actor_user_id=current_user.id)
    return to_opportunity_detail(updated)


def ensure_opportunity_is_active(opportunity: Opportunity) -> None:
    if opportunity.archived_at is not None:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="restore the archived opportunity before modifying it",
        )


@router.post("/{opportunity_id}/manual-reply", response_model=OpportunityDetailRead)
async def manual_reply(
    payload: ManualReplyRequest,
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    message_repo: MessageRepository = Depends(get_message_repo),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
) -> OpportunityDetailRead:
    ensure_opportunity_is_active(opportunity)
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
    ensure_opportunity_is_active(opportunity)
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
    ensure_opportunity_is_active(opportunity)
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


# operator 手动驱动的合法流转；发送→等待→人工确认结果，被拒可重试。
FRIEND_REQUEST_TRANSITIONS: dict[str, set[str]] = {
    "not_sent": {"pending"},
    "pending": {"accepted", "rejected"},
    "rejected": {"not_sent"},
}


@router.post("/{opportunity_id}/friend-request", response_model=OpportunityDetailRead)
async def update_friend_request(
    payload: FriendRequestUpdate,
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    """持久化好友申请进度（发送/通过/被拒/重试），非法流转返回 409。

    平台没有自动发送好友申请的 IM 能力，本端点只记录操作员声明的真实进度；
    "已通过"必须由操作员确认回填，服务端不做任何定时伪造。
    """
    if opportunity.source_type != "group" or opportunity.friend_request_status == "n/a":
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="该商机来自私聊，无需好友申请",
        )
    allowed = FRIEND_REQUEST_TRANSITIONS.get(opportunity.friend_request_status, set())
    if payload.status not in allowed:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"好友申请状态不能从 {opportunity.friend_request_status} 变为 {payload.status}",
        )
    updated = await repo.set_friend_request(opportunity, status=payload.status)
    return to_opportunity_detail(updated)


@router.patch("/{opportunity_id}/status", response_model=OpportunityDetailRead)
async def update_status(
    payload: OpportunityStatusUpdate,
    _: User = Depends(require_user),
    opportunity: Opportunity = Depends(get_opportunity_or_404),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> OpportunityDetailRead:
    ensure_opportunity_is_active(opportunity)
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
    ensure_opportunity_is_active(opportunity)
    if opportunity.id != opportunity_id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="id mismatch")
    updated = await repo.update_status(opportunity, opportunity.status, assigned_to=operator_id)
    return to_opportunity_detail(updated)
