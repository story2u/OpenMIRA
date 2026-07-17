import os
from datetime import timedelta
from uuid import uuid4

from fastapi import FastAPI
from fastapi.security import HTTPAuthorizationCredentials
from fastapi.testclient import TestClient

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.deps import (
    DevicePrincipal,
    get_analysis_run_service,
    get_device_agent_routing_service,
    require_admin,
    require_analysis_run_principal,
    require_device_principal,
)
from app.api.v1.routes import analysis_runs as analysis_runs_route
from app.application.use_cases.analysis_run import (
    AnalysisRunClaim,
    AnalysisRunTokenPrincipal,
)
from app.core.config import Settings
from app.core.security import (
    ANALYSIS_RUN_TOKEN_PURPOSE,
    create_access_token,
    create_analysis_run_token,
    create_signed_token,
)
from app.domain.enums import (
    AnalysisRunExecutor,
    AnalysisRunStatus,
    DevicePlatform,
    IMChannel,
    LinkSafetyStatus,
)
from app.domain.ports import LinkInspection
from app.domain.services.analysis_run import (
    AnalysisRunTokenRejectedError,
    AnalysisRunUnavailableError,
    AnalysisRolloutEvidence,
    AnalysisRolloutReadiness,
)
from app.infrastructure.db.models import AnalysisRun, Device, Message, User, utc_now


