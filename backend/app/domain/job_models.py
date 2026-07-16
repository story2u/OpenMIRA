from __future__ import annotations

from decimal import Decimal
from typing import Annotated

from pydantic import BaseModel, Field

from app.domain.enums import (
    JobEmploymentType,
    JobMessageClassification,
    JobSeniority,
    JobWorkMode,
    SalaryPeriod,
)


class ExtractedSalary(BaseModel):
    raw: str | None = Field(default=None, max_length=500)
    minimum: Decimal | None = Field(default=None, ge=0)
    maximum: Decimal | None = Field(default=None, ge=0)
    currency: str | None = Field(default=None, min_length=3, max_length=3)
    period: SalaryPeriod = SalaryPeriod.UNKNOWN
    negotiable: bool | None = None


class ExtractedContactMethod(BaseModel):
    type: str = Field(min_length=1, max_length=50)
    value: str = Field(min_length=1, max_length=500)


class ExtractedJob(BaseModel):
    job_title: str = Field(min_length=1, max_length=500)
    normalized_job_title: str | None = Field(default=None, max_length=500)
    company_name: str | None = Field(default=None, max_length=500)
    department: str | None = Field(default=None, max_length=500)
    company_industry: str | None = Field(default=None, max_length=255)
    company_stage: str | None = Field(default=None, max_length=255)
    location_text: str | None = Field(default=None, max_length=500)
    country_code: str | None = Field(default=None, min_length=2, max_length=2)
    city: str | None = Field(default=None, max_length=255)
    timezone: str | None = Field(default=None, max_length=100)
    work_mode: JobWorkMode = JobWorkMode.UNKNOWN
    employment_type: JobEmploymentType = JobEmploymentType.UNKNOWN
    seniority: JobSeniority = JobSeniority.UNKNOWN
    salary: ExtractedSalary = Field(default_factory=ExtractedSalary)
    equity_mentioned: bool | None = None
    requirements_summary: str | None = Field(default=None, max_length=4000)
    required_skills: list[Annotated[str, Field(max_length=100)]] = Field(
        default_factory=list, max_length=30
    )
    preferred_skills: list[Annotated[str, Field(max_length=100)]] = Field(
        default_factory=list, max_length=30
    )
    minimum_years_experience: float | None = Field(default=None, ge=0, le=80)
    maximum_years_experience: float | None = Field(default=None, ge=0, le=80)
    degree_required: bool | None = None
    degree_level: str | None = Field(default=None, max_length=100)
    degree_field: str | None = Field(default=None, max_length=255)
    english_level: str | None = Field(default=None, max_length=100)
    other_language_requirements: list[str] = Field(default_factory=list, max_length=20)
    visa_sponsorship: bool | None = None
    work_authorization_text: str | None = Field(default=None, max_length=1000)
    relocation_support: bool | None = None
    age_requirement_text: str | None = Field(default=None, max_length=500)
    application_url: str | None = Field(default=None, max_length=2000)
    application_deadline: str | None = Field(default=None, max_length=50)
    contact_methods: list[ExtractedContactMethod] = Field(default_factory=list, max_length=20)


class JobAgentAnalysis(BaseModel):
    classification: JobMessageClassification
    classification_confidence: float = Field(ge=0, le=1)
    noise_reasons: list[str] = Field(default_factory=list, max_length=20)
    job: ExtractedJob | None = None
    field_evidence: dict[str, str] = Field(default_factory=dict)
    missing_fields: list[str] = Field(default_factory=list, max_length=50)
    compliance_flags: list[str] = Field(default_factory=list, max_length=20)
    extraction_confidence: float = Field(default=0, ge=0, le=1)

    def is_formal_job(self) -> bool:
        return (
            self.classification
            in {
                JobMessageClassification.JOB_POST,
                JobMessageClassification.JOB_REPOST,
            }
            and self.job is not None
        )


class JobSearchProfilePreview(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    target_roles: list[str] = Field(default_factory=list, max_length=30)
    excluded_roles: list[str] = Field(default_factory=list, max_length=30)
    target_industries: list[str] = Field(default_factory=list, max_length=30)
    preferred_seniority: list[JobSeniority] = Field(default_factory=list)
    candidate_skills: list[str] = Field(default_factory=list, max_length=100)
    years_experience: float | None = Field(default=None, ge=0, le=80)
    education_level: str | None = Field(default=None, max_length=100)
    english_level: str | None = Field(default=None, max_length=100)
    other_languages: list[str] = Field(default_factory=list, max_length=30)
    preferred_countries: list[str] = Field(default_factory=list, max_length=50)
    preferred_cities: list[str] = Field(default_factory=list, max_length=50)
    preferred_timezones: list[str] = Field(default_factory=list, max_length=50)
    work_modes: list[JobWorkMode] = Field(default_factory=list)
    employment_types: list[JobEmploymentType] = Field(default_factory=list)
    minimum_salary: Decimal | None = Field(default=None, ge=0)
    salary_currency: str | None = Field(default=None, min_length=3, max_length=3)
    salary_period: SalaryPeriod | None = None
    visa_sponsorship_required: bool | None = None
    relocation_acceptable: bool | None = None
    required_keywords: list[str] = Field(default_factory=list, max_length=50)
    preferred_keywords: list[str] = Field(default_factory=list, max_length=50)
    excluded_keywords: list[str] = Field(default_factory=list, max_length=50)
    require_salary_disclosed: bool = False
    minimum_match_score: int = Field(default=0, ge=0, le=100)
    notification_enabled: bool = False
    requires_confirmation: bool = True
