from __future__ import annotations

import asyncio
import ipaddress
import re
import socket
from collections.abc import Awaitable, Callable
from html.parser import HTMLParser
from urllib.parse import urlsplit

import httpx

from app.domain.enums import LinkSafetyStatus
from app.domain.ports import LinkInspection

EMAIL_PATTERN = re.compile(r"(?<![\w.+-])([\w.+-]+@[\w.-]+\.[A-Za-z]{2,63})(?![\w.-])")
REDIRECT_CODES = {301, 302, 303, 307, 308}
TEXT_CONTENT_TYPES = (
    "text/",
    "application/json",
    "application/ld+json",
    "application/xhtml+xml",
    "application/xml",
)

AddressResolver = Callable[[str, int], Awaitable[list[ipaddress.IPv4Address | ipaddress.IPv6Address]]]


class _VisibleTextParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self._ignored_depth = 0
        self.title = ""
        self._in_title = False
        self.parts: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in {"script", "style", "noscript", "svg"}:
            self._ignored_depth += 1
        if tag == "title":
            self._in_title = True

    def handle_endtag(self, tag: str) -> None:
        if tag in {"script", "style", "noscript", "svg"} and self._ignored_depth:
            self._ignored_depth -= 1
        if tag == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        normalized = " ".join(data.split())
        if not normalized or self._ignored_depth:
            return
        if self._in_title:
            self.title = f"{self.title} {normalized}".strip()
        self.parts.append(normalized)


async def resolve_public_addresses(
    hostname: str,
    port: int,
) -> list[ipaddress.IPv4Address | ipaddress.IPv6Address]:
    records = await asyncio.to_thread(
        socket.getaddrinfo,
        hostname,
        port,
        socket.AF_UNSPEC,
        socket.SOCK_STREAM,
    )
    return list({ipaddress.ip_address(record[4][0]) for record in records})


