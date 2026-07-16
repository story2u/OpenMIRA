from __future__ import annotations

import re
from dataclasses import dataclass, field
from decimal import Decimal

from app.domain.enums import JobEligibility


def terms(values: list[str] | tuple[str, ...]) -> set[str]:
    result: set[str] = set()
    for value in values:
        normalized = re.sub(r"[^\w+#.-]+", " ", value.casefold())
        result.update(token for token in normalized.split() if len(token) > 1)
    return result


@dataclass(frozen=True, slots=True)
class JobFacts:
    title: str
    company_industry: str | None = None
    required_skills: tuple[str, ...] = ()
    preferred_skills: tuple[str, ...] = ()
    seniority: str = "unknown"
    country_code: str | None = None
    city: str | None = None
    timezone: str | None = None
    work_mode: str = "unknown"
    employment_type: str = "unknown"
    salary_min: Decimal | None = None
    salary_max: Decimal | None = None
    salary_currency: str | None = None
    salary_period: str = "unknown"
    english_level: str | None = None
    visa_sponsorship: bool | None = None
    searchable_text: str = ""


@dataclass(frozen=True, slots=True)
class SearchPreferences:
    target_roles: tuple[str, ...] = ()
    excluded_roles: tuple[str, ...] = ()
    target_industries: tuple[str, ...] = ()
    preferred_seniority: tuple[str, ...] = ()
    candidate_skills: tuple[str, ...] = ()
    preferred_countries: tuple[str, ...] = ()
    preferred_cities: tuple[str, ...] = ()
    preferred_timezones: tuple[str, ...] = ()
    work_modes: tuple[str, ...] = ()
    employment_types: tuple[str, ...] = ()
    minimum_salary: Decimal | None = None
    salary_currency: str | None = None
    salary_period: str | None = None
    visa_sponsorship_required: bool | None = None
    required_keywords: tuple[str, ...] = ()
    preferred_keywords: tuple[str, ...] = ()
    excluded_keywords: tuple[str, ...] = ()
    require_salary_disclosed: bool = False


@dataclass(frozen=True, slots=True)
class JobMatchDecision:
    eligibility: JobEligibility
    match_score: int
    matched_reasons: list[str] = field(default_factory=list)
    mismatch_reasons: list[str] = field(default_factory=list)
    unknown_constraints: list[str] = field(default_factory=list)
    score_breakdown: dict[str, int] = field(default_factory=dict)


