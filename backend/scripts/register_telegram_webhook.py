"""Register the production Telegram webhook without exposing tokens in command output."""

import asyncio

import httpx

from app.core.config import get_settings


ALLOWED_UPDATES = [
    "message",
    "edited_message",
    "channel_post",
    "edited_channel_post",
    "business_connection",
    "business_message",
    "edited_business_message",
]


async def register() -> None:
    settings = get_settings()
    if settings.telegram_integration_mode != "live":
        print("Telegram webhook registration skipped: integration mode is not live")
        return
    if not (
        settings.telegram_bot_configured
        and settings.telegram_webhook_secret
        and settings.telegram_webhook_secret != "change-me"
        and settings.telegram_webhook_url
    ):
        print("Telegram webhook registration skipped: Bot token, secret, or URL is not configured")
        return
    endpoint = f"https://api.telegram.org/bot{settings.telegram_bot_token}/setWebhook"
    payload = {
        "url": settings.telegram_webhook_url,
        "secret_token": settings.telegram_webhook_secret,
        "allowed_updates": ALLOWED_UPDATES,
    }
    try:
        async with httpx.AsyncClient(timeout=15.0) as client:
            response = await client.post(endpoint, json=payload)
            response.raise_for_status()
            data = response.json()
    except (httpx.HTTPError, ValueError) as exc:
        raise SystemExit("Telegram webhook registration failed") from exc
    if not data.get("ok"):
        raise SystemExit("Telegram webhook registration was rejected")
    print("Telegram webhook registered")


if __name__ == "__main__":
    asyncio.run(register())
