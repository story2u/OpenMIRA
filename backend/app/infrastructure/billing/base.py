from typing import Protocol

from app.infrastructure.billing.revenuecat_models import RevenueCatCustomerSnapshot


class BillingCustomerProvider(Protocol):
    async def get_customer(self, app_user_id: str) -> RevenueCatCustomerSnapshot: ...
