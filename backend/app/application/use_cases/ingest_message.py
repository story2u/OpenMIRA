from dataclasses import dataclass, field
from uuid import UUID, uuid4

from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.core.time_window import WorkScheduleConfig, WorkScheduleService, WorkTimeService
from app.domain.enums import OpportunityStatus, RuleType
from app.domain.ports import ConversationTurn, DetectionRule, InboundMessage, TaskQueue
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.db.models import Message, Opportunity
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    RuleRepository,
    SubscriptionRepository,
    UserSettingsRepository,
)


@dataclass(frozen=True, slots=True)
class _DetectionPrefs:
    extra_rules: list[DetectionRule] = field(default_factory=list)
    ai_semantics_enabled: bool = True


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
        user_settings_repo: UserSettingsRepository | None = None,
    ) -> None:
        self.message_repo = message_repo
        self.opportunity_repo = opportunity_repo
        self.rule_repo = rule_repo
        self.detector = detector
        self.work_time = work_time
        self.user_settings_repo = user_settings_repo
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
        conversation = await self.message_repo.list_by_conversation(
            message.channel,
            message.conversation_id,
            message.owner_user_id,
            limit=7,
        )

        detection_prefs = await self._detection_prefs(inbound.owner_user_id)
        rules = await self.rule_repo.enabled_detection_rules()
        rules = rules + detection_prefs.extra_rules
        detector = (
            self.detector
            if detection_prefs.ai_semantics_enabled
            else OpportunityDetector(ai_classifier=None)
        )

        detection = await detector.detect(
            text=inbound.text or "",
            rules=rules,
            conversation=self._detection_context(conversation, current_message_id=message.id),
            source_type=inbound.source_type,
            group_name=inbound.group_name,
        )

        if not detection.is_opportunity:
            await self.message_repo.mark_processed(message.id)
            await self.agent_scheduler.execute(
                message,
                idempotency_key=f"message:{message.id}:automatic",
            )
            return message

        working_time, user_auto_reply_enabled = await self._reply_routing(inbound.owner_user_id)
        if (
            inbound.force_human_review
            or detection.requires_human_review
            or working_time
            or not user_auto_reply_enabled
            or not inbound.auto_reply_allowed
        ):
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

        if status != OpportunityStatus.AI_AUTO_REPLY:
            self.task_queue.notify_reviewers(opportunity.id)

        schedule_result = await self.agent_scheduler.execute(
            message,
            idempotency_key=f"message:{message.id}:automatic",
        )
        if status == OpportunityStatus.AI_AUTO_REPLY and not schedule_result.enqueued:
            opportunity = await self.opportunity_repo.update_status(
                opportunity,
                OpportunityStatus.PENDING_HUMAN,
            )
            self.task_queue.notify_reviewers(opportunity.id)

        return opportunity

    async def _detection_prefs(self, owner_user_id: UUID | None) -> "_DetectionPrefs":
        """把用户自定义关键词转成 KEYWORD 规则叠加到全局规则；无 owner 或无偏好则用默认。"""
        if not owner_user_id or not self.user_settings_repo:
            return _DetectionPrefs(extra_rules=[], ai_semantics_enabled=True)
        pref = await self.user_settings_repo.get_detection(owner_user_id)
        if not pref:
            return _DetectionPrefs(extra_rules=[], ai_semantics_enabled=True)
        extra_rules: list[DetectionRule] = []
        if pref.keywords:
            extra_rules.append(
                DetectionRule(
                    id=uuid4(),
                    name="用户关键词",
                    rule_type=RuleType.KEYWORD,
                    pattern=",".join(pref.keywords),
                    score=0.5,
                    priority=10,
                )
            )
        return _DetectionPrefs(
            extra_rules=extra_rules,
            ai_semantics_enabled=pref.ai_semantics_enabled,
        )

    async def _reply_routing(self, owner_user_id: UUID | None) -> tuple[bool, bool]:
        """Return (is_working_time, user explicitly enabled after-hours auto reply)."""
        if owner_user_id and self.user_settings_repo:
            schedule = await self.user_settings_repo.get_work_schedule(owner_user_id)
            if schedule:
                service = WorkScheduleService(
                    WorkScheduleConfig(
                        timezone=schedule.timezone,
                        slots=schedule.slots,
                        auto_reply_outside_hours=schedule.auto_reply_outside_hours,
                    )
                )
                return service.is_working_time(), schedule.auto_reply_outside_hours
        return self.work_time.is_working_time(), False

    def _detection_context(
        self,
        messages: list[Message],
        *,
        current_message_id: UUID,
    ) -> list[ConversationTurn]:
        remaining_chars = 4000
        turns: list[ConversationTurn] = []
        for candidate in reversed(messages):
            if candidate.id == current_message_id or not candidate.text:
                continue
            text = candidate.text.strip()
            if not text or remaining_chars <= 0:
                continue
            text = text[: min(1000, remaining_chars)]
            turns.append(
                ConversationTurn(
                    sender_display_name=candidate.sender_display_name,
                    direction=candidate.direction,
                    text=text,
                )
            )
            remaining_chars -= len(text)
            if len(turns) >= 6:
                break
        return list(reversed(turns))
