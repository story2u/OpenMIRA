"""Small, audited Telegram Bot API boundary for the native connection handshake."""

from dataclasses import dataclass
from typing import Any

import httpx

from app.core.config import Settings


class TelegramBotApiError(RuntimeError):
    """A stable error that does not leak Bot API payloads or configuration."""


@dataclass(frozen=True)
class VerifiedTelegramChat:
    chat_id: str
    source_type: str
    display_name: str
    username: str | None


class TelegramBotConnector:
    def __init__(self, settings: Settings, *, client: httpx.AsyncClient | None = None) -> None:
        self.settings = settings
        self._client = client

    async def send_chat_picker(
        self,
        *,
        chat_id: str,
        group_request_id: int,
        channel_request_id: int,
    ) -> None:
        reply_markup = {
            "keyboard": [
                [
                    {
                        "text": "选择群组",
                        "request_chat": {
                            "request_id": group_request_id,
                            "chat_is_channel": False,
                            "bot_is_member": True,
                            "request_title": True,
                            "request_username": True,
                        },
                    },
                    {
                        "text": "选择频道",
                        "request_chat": {
                            "request_id": channel_request_id,
                            "chat_is_channel": True,
                            "bot_is_member": True,
                            "request_title": True,
                            "request_username": True,
                        },
                    },
                ]
            ],
            "resize_keyboard": True,
            "one_time_keyboard": True,
        }
        await self._call(
            "sendMessage",
            {
                "chat_id": chat_id,
                "text": "请选择要监听的群组或频道。只有已添加本机器人的会话可以完成连接。",
                "reply_markup": reply_markup,
            },
        )

    async def send_business_instructions(self, *, chat_id: str) -> None:
        await self._call(
            "sendMessage",
            {
                "chat_id": chat_id,
                "text": (
                    "已确认此账号。请在 Telegram 的 Business 设置中将本机器人添加为聊天机器人；"
                    "连接完成后，此页面会自动更新状态。"
                ),
            },
        )

    async def verify_shared_chat(self, chat_id: str) -> VerifiedTelegramChat:
        chat = await self._call("getChat", {"chat_id": chat_id})
        chat_type = str(chat.get("type") or "")
        if chat_type not in {"group", "supergroup", "channel"}:
            raise TelegramBotApiError("selected chat is not a group or channel")
        me = await self._call("getMe", {})
        bot_id = me.get("id")
        if not isinstance(bot_id, int):
            raise TelegramBotApiError("Bot identity could not be verified")
        member = await self._call("getChatMember", {"chat_id": chat_id, "user_id": bot_id})
        if member.get("status") in {"left", "kicked"}:
            raise TelegramBotApiError("Bot is not a member of the selected chat")
        source_type = "channel" if chat_type == "channel" else "group"
        return VerifiedTelegramChat(
            chat_id=str(chat.get("id", chat_id)),
            source_type=source_type,
            display_name=str(chat.get("title") or chat.get("username") or "Telegram 来源"),
            username=str(chat["username"]) if chat.get("username") else None,
        )

    async def _call(self, method: str, payload: dict[str, Any]) -> dict[str, Any]:
        if not self.settings.telegram_bot_configured:
            raise TelegramBotApiError("Telegram Bot is not configured")
        url = f"https://api.telegram.org/bot{self.settings.telegram_bot_token}/{method}"
        try:
            if self._client:
                response = await self._client.post(url, json=payload)
            else:
                async with httpx.AsyncClient(timeout=10.0) as client:
                    response = await client.post(url, json=payload)
            response.raise_for_status()
            data = response.json()
        except (httpx.HTTPError, ValueError) as exc:
            raise TelegramBotApiError("Telegram Bot API request failed") from exc
        if not data.get("ok") or not isinstance(data.get("result"), dict):
            raise TelegramBotApiError("Telegram Bot API rejected the request")
        return data["result"]
