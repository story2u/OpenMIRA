import re
from dataclasses import dataclass
from typing import Any
from uuid import UUID

from telethon import TelegramClient, events
from telethon.sessions import StringSession
from telethon.tl.custom.message import Message
from telethon.tl.types import Channel, Chat, MessageEntityTextUrl, MessageEntityUrl, User

from app.domain.enums import IMChannel
from app.domain.ports import InboundMessage


@dataclass(frozen=True)
class TelegramUserClientConfig:
    user_id: UUID
    api_id: int
    api_hash: str
    session_string: str
    chats: list[str | int]
    backfill_limit: int = 30


class TelegramUserClient:
    def __init__(self, config: TelegramUserClientConfig) -> None:
        self.config = config
        self.client = TelegramClient(
            StringSession(config.session_string),
            config.api_id,
            config.api_hash,
        )

    async def start(self) -> None:
        await self.client.start()

    async def disconnect(self) -> None:
        await self.client.disconnect()

    def new_message_event(self):
        chats = self.normalized_chats()
        return events.NewMessage(chats=chats or None)

    async def iter_backfill_messages(self):
        for chat in self.normalized_chats():
            async for message in self.client.iter_messages(
                chat,
                limit=self.config.backfill_limit,
                reverse=True,
            ):
                inbound = await self.to_inbound_message(message)
                if inbound:
                    yield inbound

    async def to_inbound_message(self, message: Message) -> InboundMessage | None:
        text = message.raw_text or ""
        if not text.strip():
            return None

        chat = await message.get_chat()
        sender = await message.get_sender()
        chat_id = message.chat_id
        if chat_id is None or message.id is None:
            return None

        source_type = "private" if isinstance(chat, User) else "group"
        group_name = self._chat_title(chat) if source_type == "group" else None

        return InboundMessage(
            owner_user_id=self.config.user_id,
            channel=IMChannel.TELEGRAM,
            external_message_id=f"user:{self.config.user_id}:{chat_id}:{message.id}",
            conversation_id=str(chat_id),
            sender_external_id=str(getattr(sender, "id", "")) if sender else None,
            sender_display_name=self._sender_name(sender),
            text=text,
            source_type=source_type,
            group_name=group_name,
            raw_message_links=self._extract_links(text, message.entities or []),
            raw_payload={
                "telegram_user_client": True,
                "owner_user_id": str(self.config.user_id),
                "chat_id": chat_id,
                "message_id": message.id,
                "group_name": group_name,
            },
            sent_at=message.date,
        )

    def normalized_chats(self) -> list[int | str]:
        chats: list[int | str] = []
        for chat in self.config.chats:
            if isinstance(chat, int):
                chats.append(chat)
                continue
            value = chat.strip()
            if not value:
                continue
            if re.fullmatch(r"-?\d+", value):
                chats.append(int(value))
            else:
                chats.append(value)
        return chats

    def _chat_title(self, chat: Any) -> str | None:
        if isinstance(chat, (Chat, Channel)):
            return getattr(chat, "title", None)
        return None

    def _sender_name(self, sender: Any) -> str | None:
        if not sender:
            return None
        if isinstance(sender, User):
            names = [sender.first_name, sender.last_name]
            return " ".join(item for item in names if item) or sender.username
        return getattr(sender, "title", None) or getattr(sender, "username", None)

    def _extract_links(self, text: str, entities: list[Any]) -> list[str]:
        links: list[str] = []
        for entity in entities:
            if isinstance(entity, MessageEntityTextUrl):
                links.append(entity.url)
                continue
            if isinstance(entity, MessageEntityUrl):
                links.append(text[entity.offset : entity.offset + entity.length])

        links.extend(re.findall(r"https?://[^\s<>()\"']+", text))
        return list(dict.fromkeys(links))
