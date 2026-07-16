from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status

from app.api.deps import (
    get_message_repo,
    get_pi_agent_client,
    get_source_functional_profile_repo,
    get_subscription_repo,
    require_user,
)
from app.application.dto import SourceFunctionalProfileRead, SourceFunctionOverrideRequest
from app.application.job_mappers import to_source_profile_read
from app.application.use_cases.reprofile_source_function import (
    ReprofileSourceFunctionUseCase,
    SourceProfileQuotaExceeded,
)
from app.core.config import Settings, get_settings
from app.infrastructure.agent.pi_client import PiAgentClient, PiAgentError
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import (
    MessageRepository,
    SourceFunctionalProfileRepository,
    SubscriptionRepository,
)

router = APIRouter()


async def _profile_or_404(
    profile_id: UUID,
    owner_user_id: UUID,
    repo: SourceFunctionalProfileRepository,
):
    profile = await repo.get_by_id_for_owner(profile_id, owner_user_id)
    if not profile:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="source profile not found"
        )
    return profile


@router.get("/{profile_id}/functional-profile", response_model=SourceFunctionalProfileRead)
async def get_functional_profile(
    profile_id: UUID,
    current_user: User = Depends(require_user),
    repo: SourceFunctionalProfileRepository = Depends(get_source_functional_profile_repo),
) -> SourceFunctionalProfileRead:
    return to_source_profile_read(await _profile_or_404(profile_id, current_user.id, repo))


@router.post(
    "/{profile_id}/functional-profile/recompute",
    response_model=SourceFunctionalProfileRead,
)
async def recompute_functional_profile(
    profile_id: UUID,
    current_user: User = Depends(require_user),
    repo: SourceFunctionalProfileRepository = Depends(get_source_functional_profile_repo),
    message_repo: MessageRepository = Depends(get_message_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    agent: PiAgentClient = Depends(get_pi_agent_client),
    settings: Settings = Depends(get_settings),
) -> SourceFunctionalProfileRead:
    profile = await _profile_or_404(profile_id, current_user.id, repo)
    if not settings.pi_agent_enabled or not settings.effective_pi_agent_api_key:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Pi Agent is not configured",
        )
    try:
        saved = await ReprofileSourceFunctionUseCase(
            agent=agent,
            message_repo=message_repo,
            profile_repo=repo,
            subscription_repo=subscription_repo,
        ).execute(profile=profile, owner_user_id=current_user.id)
    except SourceProfileQuotaExceeded as exc:
        raise HTTPException(status_code=status.HTTP_429_TOO_MANY_REQUESTS, detail=str(exc)) from exc
    except PiAgentError as exc:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="Pi Agent could not profile the source",
        ) from exc
    return to_source_profile_read(saved)


@router.patch(
    "/{profile_id}/functional-profile/override",
    response_model=SourceFunctionalProfileRead,
)
async def update_functional_profile_override(
    profile_id: UUID,
    payload: SourceFunctionOverrideRequest,
    current_user: User = Depends(require_user),
    repo: SourceFunctionalProfileRepository = Depends(get_source_functional_profile_repo),
) -> SourceFunctionalProfileRead:
    profile = await _profile_or_404(profile_id, current_user.id, repo)
    return to_source_profile_read(await repo.set_override(profile, payload.override))
