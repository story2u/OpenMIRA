from uuid import uuid4

from app.application.mappers import to_telegram_user_config_read
from app.infrastructure.db.models import TelegramMonitor, TelegramUserConfig


def test_config_marks_retention_selection_required_after_downgrade() -> None:
    user_id = uuid4()
    config = TelegramUserConfig(id=uuid4(), user_id=user_id, enabled=True, retention_limit=10)
    monitors = [
        TelegramMonitor(
            user_id=user_id,
            telegram_config_id=config.id,
            chat_id=f"group-{index}",
            quota_paused=index > 0,
        )
        for index in range(3)
    ]

    result = to_telegram_user_config_read(config, monitors, monitor_limit=1)

    assert result.activeMonitorCount == 1
    assert result.storedMonitorCount == 3
    assert result.retentionSelectionRequired is True
    assert result.monitors[1].quotaPaused is True


def test_config_remembers_selection_for_the_current_limit() -> None:
    user_id = uuid4()
    config = TelegramUserConfig(id=uuid4(), user_id=user_id, enabled=True, retention_limit=1)
    monitors = [
        TelegramMonitor(
            user_id=user_id,
            telegram_config_id=config.id,
            chat_id=f"group-{index}",
            quota_paused=index > 0,
        )
        for index in range(3)
    ]

    result = to_telegram_user_config_read(config, monitors, monitor_limit=1)

    assert result.retentionSelectionRequired is False
