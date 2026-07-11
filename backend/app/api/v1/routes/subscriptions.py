from fastapi import APIRouter, Depends

from app.api.deps import get_subscription_repo, get_telegram_user_config_repo, require_user
from app.application.dto import PlanEntitlementsRead, SubscriptionUsageRead
from app.domain.enums import PlanCode
from app.domain.services.subscription_policy import get_plan_entitlements
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import SubscriptionRepository, TelegramUserConfigRepository

router = APIRouter()


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


@router.get("/me", response_model=SubscriptionUsageRead)
async def get_my_subscription(
    current_user: User = Depends(require_user),
    subscription_repo: SubscriptionRepository = Depends(get_subscription_repo),
    telegram_repo: TelegramUserConfigRepository = Depends(get_telegram_user_config_repo),
) -> SubscriptionUsageRead:
    snapshot = await subscription_repo.get_snapshot(current_user.id)
    consumed, reserved = await subscription_repo.usage_counts(
        user_id=current_user.id,
        period=snapshot.period,
    )
    telegram_used = await telegram_repo.count_active_monitors_by_user(current_user.id)
    # WeCom currently has a global webhook and no user-owned group configuration table.
    # Keep this explicit until that real integration exists instead of presenting mock usage.
    wecom_used = 0
    limit = snapshot.entitlements.pi_agent_analysis_monthly_limit
    return SubscriptionUsageRead(
        planCode=snapshot.plan_code,
        subscriptionStatus=snapshot.subscription_status,
        periodStart=snapshot.period.start,
        periodEnd=snapshot.period.end,
        cancelAtPeriodEnd=snapshot.cancel_at_period_end,
        entitlements=to_plan_read(snapshot.plan_code),
        telegramGroupsUsed=telegram_used,
        wecomGroupsUsed=wecom_used,
        combinedGroupsUsed=telegram_used + wecom_used,
        aiAnalysesConsumed=consumed,
        aiAnalysesReserved=reserved,
        aiAnalysesRemaining=max(limit - consumed - reserved, 0),
    )
