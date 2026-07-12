from datetime import datetime, timedelta, timezone

import pytest

from app.domain.enums import PlanCode, SubscriptionStatus
from app.domain.services.subscription_policy import (
    GroupQuotaExceeded,
    effective_plan_code,
    ensure_group_quota,
    get_plan_entitlements,
    telegram_group_capacity,
    utc_calendar_month,
)


def test_plan_catalog_matches_product_limits() -> None:
    free = get_plan_entitlements(PlanCode.FREE)
    plus = get_plan_entitlements(PlanCode.PLUS)
    pro = get_plan_entitlements(PlanCode.PRO)
    maximum = get_plan_entitlements(PlanCode.MAX)

    assert (free.telegram_group_limit, free.wecom_group_limit) == (1, 1)
    assert [free.combined_group_limit, plus.combined_group_limit] == [2, 10]
    assert [pro.combined_group_limit, maximum.combined_group_limit] == [50, 100]
    assert [
        free.pi_agent_analysis_monthly_limit,
        plus.pi_agent_analysis_monthly_limit,
        pro.pi_agent_analysis_monthly_limit,
        maximum.pi_agent_analysis_monthly_limit,
    ] == [100, 1_000, 5_000, 10_000]


def test_paid_group_quota_is_shared_across_channels() -> None:
    plus = get_plan_entitlements(PlanCode.PLUS)

    ensure_group_quota(entitlements=plus, telegram_groups=4, wecom_groups=6)
    with pytest.raises(GroupQuotaExceeded, match="10 monitored groups"):
        ensure_group_quota(entitlements=plus, telegram_groups=5, wecom_groups=6)
    assert telegram_group_capacity(plus, wecom_groups=6) == 4


def test_free_group_quota_keeps_channel_specific_limits() -> None:
    free = get_plan_entitlements(PlanCode.FREE)

    ensure_group_quota(entitlements=free, telegram_groups=1, wecom_groups=1)
    assert telegram_group_capacity(free, wecom_groups=1) == 1
    with pytest.raises(GroupQuotaExceeded, match="1 Telegram group"):
        ensure_group_quota(entitlements=free, telegram_groups=2, wecom_groups=0)


def test_expired_or_non_active_subscription_falls_back_to_free() -> None:
    now = datetime(2026, 7, 11, tzinfo=timezone.utc)

    assert effective_plan_code(
        plan_code=PlanCode.PRO,
        status=SubscriptionStatus.ACTIVE,
        period_start=now - timedelta(days=1),
        period_end=now + timedelta(days=1),
        now=now,
    ) == PlanCode.PRO
    assert effective_plan_code(
        plan_code=PlanCode.MAX,
        status=SubscriptionStatus.PAST_DUE,
        period_start=now - timedelta(days=1),
        period_end=now + timedelta(days=1),
        now=now,
    ) == PlanCode.FREE
    assert effective_plan_code(
        plan_code=PlanCode.PLUS,
        status=SubscriptionStatus.ACTIVE,
        period_start=now - timedelta(days=31),
        period_end=now,
        now=now,
    ) == PlanCode.FREE


def test_annual_billing_does_not_change_utc_month_usage_period() -> None:
    period = utc_calendar_month(datetime(2026, 12, 31, 23, 59, tzinfo=timezone.utc))

    assert period.start == datetime(2026, 12, 1, tzinfo=timezone.utc)
    assert period.end == datetime(2027, 1, 1, tzinfo=timezone.utc)
