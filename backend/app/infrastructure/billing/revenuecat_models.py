from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from app.domain.enums import BillingInterval, BillingStore, BillingSubscriptionStatus


def parse_datetime(value: object) -> datetime | None:
    if not isinstance(value, str) or not value:
        return None
    normalized = value.replace("Z", "+00:00")
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        return None
    return parsed.replace(tzinfo=timezone.utc) if parsed.tzinfo is None else parsed.astimezone(timezone.utc)


def parse_store(value: object) -> BillingStore:
    normalized = str(value or "").lower()
    aliases = {
        "app_store": BillingStore.APP_STORE,
        "mac_app_store": BillingStore.APP_STORE,
        "play_store": BillingStore.PLAY_STORE,
        "paddle": BillingStore.PADDLE,
        "rc_billing": BillingStore.PADDLE,
        "test_store": BillingStore.TEST_STORE,
    }
    return aliases.get(normalized, BillingStore.UNKNOWN)


def infer_interval(product_identifier: str) -> BillingInterval:
    normalized = product_identifier.lower()
    if any(token in normalized for token in ("annual", "yearly", "year")):
        return BillingInterval.ANNUAL
    if any(token in normalized for token in ("monthly", "month")):
        return BillingInterval.MONTHLY
    return BillingInterval.UNKNOWN


@dataclass(frozen=True, slots=True)
class RevenueCatEntitlement:
    identifier: str
    product_identifier: str
    purchase_date: datetime | None
    expiration_date: datetime | None
    store: BillingStore
    is_active: bool
    will_renew: bool
    billing_issue_detected_at: datetime | None
    unsubscribe_detected_at: datetime | None
    grace_period_expires_date: datetime | None
    environment: str


@dataclass(frozen=True, slots=True)
class RevenueCatSubscription:
    external_key: str
    product_identifier: str
    store: BillingStore
    environment: str
    status: BillingSubscriptionStatus
    billing_interval: BillingInterval
    purchase_date: datetime | None
    expiration_date: datetime | None
    grace_period_expires_date: datetime | None
    will_renew: bool
    billing_issue_detected_at: datetime | None
    unsubscribe_detected_at: datetime | None
    refunded_at: datetime | None
    store_transaction_id: str | None
    entitlement_identifiers: tuple[str, ...] = ()
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True, slots=True)
class RevenueCatCustomerSnapshot:
    app_user_id: str
    active_entitlements: tuple[RevenueCatEntitlement, ...]
    subscriptions: tuple[RevenueCatSubscription, ...]
    management_url: str | None
    fetched_at: datetime


def subscription_status(raw: dict[str, Any], *, now: datetime) -> BillingSubscriptionStatus:
    expiration = parse_datetime(raw.get("expires_date"))
    grace_end = parse_datetime(raw.get("grace_period_expires_date"))
    if parse_datetime(raw.get("refunded_at")):
        return BillingSubscriptionStatus.REFUNDED
    if grace_end and grace_end > now:
        return BillingSubscriptionStatus.GRACE_PERIOD
    if expiration and expiration <= now:
        return BillingSubscriptionStatus.EXPIRED
    if parse_datetime(raw.get("billing_issues_detected_at")):
        return BillingSubscriptionStatus.BILLING_RETRY
    if parse_datetime(raw.get("unsubscribe_detected_at")):
        return BillingSubscriptionStatus.CANCELED
    if str(raw.get("period_type", "")).lower() == "trial":
        return BillingSubscriptionStatus.TRIALING
    return BillingSubscriptionStatus.ACTIVE if expiration is None or expiration > now else BillingSubscriptionStatus.UNKNOWN


def parse_customer(app_user_id: str, payload: dict[str, Any], *, fetched_at: datetime | None = None) -> RevenueCatCustomerSnapshot:
    now = fetched_at or datetime.now(timezone.utc)
    subscriber = payload.get("subscriber") if isinstance(payload.get("subscriber"), dict) else {}
    raw_subscriptions = subscriber.get("subscriptions") if isinstance(subscriber.get("subscriptions"), dict) else {}
    raw_entitlements = subscriber.get("entitlements") if isinstance(subscriber.get("entitlements"), dict) else {}

    entitlements_by_product: dict[str, list[str]] = {}
    entitlements: list[RevenueCatEntitlement] = []
    for identifier, value in raw_entitlements.items():
        if not isinstance(identifier, str) or not isinstance(value, dict):
            continue
        product = str(value.get("product_identifier") or "")
        subscription = raw_subscriptions.get(product) if isinstance(raw_subscriptions.get(product), dict) else {}
        expiration = parse_datetime(value.get("expires_date"))
        grace_end = parse_datetime(value.get("grace_period_expires_date"))
        active = expiration is None or expiration > now or bool(grace_end and grace_end > now)
        will_renew = not bool(parse_datetime(subscription.get("unsubscribe_detected_at"))) and active
        entitlement = RevenueCatEntitlement(
            identifier=identifier,
            product_identifier=product,
            purchase_date=parse_datetime(value.get("purchase_date")),
            expiration_date=expiration,
            store=parse_store(subscription.get("store")),
            is_active=active,
            will_renew=will_renew,
            billing_issue_detected_at=parse_datetime(subscription.get("billing_issues_detected_at")),
            unsubscribe_detected_at=parse_datetime(subscription.get("unsubscribe_detected_at")),
            grace_period_expires_date=grace_end,
            environment="sandbox" if subscription.get("is_sandbox") is True else "production",
        )
        entitlements.append(entitlement)
        entitlements_by_product.setdefault(product, []).append(identifier)

    subscriptions: list[RevenueCatSubscription] = []
    for product, value in raw_subscriptions.items():
        if not isinstance(product, str) or not isinstance(value, dict):
            continue
        store = parse_store(value.get("store"))
        transaction_id = str(value["store_transaction_id"]) if value.get("store_transaction_id") is not None else None
        external_key = transaction_id or f"{store.value}:{product}:{app_user_id}"
        expiration = parse_datetime(value.get("expires_date"))
        unsubscribed = parse_datetime(value.get("unsubscribe_detected_at"))
        subscriptions.append(
            RevenueCatSubscription(
                external_key=external_key,
                product_identifier=product,
                store=store,
                environment="sandbox" if value.get("is_sandbox") is True else "production",
                status=subscription_status(value, now=now),
                billing_interval=infer_interval(product),
                purchase_date=parse_datetime(value.get("purchase_date")),
                expiration_date=expiration,
                grace_period_expires_date=parse_datetime(value.get("grace_period_expires_date")),
                will_renew=unsubscribed is None and (expiration is None or expiration > now),
                billing_issue_detected_at=parse_datetime(value.get("billing_issues_detected_at")),
                unsubscribe_detected_at=unsubscribed,
                refunded_at=parse_datetime(value.get("refunded_at")),
                store_transaction_id=transaction_id,
                entitlement_identifiers=tuple(entitlements_by_product.get(product, [])),
                metadata={"period_type": str(value.get("period_type") or "unknown")[:32]},
            )
        )

    management_url = subscriber.get("management_url")
    return RevenueCatCustomerSnapshot(
        app_user_id=app_user_id,
        active_entitlements=tuple(item for item in entitlements if item.is_active),
        subscriptions=tuple(subscriptions),
        management_url=management_url if isinstance(management_url, str) else None,
        fetched_at=now,
    )
