from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Request, Response, status

from app.api.deps import (
    DevicePrincipal,
    get_analysis_link_inspector,
    get_analysis_run_service,
    get_device_agent_routing_service,
    require_admin,
    require_analysis_run_principal,
    require_device_principal,
)
from app.application.dto import (
    AnalysisRunClaimRead,
    AnalysisRunClaimRequest,
    AnalysisRunCompleteRequest,
    AnalysisRunFailRequest,
    AnalysisRunHeartbeatRequest,
    AnalysisRunInputRead,
    AnalysisRunLinksRead,
    AnalysisRunNextClaimRead,
    AnalysisRolloutReadinessRead,
    AnalysisRunRead,
    AnalysisRunShadowClaimRead,
)
from app.application.use_cases.analysis_run import (
    AnalysisRunClaim,
    AnalysisRunService,
    AnalysisRunTokenPrincipal,
    DeviceAgentRoutingService,
)
from app.domain.services.analysis_run import (
    AnalysisRunConflictError,
    AnalysisRunLeaseExpiredError,
    AnalysisRunNotFoundError,
    AnalysisRunQuotaExceededError,
    AnalysisRunTokenRejectedError,
    AnalysisRunUnavailableError,
    AnalysisRunVersionConflictError,
)
from app.infrastructure.agent.link_inspector import SafeLinkInspector
from app.infrastructure.db.models import AnalysisRun

router = APIRouter()


@router.get("/rollout-readiness", response_model=AnalysisRolloutReadinessRead)
async def get_analysis_rollout_readiness(
    _: object = Depends(require_admin),
    routing: DeviceAgentRoutingService = Depends(get_device_agent_routing_service),
) -> AnalysisRolloutReadinessRead:
    readiness = await routing.rollout_readiness()
    evidence = readiness.evidence
    settings = routing.settings
    return AnalysisRolloutReadinessRead(
        ready=readiness.ready,
        enforced=settings.device_agent_rollout_require_shadow_ready,
        primaryGateOpen=await routing.primary_globally_ready(readiness=readiness),
        terminalSamples=evidence.terminal_samples,
        completedSamples=evidence.completed_samples,
        matchedSamples=evidence.matched_samples,
        successRate=evidence.success_rate,
        matchRate=evidence.match_rate,
        p95Seconds=evidence.p95_seconds,
        minimumSamples=settings.device_agent_rollout_min_shadow_samples,
        minimumSuccessRate=settings.device_agent_rollout_min_shadow_success_rate,
        minimumMatchRate=settings.device_agent_rollout_min_shadow_match_rate,
        maximumP95Seconds=settings.device_agent_rollout_max_p95_seconds,
        rolloutPercentage=settings.rn_device_agent_rollout_percentage,
        allowlistedDeviceCount=len(settings.device_agent_rollout_device_ids),
        reasons=list(routing.primary_gate_reasons(readiness)),
    )


def _run_read(run: AnalysisRun) -> AnalysisRunRead:
    return AnalysisRunRead(
        id=run.id,
        messageId=run.message_id,
        deviceId=run.device_id,
        status=run.status,
        executedBy=run.executor,
        mode=run.mode,
        runtimeVersion=run.runtime_version,
        schemaVersion=run.schema_version,
        modelAlias=run.model_alias,
        policyVersion=run.policy_version,
        sourceMessageVersion=run.source_message_version,
        lockVersion=run.lock_version,
        leaseExpiresAt=run.lease_expires_at,
        claimedAt=run.claimed_at,
        heartbeatAt=run.heartbeat_at,
        completedAt=run.completed_at,
        failedAt=run.failed_at,
        expiredAt=run.expired_at,
        failureCode=run.failure_code,
        shadowMatch=run.shadow_match,
        shadowDifferenceCount=run.shadow_difference_count,
    )


def _claim_read(claim: AnalysisRunClaim) -> AnalysisRunClaimRead:
    run = claim.run
    message = claim.message
    return AnalysisRunClaimRead(
        **_run_read(run).model_dump(),
        runToken=claim.run_token,
        input=AnalysisRunInputRead(
            messageId=message.id,
            sourceMessageVersion=run.source_message_version,
            channel=message.channel,
            senderDisplayName=message.sender_display_name,
            sourceType=message.source_type,
            groupName=message.group_name,
            text=message.text or "",
            links=message.raw_message_links[:10],
        ),
    )