class SafeLinkInspector:
    def __init__(
        self,
        *,
        max_links: int,
        max_content_bytes: int,
        max_text_chars: int,
        timeout_seconds: float,
        max_redirects: int = 3,
        resolver: AddressResolver = resolve_public_addresses,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.max_links = max_links
        self.max_content_bytes = max_content_bytes
        self.max_text_chars = max_text_chars
        self.timeout_seconds = timeout_seconds
        self.max_redirects = max_redirects
        self.resolver = resolver
        self.client = client

    async def inspect_many(self, urls: list[str]) -> list[LinkInspection]:
        selected = urls[: self.max_links]
        if self.client:
            return [await self._inspect(url, self.client) for url in selected]
        timeout = httpx.Timeout(self.timeout_seconds)
        async with httpx.AsyncClient(
            timeout=timeout,
            follow_redirects=False,
            trust_env=False,
            headers={"User-Agent": "OpportunityRadar-LinkInspector/1.0"},
        ) as client:
            return [await self._inspect(url, client) for url in selected]

    async def _inspect(self, original_url: str, client: httpx.AsyncClient) -> LinkInspection:
        current_url = original_url
        risk_reasons: list[str] = []
        try:
            for redirect_count in range(self.max_redirects + 1):
                validation_error = await self._validate_target(current_url)
                if validation_error:
                    return LinkInspection(
                        url=original_url,
                        final_url=current_url,
                        status=LinkSafetyStatus.SUSPICIOUS,
                        risk_reasons=[validation_error],
                    )
                if urlsplit(current_url).scheme == "http":
                    risk_reasons.append("链接使用未加密的 HTTP")

                async with client.stream("GET", current_url) as response:
                    if response.status_code in REDIRECT_CODES:
                        location = response.headers.get("location")
                        if not location:
                            return LinkInspection(
                                url=original_url,
                                final_url=current_url,
                                status=LinkSafetyStatus.SUSPICIOUS,
                                http_status=response.status_code,
                                risk_reasons=[*risk_reasons, "重定向响应缺少 Location"],
                            )
                        if redirect_count >= self.max_redirects:
                            return LinkInspection(
                                url=original_url,
                                final_url=current_url,
                                status=LinkSafetyStatus.SUSPICIOUS,
                                http_status=response.status_code,
                                risk_reasons=[*risk_reasons, "重定向次数超过限制"],
                            )
                        current_url = str(response.url.join(location))
                        continue

                    content_type = response.headers.get("content-type", "").split(";", 1)[0].lower()
                    if content_type and not content_type.startswith(TEXT_CONTENT_TYPES):
                        return LinkInspection(
                            url=original_url,
                            final_url=current_url,
                            status=LinkSafetyStatus.SUSPICIOUS,
                            http_status=response.status_code,
                            content_type=content_type,
                            risk_reasons=[*risk_reasons, "响应不是允许分析的文本类型"],
                        )

                    body = bytearray()
                    async for chunk in response.aiter_bytes():
                        body.extend(chunk)
                        if len(body) > self.max_content_bytes:
                            return LinkInspection(
                                url=original_url,
                                final_url=current_url,
                                status=LinkSafetyStatus.SUSPICIOUS,
                                http_status=response.status_code,
                                content_type=content_type or None,
                                risk_reasons=[*risk_reasons, "响应正文超过分析上限"],
                            )

                    encoding = response.encoding or "utf-8"
                    text = bytes(body).decode(encoding, errors="replace")
                    title, visible_text = self._visible_text(text, content_type)
                    visible_text = visible_text[: self.max_text_chars]
                    emails = list(dict.fromkeys(EMAIL_PATTERN.findall(visible_text)))[:20]
                    if response.status_code >= 400:
                        risk_reasons.append(f"目标返回 HTTP {response.status_code}")
                    return LinkInspection(
                        url=original_url,
                        final_url=current_url,
                        status=(
                            LinkSafetyStatus.SAFE
                            if not risk_reasons and response.status_code < 400
                            else LinkSafetyStatus.SUSPICIOUS
                        ),
                        http_status=response.status_code,
                        content_type=content_type or None,
                        title=title,
                        text=visible_text,
                        emails=emails,
                        risk_reasons=list(dict.fromkeys(risk_reasons)),
                    )
        except (httpx.HTTPError, OSError, UnicodeError) as exc:
            return LinkInspection(
                url=original_url,
                final_url=current_url,
                status=LinkSafetyStatus.SUSPICIOUS,
                risk_reasons=[f"链接读取失败：{type(exc).__name__}"],
            )
        return LinkInspection(
            url=original_url,
            final_url=current_url,
            status=LinkSafetyStatus.SUSPICIOUS,
            risk_reasons=["链接分析未完成"],
        )

    async def _validate_target(self, url: str) -> str | None:
        try:
            parsed = urlsplit(url)
            port = parsed.port
        except ValueError:
            return "URL 格式或端口无效"
        if parsed.scheme not in {"http", "https"}:
            return "只允许 HTTP(S) 链接"
        if parsed.username or parsed.password:
            return "URL 不允许包含凭据"
        if not parsed.hostname:
            return "URL 缺少主机名"
        expected_port = 443 if parsed.scheme == "https" else 80
        if port not in {None, expected_port}:
            return "URL 使用了不允许的端口"
        try:
            addresses = await self.resolver(parsed.hostname, expected_port)
        except (OSError, ValueError):
            return "域名解析失败"
        if not addresses:
            return "域名没有可用地址"
        if any(not address.is_global for address in addresses):
            return "目标解析到本机、私网或保留地址"
        return None

    def _visible_text(self, source: str, content_type: str) -> tuple[str | None, str]:
        if "html" not in content_type:
            return None, " ".join(source.split())
        parser = _VisibleTextParser()
        parser.feed(source)
        return parser.title[:500] or None, " ".join(parser.parts)
