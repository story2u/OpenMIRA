from app.core.config import Settings
from app.infrastructure.im.telegram_user import TelegramUserClient


def test_telegram_user_client_normalizes_chat_ids() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        admin_api_token="test-token",
        telegram_user_api_id=1,
        telegram_user_api_hash="hash",
        telegram_user_chats=["-1001234567890", "public_jobs_channel", 42, ""],
    )

    client = TelegramUserClient(settings)

    assert client.normalized_chats() == [-1001234567890, "public_jobs_channel", 42]


def test_telegram_user_client_extracts_unique_links() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        admin_api_token="test-token",
        telegram_user_api_id=1,
        telegram_user_api_hash="hash",
    )

    client = TelegramUserClient(settings)

    assert client._extract_links("Apply: https://example.com/job https://example.com/job", []) == [
        "https://example.com/job"
    ]
