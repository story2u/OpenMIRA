from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.infrastructure.im.telegram import TelegramAdapter


@pytest.mark.asyncio
async def test_telegram_adapter_parses_group_job_message() -> None:
    adapter = TelegramAdapter(
        SimpleNamespace(
            telegram_bot_token="token",
            telegram_webhook_secret="secret",
            im_send_enabled=False,
        )
    )

    inbound = await adapter.parse_webhook(
        {
            "message": {
                "message_id": 42,
                "chat": {"id": -100123, "type": "supergroup", "title": "AI 招聘群"},
                "from": {"id": 7, "username": "hr_alice", "first_name": "Alice"},
                "text": "招聘 Python 后端工程师，远程，25k-35k，联系 @hr_alice https://jobs.example.com/1",
                "entities": [
                    {"type": "url", "offset": 42, "length": 26},
                ],
            }
        },
        {"x-telegram-bot-api-secret-token": "secret"},
    )

    assert inbound is not None
    assert inbound.external_message_id == "-100123:42"
    assert inbound.source_type == "group"
    assert inbound.group_name == "AI 招聘群"
    assert inbound.sender_display_name == "Alice"
    assert "https://jobs.example.com/1" in inbound.raw_message_links


@pytest.mark.asyncio
async def test_telegram_adapter_namespaces_connection_messages_and_sets_owner() -> None:
    owner_id = uuid4()
    adapter = TelegramAdapter(
        SimpleNamespace(
            telegram_bot_token="token",
            telegram_webhook_secret="secret",
            im_send_enabled=False,
        )
    )

    inbound = await adapter.parse_webhook(
        {
            "message": {
                "message_id": 9,
                "chat": {"id": -1001, "type": "group", "title": "群"},
                "text": "hello",
            }
        },
        {"x-telegram-bot-api-secret-token": "secret"},
        owner_user_id=owner_id,
        external_id_prefix="connection:123",
    )

    assert inbound is not None
    assert inbound.owner_user_id == owner_id
    assert inbound.external_message_id == "connection:123:-1001:9"
    assert inbound.auto_reply_allowed is False


@pytest.mark.asyncio
async def test_telegram_dry_run_receipt_is_not_delivered() -> None:
    adapter = TelegramAdapter(
        SimpleNamespace(
            telegram_bot_token="token",
            telegram_webhook_secret="secret",
            im_send_enabled=False,
        )
    )

    receipt = await adapter.send_message("123", "hello")

    assert receipt.delivered is False
    assert receipt.raw_response["dry_run"] is True
