from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import httpx

from app.core.config import Settings


@dataclass(slots=True)
class OpenAICompatibleProviderStream:
    response: httpx.Response
    client: httpx.AsyncClient

    async def close(self) -> None:
        await self.response.aclose()
        await self.client.aclose()


class OpenAICompatibleGatewayClient:
    def __init__(
        self,
        settings: Settings,
        *,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self.settings = settings
        self.transport = transport

    async def open_stream(
        self,
        payload: dict[str, Any],
        *,
        gateway_request_id: str,
    ) -> OpenAICompatibleProviderStream:
        timeout = httpx.Timeout(
            self.settings.device_agent_gateway_timeout_seconds,
            connect=self.settings.device_agent_gateway_connect_timeout_seconds,
        )
        client = httpx.AsyncClient(
            timeout=timeout,
            follow_redirects=False,
            http2=True,
            limits=httpx.Limits(max_connections=10, max_keepalive_connections=5),
            transport=self.transport,
        )
        request = client.build_request(
            "POST",
            f"{self.settings.device_agent_gateway_base_url.rstrip('/')}/chat/completions",
            headers={
                "Authorization": f"Bearer {self.settings.device_agent_gateway_api_key}",
                "Accept": "text/event-stream",
                "Content-Type": "application/json",
                "X-Client-Request-Id": gateway_request_id,
            },
            json=payload,
        )
        try:
            response = await client.send(request, stream=True)
        except BaseException:
            await client.aclose()
            raise
        return OpenAICompatibleProviderStream(response=response, client=client)
