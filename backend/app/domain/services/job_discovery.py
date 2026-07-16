from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from datetime import datetime, timedelta

from app.domain.enums import JobMessageClassification, SourcePrimaryFunction

PROFILE_TTL = timedelta(days=7)

RECRUITMENT_TERMS = {
    "招聘",
    "急招",
    "诚聘",
    "内推",
    "职位",
    "岗位",
    "简历",
    "hiring",
    "job",
    "jobs",
    "opening",
    "vacancy",
    "recruit",
    "career",
}
TECH_TERMS = {"python", "java", "golang", "ai", "开发", "技术", "工程师", "架构", "product"}
TRAINING_TERMS = {"培训", "课程", "训练营", "包就业", "付费学习", "course", "bootcamp"}
SCAM_TERMS = {"先交", "押金", "日赚", "刷单", "垫付", "稳赚", "无需经验", "缴费入职"}
CANDIDATE_TERMS = {"本人", "求职", "求内推", "找工作", "available for work", "looking for work"}
JOB_STRUCTURE_TERMS = {
    "薪资",
    "月薪",
    "年薪",
    "k",
    "remote",
    "远程",
    "全职",
    "兼职",
    "合同",
    "实习",
    "要求",
    "经验",
    "工作地点",
    "base",
    "salary",
    "apply",
    "cv",
    "resume",
}


def normalize_text(value: str) -> str:
    return re.sub(r"\s+", " ", value.strip().casefold())


def source_fingerprint(name: str, description: str | None, username: str | None) -> str:
    payload = "\n".join(normalize_text(item or "") for item in (name, description, username))
    return hashlib.sha256(payload.encode()).hexdigest()


@dataclass(frozen=True, slots=True)
class SourceProfileDecision:
    primary_function: SourcePrimaryFunction
    secondary_functions: list[str]
    industry_tags: list[str]
    region_tags: list[str]
    language_tags: list[str]
    job_signal_prior: float
    estimated_noise_level: float
    reliability_score: float
    confidence: float
    evidence: list[str]
    sampled_message_count: int
    expires_at: datetime


def profile_source(
    *,
    name: str,
    description: str | None,
    username: str | None,
    samples: list[str],
    now: datetime,
) -> SourceProfileDecision:
    metadata = normalize_text(" ".join(filter(None, [name, description or "", username or ""])))
    normalized_samples = [normalize_text(item)[:1000] for item in samples if normalize_text(item)]
    sample_text = " ".join(normalized_samples)
    recruitment_hits = sum(term in metadata for term in RECRUITMENT_TERMS)
    sample_job_hits = sum(
        any(term in sample for term in RECRUITMENT_TERMS) for sample in normalized_samples
    )
    sample_training_hits = sum(
        any(term in sample for term in TRAINING_TERMS) for sample in normalized_samples
    )
    sample_ratio = sample_job_hits / max(len(normalized_samples), 1)
    evidence: list[str] = []
    secondary: list[str] = []

    if recruitment_hits:
        evidence.append("source metadata contains recruitment terms")
    if sample_job_hits:
        evidence.append(
            f"{sample_job_hits} of {len(normalized_samples)} samples contain job signals"
        )

    if recruitment_hits >= 1 and (sample_ratio >= 0.2 or not normalized_samples):
        primary = SourcePrimaryFunction.RECRUITMENT
        prior = min(0.96, 0.72 + sample_ratio * 0.25)
        noise = max(0.1, 0.45 - sample_ratio * 0.35)
    elif any(term in metadata for term in TECH_TERMS):
        primary = SourcePrimaryFunction.TECHNICAL_DISCUSSION
        prior = min(0.7, 0.2 + sample_ratio)
        noise = 0.55
        if sample_job_hits:
            secondary.append(SourcePrimaryFunction.CAREER_NETWORKING.value)
    elif any(term in metadata for term in TRAINING_TERMS) or sample_training_hits > sample_job_hits:
        primary = SourcePrimaryFunction.EDUCATION_TRAINING
        prior = 0.15
        noise = 0.75
    elif any(term in metadata for term in {"广告", "推广", "交易", "market"}):
        primary = SourcePrimaryFunction.ADVERTISING
        prior = 0.1
        noise = 0.9
    elif normalized_samples:
        primary = SourcePrimaryFunction.GENERAL_CHAT
        prior = min(0.55, 0.1 + sample_ratio)
        noise = 0.7
    else:
        primary = SourcePrimaryFunction.UNKNOWN
        prior = 0.35
        noise = 0.6

    confidence = min(0.95, 0.45 + min(recruitment_hits, 2) * 0.15 + len(samples) * 0.025)
    if not normalized_samples:
        confidence = min(confidence, 0.6)
        evidence.append("profile is based on source metadata only")

    languages = []
    joined = f"{metadata} {sample_text}"
    if re.search(r"[\u4e00-\u9fff]", joined):
        languages.append("zh")
    if re.search(r"[a-z]", joined):
        languages.append("en")
    industries = sorted({term for term in TECH_TERMS if term in joined})[:10]
    return SourceProfileDecision(
        primary_function=primary,
        secondary_functions=secondary,
        industry_tags=industries,
        region_tags=[],
        language_tags=languages,
        job_signal_prior=round(prior, 3),
        estimated_noise_level=round(noise, 3),
        reliability_score=round(max(0.2, 0.75 - noise * 0.35), 3),
        confidence=round(confidence, 3),
        evidence=evidence[:8],
        sampled_message_count=len(normalized_samples),
        expires_at=now + PROFILE_TTL,
    )


