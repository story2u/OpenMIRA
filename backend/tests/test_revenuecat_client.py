import json

import httpx
import pytest

from app.infrastructure.billing.revenuecat_client import RevenueCatClient, RevenueCatError


@pytest.mark.asyncio
async def test_revenuecat_client_retries_429_with_a_finite_limit(monkeypatch) -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        assert request.headers["Authorization"] == "Bearer server-secret"
        if calls < 3:
            return httpx.Response(429, request=request)
        return httpx.Response(
            200,
            request=request,
            content=json.dumps({"subscriber": {"entitlements": {}, "subscriptions": {}}}),
        )

    async def no_sleep(_: float) -> None:
        return None

    monkeypatch.setattr("app.infrastructure.billing.revenuecat_client.asyncio.sleep", no_sleep)
    client = RevenueCatClient(secret_api_key="server-secret", transport=httpx.MockTransport(handler))
    try:
        snapshot = await client.get_customer("user/id")
    finally:
        await client.aclose()

    assert calls == 3
    assert snapshot.app_user_id == "user/id"


@pytest.mark.asyncio
async def test_revenuecat_client_error_never_contains_secret() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(503, request=request, text="upstream failure server-secret")

    client = RevenueCatClient(
        secret_api_key="server-secret",
        max_attempts=1,
        transport=httpx.MockTransport(handler),
    )
    try:
        with pytest.raises(RevenueCatError) as raised:
            await client.get_customer("user-id")
    finally:
        await client.aclose()

    assert "server-secret" not in str(raised.value)
    assert "503" in str(raised.value)
