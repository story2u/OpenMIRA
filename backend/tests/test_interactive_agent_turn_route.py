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
    get_interactive_agent_turn_service,
    require_device_principal,
    require_interactive_agent_turn_principal,
)
from app.api.v1.routes import interactive_agent_turns as turns_route
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentTurnClaim,
    InteractiveAgentTurnTokenPrincipal,
)
from app.core.config import Settings
from app.core.security import (
    INTERACTIVE_AGENT_TURN_TOKEN_PURPOSE,
    create_access_token,
    create_interactive_agent_turn_token,
    create_signed_token,
)
from app.domain.enums import DevicePlatform, InteractiveAgentTurnStatus
from app.domain.services.interactive_agent import (
    InteractiveAgentTurnTokenRejectedError,
    InteractiveAgentTurnUnavailableError,
)
from app.infrastructure.db.models import Device, InteractiveAgentTurn, User, utc_now


class FakeInteractiveAgentTurnService:
    def __init__(self, claim: InteractiveAgentTurnClaim) -> None:
        self.claim_result = claim
        self.error: Exception | None = None
        self.received = None

    async def claim(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.claim_result

    async def heartbeat(self, principal, *, expected_lock_version: int):
        self.received = (principal, expected_lock_version)
        if self.error:
            raise self.error
        return self.claim_result.turn

    async def complete(self, principal, *, expected_lock_version: int):
        self.received = (principal, expected_lock_version)
        if self.error:
            raise self.error
        return self.claim_result.turn

    async def fail(self, principal, *, expected_lock_version: int, failure_code: str):
        self.received = (principal, expected_lock_version, failure_code)
        if self.error:
            raise self.error
        return self.claim_result.turn

    async def expire(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        return self.claim_result.turn


def build_client():
    owner = User(id=uuid4(), email="interactive-route@example.test")
    device = Device(
        id=uuid4(),
        owner_user_id=owner.id,
        installation_id_hash="a" * 64,
        platform=DevicePlatform.IOS,
        app_variant="development",
        app_version="1.0.0",
        app_build="1",
        capabilities={},
    )
    now = utc_now()
    turn = InteractiveAgentTurn(
        id=uuid4(),
        owner_user_id=owner.id,
        device_id=device.id,
        local_session_id=uuid4(),
        idempotency_key="route-turn-1",
        usage_ledger_id=uuid4(),
        status=InteractiveAgentTurnStatus.CLAIMED,
        runtime_version="pi-0.80.6",
        schema_version=1,
        model_alias="radar-interactive-v1",
        policy_version="interactive-read-only-v1",
        lock_version=1,
        request_count=0,
        token_nonce_hash="b" * 64,
        claimed_at=now,
        lease_expires_at=now + timedelta(minutes=5),
    )
    claim = InteractiveAgentTurnClaim(
        turn=turn,
        turn_token="interactive-turn-token-secret",
        created=True,
    )
    service = FakeInteractiveAgentTurnService(claim)
    token_principal = InteractiveAgentTurnTokenPrincipal(
        turn_id=turn.id,
        owner_user_id=owner.id,
        device_id=device.id,
        nonce="c" * 64,
    )
    app = FastAPI()
    app.include_router(turns_route.router, prefix="/agent/interactive/turns")
    app.dependency_overrides[require_device_principal] = lambda: DevicePrincipal(
        owner,
        device,
    )
    app.dependency_overrides[require_interactive_agent_turn_principal] = (
        lambda: token_principal
    )
    app.dependency_overrides[get_interactive_agent_turn_service] = lambda: service
    return TestClient(app), service, owner, device, turn


def test_claim_returns_only_content_free_lease_metadata_and_no_store_token() -> None:
    client, service, owner, device, turn = build_client()

    response = client.post(
        "/agent/interactive/turns",
        json={
            "localSessionId": str(turn.local_session_id),
            "idempotencyKey": "route-turn-1",
        },
    )

    assert response.status_code == 201
    assert response.headers["cache-control"] == "no-store"
    assert response.json()["turnToken"] == "interactive-turn-token-secret"
    assert response.json()["id"] == str(turn.id)
    assert service.received == {
        "owner_user_id": owner.id,
        "device": device,
        "local_session_id": turn.local_session_id,
        "idempotency_key": "route-turn-1",
    }
    assert {
        "prompt",
        "messages",
        "toolArgs",
        "toolResult",
        "response",
    }.isdisjoint(response.json())


def test_retried_claim_returns_200_and_request_contract_is_strict() -> None:
    client, service, _, _, turn = build_client()
    service.claim_result = InteractiveAgentTurnClaim(
        turn=turn,
        turn_token="retried-token",
        created=False,
    )

    retried = client.post(
        "/agent/interactive/turns",
        json={
            "localSessionId": str(turn.local_session_id),
            "idempotencyKey": "route-turn-1",
        },
    )
    unexpected = client.post(
        "/agent/interactive/turns",
        json={
            "localSessionId": str(turn.local_session_id),
            "idempotencyKey": "route-turn-1",
            "prompt": "must remain local",
        },
    )

    assert retried.status_code == 200
    assert unexpected.status_code == 422


def test_turn_lifecycle_routes_are_token_bound_and_failure_codes_are_bounded() -> None:
    client, service, _, _, turn = build_client()

    heartbeat = client.post(
        f"/agent/interactive/turns/{turn.id}/heartbeat",
        json={"expectedLockVersion": 1},
    )
    completed = client.post(
        f"/agent/interactive/turns/{turn.id}/complete",
        json={"expectedLockVersion": 1},
    )
    invalid_failure = client.post(
        f"/agent/interactive/turns/{turn.id}/fail",
        json={"expectedLockVersion": 1, "failureCode": "contains secret spaces"},
    )
    wrong_turn = client.post(
        f"/agent/interactive/turns/{uuid4()}/heartbeat",
        json={"expectedLockVersion": 1},
    )

    assert heartbeat.status_code == 200
    assert heartbeat.headers["cache-control"] == "no-store"
    assert completed.status_code == 200
    assert invalid_failure.status_code == 422
    assert wrong_turn.status_code == 404
    assert service.received[1] == 1


def test_turn_failures_return_stable_non_sensitive_errors() -> None:
    client, service, _, _, turn = build_client()
    service.error = InteractiveAgentTurnUnavailableError("provider detail")
    unavailable = client.post(
        "/agent/interactive/turns",
        json={
            "localSessionId": str(turn.local_session_id),
            "idempotencyKey": "route-turn-1",
        },
    )
    service.error = InteractiveAgentTurnTokenRejectedError("nonce detail")
    rejected = client.post(
        f"/agent/interactive/turns/{turn.id}/heartbeat",
        json={"expectedLockVersion": 1},
    )

    assert unavailable.status_code == 503
    assert unavailable.json() == {"detail": "interactive Agent is unavailable"}
    assert "provider detail" not in unavailable.text
    assert rejected.status_code == 401
    assert rejected.json() == {"detail": "invalid token"}
    assert "nonce detail" not in rejected.text


async def test_interactive_turn_token_has_distinct_purpose_and_bound_ids() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        jwt_secret_key="interactive-route-secret",
    )
    owner_id = uuid4()
    device_id = uuid4()
    turn_id = uuid4()
    token = create_interactive_agent_turn_token(
        turn_id=turn_id,
        owner_user_id=owner_id,
        device_id=device_id,
        settings=settings,
    )

    principal = await require_interactive_agent_turn_principal(
        credentials=HTTPAuthorizationCredentials(scheme="Bearer", credentials=token),
        settings=settings,
    )

    assert principal.turn_id == turn_id
    assert principal.owner_user_id == owner_id
    assert principal.device_id == device_id
    access_token = create_access_token(
        subject=owner_id,
        device_id=device_id,
        settings=settings,
    )
    try:
        await require_interactive_agent_turn_principal(
            credentials=HTTPAuthorizationCredentials(
                scheme="Bearer",
                credentials=access_token,
            ),
            settings=settings,
        )
    except Exception as exc:
        assert getattr(exc, "status_code", None) == 401
    else:
        raise AssertionError("an access token must not authorize an interactive turn")

    malformed_nonce = create_signed_token(
        {
            "sub": str(owner_id),
            "did": str(device_id),
            "tid": str(turn_id),
            "purpose": INTERACTIVE_AGENT_TURN_TOKEN_PURPOSE,
            "nonce": "not-a-valid-nonce",
        },
        settings=settings,
        expires_delta=timedelta(minutes=5),
    )
    try:
        await require_interactive_agent_turn_principal(
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
