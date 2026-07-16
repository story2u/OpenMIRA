from datetime import UTC, datetime
from uuid import UUID, uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.deps import (
    get_job_opportunity_repo,
    get_job_search_profile_repo,
    require_user,
)
from app.api.v1.routes import job_search_profiles, jobs
from app.domain.enums import IMChannel, OpportunityType
from app.infrastructure.db.models import JobOpportunityDetail, JobSearchProfile, Opportunity, User


class FakeProfileRepo:
    def __init__(self, owner_id: UUID, profile: JobSearchProfile | None = None) -> None:
        self.owner_id = owner_id
        self.profile = profile

    async def list_for_owner(self, owner_id: UUID):
        return [self.profile] if owner_id == self.owner_id and self.profile else []

    async def get_for_owner(self, profile_id: UUID, owner_id: UUID):
        if (
            self.profile
            and self.profile.id == profile_id
            and self.profile.user_id == owner_id
            and owner_id == self.owner_id
        ):
            return self.profile
        return None


class FakeJobRepo:
    def __init__(self, opportunity: Opportunity, detail: JobOpportunityDetail) -> None:
        self.opportunity = opportunity
        self.detail = detail

    async def list_for_owner(self, **kwargs):
        if kwargs["owner_user_id"] != self.opportunity.owner_user_id:
            return [], 0
        return [(self.opportunity, self.detail, None)], 1

    async def source_counts(self, opportunity_ids, owner_user_id):
        return {self.opportunity.id: 1}


def _models(owner_id: UUID):
    now = datetime(2026, 7, 16, 8, tzinfo=UTC)
    opportunity = Opportunity(
        id=uuid4(),
        owner_user_id=owner_id,
        opportunity_type=OpportunityType.JOB,
        channel=IMChannel.TELEGRAM,
        conversation_id="jobs-1",
        title="Python Backend Engineer",
        summary="FastAPI role",
        created_at=now,
        updated_at=now,
    )
    detail = JobOpportunityDetail(
        opportunity_id=opportunity.id,
        source_channel=IMChannel.TELEGRAM,
        source_chat_id="jobs-1",
        source_chat_name="Example Remote Jobs",
        source_message_id="42",
        posted_at=now,
        job_title="Python Backend Engineer",
        requirements_summary="FastAPI role",
        content_fingerprint="a" * 64,
        field_evidence={"job_title": "Python Backend Engineer"},
        extraction_confidence=0.91,
    )
    profile = JobSearchProfile(
        id=uuid4(),
        user_id=owner_id,
        name="后端",
        is_default=True,
        target_roles=["Python Backend Engineer"],
        created_at=now,
        updated_at=now,
    )
    return opportunity, detail, profile


def test_jobs_list_is_owner_scoped_and_exposes_real_job_fields() -> None:
    owner_id = uuid4()
    opportunity, detail, profile = _models(owner_id)
    app = FastAPI()
    app.include_router(jobs.router, prefix="/jobs")
    app.dependency_overrides[require_user] = lambda: User(id=owner_id, email="owner@example.test")
    app.dependency_overrides[get_job_search_profile_repo] = lambda: FakeProfileRepo(
        owner_id, profile
    )
    app.dependency_overrides[get_job_opportunity_repo] = lambda: FakeJobRepo(
        opportunity, detail
    )

    response = TestClient(app).get("/jobs")

    assert response.status_code == 200
    assert response.json()["items"][0]["jobTitle"] == "Python Backend Engineer"
    assert response.json()["items"][0]["sourceChatName"] == "Example Remote Jobs"


def test_profile_lookup_hides_another_users_profile() -> None:
    owner_id = uuid4()
    _, _, profile = _models(uuid4())
    app = FastAPI()
    app.include_router(job_search_profiles.router, prefix="/job-search-profiles")
    app.dependency_overrides[require_user] = lambda: User(id=owner_id, email="owner@example.test")
    app.dependency_overrides[get_job_search_profile_repo] = lambda: FakeProfileRepo(
        owner_id, profile
    )

    response = TestClient(app).get(f"/job-search-profiles/{profile.id}")

    assert response.status_code == 404


def test_profile_contract_rejects_protected_attributes() -> None:
    owner_id = uuid4()
    app = FastAPI()
    app.include_router(job_search_profiles.router, prefix="/job-search-profiles")
    app.dependency_overrides[require_user] = lambda: User(id=owner_id, email="owner@example.test")

    response = TestClient(app).post(
        "/job-search-profiles",
        json={"name": "违规档案", "targetRoles": ["Engineer"], "age": 35},
    )

    assert response.status_code == 422
