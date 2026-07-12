from typing import Literal
from urllib.parse import urlparse

from fastapi import APIRouter, Depends, HTTPException, Query, status
from redis.asyncio import Redis

from app.api.deps import (
    get_subscription_repo,
    get_redis_client,
    get_telegram_connection_repo,
    get_telegram_user_config_repo,
    get_user_repo,
    require_user,
)
from app.application.dto import (
    PlanEntitlementsRead,
    SubscriptionCatalogPlanRead,
    SubscriptionManagementRead,
    SubscriptionUsageRead,
)
from app.application.use_cases.sync_revenuecat_customer import SyncRevenueCatCustomer
from app.core.config import Settings, get_settings
from app.core.security import decrypt_secret
from app.domain.enums import BillingInterval, BillingStore, PlanCode
from app.domain.services.subscription_policy import get_plan_entitlements
from app.infrastructure.billing.revenuecat_client import RevenueCatClient, RevenueCatError
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import (
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
    UserRepository,
)

router = APIRouter()
PLAN_DISPLAY_NAMES = {
    PlanCode.FREE: "Free",
    PlanCode.PLUS: "Plus",
    PlanCode.PRO: "Pro",
    PlanCode.MAX: "Max",
}
PLAN_RANK = {PlanCode.FREE: 0, PlanCode.PLUS: 1, PlanCode.PRO: 2, PlanCode.MAX: 3}


def to_plan_read(plan_code: PlanCode) -> PlanEntitlementsRead:
    entitlements = get_plan_entitlements(plan_code)
    return PlanEntitlementsRead(
        planCode=plan_code,
        telegramGroupLimit=entitlements.telegram_group_limit,
        wecomGroupLimit=entitlements.wecom_group_limit,
        combinedGroupLimit=entitlements.combined_group_limit,
        piAgentAnalysisMonthlyLimit=entitlements.pi_agent_analysis_monthly_limit,
    )


@router.get("/plans", response_model=list[PlanEntitlementsRead])
async def list_plans(_: User = Depends(require_user)) -> list[PlanEntitlementsRead]:
    return [to_plan_read(plan_code) for plan_code in PlanCode]


@router.get("/catalog", response_model=list[SubscriptionCatalogPlanRead])
async def get_catalog(_: User = Depends(require_user)) -> list[SubscriptionCatalogPlanRead]:
    result: list[SubscriptionCatalogPlanRead] = []
    for plan_code in PlanCode:
        intervals = [] if plan_code == PlanCode.FREE else [BillingInterval.MONTHLY, BillingInterval.ANNUAL]
        packages = [f"{plan_code.value}_{interval.value}" for interval in intervals]
        result.append(
            SubscriptionCatalogPlanRead(
                planCode=plan_code,
                displayName=PLAN_DISPLAY_NAMES[plan_code],
                rank=PLAN_RANK[plan_code],
                entitlements=to_plan_read(plan_code),
                availableIntervals=intervals,
                revenuecatPackageIdentifiers=packages,
            )
        )
    return result


def management_url(snapshot, settings: Settings) -> str | None:
    if not snapshot.management_url_encrypted:
        return None
    try:
        value = decrypt_secret(snapshot.management_url_encrypted, settings)
    except ValueError:
        return None
    parsed = urlparse(value)
    return value if parsed.scheme == "https" and parsed.hostname else None


