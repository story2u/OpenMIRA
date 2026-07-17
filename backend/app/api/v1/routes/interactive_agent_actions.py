from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Response, status

from app.api.deps import (
    get_interactive_agent_action_service,
    get_message_repo,
    require_interactive_agent_approval_principal,
    require_interactive_agent_turn_principal,
)
from app.application.dto import (
    InteractiveAgentApprovalDecisionRead,
    InteractiveAgentApprovalDecisionRequest,
    InteractiveAgentApprovedSendRead,
    InteractiveAgentApprovedSendRequest,
)
from app.application.mappers import to_chat_message_read, to_opportunity_detail
from app.application.use_cases.interactive_agent_action import (
    InteractiveAgentActionService,
    InteractiveAgentApprovalTokenPrincipal,
)
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentTurnTokenPrincipal,
)
from app.domain.services.interactive_agent_action import (
    InteractiveAgentActionConflictError,
    InteractiveAgentActionExpiredError,
    InteractiveAgentActionProjectionError,
    InteractiveAgentActionRejectedError,
    InteractiveAgentActionUncertainError,
    InteractiveAgentActionUnavailableError,
)
from app.infrastructure.db.repositories import MessageRepository

router = APIRouter()


def _raise_action_error(exc: Exception) -> None:
    if isinstance(exc, InteractiveAgentActionUnavailableError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="interactive Agent external actions are unavailable",
        ) from exc
    if isinstance(exc, InteractiveAgentActionExpiredError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent approval expired",
        ) from exc
    if isinstance(exc, InteractiveAgentActionConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent action conflict",
        ) from exc
    if isinstance(exc, InteractiveAgentActionRejectedError):
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="interactive Agent action rejected",
        ) from exc
    if isinstance(exc, InteractiveAgentActionProjectionError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="reply was delivered but its local projection is incomplete",
        ) from exc
    if isinstance(exc, InteractiveAgentActionUncertainError):
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="reply delivery outcome is uncertain; review before retrying",
        ) from exc
    raise exc


@router.post(
    "/approvals",
    response_model=InteractiveAgentApprovalDecisionRead,
)
async def decide_interactive_agent_action(
    payload: InteractiveAgentApprovalDecisionRequest,
    response: Response,
    principal: InteractiveAgentTurnTokenPrincipal = Depends(
        require_interactive_agent_turn_principal
    ),
    service: InteractiveAgentActionService = Depends(get_interactive_agent_action_service),
) -> InteractiveAgentApprovalDecisionRead:
    try:
        decision = await service.decide(
            principal,
            approved=payload.approved,
            expected_version=payload.expectedVersion,
            idempotency_key=payload.idempotencyKey,
            opportunity_id=payload.opportunityId,
            text=payload.text,
            tool_call_id=payload.toolCallId,
        )
    except Exception as exc:
        _raise_action_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return InteractiveAgentApprovalDecisionRead(
        id=decision.approval.id,
        status=decision.approval.status,
        toolCallId=decision.approval.tool_call_id,
        opportunityId=decision.approval.opportunity_id,
        expectedVersion=decision.approval.expected_version,
        expiresAt=decision.approval.expires_at,
        approvalToken=decision.approval_token,
    )


@router.post(
    "/send-reply",
    response_model=InteractiveAgentApprovedSendRead,
)
async def execute_interactive_agent_send_reply(
    payload: InteractiveAgentApprovedSendRequest,
    response: Response,
    principal: InteractiveAgentApprovalTokenPrincipal = Depends(
        require_interactive_agent_approval_principal
    ),
    service: InteractiveAgentActionService = Depends(get_interactive_agent_action_service),
    message_repo: MessageRepository = Depends(get_message_repo),
) -> InteractiveAgentApprovedSendRead:
    try:
        result = await service.execute_send_reply(
            principal,
            expected_version=payload.expectedVersion,
            idempotency_key=payload.idempotencyKey,
            opportunity_id=payload.opportunityId,
            text=payload.text,
        )
    except Exception as exc:
        _raise_action_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return InteractiveAgentApprovedSendRead(
        approvalId=principal.approval_id,
        opportunity=to_opportunity_detail(result.opportunity),
        message=to_chat_message_read(result.message),
        messageTotal=await message_repo.count_by_opportunity(result.opportunity.id),
    )
