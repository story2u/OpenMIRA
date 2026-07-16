from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest

from app.application.use_cases.persist_job_opportunity import PersistJobOpportunityUseCase
from app.domain.enums import (
    IMChannel,
    JobEmploymentType,
    JobMessageClassification,
    JobSeniority,
    JobWorkMode,
    MessageDirection,
    SalaryPeriod,
    SourcePrimaryFunction,
)
from app.domain.job_models import ExtractedJob, ExtractedSalary, JobAgentAnalysis
from app.infrastructure.db.models import Message, SourceFunctionalProfile


class FakeJobRepository:
    def __init__(self) -> None:
        self.saved = None

    async def find_duplicate(self, **kwargs):
        return None

    async def save_projection(self, **kwargs):
        self.saved = kwargs
        return kwargs["opportunity"]


def make_message() -> Message:
    sent_at = datetime(2026, 7, 1, 3, 4, tzinfo=UTC)
    return Message(
        id=uuid4(),
        owner_user_id=uuid4(),
        channel=IMChannel.TELEGRAM,
        external_message_id="public:42",
        conversation_id="-1001",
        sender_display_name="Recruiter Example",
        direction=MessageDirection.INCOMING,
        text="招聘 Senior Python Engineer，远程，SGD 8k/月，35 岁以下，https://jobs.example.com/42",
        source_type="group",
        group_name="Example Remote Jobs",
        raw_payload={
            "channel_post": {
                "message_id": 42,
                "chat": {"username": "example_remote_jobs"},
            }
        },
        sent_at=sent_at,
    )


def make_analysis(*, evidence: bool = True) -> JobAgentAnalysis:
    return JobAgentAnalysis(
        classification=JobMessageClassification.JOB_POST,
        classification_confidence=0.95,
        job=ExtractedJob(
            job_title="Senior Python Engineer",
            company_name=None,
            location_text="Remote",
            work_mode=JobWorkMode.REMOTE,
            employment_type=JobEmploymentType.FULL_TIME,
            seniority=JobSeniority.SENIOR,
            salary=ExtractedSalary(
                raw="SGD 8k/月",
                minimum=8000,
                maximum=8000,
                currency="SGD",
                period=SalaryPeriod.MONTHLY,
            ),
            age_requirement_text="35 岁以下",
            application_url="https://jobs.example.com/42",
        ),
        field_evidence=(
            {
                "job_title": "Senior Python Engineer",
                "location": "远程",
                "work_mode": "远程",
                "salary": "SGD 8k/月",
                "age_requirement": "35 岁以下",
                "application_url": "https://jobs.example.com/42",
            }
            if evidence
            else {}
        ),
        extraction_confidence=0.9,
    )


def make_profile(message: Message) -> SourceFunctionalProfile:
    return SourceFunctionalProfile(
        owner_user_id=message.owner_user_id,
        channel=message.channel,
        external_source_id=message.conversation_id,
        source_display_name=message.group_name or "",
        primary_function=SourcePrimaryFunction.RECRUITMENT,
        reliability_score=0.8,
        expires_at=datetime.now(UTC) + timedelta(days=7),
    )


@pytest.mark.asyncio
async def test_persists_platform_time_and_distinct_source_and_application_urls() -> None:
    repository = FakeJobRepository()
    message = make_message()
    result = await PersistJobOpportunityUseCase(repository).execute(
        message=message,
        analysis=make_analysis(),
        source_profile=make_profile(message),
        existing_opportunity=None,
    )
    assert result.opportunity is not None
    detail = repository.saved["detail"]
    assert detail.posted_at == message.sent_at
    assert detail.source_message_url == "https://t.me/example_remote_jobs/42"
    assert detail.application_url == "https://jobs.example.com/42"
    assert "potentialAgeDiscrimination" in detail.compliance_flags


@pytest.mark.asyncio
async def test_rejects_formal_job_without_field_evidence() -> None:
    repository = FakeJobRepository()
    message = make_message()
    result = await PersistJobOpportunityUseCase(repository).execute(
        message=message,
        analysis=make_analysis(evidence=False),
        source_profile=make_profile(message),
        existing_opportunity=None,
    )
    assert result.opportunity is None
    assert result.rejected_reason and "missing evidence" in result.rejected_reason
    assert repository.saved is None
