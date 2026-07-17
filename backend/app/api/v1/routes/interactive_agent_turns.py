from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Response, status

from app.api.deps import (
    DevicePrincipal,
    get_interactive_agent_turn_service,
    require_device_principal,
    require_interactive_agent_turn_principal,
)
from app.application.dto import (
    InteractiveAgentTurnClaimRead,
    InteractiveAgentTurnClaimRequest,
    InteractiveAgentTurnCompleteRequest,
    InteractiveAgentTurnFailRequest,
    InteractiveAgentTurnHeartbeatRequest,
    InteractiveAgentTurnRead,
)
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentTurnService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.domain.services.interactive_agent import (
    InteractiveAgentTurnConflictError,
    InteractiveAgentTurnLeaseExpiredError,
    InteractiveAgentTurnNotFoundError,
    InteractiveAgentTurnQuotaExceededError,
    InteractiveAgentTurnTokenRejectedError,
    InteractiveAgentTurnUnavailableError,
    InteractiveAgentTurnVersionConflictError,
)
from app.infrastructure.db.models import InteractiveAgentTurn

router = APIRouter()


def _turn_read(turn: InteractiveAgentTurn) -> InteractiveAgentTurnRead:
    return InteractiveAgentTurnRead(
        id=turn.id,
        localSessionId=turn.local_session_id,
        deviceId=turn.device_id,
        status=turn.status,
        runtimeVersion=turn.runtime_version,
        schemaVersion=turn.schema_version,
        modelAlias=turn.model_alias,
        policyVersion=turn.policy_version,
        lockVersion=turn.lock_version,
        requestCount=turn.request_count,
        leaseExpiresAt=turn.lease_expires_at,
        claimedAt=turn.claimed_at,
        heartbeatAt=turn.heartbeat_at,
        completedAt=turn.completed_at,
        failedAt=turn.failed_at,
        expiredAt=turn.expired_at,
        failureCode=turn.failure_code,
    )


def _raise_turn_error(exc: Exception) -> None:
    if isinstance(exc, InteractiveAgentTurnUnavailableError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="interactive Agent is unavailable",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnNotFoundError):
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="interactive Agent turn not found",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnQuotaExceededError):
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail="interactive Agent monthly turn quota exceeded",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnTokenRejectedError):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnLeaseExpiredError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent turn lease expired",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnVersionConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent turn version conflict",
        ) from exc
    if isinstance(exc, InteractiveAgentTurnConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent turn conflict",
        ) from exc
    raise exc


@router.post("", response_model=InteractiveAgentTurnClaimRead)
async def claim_interactive_agent_turn(
    payload: InteractiveAgentTurnClaimRequest,
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: InteractiveAgentTurnService = Depends(get_interactive_agent_turn_service),
) -> InteractiveAgentTurnClaimRead:
    assert principal.device is not None
    try:
        claim = await service.claim(
            owner_user_id=principal.user.id,
            device=principal.device,
            local_session_id=payload.localSessionId,
            idempotency_key=payload.idempotencyKey,
        )
    except Exception as exc:
        _raise_turn_error(exc)
    response.status_code = status.HTTP_201_CREATED if claim.created else status.HTTP_200_OK
    response.headers["Cache-Control"] = "no-store"
    return InteractiveAgentTurnClaimRead(
        **_turn_read(claim.turn).model_dump(),
        turnToken=claim.turn_token,
    )


@router.post("/{turn_id}/heartbeat", response_model=InteractiveAgentTurnRead)
async def heartbeat_interactive_agent_turn(
    turn_id: UUID,
    payload: InteractiveAgentTurnHeartbeatRequest,
    response: Response,
    principal: InteractiveAgentTurnTokenPrincipal = Depends(
        require_interactive_agent_turn_principal
    ),
    service: InteractiveAgentTurnService = Depends(get_interactive_agent_turn_service),
) -> InteractiveAgentTurnRead:
    if principal.turn_id != turn_id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="interactive Agent turn not found",
        )
    try:
        turn = await service.heartbeat(
            principal,
            expected_lock_version=payload.expectedLockVersion,
        )
    except Exception as exc:
        _raise_turn_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _turn_read(turn)


@router.post("/{turn_id}/complete", response_model=InteractiveAgentTurnRead)
async def complete_interactive_agent_turn(
    turn_id: UUID,
    payload: InteractiveAgentTurnCompleteRequest,
    response: Response,
    principal: InteractiveAgentTurnTokenPrincipal = Depends(
        require_interactive_agent_turn_principal
    ),
    service: InteractiveAgentTurnService = Depends(get_interactive_agent_turn_service),
) -> InteractiveAgentTurnRead:
    if principal.turn_id != turn_id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="interactive Agent turn not found",
        )
    try:
        turn = await service.complete(
            principal,
            expected_lock_version=payload.expectedLockVersion,
        )
    except Exception as exc:
        _raise_turn_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _turn_read(turn)


@router.post("/{turn_id}/fail", response_model=InteractiveAgentTurnRead)
async def fail_interactive_agent_turn(
    turn_id: UUID,
    payload: InteractiveAgentTurnFailRequest,
    response: Response,
    principal: InteractiveAgentTurnTokenPrincipal = Depends(
        require_interactive_agent_turn_principal
    ),
    service: InteractiveAgentTurnService = Depends(get_interactive_agent_turn_service),
) -> InteractiveAgentTurnRead:
    if principal.turn_id != turn_id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="interactive Agent turn not found",
        )
    try:
        turn = await service.fail(
            principal,
            expected_lock_version=payload.expectedLockVersion,
            failure_code=payload.failureCode,
        )
    except Exception as exc:
        _raise_turn_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _turn_read(turn)


@router.post("/{turn_id}/expire", response_model=InteractiveAgentTurnRead)
async def expire_interactive_agent_turn(
    turn_id: UUID,
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: InteractiveAgentTurnService = Depends(get_interactive_agent_turn_service),
) -> InteractiveAgentTurnRead:
    assert principal.device is not None
    try:
        turn = await service.expire(
            owner_user_id=principal.user.id,
            device_id=principal.device.id,
            turn_id=turn_id,
        )
    except Exception as exc:
        _raise_turn_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _turn_read(turn)
