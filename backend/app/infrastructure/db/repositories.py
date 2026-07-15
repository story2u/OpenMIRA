from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from uuid import UUID

import structlog
from sqlalchemy import String, cast, func, update
from sqlalchemy.dialects.postgresql import ARRAY
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from sqlmodel import col, select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    AgentAnalysisStatus,
    AutoReplyDecisionReason,
    AutoReplyDeliveryStatus,
    BillingEventStatus,
    BillingInterval,
    BillingProvider,
    BillingStore,
    BillingSubscriptionStatus,
    FrontendOpportunityStatus,
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityArchiveAction,
    OpportunityArchiveScope,
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
    WeComConnectionStatus,
    WeComDeliveryStatus,
    WeComEventStatus,
    WeComReceiveCapability,
    WeComSendCapability,
    WeComSourceType,
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
from app.domain.services.opportunity_state import InvalidOpportunityTransition
from app.infrastructure.db.models import (
    AppConfig,
    AuthAccount,
    AutoReplyDelivery,
    BillingEvent,
    BillingSubscription,
    Message,
    Opportunity,
    OpportunityArchiveEvent,
    PasswordResetChallenge,
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
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
    WeComConnection,
    WeComArchiveConnection,
    WeComArchiveCursor,
    WeComArchiveEvent,
    WeComArchiveMemberBinding,
    WeComOutboundDelivery,
    WeComSource,
    WeComWebhookEvent,
    utc_now,
)

logger = structlog.get_logger(__name__)


async def _count_active_wecom_group_sources(session: AsyncSession, owner_user_id: UUID) -> int:
    result = await session.exec(
        select(func.count())
        .select_from(WeComSource)
        .where(
            WeComSource.owner_user_id == owner_user_id,
            WeComSource.enabled.is_(True),
            WeComSource.quota_paused.is_(False),
            WeComSource.source_type.in_(
                [WeComSourceType.INTERNAL_GROUP, WeComSourceType.EXTERNAL_GROUP]
            ),
        )
    )
    return int(result.one())


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


class PasswordResetRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create(
        self,
        *,
        user_id: UUID,
        token_digest: str,
        code_digest: str,
        expires_at: datetime,
    ) -> PasswordResetChallenge:
        now = utc_now()
        await self.session.exec(
            update(PasswordResetChallenge)
            .where(
                PasswordResetChallenge.user_id == user_id,
                PasswordResetChallenge.used_at.is_(None),
            )
            .values(used_at=now, updated_at=now)
        )
        challenge = PasswordResetChallenge(
            user_id=user_id,
            token_digest=token_digest,
            code_digest=code_digest,
            expires_at=expires_at,
        )
        self.session.add(challenge)
        await self.session.commit()
        await self.session.refresh(challenge)
        return challenge

    async def active_by_token(self, token_digest: str) -> PasswordResetChallenge | None:
        result = await self.session.exec(
            select(PasswordResetChallenge)
            .where(
                PasswordResetChallenge.token_digest == token_digest,
                PasswordResetChallenge.used_at.is_(None),
                PasswordResetChallenge.expires_at > utc_now(),
            )
            .with_for_update()
        )
        return result.first()

    async def latest_active_for_email(
        self, email: str
    ) -> tuple[PasswordResetChallenge, User] | None:
        result = await self.session.exec(
            select(PasswordResetChallenge, User)
            .join(User, User.id == PasswordResetChallenge.user_id)
            .where(
                User.email == email.lower().strip(),
                User.is_active.is_(True),
                PasswordResetChallenge.used_at.is_(None),
                PasswordResetChallenge.expires_at > utc_now(),
            )
            .order_by(col(PasswordResetChallenge.created_at).desc())
            .with_for_update()
        )
        return result.first()

    async def user_for_challenge(self, challenge: PasswordResetChallenge) -> User | None:
        return await self.session.get(User, challenge.user_id)

    async def register_failed_attempt(
        self, challenge: PasswordResetChallenge, *, max_attempts: int
    ) -> None:
        now = utc_now()
        challenge.failed_attempts += 1
        challenge.updated_at = now
        if challenge.failed_attempts >= max_attempts:
            challenge.used_at = now
        self.session.add(challenge)
        await self.session.commit()

    async def invalidate(self, challenge: PasswordResetChallenge) -> None:
        challenge.used_at = utc_now()
        challenge.updated_at = utc_now()
        self.session.add(challenge)
        await self.session.commit()

    async def replace_password(
        self,
        *,
        user: User,
        password_hash: str,
        challenge: PasswordResetChallenge | None = None,
    ) -> User:
        now = utc_now()
        user.password_hash = password_hash
        user.auth_version += 1
        user.updated_at = now
        self.session.add(user)
        if challenge:
            await self.session.exec(
                update(PasswordResetChallenge)
                .where(
                    PasswordResetChallenge.user_id == user.id,
                    PasswordResetChallenge.used_at.is_(None),
                )
                .values(used_at=now, updated_at=now)
            )
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
            # End the read transaction without expiring objects returned by an earlier
            # successful transition on this session.
            await self.session.commit()
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

    async def reconciliation_user_ids(
        self, *, limit: int, now: datetime | None = None
    ) -> list[UUID]:
        current = now or utc_now()
        statement = (
            select(SubscriptionAccount.user_id)
            .where(
                SubscriptionAccount.billing_provider == BillingProvider.REVENUECAT.value,
                (
                    SubscriptionAccount.status.in_(
                        [SubscriptionStatus.ACTIVE, SubscriptionStatus.TRIALING]
                    )
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
        account.status = (
            SubscriptionStatus.ACTIVE
            if effective_plan != PlanCode.FREE
            else SubscriptionStatus.INACTIVE
        )
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

    async def get_by_external_id(
        self, channel: IMChannel, external_message_id: str
    ) -> Message | None:
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

    async def claim_agent_analysis(
        self, message_id: UUID, *, force: bool = False
    ) -> Message | None:
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
        opportunity_id: UUID | None,
        external_message_id: str,
        raw_payload: dict,
        owner_user_id: UUID | None = None,
        sender_display_name: str = "商机助手",
    ) -> Message:
        message = Message(
            owner_user_id=owner_user_id,
            channel=channel,
            external_message_id=external_message_id,
            conversation_id=conversation_id,
            sender_display_name=sender_display_name,
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
        owner_user_id: UUID | None,
        limit: int = 20,
    ) -> list[Message]:
        statement = (
            select(Message)
            .where(
                Message.channel == channel,
                Message.conversation_id == conversation_id,
                Message.owner_user_id == owner_user_id,
            )
            .order_by(col(Message.sent_at).desc())
            .limit(limit)
        )
        result = await self.session.exec(statement)
        return list(reversed(result.all()))


class AutoReplyDeliveryRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def reserve(
        self,
        *,
        owner_user_id: UUID,
        opportunity_id: UUID,
        source_message_id: UUID,
        channel: IMChannel,
        conversation_id: str,
        idempotency_key: str,
    ) -> tuple[AutoReplyDelivery, bool]:
        existing = await self._get_by_key(owner_user_id, idempotency_key)
        if existing:
            return existing, False
        delivery = AutoReplyDelivery(
            owner_user_id=owner_user_id,
            opportunity_id=opportunity_id,
            source_message_id=source_message_id,
            channel=channel,
            conversation_id=conversation_id,
            idempotency_key=idempotency_key,
        )
        self.session.add(delivery)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            existing = await self._get_by_key(owner_user_id, idempotency_key)
            if existing:
                return existing, False
            raise
        await self.session.refresh(delivery)
        return delivery, True

    async def claim_candidate(self, delivery_id: UUID) -> AutoReplyDelivery | None:
        delivery = await self.session.get(AutoReplyDelivery, delivery_id, with_for_update=True)
        if not delivery or delivery.status != AutoReplyDeliveryStatus.CANDIDATE:
            return None
        now = utc_now()
        delivery.status = AutoReplyDeliveryStatus.GENERATING
        delivery.attempt_count += 1
        delivery.updated_at = now
        self.session.add(delivery)
        await self.session.commit()
        await self.session.refresh(delivery)
        return delivery

    async def mark_blocked(
        self,
        delivery: AutoReplyDelivery,
        reason: AutoReplyDecisionReason,
    ) -> AutoReplyDelivery:
        return await self._mark(
            delivery,
            status=AutoReplyDeliveryStatus.BLOCKED,
            reason=reason,
        )

    async def mark_ready(
        self,
        delivery: AutoReplyDelivery,
        *,
        content_hash: str,
    ) -> AutoReplyDelivery:
        return await self._mark(
            delivery,
            status=AutoReplyDeliveryStatus.READY,
            reason=AutoReplyDecisionReason.ELIGIBLE,
            content_hash=content_hash,
            ready_at=utc_now(),
        )

    async def mark_sending(self, delivery: AutoReplyDelivery) -> AutoReplyDelivery:
        current = await self.session.get(AutoReplyDelivery, delivery.id, with_for_update=True)
        if not current or current.status != AutoReplyDeliveryStatus.READY:
            raise ValueError("auto reply delivery is not ready")
        return await self._mark(
            current,
            status=AutoReplyDeliveryStatus.SENDING,
            reason=AutoReplyDecisionReason.ELIGIBLE,
            sending_at=utc_now(),
        )

    async def claim_ready_for_send(
        self, delivery: AutoReplyDelivery
    ) -> tuple[AutoReplyDelivery, Opportunity] | None:
        current = await self.session.get(AutoReplyDelivery, delivery.id, with_for_update=True)
        if not current or current.status != AutoReplyDeliveryStatus.READY:
            return None
        opportunity = await self.session.get(
            Opportunity,
            current.opportunity_id,
            with_for_update=True,
        )
        if (
            not opportunity
            or opportunity.status != OpportunityStatus.AI_AUTO_REPLY
            or opportunity.archived_at is not None
            or opportunity.assigned_to
        ):
            return None
        now = utc_now()
        opportunity.assigned_to = "ai:auto_reply"
        opportunity.updated_at = now
        current.status = AutoReplyDeliveryStatus.SENDING
        current.decision_reason = AutoReplyDecisionReason.ELIGIBLE
        current.sending_at = now
        current.updated_at = now
        self.session.add(opportunity)
        self.session.add(current)
        await self.session.commit()
        await self.session.refresh(current)
        await self.session.refresh(opportunity)
        return current, opportunity

    async def mark_sent(
        self,
        delivery: AutoReplyDelivery,
        *,
        provider_message_id: str,
    ) -> AutoReplyDelivery:
        return await self._mark(
            delivery,
            status=AutoReplyDeliveryStatus.SENT,
            reason=AutoReplyDecisionReason.ELIGIBLE,
            provider_message_id=provider_message_id,
            sent_at=utc_now(),
        )

    async def mark_dry_run(self, delivery: AutoReplyDelivery) -> AutoReplyDelivery:
        return await self._mark(
            delivery,
            status=AutoReplyDeliveryStatus.DRY_RUN,
            reason=AutoReplyDecisionReason.DELIVERY_DRY_RUN,
        )

    async def mark_failed(self, delivery: AutoReplyDelivery, error: str) -> AutoReplyDelivery:
        await self.session.rollback()
        current = await self.session.get(AutoReplyDelivery, delivery.id)
        if not current:
            return delivery
        return await self._mark(
            current,
            status=AutoReplyDeliveryStatus.FAILED,
            reason=AutoReplyDecisionReason.PROVIDER_ERROR,
            error=error[:500],
        )

    async def mark_sending_uncertain(
        self, delivery: AutoReplyDelivery, error: str
    ) -> AutoReplyDelivery:
        await self.session.rollback()
        current = await self.session.get(AutoReplyDelivery, delivery.id)
        if not current:
            return delivery
        return await self._mark(
            current,
            status=AutoReplyDeliveryStatus.SENDING,
            reason=AutoReplyDecisionReason.PROVIDER_ERROR,
            error=error[:500],
        )

    async def reload_after_rollback(self, delivery_id: UUID) -> AutoReplyDelivery | None:
        await self.session.rollback()
        return await self.session.get(AutoReplyDelivery, delivery_id)

    async def count_sent_since(
        self,
        *,
        owner_user_id: UUID,
        channel: IMChannel,
        conversation_id: str,
        since: datetime,
    ) -> int:
        result = await self.session.exec(
            select(func.count())
            .select_from(AutoReplyDelivery)
            .where(
                AutoReplyDelivery.owner_user_id == owner_user_id,
                AutoReplyDelivery.channel == channel,
                AutoReplyDelivery.conversation_id == conversation_id,
                AutoReplyDelivery.status == AutoReplyDeliveryStatus.SENT,
                AutoReplyDelivery.sent_at >= since,
            )
        )
        return int(result.one())

    async def _get_by_key(
        self, owner_user_id: UUID, idempotency_key: str
    ) -> AutoReplyDelivery | None:
        result = await self.session.exec(
            select(AutoReplyDelivery).where(
                AutoReplyDelivery.owner_user_id == owner_user_id,
                AutoReplyDelivery.idempotency_key == idempotency_key,
            )
        )
        return result.first()

    async def _mark(
        self,
        delivery: AutoReplyDelivery,
        *,
        status: AutoReplyDeliveryStatus,
        reason: AutoReplyDecisionReason,
        content_hash: str | None = None,
        provider_message_id: str | None = None,
        ready_at: datetime | None = None,
        sending_at: datetime | None = None,
        sent_at: datetime | None = None,
        error: str | None = None,
    ) -> AutoReplyDelivery:
        delivery.status = status
        delivery.decision_reason = reason
        if content_hash is not None:
            delivery.content_hash = content_hash
        if provider_message_id is not None:
            delivery.provider_message_id = provider_message_id
        if ready_at is not None:
            delivery.ready_at = ready_at
        if sending_at is not None:
            delivery.sending_at = sending_at
        if sent_at is not None:
            delivery.sent_at = sent_at
        delivery.error = error
        delivery.updated_at = utc_now()
        self.session.add(delivery)
        await self.session.commit()
        await self.session.refresh(delivery)
        return delivery


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
        archive_scope: OpportunityArchiveScope = OpportunityArchiveScope.ACTIVE,
        limit: int = 100,
        offset: int = 0,
    ) -> list[Opportunity]:
        statement = select(Opportunity)
        if frontend_status:
            statement = statement.where(
                Opportunity.status.in_(FRONTEND_STATUS_MAP[frontend_status])
            )
        if channel:
            statement = statement.where(Opportunity.channel == channel)
        if owner_user_id:
            statement = statement.where(Opportunity.owner_user_id == owner_user_id)
        if archive_scope == OpportunityArchiveScope.ACTIVE:
            statement = statement.where(Opportunity.archived_at.is_(None))
        elif archive_scope == OpportunityArchiveScope.ARCHIVED:
            statement = statement.where(Opportunity.archived_at.is_not(None))
        statement = (
            statement.order_by(col(Opportunity.last_message_at).desc()).offset(offset).limit(limit)
        )
        result = await self.session.exec(statement)
        return list(result.all())

    def _dashboard_filters(
        self,
        *,
        owner_user_id: UUID,
        frontend_status: FrontendOpportunityStatus | None,
        channel: IMChannel | None,
        source_type: str | None,
        created_from: datetime | None,
        created_to: datetime | None,
        trust_ranges: list[tuple[int, int]] | None,
        sop_stages: list[str] | None,
        keywords: list[str] | None,
    ) -> list:
        """把看板筛选翻译成 SQL 谓词；所有查询都在数据库层完成，绝不先取全量再内存过滤。"""
        from sqlalchemy import and_, or_

        clauses = [
            Opportunity.owner_user_id == owner_user_id,
            Opportunity.archived_at.is_(None),
        ]
        if frontend_status:
            clauses.append(Opportunity.status.in_(FRONTEND_STATUS_MAP[frontend_status]))
        if channel:
            clauses.append(Opportunity.channel == channel)
        if source_type:
            clauses.append(Opportunity.source_type == source_type)
        if created_from:
            clauses.append(Opportunity.created_at >= created_from)
        if created_to:
            clauses.append(Opportunity.created_at <= created_to)
        if trust_ranges:
            clauses.append(
                or_(
                    *[
                        and_(Opportunity.trust_score >= low, Opportunity.trust_score <= high)
                        for low, high in trust_ranges
                    ]
                )
            )
        if sop_stages:
            clauses.append(Opportunity.sop_stage.in_(sop_stages))
        if keywords:
            # matched_keywords 是 JSONB 数组，任一关键词命中即保留（?| 存在性运算符）。
            clauses.append(
                col(Opportunity.matched_keywords).op("?|")(cast(list(keywords), ARRAY(String)))
            )
        return clauses

    async def dashboard(
        self,
        *,
        owner_user_id: UUID,
        frontend_status: FrontendOpportunityStatus | None = None,
        channel: IMChannel | None = None,
        source_type: str | None = None,
        created_from: datetime | None = None,
        created_to: datetime | None = None,
        trust_ranges: list[tuple[int, int]] | None = None,
        sop_stages: list[str] | None = None,
        keywords: list[str] | None = None,
        sort: str = "newest",
        limit: int = 20,
        offset: int = 0,
    ) -> tuple[list[Opportunity], int]:
        """返回(当前页, 当前筛选下总数)。排序与分页均由数据库完成。"""
        clauses = self._dashboard_filters(
            owner_user_id=owner_user_id,
            frontend_status=frontend_status,
            channel=channel,
            source_type=source_type,
            created_from=created_from,
            created_to=created_to,
            trust_ranges=trust_ranges,
            sop_stages=sop_stages,
            keywords=keywords,
        )
        base = select(Opportunity)
        for clause in clauses:
            base = base.where(clause)

        order_map = {
            "newest": col(Opportunity.created_at).desc(),
            "oldest": col(Opportunity.created_at).asc(),
            "confidence": col(Opportunity.confidence).desc(),
            "trust": col(Opportunity.trust_score).desc(),
        }
        order_by = order_map.get(sort, order_map["newest"])
        # 次级排序稳定分页，避免同值行在翻页时错序。
        page_statement = (
            base.order_by(order_by, col(Opportunity.id).desc()).offset(offset).limit(limit)
        )
        page_result = await self.session.exec(page_statement)
        items = list(page_result.all())

        count_statement = select(func.count()).select_from(base.subquery())
        total = int((await self.session.exec(count_statement)).one())
        return items, total

    async def count_pending(self, owner_user_id: UUID) -> int:
        """该用户所有待处理商机数，不受当前筛选/分页影响。"""
        statement = (
            select(func.count())
            .select_from(Opportunity)
            .where(
                Opportunity.owner_user_id == owner_user_id,
                Opportunity.archived_at.is_(None),
                Opportunity.status.in_(FRONTEND_STATUS_MAP[FrontendOpportunityStatus.PENDING]),
            )
        )
        return int((await self.session.exec(statement)).one())

    async def list_attention(self, owner_user_id: UUID, *, limit: int = 20) -> list[Opportunity]:
        """待处理的重大商机（attention_required），最新优先。"""
        statement = (
            select(Opportunity)
            .where(
                Opportunity.owner_user_id == owner_user_id,
                Opportunity.archived_at.is_(None),
                Opportunity.attention_required.is_(True),
                Opportunity.status.in_(FRONTEND_STATUS_MAP[FrontendOpportunityStatus.PENDING]),
            )
            .order_by(col(Opportunity.created_at).desc())
            .limit(limit)
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def keyword_options(self, owner_user_id: UUID, *, limit: int = 200) -> list[str]:
        """当前用户真实商机里出现过的关键词并集（用于筛选面板选项）。"""
        statement = select(Opportunity.matched_keywords).where(
            Opportunity.owner_user_id == owner_user_id,
            Opportunity.archived_at.is_(None),
            func.jsonb_array_length(col(Opportunity.matched_keywords)) > 0,
        )
        result = await self.session.exec(statement)
        seen: dict[str, None] = {}
        for row in result.all():
            for keyword in row or []:
                if isinstance(keyword, str) and keyword not in seen:
                    seen[keyword] = None
                    if len(seen) >= limit:
                        return list(seen.keys())
        return list(seen.keys())

    async def archive(
        self,
        opportunity: Opportunity,
        *,
        actor_user_id: UUID,
        reason: str | None,
    ) -> Opportunity:
        if opportunity.archived_at is not None:
            return opportunity
        now = utc_now()
        opportunity.archived_at = now
        opportunity.archived_by_user_id = actor_user_id
        opportunity.archive_reason = reason
        opportunity.updated_at = now
        self.session.add(opportunity)
        self.session.add(
            OpportunityArchiveEvent(
                opportunity_id=opportunity.id,
                owner_user_id=actor_user_id,
                action=OpportunityArchiveAction.ARCHIVED,
                reason=reason,
            )
        )
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def restore(self, opportunity: Opportunity, *, actor_user_id: UUID) -> Opportunity:
        if opportunity.archived_at is None:
            return opportunity
        now = utc_now()
        opportunity.archived_at = None
        opportunity.archived_by_user_id = None
        opportunity.archive_reason = None
        opportunity.updated_at = now
        self.session.add(opportunity)
        self.session.add(
            OpportunityArchiveEvent(
                opportunity_id=opportunity.id,
                owner_user_id=actor_user_id,
                action=OpportunityArchiveAction.RESTORED,
            )
        )
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def archive_many(
        self,
        *,
        owner_user_id: UUID,
        opportunity_ids: list[UUID],
        reason: str | None,
    ) -> tuple[list[Opportunity], int]:
        statement = select(Opportunity).where(
            Opportunity.owner_user_id == owner_user_id,
            col(Opportunity.id).in_(opportunity_ids),
        )
        result = await self.session.exec(statement)
        opportunities = list(result.all())
        if len(opportunities) != len(opportunity_ids):
            raise LookupError("one or more opportunities were not found")

        now = utc_now()
        archived_count = 0
        for opportunity in opportunities:
            if opportunity.archived_at is not None:
                continue
            archived_count += 1
            opportunity.archived_at = now
            opportunity.archived_by_user_id = owner_user_id
            opportunity.archive_reason = reason
            opportunity.updated_at = now
            self.session.add(opportunity)
            self.session.add(
                OpportunityArchiveEvent(
                    opportunity_id=opportunity.id,
                    owner_user_id=owner_user_id,
                    action=OpportunityArchiveAction.ARCHIVED,
                    reason=reason,
                )
            )
        await self.session.commit()
        for opportunity in opportunities:
            await self.session.refresh(opportunity)
        return opportunities, archived_count

    async def update_status(
        self,
        opportunity: Opportunity,
        status: OpportunityStatus,
        *,
        final_reply: str | None = None,
        assigned_to: str | None = None,
        clear_assignment: bool = False,
    ) -> Opportunity:
        opportunity.status = status
        if final_reply is not None:
            opportunity.final_reply = final_reply
            opportunity.last_message_preview = final_reply
            opportunity.last_message_at = utc_now()
        if assigned_to is not None:
            opportunity.assigned_to = assigned_to
        elif clear_assignment:
            opportunity.assigned_to = None
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def claim_for_manual_reply(
        self,
        *,
        opportunity_id: UUID,
        operator_id: str,
    ) -> Opportunity:
        opportunity = await self.session.get(Opportunity, opportunity_id, with_for_update=True)
        if not opportunity:
            raise LookupError("opportunity not found")
        if (
            opportunity.assigned_to == "ai:auto_reply"
            and opportunity.status == OpportunityStatus.AI_AUTO_REPLY
        ):
            raise InvalidOpportunityTransition("automatic reply delivery is already in progress")
        opportunity.assigned_to = operator_id
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

    async def set_friend_request(self, opportunity: Opportunity, *, status: str) -> Opportunity:
        """持久化好友申请进度；pending/accepted 同步推进 SOP 阶段，其余不回退阶段。"""
        opportunity.friend_request_status = status
        if status == "pending":
            opportunity.sop_stage = "friend_requested"
        elif status == "accepted":
            opportunity.sop_stage = "ready_to_chat"
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def pending_human_older_than(self, minutes: int) -> list[Opportunity]:
        cutoff = datetime.now(timezone.utc) - timedelta(minutes=minutes)
        statement = select(Opportunity).where(
            Opportunity.status == OpportunityStatus.PENDING_HUMAN,
            Opportunity.archived_at.is_(None),
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


class UserSettingsRepository:
    """用户级设置持久化（detection / work-schedule / notifications）。owner 由登录用户确定。"""

    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    # detection ------------------------------------------------------------
    async def get_detection(self, user_id: UUID) -> UserDetectionPreference | None:
        statement = select(UserDetectionPreference).where(
            UserDetectionPreference.user_id == user_id
        )
        return (await self.session.exec(statement)).first()

    async def upsert_detection(
        self,
        *,
        user_id: UUID,
        keywords: list[str],
        ai_semantics_enabled: bool,
    ) -> UserDetectionPreference:
        pref = await self.get_detection(user_id)
        if not pref:
            pref = UserDetectionPreference(user_id=user_id)
        pref.keywords = keywords
        pref.ai_semantics_enabled = ai_semantics_enabled
        pref.updated_at = utc_now()
        self.session.add(pref)
        await self.session.commit()
        await self.session.refresh(pref)
        return pref

    # work schedule --------------------------------------------------------
    async def get_work_schedule(self, user_id: UUID) -> UserWorkSchedule | None:
        statement = select(UserWorkSchedule).where(UserWorkSchedule.user_id == user_id)
        return (await self.session.exec(statement)).first()

    async def upsert_work_schedule(
        self,
        *,
        user_id: UUID,
        timezone: str,
        slots: list[dict],
        auto_reply_outside_hours: bool,
    ) -> UserWorkSchedule:
        schedule = await self.get_work_schedule(user_id)
        if not schedule:
            schedule = UserWorkSchedule(user_id=user_id)
        schedule.timezone = timezone
        schedule.slots = slots
        schedule.auto_reply_outside_hours = auto_reply_outside_hours
        schedule.updated_at = utc_now()
        self.session.add(schedule)
        await self.session.commit()
        await self.session.refresh(schedule)
        return schedule

    # notifications --------------------------------------------------------
    async def get_notifications(self, user_id: UUID) -> UserNotificationPreference | None:
        statement = select(UserNotificationPreference).where(
            UserNotificationPreference.user_id == user_id
        )
        return (await self.session.exec(statement)).first()

    async def upsert_notifications(
        self,
        *,
        user_id: UUID,
        new_opportunity_enabled: bool,
        ai_replied_enabled: bool,
        daily_digest_enabled: bool,
        urgent_only: bool,
    ) -> UserNotificationPreference:
        pref = await self.get_notifications(user_id)
        if not pref:
            pref = UserNotificationPreference(user_id=user_id)
        pref.new_opportunity_enabled = new_opportunity_enabled
        pref.ai_replied_enabled = ai_replied_enabled
        pref.daily_digest_enabled = daily_digest_enabled
        pref.urgent_only = urgent_only
        pref.updated_at = utc_now()
        self.session.add(pref)
        await self.session.commit()
        await self.session.refresh(pref)
        return pref


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
        enabled_wecom_groups: int | None = None,
    ) -> list[TelegramMonitor]:
        user = await self.session.get(User, user_id, with_for_update=True)
        if not user or not user.is_active:
            raise ValueError("active user is required")
        existing = {
            monitor.chat_id: monitor for monitor in await self.list_monitors_by_user(user_id)
        }
        desired_chat_ids = list(dict.fromkeys(str(chat) for chat in chats))
        current_active_chat_ids = {
            monitor.chat_id
            for monitor in existing.values()
            if monitor.enabled and not monitor.quota_paused
        }
        active_selection_changed = set(desired_chat_ids) != current_active_chat_ids
        if enabled_wecom_groups is None:
            enabled_wecom_groups = await _count_active_wecom_group_sources(self.session, user_id)
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
        attempt = await self.session.get(
            TelegramConnectionAttempt, attempt_id, with_for_update=True
        )
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
            wecom_groups = await _count_active_wecom_group_sources(self.session, owner_user_id)
            ensure_group_quota(
                entitlements=entitlements,
                telegram_groups=legacy_active_count
                + await self.count_active_sources_by_user(owner_user_id)
                + 1,
                wecom_groups=wecom_groups,
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

    async def ensure_business_source(
        self,
        *,
        connection: TelegramConnection,
        external_chat_id: str,
        display_name: str,
    ) -> TelegramSource:
        if connection.connection_type != TelegramConnectionType.BUSINESS:
            raise ValueError("Telegram Business connection is required")
        result = await self.session.exec(
            select(TelegramSource).where(
                TelegramSource.connection_id == connection.id,
                TelegramSource.external_chat_id == external_chat_id,
            )
        )
        source = result.first()
        if not source:
            source = TelegramSource(
                owner_user_id=connection.owner_user_id,
                connection_id=connection.id,
                source_type=TelegramSourceType.PRIVATE,
                external_chat_id=external_chat_id,
                display_name=display_name[:255] or "Telegram Business 私聊",
                auto_reply_enabled=False,
            )
        else:
            source.display_name = display_name[:255] or source.display_name
            source.source_type = TelegramSourceType.PRIVATE
            source.updated_at = utc_now()
        self.session.add(source)
        await self.session.commit()
        await self.session.refresh(source)
        return source

    async def get_auto_reply_source(
        self,
        *,
        owner_user_id: UUID,
        conversation_id: str,
    ) -> tuple[TelegramSource, TelegramConnection] | None:
        result = await self.session.exec(
            select(TelegramSource, TelegramConnection)
            .join(TelegramConnection, TelegramConnection.id == TelegramSource.connection_id)
            .where(
                TelegramSource.owner_user_id == owner_user_id,
                TelegramSource.external_chat_id == conversation_id,
                TelegramConnection.owner_user_id == owner_user_id,
                TelegramConnection.connection_type == TelegramConnectionType.BUSINESS,
                TelegramSource.source_type == TelegramSourceType.PRIVATE,
            )
        )
        return result.first()

    async def set_source_auto_reply_enabled(
        self,
        *,
        owner_user_id: UUID,
        source_id: UUID,
        enabled: bool,
    ) -> tuple[TelegramSource, TelegramConnection] | None:
        result = await self.session.exec(
            select(TelegramSource, TelegramConnection)
            .join(TelegramConnection, TelegramConnection.id == TelegramSource.connection_id)
            .where(
                TelegramSource.id == source_id,
                TelegramSource.owner_user_id == owner_user_id,
                TelegramConnection.owner_user_id == owner_user_id,
            )
        )
        row = result.first()
        if not row:
            return None
        source, connection = row
        if enabled and (
            connection.connection_type != TelegramConnectionType.BUSINESS
            or source.source_type != TelegramSourceType.PRIVATE
            or connection.capabilities.get("can_reply") is not True
        ):
            raise ValueError(
                "automatic replies require a Telegram Business private chat with reply permission"
            )
        source.auto_reply_enabled = enabled
        source.updated_at = utc_now()
        self.session.add(source)
        await self.session.commit()
        await self.session.refresh(source)
        return source, connection

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
            wecom_groups = await _count_active_wecom_group_sources(
                self.session, attempt.owner_user_id
            )
            ensure_group_quota(
                entitlements=entitlements,
                telegram_groups=legacy_active_count + current_sources + 1,
                wecom_groups=wecom_groups,
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
            statement = select(TelegramWebhookEvent).where(
                TelegramWebhookEvent.update_id == update_id
            )
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


class WeComConnectionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def list_for_owner(self, owner_user_id: UUID) -> list[WeComConnection]:
        result = await self.session.exec(
            select(WeComConnection)
            .where(WeComConnection.owner_user_id == owner_user_id)
            .order_by(col(WeComConnection.created_at).desc())
        )
        return list(result.all())

    async def count_enabled_for_owner(self, owner_user_id: UUID) -> int:
        result = await self.session.exec(
            select(func.count())
            .select_from(WeComConnection)
            .where(
                WeComConnection.owner_user_id == owner_user_id,
                WeComConnection.enabled.is_(True),
            )
        )
        return int(result.one())

    async def get(self, connection_id: UUID) -> WeComConnection | None:
        return await self.session.get(WeComConnection, connection_id)

    async def get_for_owner(
        self,
        connection_id: UUID,
        owner_user_id: UUID,
    ) -> WeComConnection | None:
        result = await self.session.exec(
            select(WeComConnection).where(
                WeComConnection.id == connection_id,
                WeComConnection.owner_user_id == owner_user_id,
            )
        )
        return result.first()

    async def create(self, connection: WeComConnection) -> WeComConnection:
        self.session.add(connection)
        try:
            await self.session.commit()
        except IntegrityError as exc:
            await self.session.rollback()
            raise ValueError("wecom connection already exists") from exc
        await self.session.refresh(connection)
        return connection

    async def mark_verified(self, connection: WeComConnection) -> WeComConnection:
        connection.status = WeComConnectionStatus.ACTIVE
        connection.enabled = True
        connection.last_verified_at = utc_now()
        connection.last_error = None
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def mark_error(self, connection: WeComConnection, error: str) -> WeComConnection:
        connection.status = WeComConnectionStatus.ERROR
        connection.last_error = error[:1000]
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def disable_and_clear_secrets(
        self,
        connection: WeComConnection,
        *,
        cleared_secret: str,
    ) -> WeComConnection:
        connection.status = WeComConnectionStatus.DISABLED
        connection.enabled = False
        connection.secret_encrypted = cleared_secret
        connection.token_encrypted = cleared_secret
        connection.aes_key_encrypted = cleared_secret
        connection.last_error = None
        connection.updated_at = utc_now()
        self.session.add(connection)

        sources = await self.session.exec(
            select(WeComSource).where(WeComSource.connection_id == connection.id)
        )
        for source in sources.all():
            source.enabled = False
            source.updated_at = utc_now()
            self.session.add(source)

        events = await self.session.exec(
            select(WeComWebhookEvent).where(
                WeComWebhookEvent.connection_id == connection.id,
                WeComWebhookEvent.status.in_(
                    [WeComEventStatus.RECEIVED, WeComEventStatus.QUEUED, WeComEventStatus.FAILED]
                ),
            )
        )
        for event in events.all():
            event.normalized_payload_encrypted = None
            event.status = WeComEventStatus.FAILED
            event.processing_error = "connection disabled"
            event.updated_at = utc_now()
            self.session.add(event)

        await self.session.commit()
        await self.session.refresh(connection)
        return connection

    async def list_sources_for_owner(self, owner_user_id: UUID) -> list[WeComSource]:
        result = await self.session.exec(
            select(WeComSource)
            .where(WeComSource.owner_user_id == owner_user_id)
            .order_by(col(WeComSource.last_message_at).desc().nullslast())
        )
        return list(result.all())

    async def ensure_private_source(
        self,
        *,
        connection: WeComConnection,
        external_conversation_id: str,
        display_name: str,
    ) -> WeComSource:
        result = await self.session.exec(
            select(WeComSource).where(
                WeComSource.connection_id == connection.id,
                WeComSource.external_conversation_id == external_conversation_id,
            )
        )
        source = result.first()
        now = utc_now()
        if source:
            source.display_name = display_name[:255]
            source.last_message_at = now
            source.updated_at = now
        else:
            source = WeComSource(
                owner_user_id=connection.owner_user_id,
                connection_id=connection.id,
                external_conversation_id=external_conversation_id,
                display_name=display_name[:255],
                source_type=WeComSourceType.PRIVATE,
                receive_capability=WeComReceiveCapability.APP_CALLBACK,
                send_capability=WeComSendCapability.APP_MESSAGE,
                last_message_at=now,
            )
        self.session.add(source)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            result = await self.session.exec(
                select(WeComSource).where(
                    WeComSource.connection_id == connection.id,
                    WeComSource.external_conversation_id == external_conversation_id,
                )
            )
            source = result.one()
        await self.session.refresh(source)
        return source

    async def get_source_for_conversation(
        self,
        connection_id: UUID,
        external_conversation_id: str,
    ) -> WeComSource | None:
        result = await self.session.exec(
            select(WeComSource).where(
                WeComSource.connection_id == connection_id,
                WeComSource.external_conversation_id == external_conversation_id,
            )
        )
        return result.first()


class WeComEventRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def reserve(
        self,
        *,
        connection: WeComConnection,
        provider_event_id: str,
        event_type: str,
        payload_hash: str,
        normalized_payload_encrypted: str | None,
        ignored: bool = False,
    ) -> tuple[WeComWebhookEvent, bool]:
        event = WeComWebhookEvent(
            connection_id=connection.id,
            owner_user_id=connection.owner_user_id,
            provider_event_id=provider_event_id,
            event_type=event_type,
            payload_hash=payload_hash,
            normalized_payload_encrypted=normalized_payload_encrypted,
            status=WeComEventStatus.IGNORED if ignored else WeComEventStatus.RECEIVED,
            processed_at=utc_now() if ignored else None,
        )
        self.session.add(event)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            result = await self.session.exec(
                select(WeComWebhookEvent).where(
                    WeComWebhookEvent.connection_id == connection.id,
                    WeComWebhookEvent.provider_event_id == provider_event_id,
                )
            )
            existing = result.one()
            retryable = existing.status == WeComEventStatus.FAILED
            if retryable and normalized_payload_encrypted:
                existing.normalized_payload_encrypted = normalized_payload_encrypted
                existing.status = WeComEventStatus.RECEIVED
                existing.processing_error = None
                existing.updated_at = utc_now()
                self.session.add(existing)
                await self.session.commit()
                await self.session.refresh(existing)
            return existing, retryable
        await self.session.refresh(event)
        return event, not ignored

    async def mark_queued(self, event: WeComWebhookEvent) -> None:
        event.status = WeComEventStatus.QUEUED
        event.queued_at = utc_now()
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def begin_processing(self, event_id: UUID) -> WeComWebhookEvent | None:
        event = await self.session.get(WeComWebhookEvent, event_id, with_for_update=True)
        if not event or event.status in {WeComEventStatus.COMPLETED, WeComEventStatus.IGNORED}:
            await self.session.rollback()
            return None
        if event.status == WeComEventStatus.PROCESSING and event.updated_at > utc_now() - timedelta(
            minutes=10
        ):
            await self.session.rollback()
            return None
        if not event.normalized_payload_encrypted:
            event.status = WeComEventStatus.FAILED
            event.processing_error = "normalized payload is unavailable"
            event.updated_at = utc_now()
            self.session.add(event)
            await self.session.commit()
            return None
        event.status = WeComEventStatus.PROCESSING
        event.attempt_count += 1
        event.processing_error = None
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()
        await self.session.refresh(event)
        return event

    async def finish(self, event_id: UUID) -> None:
        event = await self.session.get(WeComWebhookEvent, event_id)
        if not event:
            return
        event.status = WeComEventStatus.COMPLETED
        event.normalized_payload_encrypted = None
        event.processing_error = None
        event.processed_at = utc_now()
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def fail(self, event_id: UUID, error: str, *, final: bool) -> None:
        event = await self.session.get(WeComWebhookEvent, event_id)
        if not event:
            return
        event.status = WeComEventStatus.FAILED
        event.processing_error = error[:1000]
        if final:
            event.normalized_payload_encrypted = None
            event.processed_at = utc_now()
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()


class WeComDeliveryRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def reserve(
        self,
        *,
        owner_user_id: UUID,
        connection_id: UUID,
        source_id: UUID,
        opportunity_id: UUID,
        idempotency_key: str,
        content_hash: str,
    ) -> tuple[WeComOutboundDelivery, bool]:
        delivery = WeComOutboundDelivery(
            owner_user_id=owner_user_id,
            connection_id=connection_id,
            source_id=source_id,
            opportunity_id=opportunity_id,
            idempotency_key=idempotency_key,
            content_hash=content_hash,
        )
        self.session.add(delivery)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            result = await self.session.exec(
                select(WeComOutboundDelivery).where(
                    WeComOutboundDelivery.owner_user_id == owner_user_id,
                    WeComOutboundDelivery.idempotency_key == idempotency_key,
                )
            )
            existing = result.one()
            return existing, existing.status == WeComDeliveryStatus.FAILED
        await self.session.refresh(delivery)
        return delivery, True

    async def mark_sending(self, delivery: WeComOutboundDelivery) -> None:
        delivery.status = WeComDeliveryStatus.SENDING
        delivery.attempt_count += 1
        delivery.error = None
        delivery.updated_at = utc_now()
        self.session.add(delivery)
        await self.session.commit()

    async def mark_sent(
        self,
        delivery: WeComOutboundDelivery,
        provider_message_id: str | None,
    ) -> None:
        delivery.status = WeComDeliveryStatus.SENT
        delivery.provider_message_id = provider_message_id
        delivery.sent_at = utc_now()
        delivery.error = None
        delivery.updated_at = utc_now()
        self.session.add(delivery)
        await self.session.commit()

    async def mark_failed(self, delivery: WeComOutboundDelivery, error: str) -> None:
        delivery.status = WeComDeliveryStatus.FAILED
        delivery.error = error[:1000]
        delivery.updated_at = utc_now()
        self.session.add(delivery)
        await self.session.commit()


class WeComArchiveRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def list_for_owner(self, owner_user_id: UUID) -> list[WeComArchiveConnection]:
        result = await self.session.exec(
            select(WeComArchiveConnection)
            .where(
                WeComArchiveConnection.owner_user_id == owner_user_id,
                WeComArchiveConnection.enabled.is_(True),
            )
            .order_by(col(WeComArchiveConnection.created_at).desc())
        )
        return list(result.all())

    async def count_enabled_for_owner(self, owner_user_id: UUID) -> int:
        result = await self.session.exec(
            select(func.count())
            .select_from(WeComArchiveConnection)
            .where(
                WeComArchiveConnection.owner_user_id == owner_user_id,
                WeComArchiveConnection.enabled.is_(True),
            )
        )
        return int(result.one())

    async def get(self, connection_id: UUID) -> WeComArchiveConnection | None:
        return await self.session.get(WeComArchiveConnection, connection_id)

    async def get_for_owner(
        self, connection_id: UUID, owner_user_id: UUID
    ) -> WeComArchiveConnection | None:
        result = await self.session.exec(
            select(WeComArchiveConnection).where(
                WeComArchiveConnection.id == connection_id,
                WeComArchiveConnection.owner_user_id == owner_user_id,
            )
        )
        return result.first()

    async def create_with_owner_binding(
        self,
        *,
        connection: WeComArchiveConnection,
        wecom_user_id: str,
        member_display_name: str,
    ) -> tuple[WeComArchiveConnection, WeComArchiveMemberBinding, WeComArchiveCursor]:
        existing_result = await self.session.exec(
            select(WeComArchiveConnection).where(
                WeComArchiveConnection.owner_user_id == connection.owner_user_id,
                WeComArchiveConnection.corp_id == connection.corp_id,
            )
        )
        existing = existing_result.first()
        if existing:
            if existing.enabled:
                raise ValueError("WeCom archive connection already exists")
            existing.display_name = connection.display_name
            existing.secret_encrypted = connection.secret_encrypted
            existing.private_key_encrypted = connection.private_key_encrypted
            existing.public_key_version = connection.public_key_version
            existing.status = WeComConnectionStatus.PENDING
            existing.enabled = True
            existing.last_error = None
            existing.updated_at = utc_now()
            binding_result = await self.session.exec(
                select(WeComArchiveMemberBinding).where(
                    WeComArchiveMemberBinding.connection_id == existing.id,
                    WeComArchiveMemberBinding.user_id == existing.owner_user_id,
                )
            )
            binding = binding_result.first()
            if not binding:
                binding = WeComArchiveMemberBinding(
                    connection_id=existing.id,
                    user_id=existing.owner_user_id,
                    wecom_user_id=wecom_user_id,
                    display_name=member_display_name,
                )
            else:
                binding.wecom_user_id = wecom_user_id
                binding.display_name = member_display_name
                binding.enabled = True
                binding.updated_at = utc_now()
            cursor = await self.cursor_for_connection(existing.id)
            if not cursor:
                cursor = WeComArchiveCursor(connection_id=existing.id)
            cursor.lease_expires_at = None
            self.session.add(existing)
            self.session.add(binding)
            self.session.add(cursor)
            try:
                await self.session.commit()
            except IntegrityError as exc:
                await self.session.rollback()
                raise ValueError(
                    "WeCom archive connection or member binding already exists"
                ) from exc
            for record in (existing, binding, cursor):
                await self.session.refresh(record)
            return existing, binding, cursor

        binding = WeComArchiveMemberBinding(
            connection_id=connection.id,
            user_id=connection.owner_user_id,
            wecom_user_id=wecom_user_id,
            display_name=member_display_name,
        )
        cursor = WeComArchiveCursor(connection_id=connection.id)
        self.session.add(connection)
        self.session.add(binding)
        self.session.add(cursor)
        try:
            await self.session.commit()
        except IntegrityError as exc:
            await self.session.rollback()
            raise ValueError("WeCom archive connection or member binding already exists") from exc
        for record in (connection, binding, cursor):
            await self.session.refresh(record)
        return connection, binding, cursor

    async def binding_for_user(
        self, connection_id: UUID, user_id: UUID
    ) -> WeComArchiveMemberBinding | None:
        result = await self.session.exec(
            select(WeComArchiveMemberBinding).where(
                WeComArchiveMemberBinding.connection_id == connection_id,
                WeComArchiveMemberBinding.user_id == user_id,
            )
        )
        return result.first()

    async def active_bindings_for_participants(
        self, connection_id: UUID, participant_ids: set[str]
    ) -> list[WeComArchiveMemberBinding]:
        if not participant_ids:
            return []
        result = await self.session.exec(
            select(WeComArchiveMemberBinding).where(
                WeComArchiveMemberBinding.connection_id == connection_id,
                WeComArchiveMemberBinding.enabled.is_(True),
                col(WeComArchiveMemberBinding.wecom_user_id).in_(participant_ids),
            )
        )
        return list(result.all())

    async def mark_binding_matched(self, binding: WeComArchiveMemberBinding) -> None:
        binding.last_matched_at = utc_now()
        binding.updated_at = utc_now()
        self.session.add(binding)
        await self.session.commit()

    async def list_sources_for_owner(self, owner_user_id: UUID) -> list[WeComSource]:
        result = await self.session.exec(
            select(WeComSource)
            .where(
                WeComSource.owner_user_id == owner_user_id,
                WeComSource.archive_connection_id.is_not(None),
            )
            .order_by(col(WeComSource.last_message_at).desc().nullslast())
        )
        return list(result.all())

    async def cursor_for_connection(self, connection_id: UUID) -> WeComArchiveCursor | None:
        result = await self.session.exec(
            select(WeComArchiveCursor).where(WeComArchiveCursor.connection_id == connection_id)
        )
        return result.first()

    async def acquire_poll_lease(
        self, connection_id: UUID, *, lease_seconds: int
    ) -> WeComArchiveCursor | None:
        now = utc_now()
        result = await self.session.exec(
            select(WeComArchiveCursor)
            .where(
                WeComArchiveCursor.connection_id == connection_id,
                (WeComArchiveCursor.lease_expires_at.is_(None))
                | (WeComArchiveCursor.lease_expires_at <= now),
            )
            .with_for_update(skip_locked=True)
        )
        cursor = result.first()
        if not cursor:
            return None
        cursor.lease_expires_at = now + timedelta(seconds=lease_seconds)
        cursor.updated_at = now
        self.session.add(cursor)
        await self.session.commit()
        await self.session.refresh(cursor)
        return cursor

    async def active_connection_ids(self, *, limit: int) -> list[UUID]:
        result = await self.session.exec(
            select(WeComArchiveConnection.id)
            .where(
                WeComArchiveConnection.enabled.is_(True),
                WeComArchiveConnection.status == WeComConnectionStatus.ACTIVE,
            )
            .order_by(col(WeComArchiveConnection.last_polled_at).asc().nullsfirst())
            .limit(limit)
        )
        return list(result.all())

    async def reserve_event(
        self,
        *,
        connection_id: UUID,
        provider_message_id: str,
        sequence: int,
        message_type: str,
        payload_hash: str,
    ) -> tuple[WeComArchiveEvent, bool]:
        event = WeComArchiveEvent(
            connection_id=connection_id,
            provider_message_id=provider_message_id,
            sequence=sequence,
            message_type=message_type,
            payload_hash=payload_hash,
            status=WeComEventStatus.PROCESSING,
            attempt_count=1,
        )
        try:
            async with self.session.begin_nested():
                self.session.add(event)
                await self.session.flush()
        except IntegrityError:
            result = await self.session.exec(
                select(WeComArchiveEvent).where(
                    WeComArchiveEvent.connection_id == connection_id,
                    WeComArchiveEvent.provider_message_id == provider_message_id,
                )
            )
            existing = result.one()
            if existing.status in {WeComEventStatus.COMPLETED, WeComEventStatus.IGNORED}:
                return existing, False
            existing.status = WeComEventStatus.PROCESSING
            existing.attempt_count += 1
            existing.processing_error = None
            existing.updated_at = utc_now()
            self.session.add(existing)
            await self.session.commit()
            await self.session.refresh(existing)
            return existing, True
        await self.session.commit()
        await self.session.refresh(event)
        return event, True

    async def complete_event(
        self, event: WeComArchiveEvent, *, matched_user_count: int, ignored: bool = False
    ) -> None:
        event.status = WeComEventStatus.IGNORED if ignored else WeComEventStatus.COMPLETED
        event.matched_user_count = matched_user_count
        event.processing_error = None
        event.processed_at = utc_now()
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def fail_event(self, event: WeComArchiveEvent, error: str) -> None:
        event.status = WeComEventStatus.FAILED
        event.processing_error = error[:1000]
        event.updated_at = utc_now()
        self.session.add(event)
        await self.session.commit()

    async def ensure_archive_source(
        self,
        *,
        connection_id: UUID,
        owner_user_id: UUID,
        external_conversation_id: str,
        display_name: str,
        source_type: WeComSourceType,
        quota_paused: bool = False,
        quota_reason: str | None = None,
    ) -> WeComSource:
        result = await self.session.exec(
            select(WeComSource).where(
                WeComSource.archive_connection_id == connection_id,
                WeComSource.owner_user_id == owner_user_id,
                WeComSource.external_conversation_id == external_conversation_id,
            )
        )
        source = result.first()
        now = utc_now()
        if source:
            source.display_name = display_name[:255]
            source.last_message_at = now
            source.updated_at = now
        else:
            source = WeComSource(
                owner_user_id=owner_user_id,
                archive_connection_id=connection_id,
                external_conversation_id=external_conversation_id,
                display_name=display_name[:255],
                source_type=source_type,
                receive_capability=WeComReceiveCapability.MESSAGE_ARCHIVE,
                send_capability=WeComSendCapability.MANUAL_ONLY,
                quota_paused=quota_paused,
                quota_reason=quota_reason,
                last_message_at=now,
            )
        self.session.add(source)
        try:
            await self.session.commit()
        except IntegrityError:
            await self.session.rollback()
            result = await self.session.exec(
                select(WeComSource).where(
                    WeComSource.archive_connection_id == connection_id,
                    WeComSource.owner_user_id == owner_user_id,
                    WeComSource.external_conversation_id == external_conversation_id,
                )
            )
            source = result.one()
        await self.session.refresh(source)
        return source

    async def get_archive_source(
        self,
        *,
        connection_id: UUID,
        owner_user_id: UUID,
        external_conversation_id: str,
    ) -> WeComSource | None:
        result = await self.session.exec(
            select(WeComSource).where(
                WeComSource.archive_connection_id == connection_id,
                WeComSource.owner_user_id == owner_user_id,
                WeComSource.external_conversation_id == external_conversation_id,
            )
        )
        return result.first()

    async def active_group_counts(self, owner_user_id: UUID) -> tuple[int, int]:
        telegram_result = await self.session.exec(
            select(func.count())
            .select_from(TelegramSource)
            .join(
                TelegramConnection,
                TelegramSource.connection_id == TelegramConnection.id,
            )
            .where(
                TelegramSource.owner_user_id == owner_user_id,
                TelegramSource.enabled.is_(True),
                TelegramSource.quota_paused.is_(False),
                TelegramConnection.enabled.is_(True),
                TelegramConnection.status == TelegramConnectionStatus.CONNECTED,
                TelegramSource.source_type.in_(
                    [TelegramSourceType.GROUP, TelegramSourceType.CHANNEL]
                ),
            )
        )
        wecom_groups = await _count_active_wecom_group_sources(self.session, owner_user_id)
        return int(telegram_result.one()), wecom_groups

    async def reconcile_source_quota_for_user(
        self, *, owner_user_id: UUID, capacity: int
    ) -> list[WeComSource]:
        result = await self.session.exec(
            select(WeComSource)
            .where(
                WeComSource.owner_user_id == owner_user_id,
                WeComSource.enabled.is_(True),
                WeComSource.source_type.in_(
                    [WeComSourceType.INTERNAL_GROUP, WeComSourceType.EXTERNAL_GROUP]
                ),
            )
            .order_by(
                col(WeComSource.quota_paused).asc(),
                col(WeComSource.created_at).asc(),
            )
        )
        sources = list(result.all())
        for index, source in enumerate(sources):
            paused = index >= max(capacity, 0)
            source.quota_paused = paused
            source.quota_reason = (
                "current subscription does not have capacity for this WeCom group"
                if paused
                else None
            )
            source.updated_at = utc_now()
            self.session.add(source)
        await self.session.commit()
        return sources

    async def finish_poll(
        self,
        *,
        connection: WeComArchiveConnection,
        cursor: WeComArchiveCursor,
        last_sequence: int,
        batch_size: int,
        verified: bool = False,
    ) -> None:
        now = utc_now()
        cursor.last_seq = max(cursor.last_seq, last_sequence)
        cursor.last_batch_size = batch_size
        cursor.lease_expires_at = None
        cursor.updated_at = now
        connection.status = WeComConnectionStatus.ACTIVE
        connection.last_polled_at = now
        connection.last_error = None
        if verified:
            connection.last_verified_at = now
        connection.updated_at = now
        self.session.add(cursor)
        self.session.add(connection)
        await self.session.commit()

    async def release_poll_failure(
        self, connection: WeComArchiveConnection, cursor: WeComArchiveCursor, error: str
    ) -> None:
        cursor.lease_expires_at = None
        cursor.updated_at = utc_now()
        connection.last_error = error[:1000]
        connection.updated_at = utc_now()
        self.session.add(cursor)
        self.session.add(connection)
        await self.session.commit()

    async def mark_connection_error(self, connection: WeComArchiveConnection, error: str) -> None:
        connection.last_error = error[:1000]
        connection.updated_at = utc_now()
        self.session.add(connection)
        await self.session.commit()

    async def disable_and_clear_secrets(
        self, connection: WeComArchiveConnection, *, cleared_secret: str
    ) -> None:
        connection.enabled = False
        connection.status = WeComConnectionStatus.DISABLED
        connection.secret_encrypted = cleared_secret
        connection.private_key_encrypted = cleared_secret
        connection.last_error = None
        connection.updated_at = utc_now()
        self.session.add(connection)
        bindings = await self.session.exec(
            select(WeComArchiveMemberBinding).where(
                WeComArchiveMemberBinding.connection_id == connection.id
            )
        )
        for binding in bindings.all():
            binding.enabled = False
            binding.updated_at = utc_now()
            self.session.add(binding)
        sources = await self.session.exec(
            select(WeComSource).where(WeComSource.archive_connection_id == connection.id)
        )
        for source in sources.all():
            source.enabled = False
            source.updated_at = utc_now()
            self.session.add(source)
        await self.session.commit()


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
