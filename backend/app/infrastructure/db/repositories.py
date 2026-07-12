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
    BillingEventStatus,
    BillingProvider,
    BillingInterval,
    BillingStore,
    BillingSubscriptionStatus,
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
    BillingSubscription,
    BillingEvent,
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
    billing_period_start: datetime | None
    billing_period_end: datetime | None
    usage_period_start: datetime
    usage_period_end: datetime
    entitlements: PlanEntitlements
    cancel_at_period_end: bool
    effective_store: BillingStore | None
    billing_interval: BillingInterval | None
    entitlement_expires_at: datetime | None
    will_renew: bool
    billing_issue: bool
    multiple_active_subscriptions: bool
    last_synced_at: datetime | None
    management_url_encrypted: str | None

    @property
    def billing_period(self) -> BillingPeriod | None:
        if self.billing_period_start is None or self.billing_period_end is None:
            return None
        return BillingPeriod(start=self.billing_period_start, end=self.billing_period_end)

    @property
    def usage_period(self) -> BillingPeriod:
        return BillingPeriod(start=self.usage_period_start, end=self.usage_period_end)

    @property
    def period(self) -> BillingPeriod:
        """Backward-compatible alias; usage accounting must always use the UTC month."""
        return self.usage_period


@dataclass(frozen=True, slots=True)
class UsageReservation:
    allowed: bool
    created: bool
    ledger: UsageLedger | None
    limit: int
    allocated: int


@dataclass(frozen=True, slots=True)
class BillingSubscriptionWrite:
    external_key: str
    store: BillingStore
    environment: str
    external_product_id: str
    external_transaction_id: str | None
    revenuecat_entitlement_id: str | None
    plan_code: PlanCode
    billing_interval: BillingInterval
    status: BillingSubscriptionStatus
    current_period_start: datetime | None
    current_period_end: datetime | None
    grace_period_end: datetime | None
    will_renew: bool
    cancel_at_period_end: bool
    billing_issue_detected_at: datetime | None
    last_synced_at: datetime
    metadata: dict


@dataclass(frozen=True, slots=True)
class BillingEventReservation:
    event: BillingEvent
    should_enqueue: bool
    duplicate: bool


class BillingEventRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def reserve_revenuecat_event(
        self,
        *,
        provider_event_id: str,
        event_type: str,
        app_user_ids: list[UUID],
        environment: str | None,
        payload_hash: str,
    ) -> BillingEventReservation:
        joined_user_ids = ",".join(str(item) for item in app_user_ids) or None
        event = BillingEvent(
            provider=BillingProvider.REVENUECAT,
            provider_event_id=provider_event_id,
            event_type=event_type[:128],
            app_user_id=joined_user_ids,
            environment=environment[:32] if environment else None,
            payload_hash=payload_hash,
            status=BillingEventStatus.QUEUED,
            queued_at=utc_now(),
        )
        self.session.add(event)
        try:
            await self.session.commit()
            await self.session.refresh(event)
            return BillingEventReservation(event=event, should_enqueue=True, duplicate=False)
        except IntegrityError:
            await self.session.rollback()
        result = await self.session.exec(
            select(BillingEvent).where(
                BillingEvent.provider == BillingProvider.REVENUECAT,
                BillingEvent.provider_event_id == provider_event_id,
            )
        )
        existing = result.first()
        if not existing:
            raise RuntimeError("billing event reservation conflict could not be recovered")
        if existing.payload_hash != payload_hash:
            raise ValueError("provider event ID was reused with a different payload")
        if existing.status == BillingEventStatus.FAILED:
            existing.status = BillingEventStatus.QUEUED
            existing.queued_at = utc_now()
            existing.processing_error = None
            existing.updated_at = utc_now()
            self.session.add(existing)
            await self.session.commit()
            await self.session.refresh(existing)
            return BillingEventReservation(event=existing, should_enqueue=True, duplicate=True)
        return BillingEventReservation(event=existing, should_enqueue=False, duplicate=True)

    async def begin_processing(self, event_id: UUID) -> BillingEvent | None:
        event = await self.session.get(BillingEvent, event_id, with_for_update=True)
        if not event or event.status in {
            BillingEventStatus.COMPLETED,
            BillingEventStatus.ORPHANED,
            BillingEventStatus.PROCESSING,
        }:
            await self.session.rollback()
            return None
        event.status = BillingEventStatus.PROCESSING
        event.attempt_count += 1
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()
        await self.session.refresh(event)
        return event

    async def finish(self, event_id: UUID, *, orphaned: bool = False) -> None:
        event = await self.session.get(BillingEvent, event_id)
        if not event:
            return
        event.status = BillingEventStatus.ORPHANED if orphaned else BillingEventStatus.COMPLETED
        event.processed_at = utc_now()
        event.processing_error = None
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def fail(self, event_id: UUID, error: str) -> None:
        event = await self.session.get(BillingEvent, event_id)
        if not event:
            return
        event.status = BillingEventStatus.FAILED
        event.processing_error = error[:1000]
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def reconciliation_user_ids(self, *, limit: int, now: datetime | None = None) -> list[UUID]:
        current = now or utc_now()
        statement = (
            select(SubscriptionAccount.user_id)
            .where(
                SubscriptionAccount.billing_provider == BillingProvider.REVENUECAT.value,
                (
                    SubscriptionAccount.status.in_([SubscriptionStatus.ACTIVE, SubscriptionStatus.TRIALING])
                    | (SubscriptionAccount.entitlement_expires_at <= current + timedelta(hours=48))
                    | SubscriptionAccount.billing_issue.is_(True)
                ),
            )
            .order_by(col(SubscriptionAccount.last_synced_at).asc().nullsfirst())
            .limit(limit)
        )
        result = await self.session.exec(statement)
        user_ids = list(result.all())
        failed_result = await self.session.exec(
            select(BillingEvent.app_user_id).where(
                BillingEvent.status == BillingEventStatus.FAILED,
                BillingEvent.received_at >= current - timedelta(hours=48),
                BillingEvent.app_user_id.is_not(None),
            )
        )
        seen = set(user_ids)
        for joined in failed_result.all():
            for raw_user_id in joined.split(",") if joined else []:
                try:
                    user_id = UUID(raw_user_id)
                except ValueError:
                    continue
                if user_id not in seen and len(user_ids) < limit:
                    user_ids.append(user_id)
                    seen.add(user_id)
        return user_ids


class SubscriptionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_account(self, user_id: UUID) -> SubscriptionAccount | None:
        statement = select(SubscriptionAccount).where(SubscriptionAccount.user_id == user_id)
        result = await self.session.exec(statement)
        return result.first()

    async def project_revenuecat_customer(
        self,
        *,
        user_id: UUID,
        subscriptions: list[BillingSubscriptionWrite],
        effective_plan: PlanCode,
        effective_store: BillingStore | None,
        billing_interval: BillingInterval | None,
        entitlement_started_at: datetime | None,
        entitlement_expires_at: datetime | None,
        billing_period_start: datetime | None,
        billing_period_end: datetime | None,
        will_renew: bool,
        cancel_at_period_end: bool,
        billing_issue: bool,
        multiple_active_subscriptions: bool,
        last_synced_at: datetime,
        management_url_encrypted: str | None,
    ) -> SubscriptionAccount:
        existing_result = await self.session.exec(
            select(BillingSubscription).where(
                BillingSubscription.user_id == user_id,
                BillingSubscription.provider == BillingProvider.REVENUECAT,
            )
        )
        existing = {item.external_key: item for item in existing_result.all()}
        current_keys: set[str] = set()
        for item in subscriptions:
            current_keys.add(item.external_key)
            record = existing.get(item.external_key)
            if not record:
                record = BillingSubscription(
                    user_id=user_id,
                    provider=BillingProvider.REVENUECAT,
                    store=item.store,
                    environment=item.environment,
                    external_key=item.external_key,
                    external_product_id=item.external_product_id,
                    plan_code=item.plan_code,
                )
            record.store = item.store
            record.environment = item.environment
            record.external_product_id = item.external_product_id
            record.external_transaction_id = item.external_transaction_id
            record.revenuecat_entitlement_id = item.revenuecat_entitlement_id
            record.plan_code = item.plan_code
            record.billing_interval = item.billing_interval
            record.status = item.status
            record.current_period_start = item.current_period_start
            record.current_period_end = item.current_period_end
            record.grace_period_end = item.grace_period_end
            record.will_renew = item.will_renew
            record.cancel_at_period_end = item.cancel_at_period_end
            record.billing_issue_detected_at = item.billing_issue_detected_at
            record.last_synced_at = item.last_synced_at
            record.metadata_json = item.metadata
            record.updated_at = last_synced_at
            self.session.add(record)

        for external_key, record in existing.items():
            if external_key not in current_keys:
                record.status = BillingSubscriptionStatus.INACTIVE
                record.will_renew = False
                record.last_synced_at = last_synced_at
                record.updated_at = last_synced_at
                self.session.add(record)

        account = await self.get_account(user_id)
        if not account:
            account = SubscriptionAccount(user_id=user_id)
        account.plan_code = effective_plan
        account.status = SubscriptionStatus.ACTIVE if effective_plan != PlanCode.FREE else SubscriptionStatus.INACTIVE
        account.billing_provider = BillingProvider.REVENUECAT.value
        account.effective_store = effective_store
        account.billing_interval = billing_interval
        account.entitlement_started_at = entitlement_started_at
        account.entitlement_expires_at = entitlement_expires_at
        account.current_period_start = billing_period_start
        account.current_period_end = billing_period_end
        account.will_renew = will_renew
        account.cancel_at_period_end = cancel_at_period_end
        account.billing_issue = billing_issue
        account.multiple_active_subscriptions = multiple_active_subscriptions
        account.last_synced_at = last_synced_at
        account.management_url_encrypted = management_url_encrypted
        account.updated_at = last_synced_at
        self.session.add(account)
        await self.session.commit()
        await self.session.refresh(account)
        return account

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
        usage_period = utc_calendar_month(current)
        return SubscriptionSnapshot(
            plan_code=plan_code,
            subscription_status=(account.status if account else SubscriptionStatus.INACTIVE),
            billing_period_start=(account.current_period_start if account else None),
            billing_period_end=(account.current_period_end if account else None),
            usage_period_start=usage_period.start,
            usage_period_end=usage_period.end,
            entitlements=get_plan_entitlements(plan_code),
            cancel_at_period_end=bool(account and account.cancel_at_period_end),
            effective_store=(account.effective_store if account else None),
            billing_interval=(account.billing_interval if account else None),
            entitlement_expires_at=(account.entitlement_expires_at if account else None),
            will_renew=bool(account and account.will_renew),
            billing_issue=bool(account and account.billing_issue),
            multiple_active_subscriptions=bool(account and account.multiple_active_subscriptions),
            last_synced_at=(account.last_synced_at if account else None),
            management_url_encrypted=(account.management_url_encrypted if account else None),
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
        allocated = await self._allocated_usage(user_id, snapshot.usage_period)
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
            period_start=snapshot.usage_period_start,
            period_end=snapshot.usage_period_end,
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

    async def get_pending_attempt_for_owner(
        self,
        *,
        owner_user_id: UUID,
        connection_type: TelegramConnectionType,
    ) -> TelegramConnectionAttempt | None:
        statement = (
            select(TelegramConnectionAttempt)
            .where(
                TelegramConnectionAttempt.owner_user_id == owner_user_id,
                TelegramConnectionAttempt.connection_type == connection_type,
                TelegramConnectionAttempt.status == TelegramConnectionAttemptStatus.PENDING,
            )
            .order_by(col(TelegramConnectionAttempt.created_at).desc())
        )
        result = await self.session.exec(statement)
        for attempt in result.all():
            await self._expire_attempt_if_needed(attempt)
            if attempt.status == TelegramConnectionAttemptStatus.PENDING:
                return attempt
        return None

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

    async def get_attempt(self, attempt_id: UUID) -> TelegramConnectionAttempt | None:
        attempt = await self.session.get(TelegramConnectionAttempt, attempt_id)
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

    async def list_pending_mtproto_qr_attempts(self) -> list[TelegramConnectionAttempt]:
        statement = select(TelegramConnectionAttempt).where(
            TelegramConnectionAttempt.connection_type == TelegramConnectionType.MTPROTO_QR,
            TelegramConnectionAttempt.status == TelegramConnectionAttemptStatus.PENDING,
            TelegramConnectionAttempt.expires_at > utc_now(),
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def set_qr_url_encrypted(
        self, *, attempt_id: UUID, qr_url_encrypted: str
    ) -> TelegramConnectionAttempt | None:
        attempt = await self.session.get(TelegramConnectionAttempt, attempt_id)
        if not attempt or attempt.status != TelegramConnectionAttemptStatus.PENDING:
            return None
        attempt.qr_url_encrypted = qr_url_encrypted
        attempt.updated_at = utc_now()
        self.session.add(attempt)
        await self.session.commit()
        await self.session.refresh(attempt)
        return attempt

    async def complete_mtproto_qr_attempt(
        self,
        *,
        attempt_id: UUID,
        telegram_account_id: str,
        label: str,
        credential_encrypted: str,
    ) -> TelegramConnection | None:
        attempt = await self.session.get(TelegramConnectionAttempt, attempt_id, with_for_update=True)
        if not attempt or attempt.status != TelegramConnectionAttemptStatus.PENDING:
            return None
        existing_result = await self.session.exec(
            select(TelegramConnection).where(
                TelegramConnection.owner_user_id == attempt.owner_user_id,
                TelegramConnection.connection_type == TelegramConnectionType.MTPROTO_QR,
            )
        )
        connection = existing_result.first()
        now = utc_now()
        if not connection:
            connection = TelegramConnection(
                owner_user_id=attempt.owner_user_id,
                connection_type=TelegramConnectionType.MTPROTO_QR,
                label=label,
            )
        connection.telegram_account_id = telegram_account_id
        connection.credential_encrypted = credential_encrypted
        connection.capabilities = {"receive_group_messages": True, "receive_channel_posts": True}
        connection.status = TelegramConnectionStatus.CONNECTED
        connection.enabled = True
        connection.last_error = None
        connection.last_checked_at = now
        connection.updated_at = now
        self.session.add(connection)
        await self.session.flush()
        attempt.status = TelegramConnectionAttemptStatus.COMPLETED
        attempt.connection_id = connection.id
        attempt.qr_url_encrypted = None
        attempt.completed_at = now
        attempt.updated_at = now
        self.session.add(attempt)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def add_mtproto_source(
        self,
        *,
        owner_user_id: UUID,
        connection_id: UUID,
        external_chat_id: str,
        source_type: TelegramSourceType,
        display_name: str,
        username: str | None,
        entitlements: PlanEntitlements,
        legacy_active_count: int,
    ) -> TelegramSource:
        connection = await self.get_connection_for_owner(
            owner_user_id=owner_user_id, connection_id=connection_id
        )
        if not connection or connection.connection_type != TelegramConnectionType.MTPROTO_QR:
            raise ValueError("MTProto connection not found")
        result = await self.session.exec(
            select(TelegramSource).where(
                TelegramSource.connection_id == connection_id,
                TelegramSource.external_chat_id == external_chat_id,
            )
        )
        source = result.first()
        if not source:
            ensure_group_quota(
                entitlements=entitlements,
                telegram_groups=legacy_active_count + await self.count_active_sources_by_user(owner_user_id) + 1,
                wecom_groups=0,
            )
            source = TelegramSource(
                owner_user_id=owner_user_id,
                connection_id=connection_id,
                external_chat_id=external_chat_id,
                source_type=source_type,
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
        # The MTProto listener keys its running task by the connection revision.
        # Touch the parent whenever its selected chats change so the task reloads.
        connection.updated_at = utc_now()
        self.session.add(connection)
        self.session.add(source)
        await self.session.commit()
        await self.session.refresh(source)
        return source

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

    async def list_listenable_mtproto_connections(
        self,
    ) -> list[tuple[TelegramConnection, list[TelegramSource]]]:
        result = await self.session.exec(
            select(TelegramConnection, TelegramSource)
            .join(TelegramSource, TelegramSource.connection_id == TelegramConnection.id)
            .where(
                TelegramConnection.connection_type == TelegramConnectionType.MTPROTO_QR,
                TelegramConnection.status == TelegramConnectionStatus.CONNECTED,
                TelegramConnection.enabled.is_(True),
                TelegramConnection.credential_encrypted.is_not(None),
                TelegramSource.enabled.is_(True),
                TelegramSource.quota_paused.is_(False),
            )
        )
        grouped: dict[UUID, tuple[TelegramConnection, list[TelegramSource]]] = {}
        for connection, source in result.all():
            grouped.setdefault(connection.id, (connection, []))[1].append(source)
        return list(grouped.values())

    async def record_connection_error(self, connection_id: UUID, error: str | None) -> None:
        connection = await self.session.get(TelegramConnection, connection_id)
        if not connection:
            return
        connection.last_error = error[:1000] if error else None
        connection.last_checked_at = utc_now()
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()

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