def _raise_run_error(exc: Exception) -> None:
    if isinstance(exc, AnalysisRunUnavailableError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="device analysis is unavailable",
        ) from exc
    if isinstance(exc, AnalysisRunNotFoundError):
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="analysis run not found",
        ) from exc
    if isinstance(exc, AnalysisRunQuotaExceededError):
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail="monthly pi agent quota exceeded",
        ) from exc
    if isinstance(exc, AnalysisRunTokenRejectedError):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    if isinstance(exc, AnalysisRunLeaseExpiredError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="analysis run lease expired",
        ) from exc
    if isinstance(exc, AnalysisRunVersionConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="analysis run version conflict",
        ) from exc
    if isinstance(exc, AnalysisRunConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="analysis run conflict",
        ) from exc
    raise exc


@router.post("/claim", response_model=AnalysisRunClaimRead, status_code=status.HTTP_201_CREATED)
async def claim_analysis_run(
    payload: AnalysisRunClaimRequest,
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunClaimRead:
    assert principal.device is not None
    try:
        claim = await service.claim(
            owner_user_id=principal.user.id,
            device=principal.device,
            message_id=payload.messageId,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _claim_read(claim)


@router.post("/claim-shadow", response_model=AnalysisRunShadowClaimRead)
async def claim_shadow_analysis_run(
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunShadowClaimRead:
    assert principal.device is not None
    try:
        claim = await service.claim_shadow(
            owner_user_id=principal.user.id,
            device=principal.device,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return AnalysisRunShadowClaimRead(claim=_claim_read(claim) if claim else None)


@router.post("/claim-next", response_model=AnalysisRunNextClaimRead)
async def claim_next_analysis_run(
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunNextClaimRead:
    assert principal.device is not None
    try:
        claim = await service.claim_next(
            owner_user_id=principal.user.id,
            device=principal.device,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return AnalysisRunNextClaimRead(claim=_claim_read(claim) if claim else None)


@router.post("/{run_id}/heartbeat", response_model=AnalysisRunRead)
async def heartbeat_analysis_run(
    run_id: UUID,
    payload: AnalysisRunHeartbeatRequest,
    response: Response,
    principal: AnalysisRunTokenPrincipal = Depends(require_analysis_run_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunRead:
    if principal.run_id != run_id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="analysis run not found")
    try:
        run = await service.heartbeat(
            principal,
            expected_lock_version=payload.expectedLockVersion,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _run_read(run)


@router.post("/{run_id}/complete", response_model=AnalysisRunRead)
async def complete_analysis_run(
    run_id: UUID,
    payload: AnalysisRunCompleteRequest,
    response: Response,
    principal: AnalysisRunTokenPrincipal = Depends(require_analysis_run_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunRead:
    if principal.run_id != run_id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="analysis run not found")
    try:
        run = await service.complete(
            principal,
            expected_lock_version=payload.expectedLockVersion,
            result=payload.result,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _run_read(run)


@router.post("/{run_id}/fail", response_model=AnalysisRunRead)
async def fail_analysis_run(
    run_id: UUID,
    payload: AnalysisRunFailRequest,
    response: Response,
    principal: AnalysisRunTokenPrincipal = Depends(require_analysis_run_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunRead:
    if principal.run_id != run_id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="analysis run not found")
    try:
        run = await service.fail(
            principal,
            expected_lock_version=payload.expectedLockVersion,
            failure_code=payload.failureCode,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _run_read(run)


@router.post("/{run_id}/links/inspect", response_model=AnalysisRunLinksRead)
async def inspect_analysis_run_links(
    run_id: UUID,
    request: Request,
    response: Response,
    principal: AnalysisRunTokenPrincipal = Depends(require_analysis_run_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
    inspector: SafeLinkInspector = Depends(get_analysis_link_inspector),
) -> AnalysisRunLinksRead:
    if principal.run_id != run_id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="analysis run not found")
    async for chunk in request.stream():
        if chunk.strip():
            raise HTTPException(
                status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
                detail="link inspection does not accept URLs",
            )
    try:
        run, evidence = await service.inspect_links(principal, inspector=inspector)
    except Exception as exc:
        _raise_run_error(exc)
    assert run.link_evidence_fetched_at is not None
    response.headers["Cache-Control"] = "no-store"
    return AnalysisRunLinksRead(
        runId=run.id,
        sourceMessageVersion=run.source_message_version,
        fetchedAt=run.link_evidence_fetched_at,
        evidence=evidence,
    )


@router.post("/{run_id}/expire", response_model=AnalysisRunRead)
async def expire_analysis_run(
    run_id: UUID,
    response: Response,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: AnalysisRunService = Depends(get_analysis_run_service),
) -> AnalysisRunRead:
    assert principal.device is not None
    try:
        run = await service.expire(
            owner_user_id=principal.user.id,
            device_id=principal.device.id,
            run_id=run_id,
        )
    except Exception as exc:
        _raise_run_error(exc)
    response.headers["Cache-Control"] = "no-store"
    return _run_read(run)
