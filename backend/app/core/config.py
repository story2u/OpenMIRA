from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import AnyHttpUrl, Field, field_validator
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
    jwt_secret_key: str = ""
    access_token_expire_minutes: int = 60 * 24 * 7
    password_login_max_attempts: int = Field(default=5, ge=1, le=100)
    password_login_window_seconds: int = Field(default=300, ge=10, le=3600)
    password_reset_enabled: bool = True
    password_reset_ttl_minutes: int = Field(default=15, ge=5, le=60)
    password_reset_max_attempts: int = Field(default=5, ge=1, le=20)
    password_reset_request_limit: int = Field(default=3, ge=1, le=20)
    password_reset_request_window_seconds: int = Field(default=900, ge=60, le=86400)
    password_reset_verify_limit: int = Field(default=10, ge=1, le=100)
    password_reset_verify_window_seconds: int = Field(default=900, ge=60, le=86400)
    smtp_host: str = ""
    smtp_port: int = Field(default=587, ge=1, le=65535)
    smtp_username: str = ""
    smtp_password: str = ""
    smtp_from_email: str = ""
    smtp_from_name: str = "商机雷达"
    smtp_starttls: bool = True
    smtp_use_tls: bool = False
    smtp_timeout_seconds: float = Field(default=10.0, ge=1.0, le=60.0)
    frontend_base_url: str = "http://localhost:3000"
    cors_origins: list[AnyHttpUrl | str] = Field(default_factory=list)

    @property
    def password_reset_email_configured(self) -> bool:
        return bool(
            self.password_reset_enabled
            and self.smtp_host
            and self.smtp_from_email
            and (not self.smtp_username or self.smtp_password)
            and not (self.smtp_starttls and self.smtp_use_tls)
        )

    google_oauth_client_id: str = ""
    google_oauth_client_secret: str = ""
    google_oauth_redirect_uri: str = ""
    apple_oauth_client_id: str = ""
    apple_oauth_client_secret: str = ""
    apple_oauth_redirect_uri: str = ""
    apple_oauth_team_id: str = ""
    apple_oauth_key_id: str = ""
    apple_oauth_private_key: str = ""
    # 移动端原生登录允许的 id_token audience，逗号分隔：
    # Google 为 iOS OAuth client id（如同时配置 server client id 也一并列出），
    # Apple 为 app bundle id。Apple 默认填 iOS app 的 bundle id；Google 无稳定默认值。
    google_native_client_ids: str = ""
    apple_native_client_ids: str = "com.codeiy.im"

    default_timezone: str = "Asia/Shanghai"
    default_workdays: list[int] = Field(default_factory=lambda: [1, 2, 3, 4, 5])
    default_work_start: str = "09:00"
    default_work_end: str = "18:30"
    pending_human_sla_minutes: int = 30

    im_send_enabled: bool = True
    telegram_bot_token: str = ""
    telegram_webhook_secret: str = ""
    telegram_bot_username: str = ""
    telegram_webhook_url: str = ""
    telegram_integration_mode: Literal["mock", "live"] = "mock"
    telegram_connect_ttl_seconds: int = Field(default=600, ge=60, le=3600)
    # P2 remains unavailable until a dedicated long-lived QR worker is deployed.
    telegram_mtproto_qr_enabled: bool = False
    telegram_mtproto_qr_worker_enabled: bool = False
    telegram_mtproto_api_id: int | None = Field(default=None, ge=1)
    telegram_mtproto_api_hash: str = ""

    wecom_corp_id: str = ""
    wecom_agent_id: str = ""
    wecom_secret: str = ""
    wecom_token: str = ""
    wecom_aes_key: str = ""
    wecom_connection_limit: int = Field(default=1, ge=1, le=20)
    wecom_webhook_tolerance_seconds: int = Field(default=300, ge=30, le=3600)
    wecom_webhook_max_body_bytes: int = Field(default=256_000, ge=1024, le=1_000_000)
    wecom_archive_enabled: bool = False
    wecom_archive_sdk_path: str = "/opt/wecom-finance-sdk/libWeWorkFinanceSdk_C.so"
    wecom_archive_poll_interval_seconds: int = Field(default=10, ge=5, le=300)
    wecom_archive_batch_size: int = Field(default=100, ge=1, le=1000)
    wecom_archive_sdk_timeout_seconds: int = Field(default=5, ge=1, le=30)
    wecom_archive_lease_seconds: int = Field(default=120, ge=30, le=900)
    wecom_archive_connection_limit: int = Field(default=1, ge=1, le=20)
    wecom_archive_sync_rate_limit_seconds: int = Field(default=30, ge=5, le=3600)

    @property
    def wecom_archive_sdk_configured(self) -> bool:
        return self.wecom_archive_enabled and Path(self.wecom_archive_sdk_path).is_file()

    ai_enabled: bool = True
    litellm_model: str = "openai/gpt-4o-mini"
    openai_api_key: str = ""
    ai_auto_reply_enabled: bool = True
    ai_auto_reply_min_confidence: float = Field(default=0.85, ge=0.0, le=1.0)
    ai_auto_reply_cooldown_minutes: int = Field(default=720, ge=1, le=43_200)
    ai_auto_reply_window_hours: int = Field(default=24, ge=1, le=720)
    ai_auto_reply_max_per_window: int = Field(default=1, ge=1, le=10)
    ai_auto_reply_max_chars: int = Field(default=240, ge=20, le=1000)

    pi_agent_enabled: bool = True
    pi_agent_provider: str = "openai"
    pi_agent_model: str = "gpt-4o-mini"
    pi_agent_api_key: str = ""
    pi_agent_node_binary: str = "node"
    pi_agent_runner_path: str = "/app/pi-agent-runtime/src/index.mjs"
    pi_agent_timeout_seconds: float = Field(default=60.0, ge=5.0, le=300.0)
    pi_agent_min_opportunity_confidence: float = Field(default=0.75, ge=0.0, le=1.0)
    pi_agent_max_links: int = Field(default=5, ge=0, le=10)
    pi_agent_link_timeout_seconds: float = Field(default=10.0, ge=1.0, le=30.0)
    pi_agent_max_content_bytes: int = Field(default=200_000, ge=10_000, le=1_000_000)
    pi_agent_max_link_text_chars: int = Field(default=12_000, ge=1_000, le=20_000)

    revenuecat_enabled: bool = False
    revenuecat_secret_api_key: str = ""
    revenuecat_project_id: str = ""
    revenuecat_webhook_auth_token: str = ""
    revenuecat_webhook_hmac_secret: str = ""
    revenuecat_webhook_tolerance_seconds: int = Field(default=300, ge=30, le=3600)
    revenuecat_sync_timeout_seconds: float = Field(default=10.0, ge=1.0, le=60.0)
    revenuecat_sync_rate_limit_seconds: int = Field(default=30, ge=1, le=3600)
    revenuecat_reconcile_enabled: bool = False
    revenuecat_reconcile_interval_hours: int = Field(default=24, ge=1, le=168)
    revenuecat_reconcile_batch_size: int = Field(default=100, ge=1, le=500)
    revenuecat_entitlement_plus: str = "plus"
    revenuecat_entitlement_pro: str = "pro"
    revenuecat_entitlement_max: str = "max"

    @property
    def revenuecat_server_available(self) -> bool:
        return bool(self.revenuecat_enabled and self.revenuecat_secret_api_key)

    @property
    def revenuecat_webhook_available(self) -> bool:
        return bool(
            self.revenuecat_server_available
            and self.revenuecat_webhook_auth_token
            and self.revenuecat_webhook_hmac_secret
        )

    @field_validator("telegram_mtproto_api_id", mode="before")
    @classmethod
    def empty_mtproto_api_id_is_unset(cls, value: object) -> object:
        return None if value == "" else value

    @property
    def effective_pi_agent_api_key(self) -> str:
        if self.pi_agent_api_key:
            return self.pi_agent_api_key
        if self.pi_agent_provider == "openai":
            return self.openai_api_key
        return ""

    @property
    def telegram_bot_configured(self) -> bool:
        return bool(
            self.telegram_bot_token
            and self.telegram_bot_token != "change-me"
            and self.telegram_bot_username
        )

    @property
    def telegram_mtproto_qr_available(self) -> bool:
        return bool(
            self.telegram_mtproto_qr_enabled
            and self.telegram_mtproto_qr_worker_enabled
            and self.telegram_mtproto_api_id
            and self.telegram_mtproto_api_hash
        )


@lru_cache
def get_settings() -> Settings:
    return Settings()
