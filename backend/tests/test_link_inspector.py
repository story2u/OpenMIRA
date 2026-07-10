import ipaddress

import httpx

from app.domain.enums import LinkSafetyStatus
from app.infrastructure.agent.link_inspector import SafeLinkInspector


async def public_resolver(hostname: str, port: int) -> list[ipaddress.IPv4Address]:
    del port
    if hostname == "public.example":
        return [ipaddress.ip_address("93.184.216.34")]
    return [ipaddress.ip_address(hostname)]


async def test_link_inspector_blocks_private_network_targets() -> None:
    inspector = SafeLinkInspector(
        max_links=5,
        max_content_bytes=10_000,
        max_text_chars=2_000,
        timeout_seconds=1,
        resolver=public_resolver,
        client=httpx.AsyncClient(transport=httpx.MockTransport(lambda _: httpx.Response(500))),
    )
    try:
        result = await inspector.inspect_many(["http://127.0.0.1/admin"])
    finally:
        await inspector.client.aclose()  # type: ignore[union-attr]

    assert result[0].status == LinkSafetyStatus.SUSPICIOUS
    assert "私网" in result[0].risk_reasons[0]


async def test_link_inspector_extracts_visible_email_and_marks_http_suspicious() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            headers={"content-type": "text/html; charset=utf-8"},
            text=(
                "<html><title>采购公告</title><body>联系 buyer@example.com"
                "<script>hidden@evil.example</script></body></html>"
            ),
            request=request,
        )
    )
    client = httpx.AsyncClient(transport=transport)
    inspector = SafeLinkInspector(
        max_links=5,
        max_content_bytes=10_000,
        max_text_chars=2_000,
        timeout_seconds=1,
        resolver=public_resolver,
        client=client,
    )
    try:
        result = await inspector.inspect_many(["http://public.example/rfp"])
    finally:
        await client.aclose()

    assert result[0].status == LinkSafetyStatus.SUSPICIOUS
    assert result[0].title == "采购公告"
    assert result[0].emails == ["buyer@example.com"]
    assert "hidden@evil.example" not in result[0].text


async def test_link_inspector_validates_every_redirect_target() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            302,
            headers={"location": "http://127.0.0.1/secret"},
            request=request,
        )
    )
    client = httpx.AsyncClient(transport=transport)
    inspector = SafeLinkInspector(
        max_links=5,
        max_content_bytes=10_000,
        max_text_chars=2_000,
        timeout_seconds=1,
        resolver=public_resolver,
        client=client,
    )
    try:
        result = await inspector.inspect_many(["https://public.example/start"])
    finally:
        await client.aclose()

    assert result[0].final_url == "http://127.0.0.1/secret"
    assert result[0].status == LinkSafetyStatus.SUSPICIOUS
    assert "私网" in result[0].risk_reasons[0]


async def test_link_inspector_rejects_url_credentials_without_fetching() -> None:
    requested = False

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal requested
        requested = True
        return httpx.Response(200, request=request)

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    inspector = SafeLinkInspector(
        max_links=5,
        max_content_bytes=10_000,
        max_text_chars=2_000,
        timeout_seconds=1,
        resolver=public_resolver,
        client=client,
    )
    try:
        result = await inspector.inspect_many(["https://user:password@public.example/"])
    finally:
        await client.aclose()

    assert requested is False
    assert "凭据" in result[0].risk_reasons[0]


async def test_link_inspector_stops_reading_oversized_response() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            headers={"content-type": "text/plain"},
            content=b"x" * 101,
            request=request,
        )
    )
    client = httpx.AsyncClient(transport=transport)
    inspector = SafeLinkInspector(
        max_links=5,
        max_content_bytes=100,
        max_text_chars=2_000,
        timeout_seconds=1,
        resolver=public_resolver,
        client=client,
    )
    try:
        result = await inspector.inspect_many(["https://public.example/large"])
    finally:
        await client.aclose()

    assert result[0].status == LinkSafetyStatus.SUSPICIOUS
    assert "超过分析上限" in result[0].risk_reasons[0]
