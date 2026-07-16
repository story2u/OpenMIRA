from __future__ import annotations

from app.application.dto import (
    JobMatchRead,
    JobOpportunityDetailRead,
    JobOpportunityRead,
    JobProfileParseRead,
    JobSearchProfileRead,
    JobSourceRead,
    SourceFunctionalProfileRead,
)
from app.domain.job_models import JobSearchProfilePreview
from app.infrastructure.db.models import (
    JobOpportunityDetail,
    JobOpportunityMatch,
    JobOpportunitySource,
    JobSearchProfile,
    Opportunity,
    SourceFunctionalProfile,
)


def to_job_profile_read(profile: JobSearchProfile) -> JobSearchProfileRead:
    return JobSearchProfileRead(
        id=profile.id,
        name=profile.name,
        isDefault=profile.is_default,
        enabled=profile.enabled,
        targetRoles=profile.target_roles,
        excludedRoles=profile.excluded_roles,
        targetIndustries=profile.target_industries,
        preferredSeniority=profile.preferred_seniority,
        candidateSkills=profile.candidate_skills,
        yearsExperience=profile.years_experience,
        educationLevel=profile.education_level,
        englishLevel=profile.english_level,
        otherLanguages=profile.other_languages,
        preferredCountries=profile.preferred_countries,
        preferredCities=profile.preferred_cities,
        preferredTimezones=profile.preferred_timezones,
        workModes=profile.work_modes,
        employmentTypes=profile.employment_types,
        minimumSalary=profile.minimum_salary,
        salaryCurrency=profile.salary_currency,
        salaryPeriod=profile.salary_period,
        visaSponsorshipRequired=profile.visa_sponsorship_required,
        relocationAcceptable=profile.relocation_acceptable,
        requiredKeywords=profile.required_keywords,
        preferredKeywords=profile.preferred_keywords,
        excludedKeywords=profile.excluded_keywords,
        requireSalaryDisclosed=profile.require_salary_disclosed,
        minimumMatchScore=profile.minimum_match_score,
        notificationEnabled=profile.notification_enabled,
        createdAt=profile.created_at,
        updatedAt=profile.updated_at,
    )


def to_job_profile_parse_read(preview: JobSearchProfilePreview) -> JobProfileParseRead:
    return JobProfileParseRead(
        name=preview.name,
        targetRoles=preview.target_roles,
        excludedRoles=preview.excluded_roles,
        targetIndustries=preview.target_industries,
        preferredSeniority=preview.preferred_seniority,
        candidateSkills=preview.candidate_skills,
        yearsExperience=preview.years_experience,
        educationLevel=preview.education_level,
        englishLevel=preview.english_level,
        otherLanguages=preview.other_languages,
        preferredCountries=preview.preferred_countries,
        preferredCities=preview.preferred_cities,
        preferredTimezones=preview.preferred_timezones,
        workModes=preview.work_modes,
        employmentTypes=preview.employment_types,
        minimumSalary=preview.minimum_salary,
        salaryCurrency=preview.salary_currency,
        salaryPeriod=preview.salary_period,
        visaSponsorshipRequired=preview.visa_sponsorship_required,
        relocationAcceptable=preview.relocation_acceptable,
        requiredKeywords=preview.required_keywords,
        preferredKeywords=preview.preferred_keywords,
        excludedKeywords=preview.excluded_keywords,
        requireSalaryDisclosed=preview.require_salary_disclosed,
        minimumMatchScore=preview.minimum_match_score,
        notificationEnabled=preview.notification_enabled,
        requiresConfirmation=True,
    )


def to_job_match_read(match: JobOpportunityMatch | None) -> JobMatchRead | None:
    if not match:
        return None
    return JobMatchRead(
        eligibility=match.eligibility,
        matchScore=match.match_score,
        matchedReasons=match.matched_reasons,
        mismatchReasons=match.mismatch_reasons,
        unknownConstraints=match.unknown_constraints,
        scoreBreakdown=match.score_breakdown,
    )


