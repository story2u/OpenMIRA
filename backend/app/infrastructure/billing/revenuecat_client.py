import asyncio
from urllib.parse import quote

import httpx

from app.infrastructure.billing.revenuecat_models import RevenueCatCustomerSnapshot, parse_customer


class RevenueCatError(RuntimeError):
    pass


class RevenueCatNotConfigured(RevenueCatError):
    pass


class RevenueCatClient:
    def __init__(
        self,
        *,
        secret_api_key: str,
        timeout_seconds: float = 10.0,
        base_url: str = "https://api.revenuecat.com/v1",
        max_attempts: int = 3,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        if not secret_api_key:
            raise RevenueCatNotConfigured("RevenueCat server integration is not configured")
        self._secret_api_key = secret_api_key
        self._base_url = base_url.rstrip("/")
        self._max_attempts = max(1, min(max_attempts, 5))
        self._client = httpx.AsyncClient(
            timeout=httpx.Timeout(timeout_seconds, connect=min(timeout_seconds, 5.0)),
            transport=transport,
            headers={"Authorization": f"Bearer {secret_api_key}", "Accept": "application/json"},
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def get_customer(self, app_user_id: str) -> RevenueCatCustomerSnapshot:
        encoded_user_id = quote(app_user_id, safe="")
        response: httpx.Response | None = None
        for attempt in range(self._max_attempts):
            try:
                response = await self._client.get(f"{self._base_url}/subscribers/{encoded_user_id}")
            except (httpx.TimeoutException, httpx.NetworkError) as exc:
                if attempt + 1 >= self._max_attempts:
                    raise RevenueCatError("RevenueCat customer request failed") from exc
                await asyncio.sleep(0.25 * (2**attempt))
                continue
            if response.status_code not in {429, 500, 502, 503, 504}:
                break
            if attempt + 1 >= self._max_attempts:
                break
            retry_after = response.headers.get("Retry-After")
            delay = min(float(retry_after), 2.0) if retry_after and retry_after.isdigit() else 0.25 * (2**attempt)
            await asyncio.sleep(delay)

        assert response is not None
        if response.status_code >= 400:
            raise RevenueCatError(f"RevenueCat customer request failed with status {response.status_code}")
        try:
            payload = response.json()
        except ValueError as exc:
            raise RevenueCatError("RevenueCat returned an invalid customer response") from exc
        if not isinstance(payload, dict):
            raise RevenueCatError("RevenueCat returned an invalid customer response")
        return parse_customer(app_user_id, payload)
