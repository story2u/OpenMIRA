from __future__ import annotations

import hashlib
from datetime import timedelta

import structlog

from app.core.config import Settings
from app.core.time_window import WorkScheduleConfig, WorkScheduleService
from app.domain.enums import (
    AutoReplyDecisionReason,
    AutoReplyDeliveryStatus,
    MessageSource,
    OpportunityStatus,
    TelegramConnectionStatus,
)
from app.domain.ports import ReplyGenerator, TaskQueue
from app.domain.services.auto_reply_policy import (
    AutoReplyPolicyInput,
    evaluate_auto_reply,
    validate_auto_reply_draft,
)
from app.domain.services.opportunity_state import ensure_transition_allowed
from app.infrastructure.db.models import Opportunity, utc_now
from app.infrastructure.db.repositories import (
    AutoReplyDeliveryRepository,
    MessageRepository,
    OpportunityRepository,
    TelegramConnectionRepository,
    UserSettingsRepository,
)
from app.infrastructure.im.base import AdapterRegistry

logger = structlog.get_logger(__name__)


class AIDraftUseCase:
    def __init__(
        self,
        *,
        opportunity_repo: OpportunityRepository,
        reply_generator: ReplyGenerator,
    ) -> None:
        self.opportunity_repo = opportunity_repo
        self.reply_generator = reply_generator

    async def execute(self, opportunity: Opportunity) -> str:
        draft = await self.reply_generator.generate_reply(opportunity.id)
        await self.opportunity_repo.save_ai_draft(opportunity, draft)
        return draft


