from __future__ import annotations

import hashlib
import math
import re
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timedelta

from app.domain.enums import JobMessageClassification, SourcePrimaryFunction

PROFILE_TTL = timedelta(days=7)
URL_PATTERN = re.compile(r"https?://[^\s<>()\[\]{}\"']+", re.IGNORECASE)
EMAIL_PATTERN = re.compile(r"\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b", re.IGNORECASE)
PHONE_PATTERN = re.compile(r"(?<!\w)(?:\+?\d[\d\s().-]{6,}\d)(?!\w)")
HANDLE_PATTERN = re.compile(r"(?<!\w)@[A-Za-z0-9_]{4,}")

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
    "招人",
    "诚招",
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


def contains_term(value: str, term: str) -> bool:
    if re.fullmatch(r"[a-z0-9]+", term):
        return re.search(rf"(?<![a-z0-9]){re.escape(term)}(?![a-z0-9])", value) is not None
    return term in value


def recruitment_signal_count(value: str) -> int:
    count = sum(contains_term(value, term) for term in RECRUITMENT_TERMS)
    if re.search(r"(?:^|[\s，。；：])招\s*[a-z\u4e00-\u9fff]", value):
        count += 1
    return count


def redact_source_sample(text: str, *, max_chars: int = 600) -> str:
    """Remove direct contact/link identifiers before bounded source profiling."""

    value = URL_PATTERN.sub("[url]", text)
    value = EMAIL_PATTERN.sub("[email]", value)
    value = PHONE_PATTERN.sub("[phone]", value)
    value = HANDLE_PATTERN.sub("[handle]", value)
    return re.sub(r"\s+", " ", value).strip()[:max_chars]


def source_fingerprint(name: str, description: str | None, username: str | None) -> str:
    payload = "\n".join(normalize_text(item or "") for item in (name, description, username))
    return hashlib.sha256(payload.encode()).hexdigest()


TOKEN_ALIASES = {
    "sr": "senior",
    "jr": "junior",
    "developer": "engineer",
    "dev": "engineer",
    "backend": "backend",
    "back-end": "backend",
    "remote": "remote",
    "remotely": "remote",
    "后端开发": "后端工程师",
    "后端研发": "后端工程师",
}


def _semantic_tokens(value: str) -> list[str]:
    normalized = normalize_text(value)
    tokens: list[str] = []
    for token in re.findall(r"[a-z0-9+#.-]+|[\u4e00-\u9fff]+", normalized):
        canonical = TOKEN_ALIASES.get(token, token)
        tokens.append(canonical)
        if re.fullmatch(r"[\u4e00-\u9fff]{3,}", canonical):
            tokens.extend(canonical[index : index + 2] for index in range(len(canonical) - 1))
    return tokens


def build_job_feature_embedding(
    *,
    title: str,
    company: str | None,
    location: str | None,
    skills: list[str],
    summary: str | None,
) -> dict[str, float]:
    """Build a sparse, deterministic embedding from evidence-backed job fields."""

    vector: defaultdict[str, float] = defaultdict(float)
    for value, weight in (
        (title, 3.0),
        (company or "", 3.0),
        (location or "", 1.0),
        (" ".join(skills), 2.0),
        ((summary or "")[:1000], 0.5),
    ):
        for token in _semantic_tokens(value):
            vector[token] += weight
    magnitude = math.sqrt(sum(weight * weight for weight in vector.values()))
    if magnitude == 0:
        return {}
    return {token: weight / magnitude for token, weight in vector.items()}


def cosine_similarity(left: dict[str, float], right: dict[str, float]) -> float:
    if not left or not right:
        return 0.0
    smaller, larger = (left, right) if len(left) <= len(right) else (right, left)
    return sum(value * larger.get(token, 0.0) for token, value in smaller.items())


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
    recruitment_hits = recruitment_signal_count(metadata)
    sample_job_hits = sum(recruitment_signal_count(sample) > 0 for sample in normalized_samples)
    sample_training_hits = sum(
        any(contains_term(sample, term) for term in TRAINING_TERMS) for sample in normalized_samples
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
    elif any(contains_term(metadata, term) for term in TECH_TERMS):
        primary = SourcePrimaryFunction.TECHNICAL_DISCUSSION
        prior = min(0.7, 0.2 + sample_ratio)
        noise = 0.55
        if sample_job_hits:
            secondary.append(SourcePrimaryFunction.CAREER_NETWORKING.value)
    elif (
        any(contains_term(metadata, term) for term in TRAINING_TERMS)
        or sample_training_hits > sample_job_hits
    ):
        primary = SourcePrimaryFunction.EDUCATION_TRAINING
        prior = 0.15
        noise = 0.75
    elif any(contains_term(metadata, term) for term in {"广告", "推广", "交易", "market"}):
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
    industries = sorted({term for term in TECH_TERMS if contains_term(joined, term)})[:10]
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
    if any(contains_term(normalized, term) for term in SCAM_TERMS):
        return JobPrefilterDecision(False, 0.9, JobMessageClassification.SCAM, "scam pattern")
    if any(contains_term(normalized, term) for term in TRAINING_TERMS):
        return JobPrefilterDecision(False, 0.8, JobMessageClassification.TRAINING_AD, "training ad")
    if any(contains_term(normalized, term) for term in CANDIDATE_TERMS) and not any(
        contains_term(normalized, term) for term in {"招聘", "hiring", "急招", "诚聘"}
    ):
        return JobPrefilterDecision(
            False, 0.75, JobMessageClassification.CANDIDATE_SELF_PROMOTION, "candidate intent"
        )

    recruitment_hits = recruitment_signal_count(normalized)
    structure_hits = sum(
        contains_term(normalized, term) for term in JOB_STRUCTURE_TERMS if term != "k"
    )
    structure_hits += bool(re.search(r"\d+(?:\.\d+)?\s*k\b", normalized))
    score = min(1.0, job_signal_prior * 0.35 + recruitment_hits * 0.22 + structure_hits * 0.08)
    threshold = {
        SourcePrimaryFunction.RECRUITMENT: 0.3,
        SourcePrimaryFunction.JOB_REFERRAL: 0.35,
        SourcePrimaryFunction.CAREER_NETWORKING: 0.4,
        SourcePrimaryFunction.TECHNICAL_DISCUSSION: 0.48,
        SourcePrimaryFunction.ADVERTISING: 0.72,
        SourcePrimaryFunction.EDUCATION_TRAINING: 0.75,
    }.get(primary_function, 0.48)
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
