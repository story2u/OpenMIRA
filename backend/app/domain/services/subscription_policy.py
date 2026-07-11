from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone

from app.domain.enums import PlanCode, SubscriptionStatus


@dataclass(frozen=True, slots=True)
class PlanEntitlements:
    plan_code: PlanCode
    telegram_group_limit: int | None
    wecom_group_limit: int | None
    combined_group_limit: int
    pi_agent_analysis_monthly_limit: int


@dataclass(frozen=True, slots=True)
class BillingPeriod:
    start: datetime
    end: datetime


def utc_calendar_month(now: datetime) -> BillingPeriod:
    if now.tzinfo is None:
        raise ValueError("now must be timezone-aware")
    normalized = now.astimezone(timezone.utc)
    start = datetime(normalized.year, normalized.month, 1, tzinfo=timezone.utc)
    if normalized.month == 12:
        end = datetime(normalized.year + 1, 1, 1, tzinfo=timezone.utc)
    else:
        end = datetime(normalized.year, normalized.month + 1, 1, tzinfo=timezone.utc)
    return BillingPeriod(start=start, end=end)


PLAN_CATALOG: dict[PlanCode, PlanEntitlements] = {
    PlanCode.FREE: PlanEntitlements(
        plan_code=PlanCode.FREE,
        telegram_group_limit=1,
        wecom_group_limit=1,
        combined_group_limit=2,
        pi_agent_analysis_monthly_limit=100,
    ),
    PlanCode.PLUS: PlanEntitlements(
        plan_code=PlanCode.PLUS,
        telegram_group_limit=None,
        wecom_group_limit=None,
        combined_group_limit=10,
        pi_agent_analysis_monthly_limit=1_000,
    ),
    PlanCode.PRO: PlanEntitlements(
        plan_code=PlanCode.PRO,
        telegram_group_limit=None,
        wecom_group_limit=None,
        combined_group_limit=50,
        pi_agent_analysis_monthly_limit=5_000,
    ),
    PlanCode.MAX: PlanEntitlements(
        plan_code=PlanCode.MAX,
        telegram_group_limit=None,
        wecom_group_limit=None,
        combined_group_limit=100,
        pi_agent_analysis_monthly_limit=10_000,
    ),
}


def get_plan_entitlements(plan_code: PlanCode) -> PlanEntitlements:
    return PLAN_CATALOG[plan_code]


def telegram_group_capacity(
    entitlements: PlanEntitlements,
    *,
    wecom_groups: int = 0,
) -> int:
    combined_remaining = max(entitlements.combined_group_limit - wecom_groups, 0)
    if entitlements.telegram_group_limit is None:
        return combined_remaining
    return min(entitlements.telegram_group_limit, combined_remaining)


def effective_plan_code(
    *,
    plan_code: PlanCode | None,
    status: SubscriptionStatus | None,
    period_start: datetime | None,
    period_end: datetime | None,
    now: datetime,
) -> PlanCode:
    if (
        plan_code is not None
        and plan_code != PlanCode.FREE
        and status in {SubscriptionStatus.ACTIVE, SubscriptionStatus.TRIALING}
        and period_start is not None
        and period_end is not None
        and period_start <= now < period_end
    ):
        return plan_code
    return PlanCode.FREE


class GroupQuotaExceeded(ValueError):
    pass


def ensure_group_quota(
    *,
    entitlements: PlanEntitlements,
    telegram_groups: int,
    wecom_groups: int,
) -> None:
    if telegram_groups < 0 or wecom_groups < 0:
        raise ValueError("group counts cannot be negative")
    if (
        entitlements.telegram_group_limit is not None
        and telegram_groups > entitlements.telegram_group_limit
    ):
        raise GroupQuotaExceeded(
            f"current plan allows {entitlements.telegram_group_limit} Telegram group"
            f"{'s' if entitlements.telegram_group_limit != 1 else ''}"
        )
    if (
        entitlements.wecom_group_limit is not None
        and wecom_groups > entitlements.wecom_group_limit
    ):
        raise GroupQuotaExceeded(
            f"current plan allows {entitlements.wecom_group_limit} WeCom group"
            f"{'s' if entitlements.wecom_group_limit != 1 else ''}"
        )
    if telegram_groups + wecom_groups > entitlements.combined_group_limit:
        raise GroupQuotaExceeded(
            f"current plan allows {entitlements.combined_group_limit} monitored groups in total"
        )
