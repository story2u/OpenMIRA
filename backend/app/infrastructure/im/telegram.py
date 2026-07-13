import re
from typing import Any
from uuid import UUID

import httpx

from app.core.config import Settings
from app.core.security import require_secret
from app.domain.enums import IMChannel
from app.domain.ports import InboundMessage, SendReceipt


class TelegramAdapter:
    channel = IMChannel.TELEGRAM

    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.api_base_url = f"https://api.telegram.org/bot{settings.telegram_bot_token}"

    def verify_webhook(self, headers: dict[str, str]) -> None:
        secret = headers.get("x-telegram-bot-api-secret-token", "")
        require_secret(secret, self.settings.telegram_webhook_secret, "invalid telegram secret")

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
        query: dict[str, str] | None = None,
        *,
        owner_user_id: UUID | None = None,
        external_id_prefix: str | None = None,
    ) -> InboundMessage | None:
        self.verify_webhook(headers)

        message = (
            payload.get("message")
            or payload.get("edited_message")
            or payload.get("channel_post")
            or payload.get("edited_channel_post")
            or payload.get("business_message")
            or payload.get("edited_business_message")
        )
        if not message:
            return None

        text = message.get("text") or message.get("caption")
        if not text:
            return None

        chat = message.get("chat") or {}
        sender = message.get("from") or {}
        conversation_id = str(chat.get("id"))
        message_identifier = f"{conversation_id}:{message.get('message_id')}"
        external_message_id = (
            f"{external_id_prefix}:{message_identifier}" if external_id_prefix else message_identifier
        )
        chat_type = str(chat.get("type") or "private")
        source_type = "private" if chat_type == "private" else "group"

        sender_name = " ".join(
            item
            for item in [sender.get("first_name"), sender.get("last_name")]
            if item
        ) or sender.get("username") or chat.get("title")

        return InboundMessage(
            owner_user_id=owner_user_id,
            channel=self.channel,
            external_message_id=external_message_id,
            conversation_id=conversation_id,
            sender_external_id=str(sender.get("id")) if sender.get("id") else None,
            sender_display_name=sender_name,
            text=text,
            source_type=source_type,
            group_name=str(chat.get("title")) if source_type == "group" and chat.get("title") else None,
            raw_message_links=self._extract_links(message, text),
            raw_payload=payload,
        )

    async def send_message(
        self,
        conversation_id: str,
        text: str,
        *,
        idempotency_key: str | None = None,
        opportunity_id: UUID | None = None,
        owner_user_id: UUID | None = None,
    ) -> SendReceipt:
        del idempotency_key, opportunity_id, owner_user_id
        if not self.settings.im_send_enabled:
            return SendReceipt(
                provider_message_id=None,
                raw_response={"dry_run": True, "channel": self.channel, "chat_id": conversation_id},
            )

        async with httpx.AsyncClient(timeout=10.0) as client:
            response = await client.post(
                f"{self.api_base_url}/sendMessage",
                json={"chat_id": conversation_id, "text": text},
            )
            response.raise_for_status()
            data = response.json()
            message_id = data.get("result", {}).get("message_id")
            return SendReceipt(
                provider_message_id=str(message_id) if message_id else None,
                raw_response=data,
            )

    def _extract_links(self, message: dict[str, Any], text: str) -> list[str]:
        links: list[str] = []
        for entity in [*message.get("entities", []), *message.get("caption_entities", [])]:
            entity_type = entity.get("type")
            if entity_type == "text_link" and entity.get("url"):
                links.append(str(entity["url"]))
                continue
            if entity_type == "url":
                offset = int(entity.get("offset", 0))
                length = int(entity.get("length", 0))
                if length > 0:
                    links.append(text[offset : offset + length])

        links.extend(re.findall(r"https?://[^\s<>()\"']+", text))
        return list(dict.fromkeys(links))
