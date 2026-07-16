from datetime import UTC, datetime

from app.domain.enums import JobMessageClassification, SourcePrimaryFunction
from app.domain.services.job_discovery import (
    build_job_feature_embedding,
    cosine_similarity,
    prefilter_job_message,
    profile_source,
)


def test_profiles_recruitment_source_from_name_and_samples() -> None:
    result = profile_source(
        name="Remote AI Jobs",
        description="全球 AI 招聘和内推",
        username="remote_ai_jobs",
        samples=["Hiring Python engineer, remote", "招聘算法工程师，简历请投递"],
        now=datetime(2026, 7, 16, tzinfo=UTC),
    )
    assert result.primary_function == SourcePrimaryFunction.RECRUITMENT
    assert result.job_signal_prior >= 0.8
    assert result.confidence > 0.5


def test_name_only_profile_has_lower_confidence() -> None:
    result = profile_source(
        name="Python 招聘群",
        description=None,
        username=None,
        samples=[],
        now=datetime(2026, 7, 16, tzinfo=UTC),
    )
    assert result.primary_function == SourcePrimaryFunction.RECRUITMENT
    assert result.confidence <= 0.6


def test_technical_group_still_allows_strong_job_post() -> None:
    result = prefilter_job_message(
        "Hiring Senior Python Engineer, remote full-time, salary SGD 8k monthly, apply with CV",
        primary_function=SourcePrimaryFunction.TECHNICAL_DISCUSSION,
        job_signal_prior=0.2,
    )
    assert result.should_analyze


def test_short_chinese_hiring_phrasing_reaches_agent_in_unknown_source() -> None:
    result = prefilter_job_message("招 Python 后端，20-30K，深圳，可远程，本科以上")
    assert result.should_analyze


def test_candidate_self_promotion_is_filtered() -> None:
    result = prefilter_job_message("本人五年 Java 经验，正在求职，希望大家帮忙内推")
    assert not result.should_analyze
    assert result.classification == JobMessageClassification.CANDIDATE_SELF_PROMOTION


def test_training_and_scam_are_filtered() -> None:
    training = prefilter_job_message("付费 Python 训练营，三个月包就业")
    scam = prefilter_job_message("居家兼职日赚 500，不限经验，需要先交押金")
    assert training.classification == JobMessageClassification.TRAINING_AD
    assert scam.classification == JobMessageClassification.SCAM


def test_structured_feature_embedding_matches_small_job_rewrites() -> None:
    first = build_job_feature_embedding(
        title="Senior Python Backend Engineer",
        company="Example Labs",
        location="Singapore Remote",
        skills=["Python", "FastAPI", "PostgreSQL"],
        summary="Build APIs for the data platform",
    )
    repost = build_job_feature_embedding(
        title="Sr Python Backend Developer",
        company="Example Labs",
        location="Remote Singapore",
        skills=["Python", "FastAPI", "PostgreSQL"],
        summary="Develop data platform APIs",
    )
    different = build_job_feature_embedding(
        title="Product Designer",
        company="Other Example",
        location="London",
        skills=["Figma", "Research"],
        summary="Design a consumer application",
    )

    assert cosine_similarity(first, repost) >= 0.78
    assert cosine_similarity(first, different) < 0.5
