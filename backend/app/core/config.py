from functools import lru_cache
from typing import Literal

from pydantic import AnyHttpUrl, Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    app_env: Literal["local", "dev", "staging", "prod"] = "local"
    app_name: str = "Opportunity IM Assistant API"
    debug: bool = False
    api_v1_prefix: str = "/api/v1"

    database_url: str
    redis_url: str = "redis://redis:6379/0"
    celery_broker_url: str = "redis://redis:6379/1"
    celery_result_backend: str = "redis://redis:6379/2"

    admin_api_token: str
    cors_origins: list[AnyHttpUrl | str] = Field(default_factory=list)

    default_timezone: str = "Asia/Shanghai"
    default_workdays: list[int] = Field(default_factory=lambda: [1, 2, 3, 4, 5])
    default_work_start: str = "09:00"
    default_work_end: str = "18:30"
    pending_human_sla_minutes: int = 30

    im_send_enabled: bool = False
    telegram_bot_token: str = ""
    telegram_webhook_secret: str = ""
    telegram_user_enabled: bool = False
    telegram_user_api_id: int = 0
    telegram_user_api_hash: str = ""
    telegram_user_session: str = ""
    telegram_user_chats: list[str | int] = Field(default_factory=list)
    telegram_user_backfill_limit: int = 30

    wecom_corp_id: str = ""
    wecom_agent_id: str = ""
    wecom_secret: str = ""
    wecom_token: str = ""
    wecom_aes_key: str = ""

    ai_enabled: bool = False
    litellm_model: str = "openai/gpt-4o-mini"
    openai_api_key: str = ""


@lru_cache
def get_settings() -> Settings:
    return Settings()
