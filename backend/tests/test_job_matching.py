from decimal import Decimal

from app.domain.enums import JobEligibility
from app.domain.services.job_matching import JobFacts, SearchPreferences, calculate_job_match


def test_match_is_deterministic_and_explainable() -> None:
    result = calculate_job_match(
        JobFacts(
            title="Senior Python Backend Engineer",
            required_skills=("Python", "FastAPI", "PostgreSQL"),
            seniority="senior",
            city="Berlin",
            work_mode="remote",
            employment_type="full_time",
            salary_max=Decimal("100000"),
            salary_currency="USD",
            salary_period="annual",
            english_level="professional",
            visa_sponsorship=True,
        ),
        SearchPreferences(
            target_roles=("Python Backend Engineer",),
            candidate_skills=("Python", "FastAPI"),
            preferred_seniority=("senior",),
            work_modes=("remote",),
            employment_types=("full_time",),
            minimum_salary=Decimal("80000"),
            salary_currency="USD",
            salary_period="annual",
            visa_sponsorship_required=True,
        ),
    )
    assert result.eligibility == JobEligibility.ELIGIBLE
    assert result.match_score > 70
    assert result.score_breakdown["visa"] == 5


def test_missing_job_fields_are_unknown_not_mismatch() -> None:
    result = calculate_job_match(
        JobFacts(title="Python Developer"),
        SearchPreferences(
            target_roles=("Python Developer",),
            minimum_salary=Decimal("80000"),
            salary_currency="USD",
            salary_period="annual",
            visa_sponsorship_required=True,
        ),
    )
    assert "招聘信息未说明签证支持" in result.unknown_constraints
    assert all("年龄" not in reason for reason in result.mismatch_reasons)


def test_hard_constraints_make_job_not_eligible() -> None:
    result = calculate_job_match(
        JobFacts(title="Backend Engineer", work_mode="on_site", visa_sponsorship=False),
        SearchPreferences(work_modes=("remote",), visa_sponsorship_required=True),
    )
    assert result.eligibility == JobEligibility.NOT_ELIGIBLE


def test_protected_attributes_are_not_part_of_profile_contract() -> None:
    fields = SearchPreferences.__dataclass_fields__
    assert not {"age", "gender", "race", "religion", "marital_status", "disability"} & fields.keys()
