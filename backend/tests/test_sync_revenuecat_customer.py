from datetime import UTC, datetime
from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.sync_revenuecat_customer import SyncRevenueCatCustomer
from app.core.config import Settings
from app.domain.enums import BillingStore, PlanCode
from app.infrastructure.billing.revenuecat_models import parse_customer


class FakeUserRepo:
    def __init__(self, user):
        self.user = user

    async def get(self, _):
        return self.user


class FakeSubscriptionRepo:
    def __init__(self):
        self.projected = None

    async def project_revenuecat_customer(self, **kwargs):
        self.projected = kwargs
        return SimpleNamespace(plan_code=kwargs["effective_plan"], effective_store=kwargs["effective_store"])

    async def get_snapshot(self, user_id, now):
        return SimpleNamespace(
            entitlements=SimpleNamespace(telegram_group_limit=None, combined_group_limit=100)
        )


class FakeTelegramRepo:
    def __init__(self):
        self.capacity = None

    async def count_active_monitors_by_user(self, _):
        return 0

    async def reconcile_monitor_quota_for_user(self, *, user_id, capacity):
        self.capacity = capacity
        return []


class FakeConnectionRepo:
    def __init__(self):
        self.capacity = None

    async def reconcile_source_quota_for_user(self, *, owner_user_id, capacity, legacy_active_count):
        self.capacity = capacity
        return []


def settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        admin_api_token="test",
        jwt_secret_key="test-jwt-key",
    )


@pytest.mark.asyncio
async def test_sync_uses_highest_entitlement_and_detects_multiple_stores() -> None:
    user = SimpleNamespace(id=uuid4(), is_active=True)
    customer = parse_customer(
        str(user.id),
        {
            "subscriber": {
                "entitlements": {
                    "plus": {"product_identifier": "plus_monthly", "expires_date": "2027-01-01T00:00:00Z"},
                    "pro": {"product_identifier": "pro_annual", "expires_date": "2027-01-01T00:00:00Z"},
                    "future": {"product_identifier": "future", "expires_date": "2027-01-01T00:00:00Z"},
                },
                "subscriptions": {
                    "plus_monthly": {"store": "paddle", "store_transaction_id": "paddle-1", "expires_date": "2027-01-01T00:00:00Z"},
                    "pro_annual": {"store": "app_store", "store_transaction_id": "apple-1", "expires_date": "2027-01-01T00:00:00Z"},
                    "future": {"store": "play_store", "store_transaction_id": "google-1", "expires_date": "2027-01-01T00:00:00Z"},
                },
            }
        },
        fetched_at=datetime(2026, 7, 1, tzinfo=UTC),
    )
    subscription_repo = FakeSubscriptionRepo()
    use_case = SyncRevenueCatCustomer(
        settings=settings(),
        provider=SimpleNamespace(),
        user_repo=FakeUserRepo(user),
        subscription_repo=subscription_repo,
        telegram_repo=FakeTelegramRepo(),
        telegram_connection_repo=FakeConnectionRepo(),
    )

    result = await use_case.project(user_id=user.id, customer=customer)

    assert result.plan_code == PlanCode.PRO
    assert result.effective_store == BillingStore.APP_STORE
    assert result.multiple_active_subscriptions is True
    assert subscription_repo.projected["effective_plan"] == PlanCode.PRO


@pytest.mark.asyncio
async def test_sync_rejects_customer_identity_mismatch() -> None:
    user = SimpleNamespace(id=uuid4(), is_active=True)
    customer = parse_customer("someone-else", {"subscriber": {}}, fetched_at=datetime.now(UTC))
    use_case = SyncRevenueCatCustomer(
        settings=settings(),
        provider=SimpleNamespace(),
        user_repo=FakeUserRepo(user),
        subscription_repo=FakeSubscriptionRepo(),
        telegram_repo=FakeTelegramRepo(),
        telegram_connection_repo=FakeConnectionRepo(),
    )

    with pytest.raises(ValueError, match="does not match"):
        await use_case.project(user_id=user.id, customer=customer)