def to_job_read(
    opportunity: Opportunity,
    detail: JobOpportunityDetail,
    match: JobOpportunityMatch | None,
    *,
    source_count: int,
) -> JobOpportunityRead:
    return JobOpportunityRead(
        opportunityId=opportunity.id,
        jobTitle=detail.job_title,
        companyName=detail.company_name,
        sourceChannel=detail.source_channel,
        sourceChatName=detail.source_chat_name,
        postedAt=detail.posted_at,
        locationText=detail.location_text,
        countryCode=detail.country_code,
        city=detail.city,
        workMode=detail.work_mode,
        employmentType=detail.employment_type,
        seniority=detail.seniority,
        salaryRaw=detail.salary_raw,
        salaryMin=detail.salary_min,
        salaryMax=detail.salary_max,
        salaryCurrency=detail.salary_currency,
        salaryPeriod=detail.salary_period,
        requiredSkills=detail.required_skills,
        degreeLevel=detail.degree_level,
        englishLevel=detail.english_level,
        visaSponsorship=detail.visa_sponsorship,
        applicationDeadline=detail.application_deadline,
        sourceReliabilityScore=detail.source_reliability_score,
        extractionConfidence=detail.extraction_confidence,
        sourceCount=source_count,
        conflictingSourceData=detail.conflicting_source_data,
        complianceFlags=detail.compliance_flags,
        isExpired=detail.is_expired,
        match=to_job_match_read(match),
    )


def to_job_detail_read(
    opportunity: Opportunity,
    detail: JobOpportunityDetail,
    match: JobOpportunityMatch | None,
    sources: list[JobOpportunitySource],
) -> JobOpportunityDetailRead:
    base = to_job_read(opportunity, detail, match, source_count=len(sources))
    return JobOpportunityDetailRead(
        **base.model_dump(),
        sourceMessageUrl=detail.source_message_url,
        sourceAuthorName=detail.source_author_name,
        department=detail.department,
        companyIndustry=detail.company_industry,
        companyStage=detail.company_stage,
        timezone=detail.timezone,
        salaryNegotiable=detail.salary_negotiable,
        equityMentioned=detail.equity_mentioned,
        requirementsSummary=detail.requirements_summary,
        preferredSkills=detail.preferred_skills,
        minimumYearsExperience=detail.minimum_years_experience,
        maximumYearsExperience=detail.maximum_years_experience,
        degreeRequired=detail.degree_required,
        degreeField=detail.degree_field,
        otherLanguageRequirements=detail.other_language_requirements,
        workAuthorizationText=detail.work_authorization_text,
        relocationSupport=detail.relocation_support,
        ageRequirementText=detail.age_requirement_text,
        ageRequirementPresent=detail.age_requirement_present,
        applicationUrl=detail.application_url,
        contactMethods=detail.contact_methods,
        missingFields=detail.missing_fields,
        fieldEvidence=detail.field_evidence,
        rawExcerpt=detail.raw_excerpt,
        expiredReason=detail.expired_reason,
        sources=[
            JobSourceRead(
                id=source.id,
                channel=source.source_channel,
                chatName=source.source_chat_name,
                authorName=source.source_author_name,
                postedAt=source.posted_at,
                sourceMessageUrl=source.source_message_url,
                reliabilityScore=source.source_reliability_score,
            )
            for source in sources
        ],
    )


def to_source_profile_read(profile: SourceFunctionalProfile) -> SourceFunctionalProfileRead:
    return SourceFunctionalProfileRead(
        id=profile.id,
        channel=profile.channel,
        externalSourceId=profile.external_source_id,
        sourceDisplayName=profile.source_display_name,
        sourceDescription=profile.source_description,
        primaryFunction=profile.primary_function,
        effectiveFunction=profile.manual_override or profile.primary_function,
        secondaryFunctions=profile.secondary_functions,
        industryTags=profile.industry_tags,
        regionTags=profile.region_tags,
        languageTags=profile.language_tags,
        jobSignalPrior=profile.job_signal_prior,
        estimatedNoiseLevel=profile.estimated_noise_level,
        reliabilityScore=profile.reliability_score,
        confidence=profile.confidence,
        evidence=profile.evidence,
        manualOverride=profile.manual_override,
        sampledMessageCount=profile.sampled_message_count,
        profiledAt=profile.profiled_at,
        expiresAt=profile.expires_at,
    )
