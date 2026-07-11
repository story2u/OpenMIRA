from types import SimpleNamespace

import httpx
import pytest

from app.infrastructure.im.telegram_connector import TelegramBotConnector


@pytest.mark.asyncio
async def test_verify_shared_chat_requires_a_group_and_active_bot_membership() -> None:
    calls: list[tuple[str, dict]] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        method = request.url.path.rsplit("/", 1)[-1]
        payload = __import__("json").loads(request.content)
        calls.append((method, payload))
        results = {
            "getChat": {"id": -10042, "type": "supergroup", "title": "商机群", "username": "leads"},
            "getMe": {"id": 9, "is_bot": True},
            "getChatMember": {"status": "administrator"},
        }
        return httpx.Response(200, json={"ok": True, "result": results[method]})

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    connector = TelegramBotConnector(
        SimpleNamespace(telegram_bot_configured=True, telegram_bot_token="bot-token"),
        client=client,
    )

    verified = await connector.verify_shared_chat("-10042")
    await client.aclose()

    assert verified.chat_id == "-10042"
    assert verified.source_type == "group"
    assert verified.display_name == "商机群"
    assert verified.username == "leads"
    assert calls == [
        ("getChat", {"chat_id": "-10042"}),
        ("getMe", {}),
        ("getChatMember", {"chat_id": "-10042", "user_id": 9}),
    ]