@dataclass(frozen=True, slots=True)
class JobPrefilterDecision:
    should_analyze: bool
    score: float
    classification: JobMessageClassification
    reason: str


def prefilter_job_message(
    text: str,
    *,
    primary_function: SourcePrimaryFunction = SourcePrimaryFunction.UNKNOWN,
    job_signal_prior: float = 0.35,
) -> JobPrefilterDecision:
    normalized = normalize_text(text)
    if not normalized or len(normalized) < 8:
        return JobPrefilterDecision(
            False, 0.0, JobMessageClassification.UNRELATED_CHAT, "too short"
        )
    if any(term in normalized for term in SCAM_TERMS):
        return JobPrefilterDecision(False, 0.9, JobMessageClassification.SCAM, "scam pattern")
    if any(term in normalized for term in TRAINING_TERMS):
        return JobPrefilterDecision(False, 0.8, JobMessageClassification.TRAINING_AD, "training ad")
    if any(term in normalized for term in CANDIDATE_TERMS) and not any(
        term in normalized for term in {"招聘", "hiring", "急招", "诚聘"}
    ):
        return JobPrefilterDecision(
            False, 0.75, JobMessageClassification.CANDIDATE_SELF_PROMOTION, "candidate intent"
        )

    recruitment_hits = sum(term in normalized for term in RECRUITMENT_TERMS)
    structure_hits = sum(term in normalized for term in JOB_STRUCTURE_TERMS)
    score = min(1.0, job_signal_prior * 0.35 + recruitment_hits * 0.22 + structure_hits * 0.08)
    threshold = {
        SourcePrimaryFunction.RECRUITMENT: 0.3,
        SourcePrimaryFunction.JOB_REFERRAL: 0.35,
        SourcePrimaryFunction.CAREER_NETWORKING: 0.4,
        SourcePrimaryFunction.TECHNICAL_DISCUSSION: 0.48,
        SourcePrimaryFunction.ADVERTISING: 0.72,
        SourcePrimaryFunction.EDUCATION_TRAINING: 0.75,
    }.get(primary_function, 0.55)
    should_analyze = recruitment_hits > 0 and score >= threshold
    return JobPrefilterDecision(
        should_analyze,
        round(score, 3),
        JobMessageClassification.UNKNOWN
        if should_analyze
        else JobMessageClassification.UNRELATED_CHAT,
        (
            f"job signal score {score:.2f} "
            f"{'meets' if should_analyze else 'below'} threshold {threshold:.2f}"
        ),
    )
