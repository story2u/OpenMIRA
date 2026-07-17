from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from hashlib import sha256
from typing import Any
from uuid import UUID

from app.domain.enums import AgentAnalysisStatus, AnalysisRunStatus, MessageDirection


class AnalysisRunError(Exception):
    pass


class AnalysisRunUnavailableError(AnalysisRunError):
    pass


class AnalysisRunNotFoundError(AnalysisRunError):
    pass


class AnalysisRunConflictError(AnalysisRunError):
    pass


class AnalysisRunLeaseExpiredError(AnalysisRunError):
    pass


class AnalysisRunQuotaExceededError(AnalysisRunError):
    def __init__(self, *, limit: int, allocated: int) -> None:
        super().__init__("analysis quota exceeded")
        self.limit = limit
        self.allocated = allocated


class AnalysisRunTokenRejectedError(AnalysisRunError):
    pass


class AnalysisRunVersionConflictError(AnalysisRunError):
    pass


ACTIVE_ANALYSIS_RUN_STATUSES = {
    AnalysisRunStatus.CLAIMED,
    AnalysisRunStatus.RUNNING,
}


@dataclass(frozen=True, slots=True)
class AnalysisRolloutEvidence:
    terminal_samples: int
    completed_samples: int
    matched_samples: int
    p95_seconds: float | None

    @property
    def success_rate(self) -> float:
        return self.completed_samples / self.terminal_samples if self.terminal_samples else 0.0

    @property
    def match_rate(self) -> float:
        return self.matched_samples / self.completed_samples if self.completed_samples else 0.0


@dataclass(frozen=True, slots=True)
class AnalysisRolloutReadiness:
    evidence: AnalysisRolloutEvidence
    ready: bool
    reasons: tuple[str, ...]


def device_in_analysis_rollout(
    *,
    owner_user_id: UUID,
    device_id: UUID,
    percentage: int,
    allowlist: frozenset[str] = frozenset(),
) -> bool:
    """Select a stable owner/device cohort without exposing sequential identifiers."""
    normalized_device_id = str(device_id).lower()
    if normalized_device_id in allowlist:
        return True
    if percentage <= 0:
        return False
    if percentage >= 100:
        return True
    digest = sha256(f"device-agent-v1:{owner_user_id}:{device_id}".encode()).digest()
    bucket = int.from_bytes(digest[:8], "big") % 100
    return bucket < percentage


def evaluate_analysis_rollout_readiness(
    evidence: AnalysisRolloutEvidence,
    *,
    minimum_samples: int,
    minimum_success_rate: float,
    minimum_match_rate: float,
    maximum_p95_seconds: float,
) -> AnalysisRolloutReadiness:
    reasons: list[str] = []
    if evidence.terminal_samples < minimum_samples:
        reasons.append("insufficient_shadow_samples")
    if evidence.success_rate < minimum_success_rate:
        reasons.append("shadow_success_rate_below_threshold")
    if evidence.match_rate < minimum_match_rate:
        reasons.append("shadow_match_rate_below_threshold")
    if evidence.p95_seconds is None or evidence.p95_seconds > maximum_p95_seconds:
        reasons.append("shadow_p95_above_threshold")
    return AnalysisRolloutReadiness(
        evidence=evidence,
        ready=not reasons,
        reasons=tuple(reasons),
    )


def supports_device_analysis(
    capabilities: Mapping[str, Any],
    *,
    runtime_version: str,
    schema_version: int,
) -> bool:
    reported_schema = capabilities.get("agent.schema")
    return (
        capabilities.get("client.reactNative") is True
        and capabilities.get("agent.submitAnalysis") is True
        and capabilities.get("agent.streaming") is True
        and capabilities.get("agent.runtime") == runtime_version
        and isinstance(reported_schema, int)
        and not isinstance(reported_schema, bool)
        and reported_schema == schema_version
    )


def message_can_be_claimed(
    *,
    direction: MessageDirection,
    status: AgentAnalysisStatus,
) -> bool:
    return direction == MessageDirection.INCOMING and status in {
        AgentAnalysisStatus.NOT_REQUESTED,
        AgentAnalysisStatus.QUEUED,
        AgentAnalysisStatus.FAILED,
    }


def count_shadow_differences(left: Any, right: Any, *, limit: int = 10_000) -> int:
    """Count bounded leaf differences without persisting a second comparison payload."""
    if limit <= 0:
        return 0
    if isinstance(left, Mapping) and isinstance(right, Mapping):
        differences = 0
        for key in set(left) | set(right):
            if key not in left or key not in right:
                differences += 1
            else:
                differences += count_shadow_differences(
                    left[key],
                    right[key],
                    limit=limit - differences,
                )
            if differences >= limit:
                return limit
        return differences
    if isinstance(left, list) and isinstance(right, list):
        differences = abs(len(left) - len(right))
        for left_item, right_item in zip(left, right, strict=False):
            differences += count_shadow_differences(
                left_item,
                right_item,
                limit=limit - differences,
            )
            if differences >= limit:
                return limit
        return differences
    return int(left != right)
