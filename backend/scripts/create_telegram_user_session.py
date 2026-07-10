import asyncio
import getpass

from telethon import TelegramClient
from telethon.errors import SessionPasswordNeededError
from telethon.sessions import StringSession

from app.core.config import get_settings


async def prompt(message: str) -> str:
    return (await asyncio.to_thread(input, message)).strip()


async def prompt_password(message: str) -> str:
    return await asyncio.to_thread(getpass.getpass, message)


async def main() -> None:
    settings = get_settings()
    if not settings.telegram_user_api_id or not settings.telegram_user_api_hash:
        raise SystemExit("Set TELEGRAM_USER_API_ID and TELEGRAM_USER_API_HASH first.")

    phone = await prompt("Telegram phone number, for example +8613800000000: ")
    client = TelegramClient(
        StringSession(),
        settings.telegram_user_api_id,
        settings.telegram_user_api_hash,
    )
    await client.connect()
    if not await client.is_user_authorized():
        await client.send_code_request(phone)
        code = await prompt("Login code: ")
        try:
            await client.sign_in(phone=phone, code=code)
        except SessionPasswordNeededError:
            password = await prompt_password("Two-step verification password, if enabled: ")
            await client.sign_in(password=password)

    print("\nTELEGRAM_USER_SESSION=" + client.session.save())
    await client.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
