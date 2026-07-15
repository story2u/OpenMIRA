from app.core.config import Settings


def test_ai_pipeline_is_enabled_by_default() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:pass@localhost/test",
        admin_api_token="test-token",
        _env_file=None,
    )

    assert settings.ai_enabled is True
    assert settings.pi_agent_enabled is True
    assert settings.ai_auto_reply_enabled is True
    assert settings.im_send_enabled is True


def test_deepseek_requires_generic_pi_agent_key() -> None:
    settings = Settings(
        database_url="postgresql+asyncpg://user:pass@localhost/test",
        admin_api_token="test-token",
        pi_agent_provider="deepseek",
        pi_agent_api_key="deepseek-secret",
        openai_api_key="openai-secret",
        _env_file=None,
    )

    assert settings.effective_pi_agent_api_key == "deepseek-secret"
