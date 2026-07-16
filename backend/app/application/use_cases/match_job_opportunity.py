from __future__ import annotations

from uuid import UUID

from app.domain.services.job_matching import JobFacts, SearchPreferences, calculate_job_match
from app.infrastructure.db.models import JobOpportunityDetail, JobSearchProfile
from app.infrastructure.db.repositories import (
    JobOpportunityMatchRepository,
    JobOpportunityRepository,
    JobSearchProfileRepository,
)


class MatchJobOpportunityUseCase:
    def __init__(
        self,
        *,
        job_repo: JobOpportunityRepository,
        profile_repo: JobSearchProfileRepository,
        match_repo: JobOpportunityMatchRepository,
    ) -> None:
        self.job_repo = job_repo
        self.profile_repo = profile_repo
        self.match_repo = match_repo

    async def execute(self, opportunity_id: UUID, owner_user_id: UUID) -> int:
        pair = await self.job_repo.get_detail_for_owner(opportunity_id, owner_user_id)
        if not pair:
            return 0
        _, detail = pair
        profiles = await self.profile_repo.list_enabled(owner_user_id)
        for profile in profiles:
            decision = calculate_job_match(self._job_facts(detail), self._preferences(profile))
            await self.match_repo.upsert(
                opportunity_id=opportunity_id,
                profile_id=profile.id,
                owner_user_id=owner_user_id,
                eligibility=decision.eligibility,
                match_score=decision.match_score,
                matched_reasons=decision.matched_reasons,
                mismatch_reasons=decision.mismatch_reasons,
                unknown_constraints=decision.unknown_constraints,
                score_breakdown=decision.score_breakdown,
            )
        return len(profiles)

    @staticmethod
    def _job_facts(detail: JobOpportunityDetail) -> JobFacts:
        return JobFacts(
            title=detail.job_title,
            company_industry=detail.company_industry,
            required_skills=tuple(detail.required_skills),
            preferred_skills=tuple(detail.preferred_skills),
            seniority=detail.seniority.value,
            country_code=detail.country_code,
            city=detail.city,
            timezone=detail.timezone,
            work_mode=detail.work_mode.value,
            employment_type=detail.employment_type.value,
            salary_min=detail.salary_min,
            salary_max=detail.salary_max,
            salary_currency=detail.salary_currency,
            salary_period=detail.salary_period.value,
            english_level=detail.english_level,
            visa_sponsorship=detail.visa_sponsorship,
            searchable_text=" ".join(
                filter(
                    None,
                    [
                        detail.requirements_summary,
                        detail.company_name,
                        *detail.required_skills,
                        *detail.preferred_skills,
                    ],
                )
            ),
        )

    @staticmethod
    def _preferences(profile: JobSearchProfile) -> SearchPreferences:
        return SearchPreferences(
            target_roles=tuple(profile.target_roles),
            excluded_roles=tuple(profile.excluded_roles),
            target_industries=tuple(profile.target_industries),
            preferred_seniority=tuple(profile.preferred_seniority),
            candidate_skills=tuple(profile.candidate_skills),
            preferred_countries=tuple(profile.preferred_countries),
            preferred_cities=tuple(profile.preferred_cities),
            preferred_timezones=tuple(profile.preferred_timezones),
            work_modes=tuple(profile.work_modes),
            employment_types=tuple(profile.employment_types),
            minimum_salary=profile.minimum_salary,
            salary_currency=profile.salary_currency,
            salary_period=profile.salary_period.value if profile.salary_period else None,
            visa_sponsorship_required=profile.visa_sponsorship_required,
            required_keywords=tuple(profile.required_keywords),
            preferred_keywords=tuple(profile.preferred_keywords),
            excluded_keywords=tuple(profile.excluded_keywords),
            require_salary_disclosed=profile.require_salary_disclosed,
        )
