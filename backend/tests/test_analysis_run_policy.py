from uuid import uuid4

import pytest

from app.application.use_cases.analysis_run import DeviceAgentRoutingService
from app.core.config import Settings
from app.domain.services.analysis_run import (
    AnalysisRolloutEvidence,
    count_shadow_differences,
    device_in_analysis_rollout,
    evaluate_analysis_rollout_readiness,
)


def test_shadow_difference_count_is_leaf_based_and_bounded() -> None:
    baseline = {
        "title": "Original",
        "contacts": {"email": None, "phone": "123"},
        "actions": [{"type": "reply", "required": True}],
    }
    candidate = {
        "title": "Changed",
        "contacts": {"email": "buyer@example.test"},
        "actions": [{"type": "reply", "required": False}, {"type": "archive"}],
    }

    assert count_shadow_differences(baseline, baseline) == 0
    assert count_shadow_differences(baseline, candidate) == 5
    assert count_shadow_differences(baseline, candidate, limit=3) == 3


def test_device_rollout_cohort_is_stable_bounded_and_allowlistable() -> None:
    owner_id = uuid4()
    device_id = uuid4()

    assert not device_in_analysis_rollout(
        owner_user_id=owner_id,
        device_id=device_id,
        percentage=0,
    )
    assert device_in_analysis_rollout(
        owner_user_id=owner_id,
        device_id=device_id,
        percentage=100,
    )
    first = device_in_analysis_rollout(
        owner_user_id=owner_id,
        device_id=device_id,
        percentage=10,
    )
    assert first is device_in_analysis_rollout(
        owner_user_id=owner_id,
        device_id=device_id,
        percentage=10,
    )
    assert device_in_analysis_rollout(
        owner_user_id=owner_id,
        device_id=device_id,
        percentage=0,
        allowlist=frozenset({str(device_id)}),
    )


def test_rollout_readiness_requires_volume_success_match_and_latency() -> None:
    healthy = AnalysisRolloutEvidence(
        terminal_samples=100,
        completed_samples=98,
        matched_samples=96,
        p95_seconds=40.0,
    )
    unhealthy = AnalysisRolloutEvidence(
        terminal_samples=10,
        completed_samples=8,
        matched_samples=6,
        p95_seconds=140.0,
    )

    accepted = evaluate_analysis_rollout_readiness(
        healthy,
        minimum_samples=50,
        minimum_success_rate=0.95,
        minimum_match_rate=0.95,
        maximum_p95_seconds=120.0,
    )
    rejected = evaluate_analysis_rollout_readiness(
        unhealthy,
        minimum_samples=50,
        minimum_success_rate=0.95,
        minimum_match_rate=0.95,
        maximum_p95_seconds=120.0,
    )

    assert accepted.ready is True
    assert accepted.reasons == ()
    assert rejected.ready is False
    assert set(rejected.reasons) == {
        "insufficient_shadow_samples",
        "shadow_success_rate_below_threshold",
        "shadow_match_rate_below_threshold",
        "shadow_p95_above_threshold",
    }


def test_rollout_allowlist_is_canonical_deduplicated_and_bounded() -> None:
    device_id = uuid4()
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        device_agent_rollout_allowlist=(
            f"{device_id.hex.upper()}, {device_id}, {uuid4()}"
        ),
    )

    assert len(settings.device_agent_rollout_device_ids) == 2
    assert str(device_id) in settings.device_agent_rollout_device_ids

    with pytest.raises(ValueError, match="must contain UUIDs"):
        Settings(
            database_url="postgresql+asyncpg://user:password@localhost/test",
            admin_api_token="test-admin-token",
            device_agent_rollout_allowlist="not-a-device-id",
        )


def test_primary_gate_reasons_explain_fail_closed_configuration() -> None:
    routing = DeviceAgentRoutingService(
        run_repo=None,  # type: ignore[arg-type]
        settings=Settings(
            database_url="postgresql+asyncpg://user:password@localhost/test",
            admin_api_token="test-admin-token",
        ),
    )
    readiness = evaluate_analysis_rollout_readiness(
        AnalysisRolloutEvidence(0, 0, 0, None),
        minimum_samples=50,
        minimum_success_rate=0.95,
        minimum_match_rate=0.95,
        maximum_p95_seconds=120.0,
    )

    assert set(routing.primary_gate_reasons(readiness)) == {
        "primary_rollout_disabled",
        "sync_rollout_disabled",
        "no_rollout_cohort",
        "server_fallback_disabled",
        "server_pi_unavailable",
        "device_gateway_unavailable",
        "insufficient_shadow_samples",
        "shadow_success_rate_below_threshold",
        "shadow_match_rate_below_threshold",
        "shadow_p95_above_threshold",
    }