class FakeAnalysisRunService:
    def __init__(self, claim: AnalysisRunClaim) -> None:
        self.claim_result = claim
        self.shadow_claim_result: AnalysisRunClaim | None = claim
        self.error: Exception | None = None
        self.received = None

    async def claim(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.claim_result

    async def claim_shadow(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.shadow_claim_result

    async def claim_next(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.shadow_claim_result

    async def heartbeat(self, principal, *, expected_lock_version: int):
        self.received = (principal, expected_lock_version)
        if self.error:
            raise self.error
        return self.claim_result.run

    async def complete(self, principal, *, expected_lock_version: int, result):
        self.received = (principal, expected_lock_version, result)
        if self.error:
            raise self.error
        return self.claim_result.run

    async def fail(self, principal, *, expected_lock_version: int, failure_code: str):
        self.received = (principal, expected_lock_version, failure_code)
        if self.error:
            raise self.error
        return self.claim_result.run

    async def expire(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.claim_result.run

    async def inspect_links(self, principal, *, inspector):
        self.received = (principal, inspector)
        if self.error:
            raise self.error
        self.claim_result.run.link_evidence_fetched_at = utc_now()
        self.claim_result.run.link_evidence = []
        return self.claim_result.run, [
            LinkInspection(
                url="https://example.test/info",
                final_url="https://example.test/info",
                status=LinkSafetyStatus.SAFE,
                text="Public evidence",
            )
        ]


def valid_result() -> dict:
    return {
        "is_opportunity": False,
        "confidence": 0.2,
        "title": "No opportunity",
        "summary": "No commercial intent was found.",
        "priority": "normal",
        "trust_score": 70,
        "attention_required": False,
        "link_status": "unverified",
        "link_summary": None,
        "risk_flags": [],
        "contacts": {
            "email": None,
            "phone": None,
            "telegram_handle": None,
            "wecom_id": None,
            "extraction_source": None,
        },
        "actions": [],
    }


def build_client():
    owner = User(id=uuid4(), email="analysis-route@example.test")
    device = Device(
        id=uuid4(),
        owner_user_id=owner.id,
        installation_id_hash="a" * 64,
        platform=DevicePlatform.IOS,
        app_variant="production",
        app_version="1.0.0",
        app_build="1",
        capabilities={},
    )
    message = Message(
        id=uuid4(),
        owner_user_id=owner.id,
        channel=IMChannel.TELEGRAM,
        external_message_id="analysis-route-message",
        conversation_id="analysis-route-conversation",
        direction="incoming",
        text="Bounded input",
        raw_message_links=["https://example.test/info"],
        raw_payload={"secret": "must-not-be-returned"},
    )
    run = AnalysisRun(
        id=uuid4(),
        owner_user_id=owner.id,
        message_id=message.id,
        device_id=device.id,
        usage_ledger_id=uuid4(),
        status=AnalysisRunStatus.CLAIMED,
        executor=AnalysisRunExecutor.DEVICE,
        runtime_version="pi-0.80.6",
        schema_version=1,
        model_alias="radar-analysis-v1",
        policy_version="agent-policy-v1",
        source_message_version=1,
        lock_version=1,
        token_nonce_hash="b" * 64,
        claimed_at=utc_now(),
        lease_expires_at=utc_now() + timedelta(minutes=2),
    )
    claim = AnalysisRunClaim(run=run, run_token="run-token-secret", message=message)
    service = FakeAnalysisRunService(claim)
    token_principal = AnalysisRunTokenPrincipal(
        run_id=run.id,
        owner_user_id=owner.id,
        device_id=device.id,
        nonce="c" * 64,
    )
    app = FastAPI()
    app.include_router(analysis_runs_route.router, prefix="/agent/runs")
    app.dependency_overrides[require_device_principal] = lambda: DevicePrincipal(owner, device)
    app.dependency_overrides[require_analysis_run_principal] = lambda: token_principal
    app.dependency_overrides[get_analysis_run_service] = lambda: service
    return TestClient(app), service, owner, device, message, run


def test_claim_returns_bounded_input_and_no_store_token_response() -> None:
    client, service, owner, device, message, run = build_client()

    response = client.post("/agent/runs/claim", json={"messageId": str(message.id)})

    assert response.status_code == 201
    assert response.headers["cache-control"] == "no-store"
    assert response.json()["runToken"] == "run-token-secret"
    assert response.json()["input"]["text"] == "Bounded input"
    assert "must-not-be-returned" not in response.text
    assert service.received == {
        "owner_user_id": owner.id,
        "device": device,
        "message_id": message.id,
    }
    assert response.json()["id"] == str(run.id)


def test_shadow_claim_has_no_request_body_and_wraps_optional_claim() -> None:
    client, service, owner, device, _, run = build_client()

    claimed = client.post("/agent/runs/claim-shadow")
    service.shadow_claim_result = None
    empty = client.post("/agent/runs/claim-shadow")

    assert claimed.status_code == 200
    assert claimed.headers["cache-control"] == "no-store"
    assert claimed.json()["claim"]["id"] == str(run.id)
    assert claimed.json()["claim"]["mode"] == "primary"
    assert claimed.json()["claim"]["shadowMatch"] is None
    assert claimed.json()["claim"]["shadowDifferenceCount"] is None
    assert empty.status_code == 200
    assert empty.json() == {"claim": None}
    assert service.received == {
        "owner_user_id": owner.id,
        "device": device,
    }


def test_next_claim_has_no_request_body_and_wraps_optional_primary() -> None:
    client, service, owner, device, _, run = build_client()

    claimed = client.post("/agent/runs/claim-next")
    service.shadow_claim_result = None
    empty = client.post("/agent/runs/claim-next")

    assert claimed.status_code == 200
    assert claimed.headers["cache-control"] == "no-store"
    assert claimed.json()["claim"]["id"] == str(run.id)
    assert empty.status_code == 200
    assert empty.json() == {"claim": None}
    assert service.received == {"owner_user_id": owner.id, "device": device}


def test_rollout_readiness_is_admin_only_content_free_aggregate() -> None:
    client, _, _, _, _, _ = build_client()
    allowlisted_device = uuid4()
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        rn_device_agent_rollout_percentage=25,
        rn_device_agent_rollout_enabled=True,
        rn_sync_rollout_enabled=True,
        device_agent_fallback_enabled=True,
        pi_agent_api_key="server-key",
        device_agent_gateway_enabled=True,
        device_agent_gateway_api_key="gateway-key",
        device_agent_rollout_allowlist=str(allowlisted_device),
        device_agent_rollout_min_shadow_samples=4,
        device_agent_rollout_min_shadow_success_rate=0.75,
        device_agent_rollout_min_shadow_match_rate=0.8,
        device_agent_rollout_max_p95_seconds=30,
    )

    class FakeRoutingService:
        async def rollout_readiness(self):
            return AnalysisRolloutReadiness(
                evidence=AnalysisRolloutEvidence(
                    terminal_samples=4,
                    completed_samples=3,
                    matched_samples=2,
                    p95_seconds=12.5,
                ),
                ready=False,
                reasons=("shadow_match_rate_below_threshold",),
            )

        async def primary_globally_ready(self, *, readiness):
            assert readiness.ready is False
            return False

        def primary_gate_reasons(self, readiness):
            return readiness.reasons

    routing = FakeRoutingService()
    routing.settings = settings
    client.app.dependency_overrides[require_admin] = lambda: object()
    client.app.dependency_overrides[get_device_agent_routing_service] = lambda: routing

    response = client.get("/agent/runs/rollout-readiness")

    assert response.status_code == 200
    assert response.json() == {
        "ready": False,
        "enforced": True,
        "primaryGateOpen": False,
        "terminalSamples": 4,
        "completedSamples": 3,
        "matchedSamples": 2,
        "successRate": 0.75,
        "matchRate": 2 / 3,
        "p95Seconds": 12.5,
        "minimumSamples": 4,
        "minimumSuccessRate": 0.75,
        "minimumMatchRate": 0.8,
        "maximumP95Seconds": 30.0,
        "rolloutPercentage": 25,
        "allowlistedDeviceCount": 1,
        "reasons": ["shadow_match_rate_below_threshold"],
    }
    assert str(allowlisted_device) not in response.text


def test_claim_and_run_token_failures_return_stable_non_sensitive_errors() -> None:
    client, service, _, _, message, run = build_client()
    service.error = AnalysisRunUnavailableError("provider detail")
    unavailable = client.post("/agent/runs/claim", json={"messageId": str(message.id)})

    service.error = AnalysisRunTokenRejectedError("nonce detail")
    rejected = client.post(
        f"/agent/runs/{run.id}/heartbeat",
        json={"expectedLockVersion": 1},
    )

    assert unavailable.status_code == 503
    assert unavailable.json() == {"detail": "device analysis is unavailable"}
    assert "provider detail" not in unavailable.text
    assert rejected.status_code == 401
    assert rejected.json() == {"detail": "invalid token"}
    assert "nonce detail" not in rejected.text


def test_complete_and_fail_requests_are_strict_and_bounded() -> None:
    client, service, _, _, _, run = build_client()
    invalid_result = valid_result() | {"unexpected": "field"}

    rejected_result = client.post(
        f"/agent/runs/{run.id}/complete",
        json={"expectedLockVersion": 1, "result": invalid_result},
    )
    rejected_code = client.post(
        f"/agent/runs/{run.id}/fail",
        json={"expectedLockVersion": 1, "failureCode": "contains secret spaces"},
    )
    accepted = client.post(
        f"/agent/runs/{run.id}/complete",
        json={"expectedLockVersion": 1, "result": valid_result()},
    )

    assert rejected_result.status_code == 422
    assert rejected_code.status_code == 422
    assert accepted.status_code == 200
    assert service.received[1] == 1


def test_link_inspection_is_run_bound_and_rejects_client_supplied_urls() -> None:
    client, service, _, _, _, run = build_client()

    accepted = client.post(f"/agent/runs/{run.id}/links/inspect")
    rejected = client.post(
        f"/agent/runs/{run.id}/links/inspect",
        json={"url": "http://127.0.0.1/admin"},
    )

    assert accepted.status_code == 200
    assert accepted.headers["cache-control"] == "no-store"
    assert accepted.json()["runId"] == str(run.id)
    assert accepted.json()["evidence"][0]["text"] == "Public evidence"
    assert rejected.status_code == 422
    assert rejected.json() == {"detail": "link inspection does not accept URLs"}
    assert service.received[0].run_id == run.id


async def test_analysis_run_token_has_a_distinct_purpose_and_bound_ids() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        jwt_secret_key="analysis-route-secret",
    )
    owner_id = uuid4()
    device_id = uuid4()
    run_id = uuid4()
    token = create_analysis_run_token(
        run_id=run_id,
        owner_user_id=owner_id,
        device_id=device_id,
        settings=settings,
    )

    principal = await require_analysis_run_principal(
        credentials=HTTPAuthorizationCredentials(scheme="Bearer", credentials=token),
        settings=settings,
    )

    assert principal.run_id == run_id
    assert principal.owner_user_id == owner_id
    assert principal.device_id == device_id
    access_token = create_access_token(subject=owner_id, device_id=device_id, settings=settings)
    try:
        await require_analysis_run_principal(
            credentials=HTTPAuthorizationCredentials(
                scheme="Bearer",
                credentials=access_token,
            ),
            settings=settings,
        )
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 401
    else:
        raise AssertionError("an access token must not authorize an analysis run")

    malformed_nonce = create_signed_token(
        {
            "sub": str(owner_id),
            "did": str(device_id),
            "rid": str(run_id),
            "purpose": ANALYSIS_RUN_TOKEN_PURPOSE,
            "nonce": "not-a-valid-nonce",
        },
        settings=settings,
        expires_delta=timedelta(minutes=5),
    )
    try:
        await require_analysis_run_principal(
            credentials=HTTPAuthorizationCredentials(
                scheme="Bearer",
                credentials=malformed_nonce,
            ),
            settings=settings,
        )
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 401
    else:
        raise AssertionError("a malformed nonce must be rejected")
