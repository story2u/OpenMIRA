from datetime import UTC, datetime

from app.domain.enums import BillingStore, BillingSubscriptionStatus
from app.infrastructure.billing.revenuecat_models import parse_customer


def test_customer_parser_ignores_unknown_fields_and_preserves_active_entitlement() -> None:
    snapshot = parse_customer(
        "user-id",
        {
            "subscriber": {
                "management_url": "https://example.test/manage",
                "future_field": {"ignored": True},
                "entitlements": {
                    "pro": {
                        "product_identifier": "com.codeiy.im.pro.annual",
                        "purchase_date": "2026-01-01T00:00:00Z",
                        "expires_date": "2027-01-01T00:00:00Z",
                    }
                },
                "subscriptions": {
                    "com.codeiy.im.pro.annual": {
                        "store": "future_store",
                        "store_transaction_id": "transaction-1",
                        "purchase_date": "2026-01-01T00:00:00Z",
                        "expires_date": "2027-01-01T00:00:00Z",
                        "period_type": "normal",
                        "future_status": "new-value",
                    }
                },
            }
        },
        fetched_at=datetime(2026, 7, 1, tzinfo=UTC),
    )

    assert snapshot.active_entitlements[0].identifier == "pro"
    assert snapshot.subscriptions[0].store == BillingStore.UNKNOWN
    assert snapshot.subscriptions[0].status == BillingSubscriptionStatus.ACTIVE
    assert snapshot.subscriptions[0].billing_interval.value == "annual"


def test_customer_parser_keeps_grace_period_access_and_marks_refund() -> None:
    snapshot = parse_customer(
        "user-id",
        {
            "subscriber": {
                "entitlements": {
                    "plus": {
                        "product_identifier": "plus_monthly",
                        "expires_date": "2026-06-30T00:00:00Z",
                        "grace_period_expires_date": "2026-07-03T00:00:00Z",
                    }
                },
                "subscriptions": {
                    "plus_monthly": {
                        "store": "paddle",
                        "expires_date": "2026-06-30T00:00:00Z",
                        "grace_period_expires_date": "2026-07-03T00:00:00Z",
                    },
                    "old_monthly": {
                        "store": "app_store",
                        "expires_date": "2026-08-01T00:00:00Z",
                        "refunded_at": "2026-07-01T00:00:00Z",
                    },
                },
            }
        },
        fetched_at=datetime(2026, 7, 1, tzinfo=UTC),
    )

    assert snapshot.active_entitlements[0].is_active is True
    assert snapshot.subscriptions[0].status == BillingSubscriptionStatus.GRACE_PERIOD
    assert snapshot.subscriptions[1].status == BillingSubscriptionStatus.REFUNDED
