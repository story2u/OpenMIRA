import os
from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.deps import get_opportunity_or_404, get_opportunity_repo, require_user
from app.api.v1.routes import opportunities as opportunities_route
from app.domain.enums import IMChannel, OpportunityStatus, Priority
from app.domain.services.opportunity_state import (
    InternalCommandIdempotencyConflict,
    OpportunityVersionConflict,
)
from app.infrastructure.db.models import Opportunity, User


class FakeOpportunityRepository:
    def __init__(self, opportunity: Opportunity) -> None:
        self.opportunity = opportunity
        self.received = None
        self.error: Exception | None = None

    async def transition_status(self, **kwargs):
        self.received = kwargs
        if self.error:
            raise self.error
        self.opportunity.status = kwargs["status"]
        return self.opportunity


def build_client() -> tuple[TestClient, FakeOpportunityRepository, Opportunity]:
    user = User(id=uuid4(), email="internal-command-route@example.test")
    opportunity = Opportunity(
        owner_user_id=user.id,
        channel=IMChannel.TELEGRAM,
        conversation_id="internal-command-route",
        title="Internal command route",
        summary="Status target",
        matched_keywords=[],
        raw_message_links=[],
        confidence=0.8,
        priority=Priority.NORMAL,
        status=OpportunityStatus.PENDING_HUMAN,
        last_message_preview="status",
    )
    repository = FakeOpportunityRepository(opportunity)
    app = FastAPI()
    app.include_router(opportunities_route.router, prefix="/opportunities")
    app.dependency_overrides[require_user] = lambda: user
    app.dependency_overrides[get_opportunity_or_404] = lambda: opportunity
    app.dependency_overrides[get_opportunity_repo] = lambda: repository
    return TestClient(app), repository, opportunity


def test_status_command_requires_paired_version_and_idempotency_key() -> None:
    client, repository, opportunity = build_client()

    missing_version = client.patch(
        f"/opportunities/{opportunity.id}/status",
        json={"status": "following"},
        headers={"Idempotency-Key": "status-command-123"},
    )
    missing_key = client.patch(
        f"/opportunities/{opportunity.id}/status",
        json={"status": "following", "expectedVersion": 1},
    )

    assert missing_version.status_code == 422
    assert missing_key.status_code == 422
    assert repository.received is None


def test_status_command_passes_bounded_idempotent_version_precondition() -> None:
    client, repository, opportunity = build_client()

    response = client.patch(
        f"/opportunities/{opportunity.id}/status",
        json={"status": "following", "expectedVersion": 1},
        headers={"Idempotency-Key": "status-command-123"},
    )

    assert response.status_code == 200
    assert repository.received["owner_user_id"] == opportunity.owner_user_id
    assert repository.received["expected_version"] == 1
    assert repository.received["idempotency_key"] == "status-command-123"
    assert len(repository.received["payload_hash"]) == 64


def test_status_command_returns_stable_conflicts_without_echoing_command_key() -> None:
    client, repository, opportunity = build_client()
    command_key = "status-command-secret"
    repository.error = OpportunityVersionConflict()
    version_conflict = client.patch(
        f"/opportunities/{opportunity.id}/status",
        json={"status": "following", "expectedVersion": 1},
        headers={"Idempotency-Key": command_key},
    )
    repository.error = InternalCommandIdempotencyConflict()
    key_conflict = client.patch(
        f"/opportunities/{opportunity.id}/status",
        json={"status": "following", "expectedVersion": 1},
        headers={"Idempotency-Key": command_key},
    )

    assert version_conflict.status_code == 409
    assert version_conflict.json() == {"detail": "opportunity version conflict"}
    assert key_conflict.status_code == 409
    assert key_conflict.json() == {
        "detail": "Idempotency-Key is bound to another internal command"
    }
    assert command_key not in version_conflict.text
    assert command_key not in key_conflict.text
