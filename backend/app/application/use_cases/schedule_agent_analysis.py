from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from app.domain.enums import AgentAnalysisStatus, UsageStatus
from app.domain.ports import DeviceAnalysisRouting, TaskQueue
from app.infrastructure.db.models import Message
from app.infrastructure.db.repositories import MessageRepository, SubscriptionRepository


@dataclass(frozen=True, slots=True)
class AgentAnalysisScheduleResult:
    message_id: UUID
    status: AgentAnalysisStatus
    enqueued: bool
    deferred_to_device: bool = False
    quota_limit: int | None = None
    quota_allocated: int | None = None


class ScheduleAgentAnalysisUseCase:
    def __init__(
        self,
        *,
        message_repo: MessageRepository,
        subscription_repo: SubscriptionRepository,
        task_queue: TaskQueue,
        device_routing: DeviceAnalysisRouting | None = None,
    ) -> None:
        self.message_repo = message_repo
        self.subscription_repo = subscription_repo
        self.task_queue = task_queue
        self.device_routing = device_routing

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
        if not reservation.created and reservation.ledger.status == UsageStatus.CONSUMED:
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=message.agent_analysis_status,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated,
            )
        ledger_id = reservation.ledger.id

        queued = await self.message_repo.mark_agent_queued(message.id, force=force)
        if not queued:
            await self.subscription_repo.release_usage(
                ledger_id,
                "source message no longer exists",
            )
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=AgentAnalysisStatus.FAILED,
                enqueued=False,
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated - 1,
            )

        deferred_to_device = bool(
            self.device_routing
            and await self.device_routing.has_primary_device(message.owner_user_id)
        )
        delay_seconds = (
            self.device_routing.primary_claim_window_seconds
            if deferred_to_device and self.device_routing
            else 0
        )
        if not self.task_queue.enqueue_agent_analysis(
            message.id,
            # mark_agent_queued already applied force semantics. The worker must never
            # force a second projection after a device completed during the delay.
            force=False,
            usage_ledger_id=ledger_id,
            delay_seconds=delay_seconds,
        ):
            await self.message_repo.fail_agent_analysis(
                message.id,
                "pi agent is disabled or the analysis queue is unavailable",
            )
            released = await self.subscription_repo.release_usage(
                ledger_id,
                "pi agent is disabled or the analysis queue is unavailable",
            )
            current = await self.message_repo.get(message.id)
            return AgentAnalysisScheduleResult(
                message_id=message.id,
                status=(
                    current.agent_analysis_status
                    if current is not None
                    else AgentAnalysisStatus.FAILED
                ),
                enqueued=False,
                deferred_to_device=bool(
                    current is not None
                    and current.agent_analysis_status == AgentAnalysisStatus.RUNNING
                ),
                quota_limit=reservation.limit,
                quota_allocated=reservation.allocated - int(released is not None),
            )

        return AgentAnalysisScheduleResult(
            message_id=message.id,
            status=AgentAnalysisStatus.QUEUED,
            enqueued=True,
            deferred_to_device=deferred_to_device,
            quota_limit=reservation.limit,
            quota_allocated=reservation.allocated,
        )
