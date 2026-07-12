import os
from datetime import UTC, datetime
from types import SimpleNamespace
from uuid import uuid4

import pytest
from fastapi import HTTPException

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost:5432/im")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.v1.routes.subscriptions import (
    get_catalog,
    get_subscription_management,
    sync_my_subscription,
)
from app.core.config import Settings
from app.core.security import encrypt_secret
from app.domain.enums import BillingInterval, BillingStore, PlanCode, SubscriptionStatus
from app.domain.services.subscription_policy import BillingPeriod, get_plan_entitlements


def settings(*, enabled: bool = True) -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        admin_api_token="test",
        jwt_secret_key="test-jwt",
        revenuecat_enabled=enabled,
        revenuecat_secret_api_key="server-secret" if enabled else "",
    )


def snapshot(settings_value: Settings):
    now = datetime(2026, 7, 1, tzinfo=UTC)
    return SimpleNamespace(
        plan_code=PlanCode.PRO,
        subscription_status=SubscriptionStatus.ACTIVE,
        usage_period=BillingPeriod(start=now, end=datetime(2026, 8, 1, tzinfo=UTC)),
        usage_period_start=now,
        usage_period_end=datetime(2026, 8, 1, tzinfo=UTC),
        billing_period_start=now,
        billing_period_end=datetime(2027, 7, 1, tzinfo=UTC),
        cancel_at_period_end=False,
        entitlements=get_plan_entitlements(PlanCode.PRO),
        effective_store=BillingStore.PADDLE,
        billing_interval=BillingInterval.ANNUAL,
        entitlement_expires_at=datetime(2027, 7, 1, tzinfo=UTC),
        will_renew=True,
        billing_issue=False,
        multiple_active_subscriptions=False,
        management_url_encrypted=encrypt_secret("https://billing.example.test/manage", settings_value),
        last_synced_at=now,
    )


class FakeSubscriptionRepo:
    def __init__(self, value):
        self.value = value

    async def get_snapshot(self, _):
        return self.value


class RejectingRedis:
    async def set(self, *_, **__):
        return False


@pytest.mark.asyncio
async def test_catalog_has_six_paid_packages_and_no_free_entitlement() -> None:
    catalog = await get_catalog(SimpleNamespace())

    assert catalog[0].planCode == PlanCode.FREE
    assert catalog[0].revenuecatPackageIdentifiers == []
    assert [package for plan in catalog for package in plan.revenuecatPackageIdentifiers] == [
        "plus_monthly",
        "plus_annual",
        "pro_monthly",
        "pro_annual",
        "max_monthly",
        "max_annual",
    ]


@pytest.mark.asyncio
async def test_sync_is_current_user_only_and_rate_limited_without_client_payload() -> None:
    user = SimpleNamespace(id=uuid4())
    with pytest.raises(HTTPException) as raised:
        await sync_my_subscription(
            current_user=user,
            settings=settings(),
            redis=RejectingRedis(),
            user_repo=SimpleNamespace(),
            subscription_repo=SimpleNamespace(),
            telegram_repo=SimpleNamespace(),
            telegram_connection_repo=SimpleNamespace(),
        )

    assert raised.value.status_code == 429


@pytest.mark.asyncio
async def test_management_url_is_server_supplied_and_client_scoped() -> None:
    settings_value = settings()
    result = await get_subscription_management(
        client="web",
        current_user=SimpleNamespace(id=uuid4()),
        settings=settings_value,
        subscription_repo=FakeSubscriptionRepo(snapshot(settings_value)),
    )

    assert result.store == BillingStore.PADDLE
    assert result.managementUrl == "https://billing.example.test/manage"
    assert result.canOpenInCurrentClient is True
