from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.telegram_connection_workflow import (
    TelegramConnectionWorkflow,
    TelegramConnectionWorkflowError,
)
from app.domain.enums import (
    PlanCode,
    TelegramConnectionAttemptStatus,
    TelegramConnectionType,
    TelegramSourceType,
)
from app.domain.services.subscription_policy import get_plan_entitlements
from app.infrastructure.im.telegram_connector import TelegramBotApiError, VerifiedTelegramChat


class FakeConnectionRepo:
    def __init__(self, attempt):
        self.attempt = attempt
        self.bound_account_id: str | None = None
        self.completed: dict | None = None
        self.business_completed: dict | None = None
        self.business_connection = None
        self.failed_error: str | None = None

    async def get_attempt_by_token_hash(self, _: str):
        return self.attempt

    async def get_attempt_by_request_id(self, _: int):
        return self.attempt

    async def bind_attempt_telegram_account(self, *, attempt, telegram_account_id: str):
        self.bound_account_id = telegram_account_id
        attempt.telegram_account_id = telegram_account_id
        return attempt

    async def fail_attempt(self, *, attempt, error: str):
        self.failed_error = error
        attempt.status = TelegramConnectionAttemptStatus.FAILED
        return attempt

    async def complete_bot_chat(self, **kwargs):
        self.completed = kwargs
        self.attempt.status = TelegramConnectionAttemptStatus.COMPLETED
        return SimpleNamespace(id=uuid4()), SimpleNamespace()

    async def get_connection_by_provider_connection_id(self, _: str):
        return self.business_connection

    async def complete_business_connection(self, **kwargs):
        self.business_completed = kwargs
        return SimpleNamespace(id=uuid4())

    async def set_business_connection_state(self, **kwargs):
        self.business_completed = kwargs
        return self.business_connection


class FakeLegacyRepo:
    async def count_active_monitors_by_user(self, _: object) -> int:
        return 0


class FakeSubscriptionRepo:
    async def get_snapshot(self, _: object):
        return SimpleNamespace(entitlements=get_plan_entitlements(PlanCode.FREE))


class FakeBot:
    def __init__(self):
        self.picker: dict | None = None

    async def send_chat_picker(self, **kwargs):
        self.picker = kwargs

    async def send_business_instructions(self, **_: object):
        return None

    async def verify_shared_chat(self, _: str) -> VerifiedTelegramChat:
        return VerifiedTelegramChat(
            chat_id="-10042",
            source_type="group",
            display_name="商机群",
            username="leads",
        )


class FailingBot(FakeBot):
    async def send_chat_picker(self, **_: object):
        raise TelegramBotApiError("unavailable")


def make_attempt():
    return SimpleNamespace(
        owner_user_id=uuid4(),
        status=TelegramConnectionAttemptStatus.PENDING,
        connection_type=TelegramConnectionType.BOT_CHAT,
        group_request_id=101,
        channel_request_id=102,
        telegram_account_id=None,
    )


@pytest.mark.asyncio
async def test_bot_start_binds_telegram_account_before_sending_picker() -> None:
    attempt = make_attempt()
    repo = FakeConnectionRepo(attempt)
    bot = FakeBot()
    workflow = TelegramConnectionWorkflow(
        connection_repo=repo,
        legacy_repo=FakeLegacyRepo(),
        subscription_repo=FakeSubscriptionRepo(),
        bot=bot,
    )

    result = await workflow.handle_private_start(
        {
            "message": {
                "text": "/start connect_abcdefghijklmnopqrstuvwxyz123456",
                "from": {"id": 7},
                "chat": {"id": 7},
            }
        }
    )

    assert result.handled is True
    assert repo.bound_account_id == "7"
    assert bot.picker == {"chat_id": "7", "group_request_id": 101, "channel_request_id": 102}


@pytest.mark.asyncio
async def test_bot_start_failure_marks_attempt_failed_for_the_settings_page() -> None:
    attempt = make_attempt()
    repo = FakeConnectionRepo(attempt)
    workflow = TelegramConnectionWorkflow(
        connection_repo=repo,
        legacy_repo=FakeLegacyRepo(),
        subscription_repo=FakeSubscriptionRepo(),
        bot=FailingBot(),
    )

    with pytest.raises(TelegramConnectionWorkflowError, match="could not continue"):
        await workflow.handle_private_start(
            {
                "message": {
                    "text": "/start connect_abcdefghijklmnopqrstuvwxyz123456",
                    "from": {"id": 7},
                    "chat": {"id": 7},
                }
            }
        )

    assert attempt.status == TelegramConnectionAttemptStatus.FAILED
    assert repo.failed_error == "Telegram Bot could not continue the connection"


@pytest.mark.asyncio
async def test_chat_shared_requires_the_same_telegram_account_and_expected_type() -> None:
    attempt = make_attempt()
    attempt.telegram_account_id = "7"
    repo = FakeConnectionRepo(attempt)
    workflow = TelegramConnectionWorkflow(
        connection_repo=repo,
        legacy_repo=FakeLegacyRepo(),
        subscription_repo=FakeSubscriptionRepo(),
        bot=FakeBot(),
    )

    result = await workflow.handle_chat_shared(
        {"message": {"from": {"id": 7}, "chat_shared": {"request_id": 101, "chat_id": -10042}}}
    )

    assert result.handled is True
    assert repo.completed is not None
    assert repo.completed["source_type"] == TelegramSourceType.GROUP
    assert repo.completed["external_chat_id"] == "-10042"

    attempt = make_attempt()
    attempt.telegram_account_id = "another-account"
    denied_workflow = TelegramConnectionWorkflow(
        connection_repo=FakeConnectionRepo(attempt),
        legacy_repo=FakeLegacyRepo(),
        subscription_repo=FakeSubscriptionRepo(),
        bot=FakeBot(),
    )
    with pytest.raises(TelegramConnectionWorkflowError, match="different account"):
        await denied_workflow.handle_chat_shared(
            {"message": {"from": {"id": 7}, "chat_shared": {"request_id": 101, "chat_id": -10042}}}
        )


@pytest.mark.asyncio
async def test_business_connection_must_match_the_confirmed_telegram_account() -> None:
    attempt = make_attempt()
    attempt.connection_type = TelegramConnectionType.BUSINESS
    attempt.telegram_account_id = "7"
    repo = FakeConnectionRepo(attempt)
    workflow = TelegramConnectionWorkflow(
        connection_repo=repo,
        legacy_repo=FakeLegacyRepo(),
        subscription_repo=FakeSubscriptionRepo(),
        bot=FakeBot(),
    )

    result = await workflow.handle_business_connection(
        {
            "business_connection": {
                "id": "business-connection-1",
                "is_enabled": True,
                "user": {"id": 7},
                "rights": {"can_reply": True},
            }
        }
    )

    assert result.handled is True
    assert repo.business_completed == {
        "telegram_account_id": "7",
        "provider_connection_id": "business-connection-1",
        "is_enabled": True,
        "capabilities": {"receive_private_messages": True, "can_reply": True},
    }
