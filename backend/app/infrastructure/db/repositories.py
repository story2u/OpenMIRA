from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from uuid import UUID

import structlog
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from sqlmodel import col, select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    AgentAnalysisStatus,
    FrontendOpportunityStatus,
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityStatus,
    PlanCode,
    Priority,
    SubscriptionStatus,
    TelegramConnectionAttemptStatus,
    TelegramConnectionStatus,
    TelegramConnectionType,
    TelegramSourceType,
    UsageFeature,
    UsageStatus,
)
from app.domain.ports import AgentAnalysisProjection, DetectionRule, InboundMessage
from app.domain.services.subscription_policy import (
    BillingPeriod,
    PlanEntitlements,
    effective_plan_code,
    ensure_group_quota,
    get_plan_entitlements,
    utc_calendar_month,
)
from app.infrastructure.db.models import (
    AppConfig,
    AuthAccount,
    Message,
    Opportunity,
    ReplyTemplate,
    Rule,
    SubscriptionAccount,
    TelegramConnection,
    TelegramConnectionAttempt,
    TelegramMonitor,
    TelegramSource,
    TelegramUserConfig,
    TelegramWebhookEvent,
    UsageLedger,
    User,
    utc_now,
)

logger = structlog.get_logger(__name__)


FRONTEND_STATUS_MAP: dict[FrontendOpportunityStatus, set[OpportunityStatus]] = {
    FrontendOpportunityStatus.PENDING: {
        OpportunityStatus.PENDING_HUMAN,
        OpportunityStatus.AI_AUTO_REPLY,
    },
    FrontendOpportunityStatus.REPLIED: {
        OpportunityStatus.REPLIED,
        OpportunityStatus.FOLLOWING,
    },
    FrontendOpportunityStatus.IGNORED: {
        OpportunityStatus.IGNORED,
        OpportunityStatus.CLOSED,
    },
}


class UserRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def count(self) -> int:
        result = await self.session.exec(select(func.count()).select_from(User))
        return int(result.one())

    async def get(self, user_id: UUID) -> User | None:
        return await self.session.get(User, user_id)

    async def get_by_email(self, email: str) -> User | None:
        statement = select(User).where(User.email == email.lower().strip())
        result = await self.session.exec(statement)
        return result.first()

    async def get_by_auth_account(self, provider: str, provider_subject: str) -> User | None:
        statement = (
            select(User)
            .join(AuthAccount)
            .where(
                AuthAccount.provider == provider,
                AuthAccount.provider_subject == provider_subject,
            )
        )
        result = await self.session.exec(statement)
        return result.first()

    async def list(self) -> list[User]:
        result = await self.session.exec(select(User).order_by(col(User.created_at).asc()))
        return list(result.all())

    async def create_oauth_user(
        self,
        *,
        email: str,
        display_name: str,
        avatar_url: str = "",
    ) -> User:
        user = User(
            email=email.lower().strip(),
            display_name=display_name.strip() or email.lower().strip(),
            avatar_url=avatar_url,
        )
        self.session.add(user)
        await self.session.commit()
        await self.session.refresh(user)
        return user

    async def link_auth_account(
        self,
        *,
        user: User,
        provider: str,
        provider_subject: str,
        email: str | None,
    ) -> AuthAccount:
        account = AuthAccount(
            user_id=user.id,
            provider=provider,
            provider_subject=provider_subject,
            email=email.lower().strip() if email else None,
        )
        self.session.add(account)
        await self.session.commit()
        await self.session.refresh(account)
        return account

    async def get_or_create_oauth_user(
        self,
        *,
        provider: str,
        provider_subject: str,
        email: str,
        display_name: str,
        avatar_url: str = "",
    ) -> User:
        auth_account_available = True
        try:
            user = await self.get_by_auth_account(provider, provider_subject)
        except SQLAlchemyError as exc:
            await self.session.rollback()
            auth_account_available = False
            logger.warning(
                "oauth.auth_account_lookup_failed",
                provider=provider,
                error_class=exc.__class__.__name__,
            )
            user = None
        if user:
            return await self.mark_login(user)

        user = await self.get_by_email(email)
        if not user:
            try:
                user = await self.create_oauth_user(
                    email=email,
                    display_name=display_name,
                    avatar_url=avatar_url,
                )
            except IntegrityError:
                await self.session.rollback()
                user = await self.get_by_email(email)
                if not user:
                    raise

        if auth_account_available:
            try:
                await self.link_auth_account(
                    user=user,
                    provider=provider,
                    provider_subject=provider_subject,
                    email=email,
                )
            except IntegrityError:
                await self.session.rollback()
                linked_user = await self.get_by_auth_account(provider, provider_subject)
                if linked_user:
                    return await self.mark_login(linked_user)
                raise
            except SQLAlchemyError as exc:
                await self.session.rollback()
                logger.warning(
                    "oauth.auth_account_link_failed",
                    provider=provider,
                    user_id=str(user.id),
                    error_class=exc.__class__.__name__,
                )
        return await self.mark_login(user)

    async def mark_login(self, user: User) -> User:
        user.last_login_at = utc_now()
        user.updated_at = utc_now()
        self.session.add(user)
        await self.session.commit()
        await self.session.refresh(user)
        return user


@dataclass(frozen=True, slots=True)
class SubscriptionSnapshot:
    plan_code: PlanCode
    subscription_status: SubscriptionStatus
    period: BillingPeriod
    entitlements: PlanEntitlements
    cancel_at_period_end: bool


@dataclass(frozen=True, slots=True)
class UsageReservation:
    allowed: bool
    created: bool
    ledger: UsageLedger | None
    limit: int
    allocated: int


class SubscriptionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_account(self, user_id: UUID) -> SubscriptionAccount | None:
        statement = select(SubscriptionAccount).where(SubscriptionAccount.user_id == user_id)
        result = await self.session.exec(statement)
        return result.first()

    async def get_snapshot(
        self,
        user_id: UUID,
        *,
        now: datetime | None = None,
    ) -> SubscriptionSnapshot:
        current = now or utc_now()
        account = await self.get_account(user_id)
        plan_code = effective_plan_code(
            plan_code=account.plan_code if account else None,
            status=account.status if account else None,
            period_start=account.current_period_start if account else None,
            period_end=account.current_period_end if account else None,
            now=current,
        )
        if (
            account
            and plan_code != PlanCode.FREE
            and account.current_period_start
            and account.current_period_end
        ):
            period = BillingPeriod(
                start=account.current_period_start,
                end=account.current_period_end,
            )
        else:
            period = utc_calendar_month(current)
        return SubscriptionSnapshot(
            plan_code=plan_code,
            subscription_status=(account.status if account else SubscriptionStatus.INACTIVE),
            period=period,
            entitlements=get_plan_entitlements(plan_code),
            cancel_at_period_end=bool(account and account.cancel_at_period_end),
        )

    async def reserve_agent_analysis(
        self,
        *,
        user_id: UUID,
        message_id: UUID,
        idempotency_key: str,
        now: datetime | None = None,
    ) -> UsageReservation:
        current = now or utc_now()
        normalized_key = idempotency_key.strip()[:255]
        if not normalized_key:
            raise ValueError("idempotency_key is required")
        # The user row is the per-owner serialization point, including Free users who do not
        # have a subscription_accounts row. This prevents concurrent reservations overspending.
        user = await self.session.get(User, user_id, with_for_update=True)
        if not user or not user.is_active:
            await self.session.rollback()
            return UsageReservation(False, False, None, 0, 0)

        existing_statement = select(UsageLedger).where(
            UsageLedger.user_id == user_id,
            UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
            UsageLedger.idempotency_key == normalized_key,
        )
        existing_result = await self.session.exec(existing_statement)
        existing = existing_result.first()
        snapshot = await self.get_snapshot(user_id, now=current)
        limit = snapshot.entitlements.pi_agent_analysis_monthly_limit
        allocated = await self._allocated_usage(user_id, snapshot.period)
        if existing:
            reservation = UsageReservation(
                allowed=existing.status in {UsageStatus.RESERVED, UsageStatus.CONSUMED},
                created=False,
                ledger=existing,
                limit=limit,
                allocated=allocated,
            )
            await self.session.commit()
            return reservation
        if allocated >= limit:
            await self.session.rollback()
            return UsageReservation(False, False, None, limit, allocated)

        ledger = UsageLedger(
            user_id=user_id,
            feature=UsageFeature.PI_AGENT_ANALYSIS,
            quantity=1,
            period_start=snapshot.period.start,
            period_end=snapshot.period.end,
            idempotency_key=normalized_key,
            source_message_id=message_id,
            status=UsageStatus.RESERVED,
        )
        self.session.add(ledger)
        await self.session.commit()
        await self.session.refresh(ledger)
        return UsageReservation(True, True, ledger, limit, allocated + 1)

    async def usage_counts(
        self,
        *,
        user_id: UUID,
        period: BillingPeriod,
    ) -> tuple[int, int]:
        statement = (
            select(UsageLedger.status, func.sum(UsageLedger.quantity))
            .where(
                UsageLedger.user_id == user_id,
                UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
                UsageLedger.period_start == period.start,
                UsageLedger.period_end == period.end,
                UsageLedger.status.in_([UsageStatus.RESERVED, UsageStatus.CONSUMED]),
            )
            .group_by(UsageLedger.status)
        )
        result = await self.session.exec(statement)
        counts = {status: int(quantity) for status, quantity in result.all()}
        return counts.get(UsageStatus.CONSUMED, 0), counts.get(UsageStatus.RESERVED, 0)

    async def consume_usage(self, ledger_id: UUID) -> UsageLedger | None:
        ledger = await self.session.get(UsageLedger, ledger_id, with_for_update=True)
        if not ledger:
            return None
        if ledger.status == UsageStatus.CONSUMED:
            return ledger
        if ledger.status != UsageStatus.RESERVED:
            return ledger
        now = utc_now()
        ledger.status = UsageStatus.CONSUMED
        ledger.consumed_at = now
        ledger.updated_at = now
        self.session.add(ledger)
        await self.session.commit()
        await self.session.refresh(ledger)
        return ledger

    async def release_usage(self, ledger_id: UUID, reason: str) -> UsageLedger | None:
        ledger = await self.session.get(UsageLedger, ledger_id, with_for_update=True)
        if not ledger:
            return None
        if ledger.status != UsageStatus.RESERVED:
            return ledger
        now = utc_now()
        ledger.status = UsageStatus.RELEASED
        ledger.released_at = now
        ledger.failure_reason = reason[:500]
        ledger.updated_at = now
        self.session.add(ledger)
        await self.session.commit()
        await self.session.refresh(ledger)
        return ledger

    async def _allocated_usage(self, user_id: UUID, period: BillingPeriod) -> int:
        statement = select(func.coalesce(func.sum(UsageLedger.quantity), 0)).where(
            UsageLedger.user_id == user_id,
            UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
            UsageLedger.period_start == period.start,
            UsageLedger.period_end == period.end,
            UsageLedger.status.in_([UsageStatus.RESERVED, UsageStatus.CONSUMED]),
        )
        result = await self.session.exec(statement)
        return int(result.one())


class MessageRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_by_external_id(self, channel: IMChannel, external_message_id: str) -> Message | None:
        statement = select(Message).where(
            Message.channel == channel,
            Message.external_message_id == external_message_id,
        )
        result = await self.session.exec(statement)
        return result.first()

    async def get(self, message_id: UUID) -> Message | None:
        return await self.session.get(Message, message_id)

    async def create_incoming(self, inbound: InboundMessage) -> Message:
        message = Message(
            owner_user_id=inbound.owner_user_id,
            channel=inbound.channel,
            external_message_id=inbound.external_message_id,
            conversation_id=inbound.conversation_id,
            sender_external_id=inbound.sender_external_id,
            sender_display_name=inbound.sender_display_name,
            direction=MessageDirection.INCOMING,
            text=inbound.text,
            source_type=inbound.source_type,
            group_name=inbound.group_name,
            raw_message_links=inbound.raw_message_links,
            raw_payload=inbound.raw_payload,
        )
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def mark_agent_queued(self, message_id: UUID, *, force: bool = False) -> Message | None:
        message = await self.session.get(Message, message_id)
        if not message or message.agent_analysis_status == AgentAnalysisStatus.RUNNING:
            return message
        if message.agent_analysis_status == AgentAnalysisStatus.COMPLETED and not force:
            return message
        message.agent_analysis_status = AgentAnalysisStatus.QUEUED
        message.agent_error = None
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def mark_agent_quota_exceeded(self, message_id: UUID) -> Message | None:
        message = await self.session.get(Message, message_id)
        if not message:
            return None
        now = utc_now()
        message.agent_analysis_status = AgentAnalysisStatus.QUOTA_EXCEEDED
        message.agent_error = "monthly pi agent analysis quota exceeded"
        message.updated_at = now
        self.session.add(message)
        if message.opportunity_id:
            opportunity = await self.session.get(Opportunity, message.opportunity_id)
            if opportunity:
                opportunity.agent_analysis_status = AgentAnalysisStatus.QUOTA_EXCEEDED
                opportunity.agent_analysis_error = message.agent_error
                opportunity.updated_at = now
                self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def claim_agent_analysis(self, message_id: UUID, *, force: bool = False) -> Message | None:
        message = await self.session.get(Message, message_id, with_for_update=True)
        if not message:
            return None
        if (
            message.agent_analysis_status == AgentAnalysisStatus.RUNNING
            and message.agent_started_at
            and message.agent_started_at > utc_now() - timedelta(minutes=10)
        ):
            return None
        if message.agent_analysis_status == AgentAnalysisStatus.COMPLETED and not force:
            return None
        now = utc_now()
        message.agent_analysis_status = AgentAnalysisStatus.RUNNING
        message.agent_started_at = now
        message.agent_error = None
        message.updated_at = now
        self.session.add(message)
        if message.opportunity_id:
            opportunity = await self.session.get(Opportunity, message.opportunity_id)
            if opportunity:
                opportunity.agent_analysis_status = AgentAnalysisStatus.RUNNING
                opportunity.agent_analysis_error = None
                opportunity.sop_stage = "analyzing"
                opportunity.updated_at = now
                self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def complete_agent_analysis(
        self,
        message: Message,
        projection: AgentAnalysisProjection,
    ) -> Message:
        message.agent_analysis_status = AgentAnalysisStatus.COMPLETED
        message.agent_result = projection.model_dump(mode="json")
        message.agent_error = None
        message.agent_analyzed_at = projection.analyzed_at
        message.updated_at = projection.analyzed_at
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def fail_agent_analysis(self, message_id: UUID, error: str) -> None:
        # Recover from provider or database exceptions before writing the durable failure state.
        await self.session.rollback()
        message = await self.session.get(Message, message_id)
        if not message:
            return
        now = utc_now()
        safe_error = error[:1000]
        message.agent_analysis_status = AgentAnalysisStatus.FAILED
        message.agent_error = safe_error
        message.updated_at = now
        self.session.add(message)
        if message.opportunity_id:
            opportunity = await self.session.get(Opportunity, message.opportunity_id)
            if opportunity:
                opportunity.agent_analysis_status = AgentAnalysisStatus.FAILED
                opportunity.agent_analysis_error = safe_error
                opportunity.updated_at = now
                self.session.add(opportunity)
        await self.session.commit()

    async def create_outgoing(
        self,
        *,
        channel: IMChannel,
        conversation_id: str,
        text: str,
        source: MessageSource,
        opportunity_id: UUID,
        external_message_id: str,
        raw_payload: dict,
        owner_user_id: UUID | None = None,
    ) -> Message:
        message = Message(
            owner_user_id=owner_user_id,
            channel=channel,
            external_message_id=external_message_id,
            conversation_id=conversation_id,
            sender_display_name="商机助手",
            direction=MessageDirection.OUTGOING,
            source=source,
            text=text,
            raw_payload=raw_payload,
            opportunity_id=opportunity_id,
        )
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def attach_opportunity(self, message_id: UUID, opportunity_id: UUID) -> None:
        message = await self.session.get(Message, message_id)
        if not message:
            return
        message.opportunity_id = opportunity_id
        message.processed_at = utc_now()
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()

    async def mark_processed(self, message_id: UUID) -> None:
        message = await self.session.get(Message, message_id)
        if not message:
            return
        message.processed_at = utc_now()
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()

    async def list_by_opportunity(self, opportunity_id: UUID) -> list[Message]:
        statement = (
            select(Message)
            .where(Message.opportunity_id == opportunity_id)
            .order_by(col(Message.sent_at).asc(), col(Message.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def list_by_conversation(
        self,
        channel: IMChannel,
        conversation_id: str,
        limit: int = 20,
    ) -> list[Message]:
        statement = (
            select(Message)
            .where(Message.channel == channel, Message.conversation_id == conversation_id)
            .order_by(col(Message.sent_at).desc())
            .limit(limit)
        )
        result = await self.session.exec(statement)
        return list(reversed(result.all()))


class OpportunityRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create(
        self,
        *,
        channel: IMChannel,
        owner_user_id: UUID | None,
        conversation_id: str,
        customer_external_id: str | None,
        contact_name: str | None,
        source_type: str,
        group_name: str | None,
        source_message_id: UUID,
        title: str,
        summary: str | None,
        matched_keywords: list[str],
        raw_message_links: list[str],
        confidence: float,
        priority: Priority,
        detection_reason: str | None,
        status: OpportunityStatus,
        last_message_preview: str,
    ) -> Opportunity:
        opportunity = Opportunity(
            owner_user_id=owner_user_id,
            channel=channel,
            conversation_id=conversation_id,
            customer_external_id=customer_external_id,
            contact_name=contact_name or customer_external_id or "未知联系人",
            source_type=source_type,
            group_name=group_name,
            source_message_id=source_message_id,
            title=title,
            summary=summary,
            matched_keywords=matched_keywords,
            raw_message_links=raw_message_links,
            trust_score=80 if not raw_message_links else 65,
            confidence=confidence,
            priority=priority,
            detection_reason=detection_reason,
            status=status,
            last_message_preview=last_message_preview,
            friend_request_status="not_sent" if source_type == "group" else "n/a",
        )
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def get(self, opportunity_id: UUID) -> Opportunity | None:
        return await self.session.get(Opportunity, opportunity_id)

    async def get_by_source_message(self, message_id: UUID) -> Opportunity | None:
        statement = select(Opportunity).where(Opportunity.source_message_id == message_id)
        result = await self.session.exec(statement)
        return result.first()

    async def apply_agent_projection(
        self,
        opportunity: Opportunity,
        projection: AgentAnalysisProjection,
    ) -> Opportunity:
        priority_order = {
            Priority.LOW: 0,
            Priority.NORMAL: 1,
            Priority.HIGH: 2,
            Priority.URGENT: 3,
        }
        opportunity.link_verification = projection.link_verification
        opportunity.extracted_contacts = projection.extracted_contacts
        opportunity.agent_actions = projection.actions
        opportunity.agent_analysis_status = AgentAnalysisStatus.COMPLETED
        opportunity.agent_analysis_error = None
        opportunity.agent_analyzed_at = projection.analyzed_at
        opportunity.attention_required = projection.attention_required
        opportunity.trust_score = projection.result.trust_score
        opportunity.confidence = max(opportunity.confidence, projection.result.confidence)
        if priority_order[projection.result.priority] > priority_order[opportunity.priority]:
            opportunity.priority = projection.result.priority

        link_status = projection.link_verification.get("status")
        has_contacts = any(
            projection.extracted_contacts.get(key)
            for key in ("phone", "email", "telegramHandle", "wecomId")
        )
        if link_status in {"suspicious", "malicious"}:
            opportunity.sop_stage = "analyzing"
        elif has_contacts:
            opportunity.sop_stage = (
                "contact_extracted" if opportunity.source_type == "group" else "ready_to_chat"
            )
        elif opportunity.raw_message_links:
            opportunity.sop_stage = "verified"
        opportunity.updated_at = projection.analyzed_at
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def list(
        self,
        *,
        frontend_status: FrontendOpportunityStatus | None = None,
        channel: IMChannel | None = None,
        owner_user_id: UUID | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[Opportunity]:
        statement = select(Opportunity)
        if frontend_status:
            statement = statement.where(Opportunity.status.in_(FRONTEND_STATUS_MAP[frontend_status]))
        if channel:
            statement = statement.where(Opportunity.channel == channel)
        if owner_user_id:
            statement = statement.where(Opportunity.owner_user_id == owner_user_id)
        statement = statement.order_by(col(Opportunity.last_message_at).desc()).offset(offset).limit(limit)
        result = await self.session.exec(statement)
        return list(result.all())

    async def update_status(
        self,
        opportunity: Opportunity,
        status: OpportunityStatus,
        *,
        final_reply: str | None = None,
        assigned_to: str | None = None,
    ) -> Opportunity:
        opportunity.status = status
        if final_reply is not None:
            opportunity.final_reply = final_reply
            opportunity.last_message_preview = final_reply
            opportunity.last_message_at = utc_now()
        if assigned_to is not None:
            opportunity.assigned_to = assigned_to
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def save_ai_draft(self, opportunity: Opportunity, draft: str) -> Opportunity:
        opportunity.ai_reply_draft = draft
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def pending_human_older_than(self, minutes: int) -> list[Opportunity]:
        cutoff = datetime.now(timezone.utc) - timedelta(minutes=minutes)
        statement = select(Opportunity).where(
            Opportunity.status == OpportunityStatus.PENDING_HUMAN,
            Opportunity.created_at <= cutoff,
        )
        result = await self.session.exec(statement)
        return list(result.all())


class RuleRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def enabled_detection_rules(self) -> list[DetectionRule]:
        statement = select(Rule).where(Rule.enabled.is_(True)).order_by(col(Rule.priority).asc())
        result = await self.session.exec(statement)
        return [
            DetectionRule(
                id=rule.id,
                name=rule.name,
                rule_type=rule.rule_type,
                pattern=rule.pattern,
                score=rule.score,
                priority=rule.priority,
            )
            for rule in result.all()
        ]

    async def list(self) -> list[Rule]:
        result = await self.session.exec(select(Rule).order_by(col(Rule.priority).asc()))
        return list(result.all())

    async def create(self, rule: Rule) -> Rule:
        self.session.add(rule)
        await self.session.commit()
        await self.session.refresh(rule)
        return rule

    async def get(self, rule_id: UUID) -> Rule | None:
        return await self.session.get(Rule, rule_id)

    async def save(self, rule: Rule) -> Rule:
        rule.updated_at = utc_now()
        self.session.add(rule)
        await self.session.commit()
        await self.session.refresh(rule)
        return rule

    async def delete(self, rule: Rule) -> None:
        await self.session.delete(rule)
        await self.session.commit()


class ConfigRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_value(self, key: str) -> dict | None:
        config = await self.session.get(AppConfig, key)
        return config.value if config else None

    async def set_value(self, key: str, value: dict, description: str | None = None) -> AppConfig:
        config = await self.session.get(AppConfig, key)
        if config:
            config.value = value
            config.description = description or config.description
            config.updated_at = utc_now()
        else:
            config = AppConfig(key=key, value=value, description=description)
        self.session.add(config)
        await self.session.commit()
        await self.session.refresh(config)
        return config


class TelegramUserConfigRepository:
    MONITOR_QUOTA_REASON = "paused because the current subscription group quota was exceeded"

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_by_user(self, user_id: UUID) -> TelegramUserConfig | None:
        statement = select(TelegramUserConfig).where(TelegramUserConfig.user_id == user_id)
        result = await self.session.exec(statement)
        return result.first()

    async def list_monitors_by_user(self, user_id: UUID) -> list[TelegramMonitor]:
        statement = (
            select(TelegramMonitor)
            .where(TelegramMonitor.user_id == user_id)
            .order_by(col(TelegramMonitor.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def count_active_monitors_by_user(self, user_id: UUID) -> int:
        statement = (
            select(func.count())
            .select_from(TelegramMonitor)
            .join(TelegramUserConfig, TelegramMonitor.telegram_config_id == TelegramUserConfig.id)
            .where(
                TelegramMonitor.user_id == user_id,
                TelegramUserConfig.enabled.is_(True),
                TelegramMonitor.enabled.is_(True),
                TelegramMonitor.quota_paused.is_(False),
            )
        )
        result = await self.session.exec(statement)
        return int(result.one())

    async def list_enabled_user_ids(self) -> list[UUID]:
        statement = (
            select(TelegramMonitor.user_id)
            .join(TelegramUserConfig, TelegramMonitor.telegram_config_id == TelegramUserConfig.id)
            .where(TelegramMonitor.enabled.is_(True))
            .where(TelegramUserConfig.enabled.is_(True))
            .distinct()
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def reconcile_monitor_quota_for_user(
        self,
        *,
        user_id: UUID,
        capacity: int,
    ) -> list[tuple[TelegramUserConfig, TelegramMonitor]]:
        user = await self.session.get(User, user_id, with_for_update=True)
        if not user or not user.is_active:
            await self.session.rollback()
            return []
        statement = (
            select(TelegramUserConfig, TelegramMonitor)
            .join(TelegramMonitor, TelegramMonitor.telegram_config_id == TelegramUserConfig.id)
            .where(
                TelegramUserConfig.user_id == user_id,
                TelegramUserConfig.enabled.is_(True),
                TelegramMonitor.enabled.is_(True),
            )
            .order_by(
                col(TelegramMonitor.retention_priority).desc(),
                col(TelegramMonitor.quota_paused).asc(),
                col(TelegramMonitor.created_at).asc(),
            )
        )
        result = await self.session.exec(statement)
        rows = [(row[0], row[1]) for row in result.all()]
        active: list[tuple[TelegramUserConfig, TelegramMonitor]] = []
        now = utc_now()
        for index, (config, monitor) in enumerate(rows):
            should_pause = index >= capacity
            if monitor.quota_paused != should_pause:
                monitor.quota_paused = should_pause
                monitor.updated_at = now
            monitor.quota_reason = self.MONITOR_QUOTA_REASON if should_pause else None
            self.session.add(monitor)
            if not should_pause:
                active.append((config, monitor))
        await self.session.commit()
        return active

    async def select_retained_monitors(
        self,
        *,
        user_id: UUID,
        monitor_ids: list[UUID],
        capacity: int,
    ) -> list[TelegramMonitor]:
        user = await self.session.get(User, user_id, with_for_update=True)
        if not user or not user.is_active:
            raise ValueError("active user is required")
        config = await self.get_by_user(user_id)
        if not config:
            raise ValueError("Telegram account is not configured")
        monitors = await self.list_monitors_by_user(user_id)
        enabled_monitors = [monitor for monitor in monitors if monitor.enabled]
        selected_ids = set(monitor_ids)
        enabled_ids = {monitor.id for monitor in enabled_monitors}
        if selected_ids - enabled_ids:
            raise ValueError("retained monitors must belong to the current user and be enabled")
        if len(selected_ids) > capacity:
            raise ValueError(f"current plan allows {capacity} retained Telegram monitors")
        if len(enabled_monitors) <= capacity:
            raise ValueError("monitor retention selection is not required for the current plan")
        if len(selected_ids) != capacity:
            raise ValueError(f"select exactly {capacity} Telegram monitors to retain")

        now = utc_now()
        for monitor in enabled_monitors:
            retained = monitor.id in selected_ids
            monitor.quota_paused = not retained
            monitor.quota_reason = None if retained else self.MONITOR_QUOTA_REASON
            monitor.retention_priority = 100 if retained else 0
            monitor.updated_at = now
            self.session.add(monitor)
        config.retention_limit = capacity
        config.retention_selected_at = now
        config.updated_at = now
        self.session.add(config)
        await self.session.commit()
        for monitor in monitors:
            await self.session.refresh(monitor)
        return monitors

    async def save_account_for_user(
        self,
        *,
        user_id: UUID,
        api_id: int | None,
        api_hash_encrypted: str | None = None,
        session_encrypted: str | None = None,
    ) -> TelegramUserConfig:
        config = await self.get_by_user(user_id)
        if not config:
            config = TelegramUserConfig(user_id=user_id)

        config.api_id = api_id
        if api_hash_encrypted is not None:
            config.api_hash_encrypted = api_hash_encrypted
        if session_encrypted is not None:
            config.session_encrypted = session_encrypted
        config.updated_at = utc_now()

        self.session.add(config)
        await self.session.commit()
        await self.session.refresh(config)
        return config

    async def replace_monitors_for_user(
        self,
        *,
        user_id: UUID,
        telegram_config_id: UUID,
        chats: list[str | int],
        enabled: bool,
        backfill_limit: int,
        entitlements: PlanEntitlements,
        enabled_wecom_groups: int = 0,
    ) -> list[TelegramMonitor]:
        user = await self.session.get(User, user_id, with_for_update=True)
        if not user or not user.is_active:
            raise ValueError("active user is required")
        existing = {monitor.chat_id: monitor for monitor in await self.list_monitors_by_user(user_id)}
        desired_chat_ids = list(dict.fromkeys(str(chat) for chat in chats))
        current_active_chat_ids = {
            monitor.chat_id
            for monitor in existing.values()
            if monitor.enabled and not monitor.quota_paused
        }
        active_selection_changed = set(desired_chat_ids) != current_active_chat_ids
        ensure_group_quota(
            entitlements=entitlements,
            telegram_groups=len(desired_chat_ids) if enabled else 0,
            wecom_groups=enabled_wecom_groups,
        )
        config = await self.session.get(TelegramUserConfig, telegram_config_id)
        if config:
            config.enabled = enabled
            if not enabled or active_selection_changed:
                config.retention_limit = None
                config.retention_selected_at = None
            config.updated_at = utc_now()
            self.session.add(config)

        if active_selection_changed or not enabled:
            for monitor in existing.values():
                monitor.retention_priority = 0
                self.session.add(monitor)

        for chat_id, monitor in list(existing.items()):
            if not enabled:
                monitor.enabled = False
                monitor.quota_paused = False
                monitor.quota_reason = None
                monitor.updated_at = utc_now()
                self.session.add(monitor)
            elif chat_id not in desired_chat_ids and not monitor.quota_paused:
                await self.session.delete(monitor)

        monitors: list[TelegramMonitor] = []
        for chat in desired_chat_ids:
            monitor = existing.get(chat)
            if not monitor:
                monitor = TelegramMonitor(
                    user_id=user_id,
                    telegram_config_id=telegram_config_id,
                    chat_id=chat,
                )
            monitor.enabled = enabled
            monitor.quota_paused = False
            monitor.quota_reason = None
            monitor.backfill_limit = backfill_limit
            monitor.updated_at = utc_now()
            self.session.add(monitor)
            monitors.append(monitor)

        await self.session.commit()
        for monitor in monitors:
            await self.session.refresh(monitor)
        return monitors

    async def record_monitor_error(self, monitor_id: UUID, error: str | None) -> None:
        monitor = await self.session.get(TelegramMonitor, monitor_id)
        if not monitor:
            return
        monitor.last_error = error[:1000] if error else None
        monitor.updated_at = utc_now()
        self.session.add(monitor)
        await self.session.commit()


class TelegramConnectionRepository:
    """Persistence boundary for the new user-owned Telegram connection model."""

    SOURCE_QUOTA_REASON = "paused because the current subscription group quota was exceeded"

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create_attempt(
        self,
        *,
        owner_user_id: UUID,
        connection_type: TelegramConnectionType,
        token_hash: str,
        expires_at: datetime,
        group_request_id: int | None = None,
        channel_request_id: int | None = None,
    ) -> TelegramConnectionAttempt:
        attempt = TelegramConnectionAttempt(
            owner_user_id=owner_user_id,
            connection_type=connection_type,
            token_hash=token_hash,
            expires_at=expires_at,
            group_request_id=group_request_id,
            channel_request_id=channel_request_id,
        )
        self.session.add(attempt)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            raise
        await self.session.refresh(attempt)
        return attempt

    async def get_attempt_for_owner(
        self,
        *,
        owner_user_id: UUID,
        attempt_id: UUID,
    ) -> TelegramConnectionAttempt | None:
        statement = select(TelegramConnectionAttempt).where(
            TelegramConnectionAttempt.id == attempt_id,
            TelegramConnectionAttempt.owner_user_id == owner_user_id,
        )
        result = await self.session.exec(statement)
        attempt = result.first()
        if attempt:
            await self._expire_attempt_if_needed(attempt)
        return attempt

    async def get_attempt_by_token_hash(self, token_hash: str) -> TelegramConnectionAttempt | None:
        statement = select(TelegramConnectionAttempt).where(
            TelegramConnectionAttempt.token_hash == token_hash
        )
        result = await self.session.exec(statement)
        attempt = result.first()
        if attempt:
            await self._expire_attempt_if_needed(attempt)
        return attempt

    async def cancel_attempt_for_owner(
        self,
        *,
        owner_user_id: UUID,
        attempt_id: UUID,
    ) -> TelegramConnectionAttempt | None:
        attempt = await self.get_attempt_for_owner(
            owner_user_id=owner_user_id,
            attempt_id=attempt_id,
        )
        if not attempt:
            return None
        if attempt.status == TelegramConnectionAttemptStatus.PENDING:
            attempt.status = TelegramConnectionAttemptStatus.CANCELLED
            attempt.updated_at = utc_now()
            self.session.add(attempt)
            await self.session.commit()
            await self.session.refresh(attempt)
        return attempt

    async def fail_attempt(
        self,
        *,
        attempt: TelegramConnectionAttempt,
        error: str,
    ) -> TelegramConnectionAttempt:
        if attempt.status == TelegramConnectionAttemptStatus.PENDING:
            attempt.status = TelegramConnectionAttemptStatus.FAILED
            attempt.error = error[:1000]
            attempt.updated_at = utc_now()
            self.session.add(attempt)
            await self.session.commit()
            await self.session.refresh(attempt)
        return attempt

    async def get_attempt_by_request_id(self, request_id: int) -> TelegramConnectionAttempt | None:
        statement = select(TelegramConnectionAttempt).where(
            (TelegramConnectionAttempt.group_request_id == request_id)
            | (TelegramConnectionAttempt.channel_request_id == request_id)
        )
        result = await self.session.exec(statement)
        attempt = result.first()
        if attempt:
            await self._expire_attempt_if_needed(attempt)
        return attempt

    async def bind_attempt_telegram_account(
        self,
        *,
        attempt: TelegramConnectionAttempt,
        telegram_account_id: str,
    ) -> TelegramConnectionAttempt:
        if attempt.status != TelegramConnectionAttemptStatus.PENDING:
            raise ValueError("connection attempt is no longer active")
        if attempt.telegram_account_id and attempt.telegram_account_id != telegram_account_id:
            raise ValueError("connection attempt belongs to a different Telegram account")
        attempt.telegram_account_id = telegram_account_id
        attempt.updated_at = utc_now()
        self.session.add(attempt)
        await self.session.commit()
        await self.session.refresh(attempt)
        return attempt

    async def list_connections_for_owner(self, owner_user_id: UUID) -> list[TelegramConnection]:
        statement = (
            select(TelegramConnection)
            .where(TelegramConnection.owner_user_id == owner_user_id)
            .order_by(col(TelegramConnection.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def get_connection_for_owner(
        self,
        *,
        owner_user_id: UUID,
        connection_id: UUID,
    ) -> TelegramConnection | None:
        statement = select(TelegramConnection).where(
            TelegramConnection.id == connection_id,
            TelegramConnection.owner_user_id == owner_user_id,
        )
        result = await self.session.exec(statement)
        return result.first()

    async def list_sources_for_owner(self, owner_user_id: UUID) -> list[TelegramSource]:
        statement = (
            select(TelegramSource)
            .where(TelegramSource.owner_user_id == owner_user_id)
            .order_by(col(TelegramSource.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def list_active_sources_for_chat(
        self,
        external_chat_id: str,
    ) -> list[tuple[TelegramSource, TelegramConnection]]:
        statement = (
            select(TelegramSource, TelegramConnection)
            .join(TelegramConnection, TelegramSource.connection_id == TelegramConnection.id)
            .where(
                TelegramSource.external_chat_id == external_chat_id,
                TelegramSource.enabled.is_(True),
                TelegramSource.quota_paused.is_(False),
                TelegramConnection.enabled.is_(True),
                TelegramConnection.status == TelegramConnectionStatus.CONNECTED,
            )
        )
        result = await self.session.exec(statement)
        return [(row[0], row[1]) for row in result.all()]

    async def list_enabled_sources_for_chat(
        self,
        external_chat_id: str,
    ) -> list[tuple[TelegramSource, TelegramConnection]]:
        """Return candidate owners before quota reconciliation for a webhook delivery."""
        statement = (
            select(TelegramSource, TelegramConnection)
            .join(TelegramConnection, TelegramSource.connection_id == TelegramConnection.id)
            .where(
                TelegramSource.external_chat_id == external_chat_id,
                TelegramSource.enabled.is_(True),
                TelegramConnection.enabled.is_(True),
                TelegramConnection.status == TelegramConnectionStatus.CONNECTED,
            )
        )
        result = await self.session.exec(statement)
        return [(row[0], row[1]) for row in result.all()]

    async def count_active_sources_by_user(self, owner_user_id: UUID) -> int:
        statement = (
            select(func.count())
            .select_from(TelegramSource)
            .join(TelegramConnection, TelegramSource.connection_id == TelegramConnection.id)
            .where(
                TelegramSource.owner_user_id == owner_user_id,
                TelegramSource.enabled.is_(True),
                TelegramSource.quota_paused.is_(False),
                TelegramConnection.enabled.is_(True),
                TelegramConnection.status == TelegramConnectionStatus.CONNECTED,
            )
        )
        result = await self.session.exec(statement)
        return int(result.one())

    async def reconcile_source_quota_for_user(
        self,
        *,
        owner_user_id: UUID,
        capacity: int,
        legacy_active_count: int,
    ) -> list[TelegramSource]:
        user = await self.session.get(User, owner_user_id, with_for_update=True)
        if not user or not user.is_active:
            await self.session.rollback()
            return []
        statement = (
            select(TelegramSource)
            .join(TelegramConnection, TelegramSource.connection_id == TelegramConnection.id)
            .where(
                TelegramSource.owner_user_id == owner_user_id,
                TelegramSource.enabled.is_(True),
                TelegramConnection.enabled.is_(True),
            )
            .order_by(
                col(TelegramSource.retention_priority).desc(),
                col(TelegramSource.quota_paused).asc(),
                col(TelegramSource.created_at).asc(),
            )
        )
        result = await self.session.exec(statement)
        sources = list(result.all())
        available = max(capacity - legacy_active_count, 0)
        now = utc_now()
        for index, source in enumerate(sources):
            paused = index >= available
            source.quota_paused = paused
            source.quota_reason = self.SOURCE_QUOTA_REASON if paused else None
            source.updated_at = now
            self.session.add(source)
        await self.session.commit()
        return sources

    async def complete_bot_chat(
        self,
        *,
        attempt: TelegramConnectionAttempt,
        external_chat_id: str,
        source_type: TelegramSourceType,
        display_name: str,
        username: str | None,
        entitlements: PlanEntitlements,
        legacy_active_count: int,
    ) -> tuple[TelegramConnection, TelegramSource]:
        user = await self.session.get(User, attempt.owner_user_id, with_for_update=True)
        if not user or not user.is_active:
            raise ValueError("active user is required")
        if attempt.status != TelegramConnectionAttemptStatus.PENDING:
            raise ValueError("connection attempt is no longer active")
        if attempt.connection_type != TelegramConnectionType.BOT_CHAT:
            raise ValueError("connection attempt is not a bot chat request")

        existing_connection_statement = (
            select(TelegramConnection)
            .where(
                TelegramConnection.owner_user_id == attempt.owner_user_id,
                TelegramConnection.connection_type == TelegramConnectionType.BOT_CHAT,
            )
            .order_by(col(TelegramConnection.created_at).asc())
        )
        existing_connection_result = await self.session.exec(existing_connection_statement)
        connection = existing_connection_result.first()
        if not connection:
            connection = TelegramConnection(
                owner_user_id=attempt.owner_user_id,
                connection_type=TelegramConnectionType.BOT_CHAT,
                status=TelegramConnectionStatus.CONNECTED,
                label="Telegram 群组与频道",
                capabilities={"receive_group_messages": True, "receive_channel_posts": True},
            )
            self.session.add(connection)
            await self.session.flush()
        else:
            connection.status = TelegramConnectionStatus.CONNECTED
            connection.enabled = True
            connection.last_error = None
            connection.last_checked_at = utc_now()
            connection.updated_at = utc_now()
            self.session.add(connection)

        source_statement = select(TelegramSource).where(
            TelegramSource.connection_id == connection.id,
            TelegramSource.external_chat_id == external_chat_id,
        )
        source_result = await self.session.exec(source_statement)
        source = source_result.first()
        if not source:
            current_sources = await self.count_active_sources_by_user(attempt.owner_user_id)
            ensure_group_quota(
                entitlements=entitlements,
                telegram_groups=legacy_active_count + current_sources + 1,
                wecom_groups=0,
            )
            source = TelegramSource(
                owner_user_id=attempt.owner_user_id,
                connection_id=connection.id,
                source_type=source_type,
                external_chat_id=external_chat_id,
                display_name=display_name,
                username=username,
            )
        else:
            source.source_type = source_type
            source.display_name = display_name
            source.username = username
            source.enabled = True
            source.quota_paused = False
            source.quota_reason = None
            source.last_error = None
            source.updated_at = utc_now()
        self.session.add(source)

        now = utc_now()
        attempt.status = TelegramConnectionAttemptStatus.COMPLETED
        attempt.connection_id = connection.id
        attempt.completed_at = now
        attempt.updated_at = now
        self.session.add(attempt)
        await self.session.commit()
        await self.session.refresh(connection)
        await self.session.refresh(source)
        return connection, source

    async def complete_business_connection(
        self,
        *,
        telegram_account_id: str,
        provider_connection_id: str,
        is_enabled: bool,
        capabilities: dict,
    ) -> TelegramConnection | None:
        now = utc_now()
        attempt_statement = (
            select(TelegramConnectionAttempt)
            .where(
                TelegramConnectionAttempt.connection_type == TelegramConnectionType.BUSINESS,
                TelegramConnectionAttempt.telegram_account_id == telegram_account_id,
                TelegramConnectionAttempt.status == TelegramConnectionAttemptStatus.PENDING,
                TelegramConnectionAttempt.expires_at > now,
            )
            .order_by(col(TelegramConnectionAttempt.created_at).desc())
        )
        result = await self.session.exec(attempt_statement)
        attempt = result.first()
        if not attempt:
            return None
        user = await self.session.get(User, attempt.owner_user_id, with_for_update=True)
        if not user or not user.is_active:
            raise ValueError("active user is required")
        connection = await self.get_connection_by_provider_connection_id(provider_connection_id)
        if connection and connection.owner_user_id != attempt.owner_user_id:
            raise ValueError("business connection is already owned by another user")
        if not connection:
            connection = TelegramConnection(
                owner_user_id=attempt.owner_user_id,
                connection_type=TelegramConnectionType.BUSINESS,
                provider_connection_id=provider_connection_id,
                telegram_account_id=telegram_account_id,
                label="Telegram Business 私聊",
            )
        connection.status = (
            TelegramConnectionStatus.CONNECTED if is_enabled else TelegramConnectionStatus.DISABLED
        )
        connection.enabled = is_enabled
        connection.capabilities = capabilities
        connection.last_checked_at = now
        connection.last_error = None
        connection.updated_at = now
        self.session.add(connection)
        await self.session.flush()

        attempt.status = TelegramConnectionAttemptStatus.COMPLETED
        attempt.connection_id = connection.id
        attempt.completed_at = now
        attempt.updated_at = now
        self.session.add(attempt)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def get_connection_by_provider_connection_id(
        self,
        provider_connection_id: str,
    ) -> TelegramConnection | None:
        statement = select(TelegramConnection).where(
            TelegramConnection.provider_connection_id == provider_connection_id
        )
        result = await self.session.exec(statement)
        return result.first()

    async def set_business_connection_state(
        self,
        *,
        provider_connection_id: str,
        enabled: bool,
        capabilities: dict,
    ) -> TelegramConnection | None:
        connection = await self.get_connection_by_provider_connection_id(provider_connection_id)
        if not connection:
            return None
        connection.enabled = enabled
        connection.status = (
            TelegramConnectionStatus.CONNECTED if enabled else TelegramConnectionStatus.DISABLED
        )
        connection.capabilities = capabilities
        connection.last_checked_at = utc_now()
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def set_connection_enabled(
        self,
        *,
        owner_user_id: UUID,
        connection_id: UUID,
        enabled: bool,
    ) -> TelegramConnection | None:
        connection = await self.get_connection_for_owner(
            owner_user_id=owner_user_id,
            connection_id=connection_id,
        )
        if not connection:
            return None
        connection.enabled = enabled
        connection.status = (
            TelegramConnectionStatus.CONNECTED if enabled else TelegramConnectionStatus.DISABLED
        )
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def delete_connection(
        self,
        *,
        owner_user_id: UUID,
        connection_id: UUID,
    ) -> bool:
        connection = await self.get_connection_for_owner(
            owner_user_id=owner_user_id,
            connection_id=connection_id,
        )
        if not connection:
            return False
        sources = await self.session.exec(
            select(TelegramSource).where(TelegramSource.connection_id == connection.id)
        )
        for source in sources.all():
            await self.session.delete(source)
        await self.session.delete(connection)
        await self.session.commit()
        return True

    async def delete_source(
        self,
        *,
        owner_user_id: UUID,
        source_id: UUID,
    ) -> bool:
        statement = select(TelegramSource).where(
            TelegramSource.id == source_id,
            TelegramSource.owner_user_id == owner_user_id,
        )
        result = await self.session.exec(statement)
        source = result.first()
        if not source:
            return False
        await self.session.delete(source)
        await self.session.commit()
        return True

    async def reserve_webhook_event(
        self,
        *,
        update_id: int,
        payload_hash: str,
        event_type: str,
    ) -> tuple[TelegramWebhookEvent, bool]:
        event = TelegramWebhookEvent(
            update_id=update_id,
            payload_hash=payload_hash,
            event_type=event_type,
        )
        self.session.add(event)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            statement = select(TelegramWebhookEvent).where(TelegramWebhookEvent.update_id == update_id)
            result = await self.session.exec(statement)
            existing = result.first()
            if not existing:
                raise
            # A failed delivery leaves processed_at unset so Telegram can retry the same update.
            return existing, existing.processed_at is None
        await self.session.refresh(event)
        return event, True

    async def finish_webhook_event(
        self,
        *,
        event: TelegramWebhookEvent,
        connection_id: UUID | None = None,
        error: str | None = None,
    ) -> None:
        event.connection_id = connection_id or event.connection_id
        event.error = error[:1000] if error else None
        event.processed_at = utc_now()
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def _expire_attempt_if_needed(self, attempt: TelegramConnectionAttempt) -> None:
        if (
            attempt.status == TelegramConnectionAttemptStatus.PENDING
            and attempt.expires_at <= utc_now()
        ):
            attempt.status = TelegramConnectionAttemptStatus.EXPIRED
            attempt.updated_at = utc_now()
            self.session.add(attempt)
            await self.session.commit()
            await self.session.refresh(attempt)


class ReplyTemplateRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def list(self, enabled_only: bool = True) -> list[ReplyTemplate]:
        statement = select(ReplyTemplate).order_by(col(ReplyTemplate.created_at).desc())
        if enabled_only:
            statement = statement.where(ReplyTemplate.enabled.is_(True))
        result = await self.session.exec(statement)
        return list(result.all())

    async def create(self, template: ReplyTemplate) -> ReplyTemplate:
        self.session.add(template)
        await self.session.commit()
        await self.session.refresh(template)
        return template

    async def get(self, template_id: UUID) -> ReplyTemplate | None:
        return await self.session.get(ReplyTemplate, template_id)

    async def save(self, template: ReplyTemplate) -> ReplyTemplate:
        template.updated_at = utc_now()
        self.session.add(template)
        await self.session.commit()
        await self.session.refresh(template)
        return template
