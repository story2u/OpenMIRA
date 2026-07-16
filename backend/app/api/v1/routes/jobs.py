from __future__ import annotations

from datetime import datetime
from decimal import Decimal
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.api.deps import (
    get_job_opportunity_match_repo,
    get_job_opportunity_repo,
    get_job_search_profile_repo,
    require_user,
)
from app.application.dto import (
    JobFeedbackRead,
    JobFeedbackRequest,
    JobOpportunityDetailRead,
    JobsPageRead,
)
from app.application.job_mappers import (
    to_job_detail_read,
    to_job_profile_read,
    to_job_read,
)
from app.domain.enums import IMChannel, JobEmploymentType, JobSeniority, JobWorkMode
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import (
    JobOpportunityMatchRepository,
    JobOpportunityRepository,
    JobSearchProfileRepository,
)

router = APIRouter()
SORTS = {"match", "newest", "salary", "confidence", "source_reliability"}


async def _selected_profile(
    repo: JobSearchProfileRepository, owner_user_id: UUID, profile_id: UUID | None
):
    if profile_id:
        profile = await repo.get_for_owner(profile_id, owner_user_id)
        if not profile:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="profile not found")
        return profile
    profiles = await repo.list_for_owner(owner_user_id)
    return profiles[0] if profiles else None


@router.get("", response_model=JobsPageRead)
async def list_jobs(
    profile_id: UUID | None = None,
    query: str | None = Query(default=None, max_length=200),
    source: IMChannel | None = None,
    posted_from: datetime | None = None,
    posted_to: datetime | None = None,
    work_mode: JobWorkMode | None = None,
    employment_type: JobEmploymentType | None = None,
    seniority: JobSeniority | None = None,
    country: str | None = Query(default=None, min_length=2, max_length=2),
    city: str | None = Query(default=None, max_length=255),
    salary_min: Decimal | None = Query(default=None, ge=0),
    salary_currency: str | None = Query(default=None, min_length=3, max_length=3),
    salary_disclosed: bool | None = None,
    degree_level: str | None = Query(default=None, max_length=100),
    english_level: str | None = Query(default=None, max_length=100),
    visa_sponsorship: bool | None = None,
    minimum_match_score: int | None = Query(default=None, ge=0, le=100),
    age_requirement_present: bool | None = None,
    exclude_expired: bool = True,
    sort: str = "match",
    limit: int = Query(default=20, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: JobOpportunityRepository = Depends(get_job_opportunity_repo),
    profile_repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
) -> JobsPageRead:
    if sort not in SORTS:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="invalid sort")
    if posted_from and posted_to and posted_from > posted_to:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail="posted_from must not be after posted_to",
        )
    profile = await _selected_profile(profile_repo, current_user.id, profile_id)
    rows, total = await repo.list_for_owner(
        owner_user_id=current_user.id,
        profile_id=profile.id if profile else None,
        query=query,
        source=source,
        posted_from=posted_from,
        posted_to=posted_to,
        work_mode=work_mode.value if work_mode else None,
        employment_type=employment_type.value if employment_type else None,
        seniority=seniority.value if seniority else None,
        country=country,
        city=city,
        salary_min=float(salary_min) if salary_min is not None else None,
        salary_currency=salary_currency,
        salary_disclosed=salary_disclosed,
        degree_level=degree_level,
        english_level=english_level,
        visa_sponsorship=visa_sponsorship,
        minimum_match_score=minimum_match_score,
        age_requirement_present=age_requirement_present,
        exclude_expired=exclude_expired,
        sort=sort,
        limit=limit,
        offset=offset,
    )
    counts = await repo.source_counts([row[0].id for row in rows], current_user.id)
    return JobsPageRead(
        items=[
            to_job_read(opportunity, detail, match, source_count=counts.get(opportunity.id, 1))
            for opportunity, detail, match in rows
        ],
        total=total,
        limit=limit,
        offset=offset,
        filterSummary={
            "query": query,
            "source": source,
            "workMode": work_mode,
            "minimumMatchScore": minimum_match_score,
            "excludeExpired": exclude_expired,
        },
        profile=to_job_profile_read(profile) if profile else None,
    )


@router.get("/{opportunity_id}", response_model=JobOpportunityDetailRead)
async def get_job(
    opportunity_id: UUID,
    profile_id: UUID | None = None,
    current_user: User = Depends(require_user),
    repo: JobOpportunityRepository = Depends(get_job_opportunity_repo),
    profile_repo: JobSearchProfileRepository = Depends(get_job_search_profile_repo),
    match_repo: JobOpportunityMatchRepository = Depends(get_job_opportunity_match_repo),
) -> JobOpportunityDetailRead:
    pair = await repo.get_detail_for_owner(opportunity_id, current_user.id)
    if not pair:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="job not found")
    opportunity, detail = pair
    if detail.duplicate_group_id:
        canonical = await repo.get_detail_for_owner(detail.duplicate_group_id, current_user.id)
        if canonical:
            opportunity, detail = canonical
    profile = await _selected_profile(profile_repo, current_user.id, profile_id)
    match = (
        await match_repo.get(opportunity.id, profile.id, current_user.id) if profile else None
    )
    sources = await repo.list_sources(opportunity.id, current_user.id)
    return to_job_detail_read(opportunity, detail, match, sources)


@router.post("/{opportunity_id}/feedback", response_model=JobFeedbackRead)
async def save_job_feedback(
    opportunity_id: UUID,
    payload: JobFeedbackRequest,
    current_user: User = Depends(require_user),
    repo: JobOpportunityRepository = Depends(get_job_opportunity_repo),
) -> JobFeedbackRead:
    if not await repo.get_detail_for_owner(opportunity_id, current_user.id):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="job not found")
    feedback = await repo.save_feedback(
        opportunity_id=opportunity_id,
        owner_user_id=current_user.id,
        feedback_type=payload.feedbackType,
        note=payload.note,
    )
    return JobFeedbackRead(
        id=feedback.id,
        feedbackType=feedback.feedback_type,
        note=feedback.note,
        updatedAt=feedback.updated_at,
    )