def calculate_job_match(job: JobFacts, profile: SearchPreferences) -> JobMatchDecision:
    matched: list[str] = []
    mismatch: list[str] = []
    unknown: list[str] = []
    breakdown: dict[str, int] = {}
    hard_failure = False
    title_terms = terms([job.title])
    text = f"{job.title} {job.searchable_text}".casefold()

    excluded_role_terms = terms(profile.excluded_roles)
    if title_terms & excluded_role_terms:
        mismatch.append("职位命中明确排除的岗位")
        hard_failure = True
    excluded_keywords = [item.casefold() for item in profile.excluded_keywords]
    if any(item and item in text for item in excluded_keywords):
        mismatch.append("职位命中明确排除的关键词")
        hard_failure = True

    if profile.target_roles:
        overlap = title_terms & terms(profile.target_roles)
        breakdown["role"] = min(
            25, round(25 * len(overlap) / max(len(terms(profile.target_roles)), 1))
        )
        (matched if overlap else mismatch).append(
            "岗位名称与目标岗位有交集" if overlap else "岗位名称与目标岗位差异较大"
        )
    else:
        breakdown["role"] = 0
        unknown.append("未设置目标岗位")

    skills = terms(job.required_skills)
    candidate = terms(profile.candidate_skills)
    if skills:
        overlap = skills & candidate
        breakdown["skills"] = round(25 * len(overlap) / len(skills))
        if overlap:
            matched.append(f"匹配技能：{', '.join(sorted(overlap)[:5])}")
        missing = skills - candidate
        if missing:
            mismatch.append(f"档案未声明技能：{', '.join(sorted(missing)[:5])}")
    else:
        breakdown["skills"] = 0
        unknown.append("招聘信息未说明必需技能")

    if profile.preferred_seniority:
        if job.seniority == "unknown":
            breakdown["seniority"] = 0
            unknown.append("招聘信息未说明资历级别")
        elif job.seniority in profile.preferred_seniority:
            breakdown["seniority"] = 10
            matched.append("资历级别符合偏好")
        else:
            breakdown["seniority"] = 0
            mismatch.append("资历级别不在偏好范围")
    else:
        breakdown["seniority"] = 0

    location_known = bool(
        job.country_code or job.city or job.timezone or job.work_mode != "unknown"
    )
    if profile.work_modes and job.work_mode != "unknown":
        if job.work_mode in profile.work_modes:
            breakdown["location"] = 15
            matched.append("工作模式符合偏好")
        else:
            breakdown["location"] = 0
            mismatch.append("工作模式不符合偏好")
            if "remote" in profile.work_modes and job.work_mode == "on_site":
                hard_failure = True
    elif any((profile.preferred_countries, profile.preferred_cities, profile.preferred_timezones)):
        wanted = terms(
            [*profile.preferred_countries, *profile.preferred_cities, *profile.preferred_timezones]
        )
        actual = terms([job.country_code or "", job.city or "", job.timezone or ""])
        overlap = wanted & actual
        breakdown["location"] = 15 if overlap else 0
        if overlap:
            matched.append("地点或时区符合偏好")
        elif location_known:
            mismatch.append("地点或时区不符合偏好")
        else:
            unknown.append("招聘信息未说明地点或工作模式")
    else:
        breakdown["location"] = 0

    if profile.minimum_salary is not None:
        comparable = (
            job.salary_max is not None
            and job.salary_currency == profile.salary_currency
            and job.salary_period == profile.salary_period
        )
        if not comparable:
            breakdown["salary"] = 0
            unknown.append("薪资未披露或币种/周期不可直接比较")
        elif job.salary_max < profile.minimum_salary:
            breakdown["salary"] = 0
            mismatch.append("职位最高薪资低于最低要求")
            hard_failure = True
        else:
            breakdown["salary"] = 10
            matched.append("薪资达到最低要求")
    elif job.salary_min is not None or job.salary_max is not None:
        breakdown["salary"] = 10
        matched.append("招聘信息公开薪资")
    else:
        breakdown["salary"] = 0
        if profile.require_salary_disclosed:
            unknown.append("招聘信息未公开薪资")

    if profile.employment_types:
        if job.employment_type == "unknown":
            breakdown["employment"] = 0
            unknown.append("招聘信息未说明雇佣类型")
        elif job.employment_type in profile.employment_types:
            breakdown["employment"] = 5
            matched.append("雇佣类型符合偏好")
        else:
            breakdown["employment"] = 0
            mismatch.append("雇佣类型不符合偏好")
    else:
        breakdown["employment"] = 0

    if job.english_level:
        breakdown["language"] = 5
        matched.append("招聘信息明确语言要求")
    else:
        breakdown["language"] = 0
        unknown.append("招聘信息未说明英语要求")

    if profile.visa_sponsorship_required:
        if job.visa_sponsorship is True:
            breakdown["visa"] = 5
            matched.append("职位明确提供签证支持")
        elif job.visa_sponsorship is False:
            breakdown["visa"] = 0
            mismatch.append("职位明确不提供签证支持")
            hard_failure = True
        else:
            breakdown["visa"] = 0
            unknown.append("招聘信息未说明签证支持")
    else:
        breakdown["visa"] = 0

    keyword_missing = [item for item in profile.required_keywords if item.casefold() not in text]
    if keyword_missing:
        mismatch.append(f"缺少必需关键词：{', '.join(keyword_missing[:5])}")
    preferred_hits = [item for item in profile.preferred_keywords if item.casefold() in text]
    if preferred_hits:
        matched.append(f"命中偏好关键词：{', '.join(preferred_hits[:5])}")

    score = max(0, min(100, sum(breakdown.values())))
    eligibility = (
        JobEligibility.NOT_ELIGIBLE
        if hard_failure
        else JobEligibility.UNKNOWN
        if unknown and not matched
        else JobEligibility.ELIGIBLE
    )
    return JobMatchDecision(eligibility, score, matched, mismatch, unknown, breakdown)
