from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from app.domain.enums import AgentAnalysisStatus
from app.domain.ports import TaskQueue
from app.infrastructure.db.models import Message
from app.infrastructure.db.repositories import MessageRepository, SubscriptionRepository


@dataclass(frozen=True, slots=True)
class AgentAnalysisScheduleResult:
    message_id: UUID
    status: AgentAnalysisStatus
    enqueued: bool
    quota_limit: int | None = None
    quota_allocated: int | None = None


class ScheduleAgentAnalysisUseCase:
    def __init__(
        self,
        *,
        message_repo: MessageRepository,
        subscription_repo: SubscriptionRepository,
        task_queue: TaskQueue,
    ) -> None:
        self.message_repo = message_repo
        self.subscription_repo = subscription_repo
        self.task_queue = task_queue

    async def execute(
        self,
        message: Message,
        *,
        idempotency_key: str,
        force: bool = False,
    ) -> AgentAnalysisScheduleResult:
        if not message.owner_user_id:
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=message.agent_analysis_status,
                enqueued=False,
            )

        reservation = await self.subscription_repo.reserve_agent_analysis(
            user_id=message.owner_user_id,
            message_id=message.id,
            idempotency_key=idempotency_key,
        )
        if not reservation.allowed and reservation.ledger:
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=message.agent_analysis_status,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated,
            )
        if not reservation.allowed or not reservation.ledger:
            updated = await self.message_repo.mark_agent_quota_exceeded(message.id)
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=(
                    updated.agent_analysis_status
                    if updated
                    else AgentAnalysisStatus.QUOTA_EXCEEDED
                ),
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated,
            )
        if not reservation.created:
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=message.agent_analysis_status,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated,
            )

        queued = await self.message_repo.mark_agent_queued(message.id, force=force)
        if not queued:
            await self.subscription_repo.release_usage(
                reservation.ledger.id,
                "source message no longer exists",
            )
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=AgentAnalysisStatus.FAILED,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated - 1,
            )

        if not self.task_queue.enqueue_agent_analysis(
            message.id,
            force=force,
            usage_ledger_id=reservation.ledger.id,
        ):
            await self.subscription_repo.release_usage(
                reservation.ledger.id,
                "pi agent is disabled or the analysis queue is unavailable",
            )
            await self.message_repo.fail_agent_analysis(
                message.id,
                "pi agent is disabled or the analysis queue is unavailable",
            )
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=AgentAnalysisStatus.FAILED,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated - 1,
            )

        return AgentAnalysisScheduleResult(
            message_id=message.id,
            status=AgentAnalysisStatus.QUEUED,
            enqueued=True,
            quota_limit=reservation.limit,
            quota_allocated=reservation.allocated,
        )