class AIAutoReplyUseCase:
    def __init__(
        self,
        *,
        settings: Settings,
        opportunity_repo: OpportunityRepository,
        message_repo: MessageRepository,
        delivery_repo: AutoReplyDeliveryRepository,
        telegram_repo: TelegramConnectionRepository,
        user_settings_repo: UserSettingsRepository,
        adapters: AdapterRegistry,
        reply_generator: ReplyGenerator,
        task_queue: TaskQueue,
    ) -> None:
        self.settings = settings
        self.opportunity_repo = opportunity_repo
        self.message_repo = message_repo
        self.delivery_repo = delivery_repo
        self.telegram_repo = telegram_repo
        self.user_settings_repo = user_settings_repo
        self.adapters = adapters
        self.reply_generator = reply_generator
        self.task_queue = task_queue

    async def execute(self, opportunity: Opportunity) -> Opportunity:
        if not opportunity.owner_user_id or not opportunity.source_message_id:
            return await self._send_to_human(opportunity)
        source_message = await self.message_repo.get(opportunity.source_message_id)
        if not source_message:
            return await self._send_to_human(opportunity)

        delivery, created = await self.delivery_repo.reserve(
            owner_user_id=opportunity.owner_user_id,
            opportunity_id=opportunity.id,
            source_message_id=source_message.id,
            channel=opportunity.channel,
            conversation_id=opportunity.conversation_id,
            idempotency_key=f"auto-reply:{opportunity.id}",
        )
        if not created and delivery.status == AutoReplyDeliveryStatus.SENT:
            return await self._reconcile_sent_delivery(opportunity, delivery)
        if not created and delivery.status != AutoReplyDeliveryStatus.CANDIDATE:
            logger.info(
                "auto_reply.duplicate_task",
                opportunity_id=str(opportunity.id),
                delivery_id=str(delivery.id),
                status=delivery.status,
            )
            return opportunity
        delivery = await self.delivery_repo.claim_candidate(delivery.id)
        if not delivery:
            return opportunity

        try:
            decision = await self._evaluate(opportunity, source_message.text or "")
            if not decision.allowed:
                await self.delivery_repo.mark_blocked(delivery, decision.reason)
                return await self._send_to_human(opportunity)

            draft = await self.reply_generator.generate_auto_reply(opportunity.id)
            content_decision = validate_auto_reply_draft(
                draft,
                max_chars=self.settings.ai_auto_reply_max_chars,
            )
            if not content_decision.allowed:
                await self.delivery_repo.mark_blocked(delivery, content_decision.reason)
                return await self._send_to_human(opportunity)

            await self.opportunity_repo.save_ai_draft(opportunity, draft)
            delivery = await self.delivery_repo.mark_ready(
                delivery,
                content_hash=hashlib.sha256(draft.encode("utf-8")).hexdigest(),
            )
            claimed = await self.delivery_repo.claim_ready_for_send(delivery)
            if not claimed:
                await self.delivery_repo.mark_blocked(
                    delivery,
                    AutoReplyDecisionReason.OPPORTUNITY_INACTIVE,
                )
                return await self.opportunity_repo.get(opportunity.id) or opportunity

            delivery, current = claimed
            adapter = self.adapters.get(current.channel)
            receipt = await adapter.send_message(
                current.conversation_id,
                draft,
                idempotency_key=delivery.idempotency_key,
                opportunity_id=current.id,
                owner_user_id=current.owner_user_id,
            )
            if not receipt.delivered:
                await self.delivery_repo.mark_dry_run(delivery)
                return await self._send_to_human(current)

            external_message_id = receipt.provider_message_id or f"auto-reply:{delivery.id}"
            delivery = await self.delivery_repo.mark_sent(
                delivery,
                provider_message_id=external_message_id,
            )
            await self.message_repo.create_outgoing(
                channel=current.channel,
                owner_user_id=current.owner_user_id,
                conversation_id=current.conversation_id,
                text=draft,
                source=MessageSource.AI,
                opportunity_id=current.id,
                external_message_id=external_message_id,
                raw_payload={"autoReplyDeliveryId": str(delivery.id)},
            )
            return await self.opportunity_repo.update_status(
                current,
                OpportunityStatus.REPLIED,
                final_reply=draft,
                clear_assignment=True,
            )
        except Exception as exc:
            logger.exception(
                "auto_reply.failed",
                opportunity_id=str(opportunity.id),
                delivery_id=str(delivery.id),
            )
            if delivery.status == AutoReplyDeliveryStatus.SENT:
                recovered = await self.delivery_repo.reload_after_rollback(delivery.id)
                if recovered:
                    return await self._reconcile_sent_delivery(opportunity, recovered)
                return await self.opportunity_repo.get(opportunity.id) or opportunity
            if delivery.status == AutoReplyDeliveryStatus.SENDING:
                await self.delivery_repo.mark_sending_uncertain(
                    delivery,
                    exc.__class__.__name__,
                )
                self.task_queue.notify_reviewers(opportunity.id)
                return await self.opportunity_repo.get(opportunity.id) or opportunity
            await self.delivery_repo.mark_failed(delivery, exc.__class__.__name__)
            return await self._send_to_human(opportunity)

    async def _evaluate(self, opportunity: Opportunity, message_text: str):
        schedule = await self.user_settings_repo.get_work_schedule(opportunity.owner_user_id)
        user_enabled = bool(schedule and schedule.auto_reply_outside_hours)
        is_working_time = True
        if schedule:
            is_working_time = WorkScheduleService(
                WorkScheduleConfig(
                    timezone=schedule.timezone,
                    slots=schedule.slots,
                    auto_reply_outside_hours=schedule.auto_reply_outside_hours,
                )
            ).is_working_time()

        source_row = await self.telegram_repo.get_auto_reply_source(
            owner_user_id=opportunity.owner_user_id,
            conversation_id=opportunity.conversation_id,
        )
        source_eligible = source_row is not None
        source_enabled = False
        if source_row:
            source, connection = source_row
            source_enabled = bool(
                source.auto_reply_enabled
                and source.enabled
                and not source.quota_paused
                and connection.enabled
                and connection.status == TelegramConnectionStatus.CONNECTED
                and connection.capabilities.get("can_reply") is True
            )

        now = utc_now()
        recent_sent_count = await self.delivery_repo.count_sent_since(
            owner_user_id=opportunity.owner_user_id,
            channel=opportunity.channel,
            conversation_id=opportunity.conversation_id,
            since=now - timedelta(minutes=self.settings.ai_auto_reply_cooldown_minutes),
        )
        window_sent_count = await self.delivery_repo.count_sent_since(
            owner_user_id=opportunity.owner_user_id,
            channel=opportunity.channel,
            conversation_id=opportunity.conversation_id,
            since=now - timedelta(hours=self.settings.ai_auto_reply_window_hours),
        )
        link_status = str(opportunity.link_verification.get("status") or "unverified")
        return evaluate_auto_reply(
            AutoReplyPolicyInput(
                feature_enabled=self.settings.ai_auto_reply_enabled,
                send_enabled=self.settings.im_send_enabled,
                user_enabled=user_enabled,
                is_working_time=is_working_time,
                source_eligible=source_eligible,
                source_enabled=source_enabled,
                channel=opportunity.channel,
                source_type=opportunity.source_type,
                opportunity_status=opportunity.status,
                archived=opportunity.archived_at is not None,
                assigned_to=opportunity.assigned_to,
                agent_status=opportunity.agent_analysis_status,
                confidence=opportunity.confidence,
                min_confidence=self.settings.ai_auto_reply_min_confidence,
                priority=opportunity.priority,
                attention_required=opportunity.attention_required,
                link_status=link_status,
                has_links=bool(opportunity.raw_message_links),
                message_text=message_text,
                recent_sent_count=recent_sent_count,
                window_sent_count=window_sent_count,
                max_per_window=self.settings.ai_auto_reply_max_per_window,
            )
        )

    async def _send_to_human(self, opportunity: Opportunity) -> Opportunity:
        current = await self.opportunity_repo.get(opportunity.id)
        if not current:
            return opportunity
        if current.status == OpportunityStatus.AI_AUTO_REPLY:
            if current.assigned_to and current.assigned_to != "ai:auto_reply":
                return current
            ensure_transition_allowed(current.status, OpportunityStatus.PENDING_HUMAN)
            current = await self.opportunity_repo.update_status(
                current,
                OpportunityStatus.PENDING_HUMAN,
                clear_assignment=current.assigned_to == "ai:auto_reply",
            )
            self.task_queue.notify_reviewers(current.id)
        return current

    async def _reconcile_sent_delivery(self, opportunity: Opportunity, delivery) -> Opportunity:
        """Complete local projection after provider success without sending again."""
        current = await self.opportunity_repo.get(opportunity.id)
        if not current or not delivery.provider_message_id:
            return current or opportunity
        if current.status not in {OpportunityStatus.AI_AUTO_REPLY, OpportunityStatus.REPLIED}:
            return current
        existing = await self.message_repo.get_by_external_id(
            current.channel,
            delivery.provider_message_id,
        )
        if not existing:
            await self.message_repo.create_outgoing(
                channel=current.channel,
                owner_user_id=current.owner_user_id,
                conversation_id=current.conversation_id,
                text=current.ai_reply_draft,
                source=MessageSource.AI,
                opportunity_id=current.id,
                external_message_id=delivery.provider_message_id,
                raw_payload={"autoReplyDeliveryId": str(delivery.id)},
            )
        if (
            current.status == OpportunityStatus.AI_AUTO_REPLY
            and current.assigned_to == "ai:auto_reply"
        ):
            return await self.opportunity_repo.update_status(
                current,
                OpportunityStatus.REPLIED,
                final_reply=current.ai_reply_draft,
                clear_assignment=True,
            )
        return current