async def subscription_usage_read(
    *,
    current_user: User,
    settings: Settings,
    subscription_repo: SubscriptionRepository,
    telegram_repo: TelegramUserConfigRepository,
    telegram_connection_repo: TelegramConnectionRepository,
) -> SubscriptionUsageRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    consumed, reserved = await subscription_repo.usage_counts(
        user_id=current_user.id,
        period=snapshot.usage_period,
    )
    legacy_telegram_used = await telegram_repo.count_active_monitors_by_user(current_user.id)
    telegram_used = legacy_telegram_used + await telegram_connection_repo.count_active_sources_by_user(current_user.id)
    wecom_used = 0
    limit = snapshot.entitlements.pi_agent_analysis_monthly_limit
    return SubscriptionUsageRead(
        planCode=snapshot.plan_code,
        subscriptionStatus=snapshot.subscription_status,
        periodStart=snapshot.usage_period_start,
        periodEnd=snapshot.usage_period_end,
        cancelAtPeriodEnd=snapshot.cancel_at_period_end,
        entitlements=to_plan_read(snapshot.plan_code),
        telegramGroupsUsed=telegram_used,
        wecomGroupsUsed=wecom_used,
        combinedGroupsUsed=telegram_used + wecom_used,
        aiAnalysesConsumed=consumed,
        aiAnalysesReserved=reserved,
        aiAnalysesRemaining=max(limit - consumed - reserved, 0),
        effectiveStore=snapshot.effective_store,
        billingInterval=snapshot.billing_interval,
        billingPeriodStart=snapshot.billing_period_start,
        billingPeriodEnd=snapshot.billing_period_end,
        usagePeriodStart=snapshot.usage_period_start,
        usagePeriodEnd=snapshot.usage_period_end,
        entitlementExpiresAt=snapshot.entitlement_expires_at,
        willRenew=snapshot.will_renew,
        billingIssue=snapshot.billing_issue,
        multipleActiveSubscriptions=snapshot.multiple_active_subscriptions,
        managementUrl=management_url(snapshot, settings),
        lastSyncedAt=snapshot.last_synced_at,
    )


@router.get("/me", response_model=SubscriptionUsageRead)
async def get_my_subscription(
    settings: Settings = Depends(get_settings),
    current_user: User = Depends(require_user),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    telegram_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    telegram_connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> SubscriptionUsageRead:
    return await subscription_usage_read(
        current_user=current_user,
        settings=settings,
        subscription_repo=subscription_repo,
        telegram_repo=telegram_repo,
        telegram_connection_repo=telegram_connection_repo,
    )


@router.post("/sync", response_model=SubscriptionUsageRead)
async def sync_my_subscription(
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    redis: Redis = Depends(get_redis_client),
    user_repo: UserRepository = Depends(get_user_repo),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    telegram_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
    telegram_connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
) -> SubscriptionUsageRead:
    if not settings.revenuecat_server_available:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="Payments are not configured")
    rate_key = f"billing:sync:{current_user.id}"
    if not await redis.set(rate_key, "1", ex=settings.revenuecat_sync_rate_limit_seconds, nx=True):
        raise HTTPException(status_code=status.HTTP_429_TOO_MANY_REQUESTS, detail="Subscription sync is temporarily rate limited")
    client = RevenueCatClient(
        secret_api_key=settings.revenuecat_secret_api_key,
        timeout_seconds=settings.revenuecat_sync_timeout_seconds,
    )
    try:
        await SyncRevenueCatCustomer(
            settings=settings,
            provider=client,
            user_repo=user_repo,
            subscription_repo=subscription_repo,
            telegram_repo=telegram_repo,
            telegram_connection_repo=telegram_connection_repo,
        ).execute(current_user.id)
    except RevenueCatError as exc:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="Subscription provider is temporarily unavailable") from exc
    finally:
        await client.aclose()
    return await subscription_usage_read(
        current_user=current_user,
        settings=settings,
        subscription_repo=subscription_repo,
        telegram_repo=telegram_repo,
        telegram_connection_repo=telegram_connection_repo,
    )


@router.get("/management", response_model=SubscriptionManagementRead)
async def get_subscription_management(
    client: Literal["web", "ios", "android"] = Query(default="web"),
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
) -> SubscriptionManagementRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    url = management_url(snapshot, settings)
    expected_client = {
        BillingStore.PADDLE: "web",
        BillingStore.APP_STORE: "ios",
        BillingStore.PLAY_STORE: "android",
    }.get(snapshot.effective_store)
    instructions = {
        BillingStore.PADDLE: "Manage this subscription through the secure Web billing portal.",
        BillingStore.APP_STORE: "Manage this subscription in Apple App Store subscriptions.",
        BillingStore.PLAY_STORE: "Manage this subscription in Google Play subscriptions.",
    }
    return SubscriptionManagementRead(
        store=snapshot.effective_store,
        managementUrl=url,
        instruction=instructions.get(snapshot.effective_store, "No paid subscription is currently active."),
        canOpenInCurrentClient=bool(url and expected_client == client),
    )
