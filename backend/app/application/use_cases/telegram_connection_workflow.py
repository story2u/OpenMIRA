"""Webhook-side orchestration for native Telegram connection handshakes."""

from dataclasses import dataclass
import re
from uuid import UUID

from app.core.telegram_connection_tokens import hash_connection_token
from app.domain.enums import (
    TelegramConnectionAttemptStatus,
    TelegramConnectionType,
    TelegramSourceType,
)
from app.infrastructure.db.repositories import (
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
)
from app.infrastructure.im.telegram_connector import TelegramBotConnector, TelegramBotApiError


START_COMMAND = re.compile(
    r"^/start(?:@[A-Za-z0-9_]{3,})?\s+(connect|business)_([A-Za-z0-9_-]{20,128})$"
)


class TelegramConnectionWorkflowError(ValueError):
    pass


@dataclass(frozen=True)
class TelegramConnectionWebhookResult:
    handled: bool
    connection_id: UUID | None = None


class TelegramConnectionWorkflow:
    def __init__(
        self,
        *,
        connection_repo: TelegramConnectionRepository,
        legacy_repo: TelegramUserConfigRepository,
        subscription_repo: SubscriptionRepository,
        bot: TelegramBotConnector,
    ) -> None:
        self.connection_repo = connection_repo
        self.legacy_repo = legacy_repo
        self.subscription_repo = subscription_repo
        self.bot = bot

    async def handle_private_start(
        self,
        payload: dict,
    ) -> TelegramConnectionWebhookResult:
        message = payload.get("message") or {}
        match = START_COMMAND.match(str(message.get("text") or "").strip())
        if not match:
            return TelegramConnectionWebhookResult(handled=False)
        kind, raw_token = match.groups()
        attempt = await self.connection_repo.get_attempt_by_token_hash(
            hash_connection_token(raw_token)
        )
        if not attempt or attempt.status != TelegramConnectionAttemptStatus.PENDING:
            # Never let an unrecognized connection token become a normal inbound message.
            return TelegramConnectionWebhookResult(handled=True)
        expected_type = (
            TelegramConnectionType.BOT_CHAT
            if kind == "connect"
            else TelegramConnectionType.BUSINESS
        )
        if attempt.connection_type != expected_type:
            return TelegramConnectionWebhookResult(handled=True)
        sender = message.get("from") or {}
        telegram_account_id = sender.get("id")
        chat_id = (message.get("chat") or {}).get("id")
        if telegram_account_id is None or chat_id is None:
            raise TelegramConnectionWorkflowError("Telegram account could not be verified")
        await self.connection_repo.bind_attempt_telegram_account(
            attempt=attempt,
            telegram_account_id=str(telegram_account_id),
        )
        try:
            if expected_type == TelegramConnectionType.BOT_CHAT:
                if attempt.group_request_id is None or attempt.channel_request_id is None:
                    raise TelegramConnectionWorkflowError(
                        "Telegram selection request is incomplete"
                    )
                await self.bot.send_chat_picker(
                    chat_id=str(chat_id),
                    group_request_id=attempt.group_request_id,
                    channel_request_id=attempt.channel_request_id,
                )
            else:
                await self.bot.send_business_instructions(chat_id=str(chat_id))
        except TelegramBotApiError as exc:
            await self.connection_repo.fail_attempt(
                attempt=attempt,
                error="Telegram Bot could not continue the connection",
            )
            raise TelegramConnectionWorkflowError(
                "Telegram Bot could not continue the connection"
            ) from exc
        return TelegramConnectionWebhookResult(handled=True)

    async def handle_chat_shared(
        self,
        payload: dict,
    ) -> TelegramConnectionWebhookResult:
        message = payload.get("message") or {}
        shared = message.get("chat_shared")
        if not isinstance(shared, dict):
            return TelegramConnectionWebhookResult(handled=False)
        request_id = shared.get("request_id")
        chat_id = shared.get("chat_id")
        sender_id = (message.get("from") or {}).get("id")
        if not isinstance(request_id, int) or chat_id is None or sender_id is None:
            raise TelegramConnectionWorkflowError("Telegram chat selection is malformed")
        attempt = await self.connection_repo.get_attempt_by_request_id(request_id)
        if not attempt or attempt.status != TelegramConnectionAttemptStatus.PENDING:
            return TelegramConnectionWebhookResult(handled=True)
        if attempt.connection_type != TelegramConnectionType.BOT_CHAT:
            raise TelegramConnectionWorkflowError(
                "Telegram chat selection does not match the connection type"
            )
        if attempt.telegram_account_id != str(sender_id):
            raise TelegramConnectionWorkflowError(
                "Telegram chat selection belongs to a different account"
            )
        if request_id == attempt.group_request_id:
            expected_source_type = TelegramSourceType.GROUP
        elif request_id == attempt.channel_request_id:
            expected_source_type = TelegramSourceType.CHANNEL
        else:
            raise TelegramConnectionWorkflowError("Telegram chat selection request is unknown")
        try:
            verified = await self.bot.verify_shared_chat(str(chat_id))
        except TelegramBotApiError as exc:
            await self.connection_repo.fail_attempt(
                attempt=attempt,
                error="Bot could not verify the selected chat",
            )
            raise TelegramConnectionWorkflowError("Bot could not verify the selected chat") from exc
        actual_source_type = TelegramSourceType(verified.source_type)
        if actual_source_type != expected_source_type:
            raise TelegramConnectionWorkflowError(
                "Selected Telegram chat type does not match the request"
            )
        snapshot = await self.subscription_repo.get_snapshot(attempt.owner_user_id)
        legacy_active_count = await self.legacy_repo.count_active_monitors_by_user(
            attempt.owner_user_id
        )
        try:
            connection, _ = await self.connection_repo.complete_bot_chat(
                attempt=attempt,
                external_chat_id=verified.chat_id,
                source_type=actual_source_type,
                display_name=verified.display_name,
                username=verified.username,
                entitlements=snapshot.entitlements,
                legacy_active_count=legacy_active_count,
            )
        except ValueError as exc:
            await self.connection_repo.fail_attempt(attempt=attempt, error=str(exc))
            raise TelegramConnectionWorkflowError(str(exc)) from exc
        return TelegramConnectionWebhookResult(handled=True, connection_id=connection.id)

    async def handle_business_connection(
        self,
        payload: dict,
    ) -> TelegramConnectionWebhookResult:
        business = payload.get("business_connection")
        if not isinstance(business, dict):
            return TelegramConnectionWebhookResult(handled=False)
        provider_connection_id = business.get("id")
        account_id = (business.get("user") or {}).get("id")
        if not provider_connection_id or account_id is None:
            raise TelegramConnectionWorkflowError("Telegram Business connection is malformed")
        capabilities = {
            "receive_private_messages": bool(business.get("is_enabled")),
            "can_reply": bool((business.get("rights") or {}).get("can_reply")),
        }
        try:
            existing = await self.connection_repo.get_connection_by_provider_connection_id(
                str(provider_connection_id)
            )
            if existing:
                connection = await self.connection_repo.set_business_connection_state(
                    provider_connection_id=str(provider_connection_id),
                    enabled=bool(business.get("is_enabled")),
                    capabilities=capabilities,
                )
            elif bool(business.get("is_enabled")):
                connection = await self.connection_repo.complete_business_connection(
                    telegram_account_id=str(account_id),
                    provider_connection_id=str(provider_connection_id),
                    is_enabled=True,
                    capabilities=capabilities,
                )
            else:
                connection = await self.connection_repo.set_business_connection_state(
                    provider_connection_id=str(provider_connection_id),
                    enabled=False,
                    capabilities=capabilities,
                )
        except ValueError as exc:
            raise TelegramConnectionWorkflowError(str(exc)) from exc
        return TelegramConnectionWebhookResult(
            handled=True,
            connection_id=connection.id if connection else None,
        )
