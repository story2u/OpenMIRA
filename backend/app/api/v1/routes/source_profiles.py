from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status

from app.api.deps import (
    get_message_repo,
    get_source_functional_profile_repo,
    require_user,
)
from app.application.dto import SourceFunctionalProfileRead, SourceFunctionOverrideRequest
from app.application.job_mappers import to_source_profile_read
from app.domain.services.job_discovery import profile_source, source_fingerprint
from app.infrastructure.db.models import User, utc_now
from app.infrastructure.db.repositories import MessageRepository, SourceFunctionalProfileRepository

router = APIRouter()


async def _profile_or_404(
    profile_id: UUID,
    owner_user_id: UUID,
    repo: SourceFunctionalProfileRepository,
):
    profile = await repo.get_by_id_for_owner(profile_id, owner_user_id)
    if not profile:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="source profile not found")
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
) -> SourceFunctionalProfileRead:
    profile = await _profile_or_404(profile_id, current_user.id, repo)
    samples = await message_repo.list_recent_source_samples(
        owner_user_id=current_user.id,
        channel=profile.channel,
        conversation_id=profile.external_source_id,
        limit=20,
    )
    decision = profile_source(
        name=profile.source_display_name,
        description=profile.source_description,
        username=profile.source_username,
        samples=samples,
        now=utc_now(),
    )
    saved = await repo.save_generated(
        owner_user_id=current_user.id,
        channel=profile.channel,
        external_source_id=profile.external_source_id,
        source_display_name=profile.source_display_name,
        source_description=profile.source_description,
        source_username=profile.source_username,
        source_fingerprint=source_fingerprint(
            profile.source_display_name,
            profile.source_description,
            profile.source_username,
        ),
        primary_function=decision.primary_function,
        secondary_functions=decision.secondary_functions,
        industry_tags=decision.industry_tags,
        region_tags=decision.region_tags,
        language_tags=decision.language_tags,
        job_signal_prior=decision.job_signal_prior,
        estimated_noise_level=decision.estimated_noise_level,
        reliability_score=decision.reliability_score,
        confidence=decision.confidence,
        evidence=decision.evidence,
        sampled_message_count=decision.sampled_message_count,
        expires_at=decision.expires_at,
    )
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
