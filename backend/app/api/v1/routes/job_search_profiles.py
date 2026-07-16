from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status

from app.api.deps import (
    get_job_opportunity_match_repo,
    get_job_opportunity_repo,
    get_job_search_profile_repo,
    get_pi_agent_client,
    get_settings,
    get_subscription_repo,
    require_user,
)
from app.application.dto import (
    JobProfileParseRead,
    JobProfileParseRequest,
    JobSearchProfileRead,
    JobSearchProfileUpdate,
    JobSearchProfileWrite,
)
from app.application.job_mappers import to_job_profile_parse_read, to_job_profile_read
from app.application.use_cases.match_job_opportunity import MatchJobOpportunityUseCase
from app.application.use_cases.parse_job_search_profile import (
    JobProfileQuotaExceeded,
    ParseJobSearchProfileUseCase,
)
from app.core.config import Settings
from app.infrastructure.agent.pi_client import PiAgentClient, PiAgentError
from app.infrastructure.db.models import JobSearchProfile, User
from app.infrastructure.db.repositories import (
    JobOpportunityMatchRepository,
    JobOpportunityRepository,
    JobSearchProfileRepository,
    SubscriptionRepository,
)

router = APIRouter()


def _values(items: list | None) -> list | None:
    if items is None:
        return None
    return [item.value if hasattr(item, "value") else item for item in items]


def _apply_profile_payload(
    profile: JobSearchProfile,
    payload: JobSearchProfileWrite | JobSearchProfileUpdate,
) -> None:
    fields = payload.model_fields_set if isinstance(payload, JobSearchProfileUpdate) else None
    mappings = {
        "name": "name",
        "isDefault": "is_default",
        "enabled": "enabled",
        "targetRoles": "target_roles",
        "excludedRoles": "excluded_roles",
        "targetIndustries": "target_industries",
        "preferredSeniority": "preferred_seniority",
        "candidateSkills": "candidate_skills",
        "yearsExperience": "years_experience",
        "educationLevel": "education_level",
        "englishLevel": "english_level",
        "otherLanguages": "other_languages",
        "preferredCountries": "preferred_countries",
        "preferredCities": "preferred_cities",
        "preferredTimezones": "preferred_timezones",
        "workModes": "work_modes",
        "employmentTypes": "employment_types",
        "minimumSalary": "minimum_salary",
        "salaryCurrency": "salary_currency",
        "salaryPeriod": "salary_period",
        "visaSponsorshipRequired": "visa_sponsorship_required",
        "relocationAcceptable": "relocation_acceptable",
        "requiredKeywords": "required_keywords",
        "preferredKeywords": "preferred_keywords",
        "excludedKeywords": "excluded_keywords",
        "requireSalaryDisclosed": "require_salary_disclosed",
        "minimumMatchScore": "minimum_match_score",
        "notificationEnabled": "notification_enabled",
    }
    for input_name, model_name in mappings.items():
        if fields is not None and input_name not in fields:
            continue
        value = getattr(payload, input_name)
        if isinstance(value, list):
            value = _values(value)
        if input_name == "salaryCurrency" and value:
            value = value.upper()
        setattr(profile, model_name, value)


def _matcher(
    job_repo: JobOpportunityRepository,
    profile_repo: JobSearchProfileRepository,
    match_repo: JobOpportunityMatchRepository,
) -> MatchJobOpportunityUseCase:
    return MatchJobOpportunityUseCase(
        job_repo=job_repo,
        profile_repo=profile_repo,
        match_repo=match_repo,
    )


@router.get("", response_model=list[JobSearchProfileRead])
async def list_profiles(
    current_user: User = Depends(require_user),
    repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
) -> list[JobSearchProfileRead]:
    return [to_job_profile_read(item) for item in await repo.list_for_owner(current_user.id)]


@router.post("", response_model=JobSearchProfileRead, status_code=status.HTTP_201_CREATED)
async def create_profile(
    payload: JobSearchProfileWrite,
    current_user: User = Depends(require_user),
    repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
    job_repo: JobOpportunityRepository = Depends(get_job_opportunity_repo),
    match_repo: JobOpportunityMatchRepository = Depends(get_job_opportunity_match_repo),
) -> JobSearchProfileRead:
    existing = await repo.list_for_owner(current_user.id)
    profile = JobSearchProfile(user_id=current_user.id, name=payload.name)
    _apply_profile_payload(profile, payload)
    if not existing:
        profile.is_default = True
    saved = await repo.save(profile)
    await _matcher(job_repo, repo, match_repo).execute_for_profile(saved)
    return to_job_profile_read(saved)


@router.post("/parse", response_model=JobProfileParseRead)
async def parse_profile(
    payload: JobProfileParseRequest,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    agent: PiAgentClient = Depends(get_pi_agent_client),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> JobProfileParseRead:
    if not settings.pi_agent_enabled or not settings.pi_agent_api_key:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Pi Agent is not configured",
        )
    try:
        preview = await ParseJobSearchProfileUseCase(
            agent=agent, subscription_repo=subscription_repo
        ).execute(current_user.id, payload.text)
    except JobProfileQuotaExceeded as exc:
        raise HTTPException(status_code=status.HTTP_429_TOO_MANY_REQUESTS, detail=str(exc)) from exc
    except PiAgentError as exc:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="Pi Agent could not parse the job profile",
        ) from exc
    return to_job_profile_parse_read(preview)


@router.get("/{profile_id}", response_model=JobSearchProfileRead)
async def get_profile(
    profile_id: UUID,
    current_user: User = Depends(require_user),
    repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
) -> JobSearchProfileRead:
    profile = await repo.get_for_owner(profile_id, current_user.id)
    if not profile:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="profile not found")
    return to_job_profile_read(profile)


@router.patch("/{profile_id}", response_model=JobSearchProfileRead)
async def update_profile(
    profile_id: UUID,
    payload: JobSearchProfileUpdate,
    current_user: User = Depends(require_user),
    repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
    job_repo: JobOpportunityRepository = Depends(get_job_opportunity_repo),
    match_repo: JobOpportunityMatchRepository = Depends(get_job_opportunity_match_repo),
) -> JobSearchProfileRead:
    profile = await repo.get_for_owner(profile_id, current_user.id)
    if not profile:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="profile not found")
    _apply_profile_payload(profile, payload)
    saved = await repo.save(profile)
    await _matcher(job_repo, repo, match_repo).execute_for_profile(saved)
    return to_job_profile_read(saved)


@router.delete("/{profile_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_profile(
    profile_id: UUID,
    current_user: User = Depends(require_user),
    repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
) -> None:
    profile = await repo.get_for_owner(profile_id, current_user.id)
    if not profile:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="profile not found")
    was_default = profile.is_default
    await repo.delete(profile)
    if was_default:
        remaining = await repo.list_for_owner(current_user.id)
        if remaining:
            remaining[0].is_default = True
            await repo.save(remaining[0])
