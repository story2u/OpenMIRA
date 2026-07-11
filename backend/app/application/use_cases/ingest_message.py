from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.core.time_window import WorkTimeService
from app.domain.enums import OpportunityStatus
from app.domain.ports import InboundMessage, TaskQueue
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.db.models import Message, Opportunity
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
)


class IngestMessageUseCase:
    def __init__(
        self,
        *,
        message_repo: MessageRepository,
        opportunity_repo: OpportunityRepository,
        rule_repo: RuleRepository,
        detector: OpportunityDetector,
        work_time: WorkTimeService,
        task_queue: TaskQueue,
        subscription_repo: SubscriptionRepository,
    ) -> None:
        self.message_repo = message_repo
        self.opportunity_repo = opportunity_repo
        self.rule_repo = rule_repo
        self.detector = detector
        self.work_time = work_time
        self.task_queue = task_queue
        self.agent_scheduler = ScheduleAgentAnalysisUseCase(
            message_repo=message_repo,
            subscription_repo=subscription_repo,
            task_queue=task_queue,
        )

    async def execute(self, inbound: InboundMessage) -> Message | Opportunity:
        existing = await self.message_repo.get_by_external_id(
            inbound.channel,
            inbound.external_message_id,
        )
        if existing:
            return existing

        message = await self.message_repo.create_incoming(inbound)
        detection = await self.detector.detect(
            text=inbound.text or "",
            rules=await self.rule_repo.enabled_detection_rules(),
        )

        if not detection.is_opportunity:
            await self.message_repo.mark_processed(message.id)
            await self.agent_scheduler.execute(
                message,
                idempotency_key=f"message:{message.id}:automatic",
            )
            return message

        if self.work_time.is_working_time():
            status = OpportunityStatus.PENDING_HUMAN
        else:
            status = OpportunityStatus.AI_AUTO_REPLY

        opportunity = await self.opportunity_repo.create(
            channel=message.channel,
            owner_user_id=inbound.owner_user_id,
            conversation_id=message.conversation_id,
            customer_external_id=message.sender_external_id,
            contact_name=message.sender_display_name,
            source_type=inbound.source_type,
            group_name=inbound.group_name,
            source_message_id=message.id,
            title=detection.title or "新商机",
            summary=detection.summary,
            matched_keywords=detection.matched_keywords,
            raw_message_links=inbound.raw_message_links,
            confidence=detection.confidence,
            priority=detection.priority,
            detection_reason=detection.reason,
            status=status,
            last_message_preview=message.text or "",
        )
        await self.message_repo.attach_opportunity(message.id, opportunity.id)

        if status == OpportunityStatus.AI_AUTO_REPLY:
            self.task_queue.enqueue_ai_reply(opportunity.id)
        else:
            self.task_queue.notify_reviewers(opportunity.id)

        await self.agent_scheduler.execute(
            message,
            idempotency_key=f"message:{message.id}:automatic",
        )

        return opportunity
