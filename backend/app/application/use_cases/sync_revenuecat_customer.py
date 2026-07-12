from dataclasses import dataclass
from datetime import datetime, timezone
from uuid import UUID

import structlog

from app.core.config import Settings
from app.core.security import encrypt_secret
from app.domain.enums import (
    BillingStore,
    BillingSubscriptionStatus,
    PlanCode,
)
from app.domain.services.subscription_policy import telegram_group_capacity
from app.infrastructure.billing.base import BillingCustomerProvider
from app.infrastructure.billing.revenuecat_models import RevenueCatCustomerSnapshot, RevenueCatSubscription
from app.infrastructure.db.repositories import (
    BillingSubscriptionWrite,
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
    UserRepository,
)

logger = structlog.get_logger(__name__)
PLAN_RANK = {PlanCode.FREE: 0, PlanCode.PLUS: 1, PlanCode.PRO: 2, PlanCode.MAX: 3}
ACCESS_STATUSES = {
    BillingSubscriptionStatus.TRIALING,
    BillingSubscriptionStatus.ACTIVE,
    BillingSubscriptionStatus.GRACE_PERIOD,
    BillingSubscriptionStatus.BILLING_RETRY,
    BillingSubscriptionStatus.CANCELED,
}


@dataclass(frozen=True, slots=True)
class SyncResult:
    user_id: UUID
    plan_code: PlanCode
    effective_store: BillingStore | None
    active_subscription_count: int
    multiple_active_subscriptions: bool
    synced_at: datetime


class SyncRevenueCatCustomer:
    def __init__(
        self,
        *,
        settings: Settings,
        provider: BillingCustomerProvider,
        user_repo: UserRepository,
        subscription_repo: SubscriptionRepository,
        telegram_repo: TelegramUserConfigRepository,
        telegram_connection_repo: TelegramConnectionRepository,
    ) -> None:
        self.settings = settings
        self.provider = provider
        self.user_repo = user_repo
        self.subscription_repo = subscription_repo
        self.telegram_repo = telegram_repo
        self.telegram_connection_repo = telegram_connection_repo

    def entitlement_plan(self, identifier: str) -> PlanCode | None:
        mapping = {
            self.settings.revenuecat_entitlement_plus: PlanCode.PLUS,
            self.settings.revenuecat_entitlement_pro: PlanCode.PRO,
            self.settings.revenuecat_entitlement_max: PlanCode.MAX,
        }
        return mapping.get(identifier)

    async def execute(self, user_id: UUID) -> SyncResult:
        user = await self.user_repo.get(user_id)
        if not user or not user.is_active:
            raise ValueError("active local user is required")
        customer = await self.provider.get_customer(str(user.id))
        return await self.project(user_id=user.id, customer=customer)

    async def project(self, *, user_id: UUID, customer: RevenueCatCustomerSnapshot) -> SyncResult:
        if customer.app_user_id != str(user_id):
            raise ValueError("RevenueCat customer does not match the local user")
        known_entitlements = []
        for entitlement in customer.active_entitlements:
            plan = self.entitlement_plan(entitlement.identifier)
            if plan:
                known_entitlements.append((plan, entitlement))
            else:
                logger.info("billing.revenuecat_unknown_entitlement", entitlement_id=entitlement.identifier)
        known_entitlements.sort(key=lambda item: PLAN_RANK[item[0]], reverse=True)
        effective_plan, effective_entitlement = (
            known_entitlements[0] if known_entitlements else (PlanCode.FREE, None)
        )

        writes = [self.subscription_write(item) for item in customer.subscriptions]
        active_writes = [
            item
            for item in writes
            if item.plan_code != PlanCode.FREE and item.status in ACCESS_STATUSES
        ]
        effective_subscription = next(
            (
                item
                for item in active_writes
                if item.revenuecat_entitlement_id == getattr(effective_entitlement, "identifier", None)
            ),
            None,
        )
        active_stores = {item.store for item in active_writes}
        multiple = len(active_stores) > 1
        management_url_encrypted = (
            encrypt_secret(customer.management_url, self.settings) if customer.management_url else None
        )
        account = await self.subscription_repo.project_revenuecat_customer(
            user_id=user_id,
            subscriptions=writes,
            effective_plan=effective_plan,
            effective_store=(effective_subscription.store if effective_subscription else getattr(effective_entitlement, "store", None)),
            billing_interval=(effective_subscription.billing_interval if effective_subscription else None),
            entitlement_started_at=getattr(effective_entitlement, "purchase_date", None),
            entitlement_expires_at=getattr(effective_entitlement, "expiration_date", None),
            billing_period_start=(effective_subscription.current_period_start if effective_subscription else None),
            billing_period_end=(effective_subscription.current_period_end if effective_subscription else None),
            will_renew=bool(effective_subscription and effective_subscription.will_renew),
            cancel_at_period_end=bool(effective_subscription and effective_subscription.cancel_at_period_end),
            billing_issue=bool(effective_subscription and effective_subscription.billing_issue_detected_at),
            multiple_active_subscriptions=multiple,
            last_synced_at=customer.fetched_at,
            management_url_encrypted=management_url_encrypted,
        )
        snapshot = await self.subscription_repo.get_snapshot(user_id, now=customer.fetched_at)
        legacy_active = await self.telegram_repo.count_active_monitors_by_user(user_id)
        capacity = telegram_group_capacity(snapshot.entitlements)
        await self.telegram_repo.reconcile_monitor_quota_for_user(user_id=user_id, capacity=capacity)
        await self.telegram_connection_repo.reconcile_source_quota_for_user(
            owner_user_id=user_id,
            capacity=capacity,
            legacy_active_count=min(legacy_active, capacity),
        )
        return SyncResult(
            user_id=user_id,
            plan_code=account.plan_code,
            effective_store=account.effective_store,
            active_subscription_count=len(active_writes),
            multiple_active_subscriptions=multiple,
            synced_at=customer.fetched_at,
        )

    def subscription_write(self, subscription: RevenueCatSubscription) -> BillingSubscriptionWrite:
        entitlement_plans = [
            (self.entitlement_plan(identifier), identifier)
            for identifier in subscription.entitlement_identifiers
        ]
        known = [(plan, identifier) for plan, identifier in entitlement_plans if plan]
        known.sort(key=lambda item: PLAN_RANK[item[0]], reverse=True)
        plan, entitlement_id = known[0] if known else (PlanCode.FREE, None)
        return BillingSubscriptionWrite(
            external_key=subscription.external_key,
            store=subscription.store,
            environment=subscription.environment if subscription.environment in {"sandbox", "production"} else "production",
            external_product_id=subscription.product_identifier,
            external_transaction_id=subscription.store_transaction_id,
            revenuecat_entitlement_id=entitlement_id,
            plan_code=plan,
            billing_interval=subscription.billing_interval,
            status=subscription.status,
            current_period_start=subscription.purchase_date,
            current_period_end=subscription.expiration_date,
            grace_period_end=subscription.grace_period_expires_date,
            will_renew=subscription.will_renew,
            cancel_at_period_end=subscription.unsubscribe_detected_at is not None,
            billing_issue_detected_at=subscription.billing_issue_detected_at,
            last_synced_at=datetime.now(timezone.utc),
            metadata=subscription.metadata,
        )
