from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.reprofile_source_function import ReprofileSourceFunctionUseCase
from app.domain.enums import IMChannel, SourcePrimaryFunction
from app.domain.job_models import SourceProfileAgentAssessment
from app.infrastructure.db.models import SourceFunctionalProfile


class FakeAgent:
    def __init__(self) -> None:
        self.source = None

    async def profile_source_function(self, source):
        self.source = source
        return SourceProfileAgentAssessment(
            primary_function=SourcePrimaryFunction.RECRUITMENT,
            secondary_functions=[SourcePrimaryFunction.CAREER_NETWORKING],
            industry_tags=["software"],
            region_tags=["europe"],
            language_tags=["zh"],
            job_signal_prior=0.9,
            estimated_noise_level=0.2,
            reliability_score=0.8,
            confidence=0.88,
            evidence=["most supplied samples contain hiring signals"],
        )


class FakeMessageRepository:
    async def list_recent_source_samples(self, **kwargs):
        return ["招聘 Python 工程师，联系 jobs@example.com，详情 https://jobs.example.com/1"]


class FakeProfileRepository:
    def __init__(self) -> None:
        self.values = None

    async def save_generated(self, **values):
        self.values = values
        return SimpleNamespace(**values)


class FakeSubscriptionRepository:
    def __init__(self) -> None:
        self.ledger = SimpleNamespace(id=uuid4())
        self.consumed = None
        self.released = None

    async def reserve_agent_analysis(self, **kwargs):
        return SimpleNamespace(allowed=True, created=True, ledger=self.ledger)

    async def consume_usage(self, ledger_id):
        self.consumed = ledger_id

    async def release_usage(self, ledger_id, reason):
        self.released = (ledger_id, reason)


@pytest.mark.asyncio
async def test_reprofile_uses_redacted_bounded_samples_and_consumes_quota() -> None:
    profile = SourceFunctionalProfile(
        id=uuid4(),
        owner_user_id=uuid4(),
        channel=IMChannel.TELEGRAM,
        external_source_id="source-1",
        source_display_name="Example Remote Jobs",
        source_fingerprint="fingerprint",
        primary_function=SourcePrimaryFunction.UNKNOWN,
        profiled_at=datetime(2026, 7, 16, tzinfo=UTC),
        expires_at=datetime.now(UTC) + timedelta(days=7),
    )
    agent = FakeAgent()
    profiles = FakeProfileRepository()
    subscription = FakeSubscriptionRepository()

    saved = await ReprofileSourceFunctionUseCase(
        agent=agent,  # type: ignore[arg-type]
        message_repo=FakeMessageRepository(),  # type: ignore[arg-type]
        profile_repo=profiles,  # type: ignore[arg-type]
        subscription_repo=subscription,  # type: ignore[arg-type]
    ).execute(profile=profile, owner_user_id=profile.owner_user_id)

    assert agent.source["recent_samples"] == ["招聘 Python 工程师，联系 [email]，详情 [url]"]
    assert saved.primary_function == SourcePrimaryFunction.RECRUITMENT
    assert subscription.consumed == subscription.ledger.id
    assert subscription.released is None
