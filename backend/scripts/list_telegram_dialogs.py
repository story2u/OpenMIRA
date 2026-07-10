import asyncio

from telethon import TelegramClient
from telethon.sessions import StringSession

from app.core.config import get_settings


async def main() -> None:
    settings = get_settings()
    client = TelegramClient(
        StringSession(settings.telegram_user_session),
        settings.telegram_user_api_id,
        settings.telegram_user_api_hash,
    )
    await client.start()
    async for dialog in client.iter_dialogs():
        entity = dialog.entity
        username = getattr(entity, "username", None)
        print(f"{dialog.id}\t{dialog.name}\tusername={username or '-'}")
    await client.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
