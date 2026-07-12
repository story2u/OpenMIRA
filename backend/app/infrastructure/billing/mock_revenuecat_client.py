from app.infrastructure.billing.revenuecat_models import RevenueCatCustomerSnapshot


class MockRevenueCatClient:
    """Explicit test adapter; production wiring must never select this provider."""

    def __init__(self, customers: dict[str, RevenueCatCustomerSnapshot]) -> None:
        self.customers = customers
        self.calls: list[str] = []

    async def get_customer(self, app_user_id: str) -> RevenueCatCustomerSnapshot:
        self.calls.append(app_user_id)
        return self.customers[app_user_id]
