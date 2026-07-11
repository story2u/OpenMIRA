from types import SimpleNamespace
from uuid import uuid4

from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.domain.enums import AgentAnalysisStatus, IMChannel, MessageDirection
from app.infrastructure.db.models import Message


class FakeMessageRepository:
    def __init__(self) -> None:
        self.quota_marked = False
        self.failed = False

    async def mark_agent_quota_exceeded(self, message_id):
        self.quota_marked = True
        return SimpleNamespace(agent_analysis_status=AgentAnalysisStatus.QUOTA_EXCEEDED)

    async def mark_agent_queued(self, message_id, *, force=False):
        return SimpleNamespace(id=message_id)

    async def fail_agent_analysis(self, message_id, error):
        self.failed = True


class FakeSubscriptionRepository:
    def __init__(self, reservation) -> None:
        self.reservation = reservation
        self.reserve_calls = []
        self.released = []

    async def reserve_agent_analysis(self, **kwargs):
        self.reserve_calls.append(kwargs)
        return self.reservation

    async def release_usage(self, ledger_id, reason):
        self.released.append((ledger_id, reason))


class FakeTaskQueue:
    def __init__(self, *, succeeds=True) -> None:
        self.succeeds = succeeds
        self.sent = []

    def enqueue_agent_analysis(self, message_id, **kwargs):
        self.sent.append((message_id, kwargs))
        return self.succeeds


def make_message(*, with_owner=True) -> Message:
    return Message(
        id=uuid4(),
        owner_user_id=uuid4() if with_owner else None,
        channel=IMChannel.TELEGRAM,
        external_message_id=str(uuid4()),
        conversation_id="group-1",
        direction=MessageDirection.INCOMING,
    )


async def test_ownerless_message_is_not_reserved_or_enqueued() -> None:
    message = make_message(with_owner=False)
    subscription_repo = FakeSubscriptionRepository(None)
    queue = FakeTaskQueue()

    result = await ScheduleAgentAnalysisUseCase(
        message_repo=FakeMessageRepository(),
        subscription_repo=subscription_repo,
        task_queue=queue,
    ).execute(message, idempotency_key="automatic")

    assert result.enqueued is False
    assert subscription_repo.reserve_calls == []
    assert queue.sent == []


async def test_quota_exhaustion_is_persisted_without_enqueuing() -> None:
    message = make_message()
    message_repo = FakeMessageRepository()
    subscription_repo = FakeSubscriptionRepository(
        SimpleNamespace(allowed=False, created=False, ledger=None, limit=100, allocated=100)
    )
    queue = FakeTaskQueue()

    result = await ScheduleAgentAnalysisUseCase(
        message_repo=message_repo,
        subscription_repo=subscription_repo,
        task_queue=queue,
    ).execute(message, idempotency_key="automatic")

    assert result.status == AgentAnalysisStatus.QUOTA_EXCEEDED
    assert result.quota_allocated == 100
    assert message_repo.quota_marked is True
    assert queue.sent == []


async def test_reusing_a_released_idempotency_key_is_not_reported_as_quota_exhaustion() -> None:
    message = make_message()
    message.agent_analysis_status = AgentAnalysisStatus.FAILED
    message_repo = FakeMessageRepository()
    subscription_repo = FakeSubscriptionRepository(
        SimpleNamespace(
            allowed=False,
            created=False,
            ledger=SimpleNamespace(id=uuid4()),
            limit=100,
            allocated=0,
        )
    )

    result = await ScheduleAgentAnalysisUseCase(
        message_repo=message_repo,
        subscription_repo=subscription_repo,
        task_queue=FakeTaskQueue(),
    ).execute(message, idempotency_key="released-request")

    assert result.status == AgentAnalysisStatus.FAILED
    assert message_repo.quota_marked is False


async def test_new_reservation_is_forwarded_to_the_worker() -> None:
    message = make_message()
    ledger_id = uuid4()
    subscription_repo = FakeSubscriptionRepository(
        SimpleNamespace(
            allowed=True,
            created=True,
            ledger=SimpleNamespace(id=ledger_id),
            limit=1_000,
            allocated=7,
        )
    )
    queue = FakeTaskQueue()

    result = await ScheduleAgentAnalysisUseCase(
        message_repo=FakeMessageRepository(),
        subscription_repo=subscription_repo,
        task_queue=queue,
    ).execute(message, idempotency_key="manual:request-1", force=True)

    assert result.status == AgentAnalysisStatus.QUEUED
    assert result.enqueued is True
    assert queue.sent == [
        (message.id, {"force": True, "usage_ledger_id": ledger_id}),
    ]


async def test_enqueue_failure_releases_reservation() -> None:
    message = make_message()
    ledger_id = uuid4()
    message_repo = FakeMessageRepository()
    subscription_repo = FakeSubscriptionRepository(
        SimpleNamespace(
            allowed=True,
            created=True,
            ledger=SimpleNamespace(id=ledger_id),
            limit=100,
            allocated=1,
        )
    )

    result = await ScheduleAgentAnalysisUseCase(
        message_repo=message_repo,
        subscription_repo=subscription_repo,
        task_queue=FakeTaskQueue(succeeds=False),
    ).execute(message, idempotency_key="automatic")

    assert result.status == AgentAnalysisStatus.FAILED
    assert message_repo.failed is True
    assert subscription_repo.released[0][0] == ledger_id
