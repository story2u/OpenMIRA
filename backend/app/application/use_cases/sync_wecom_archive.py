import asyncio
from dataclasses import dataclass

from app.domain.enums import IMChannel, MessageSource, WeComSourceType
from app.domain.ports import InboundMessage
from app.infrastructure.db.models import WeComArchiveConnection, WeComArchiveMemberBinding
from app.infrastructure.db.repositories import MessageRepository, WeComArchiveRepository
from app.infrastructure.db.repositories import SubscriptionRepository
from app.domain.services.subscription_policy import GroupQuotaExceeded, ensure_group_quota
from app.infrastructure.im.wecom_archive import (
    WeComArchiveCredentials,
    WeComArchiveMessage,
    WeComArchiveProvider,
)


@dataclass(frozen=True, slots=True)
class WeComArchiveSyncResult:
    fetched: int
    processed: int
    ignored: int
    projected_users: int
    last_sequence: int


class SyncWeComArchive:
    def __init__(
        self,
        *,
        repository: WeComArchiveRepository,
        provider: WeComArchiveProvider,
        ingest_message,
        message_repository: MessageRepository,
        subscription_repository: SubscriptionRepository,
        batch_size: int,
        timeout_seconds: int,
        lease_seconds: int,
    ) -> None:
        self.repository = repository
        self.provider = provider
        self.ingest_message = ingest_message
        self.message_repository = message_repository
        self.subscription_repository = subscription_repository
        self.batch_size = batch_size
        self.timeout_seconds = timeout_seconds
        self.lease_seconds = lease_seconds

    async def execute(
        self,
        connection: WeComArchiveConnection,
        credentials: WeComArchiveCredentials,
        *,
        verifying: bool = False,
    ) -> WeComArchiveSyncResult | None:
        if not connection.enabled:
            raise RuntimeError("WeCom archive connection is disabled")
        cursor = await self.repository.acquire_poll_lease(
            connection.id, lease_seconds=self.lease_seconds
        )
        if not cursor:
            return None
        current_event = None
        try:
            messages = await asyncio.to_thread(
                self.provider.fetch_messages,
                credentials,
                sequence=cursor.last_seq,
                limit=self.batch_size,
                timeout_seconds=self.timeout_seconds,
            )
            messages.sort(key=lambda item: item.sequence)
            last_sequence = cursor.last_seq
            processed = 0
            ignored = 0
            projected_users = 0
            for message in messages:
                last_sequence = max(last_sequence, message.sequence)
                event, should_process = await self.repository.reserve_event(
                    connection_id=connection.id,
                    provider_message_id=message.message_id,
                    sequence=message.sequence,
                    message_type=message.message_type,
                    payload_hash=message.payload_hash,
                )
                current_event = event
                if not should_process:
                    continue
                if not message.is_text:
                    await self.repository.complete_event(
                        event, matched_user_count=0, ignored=True
                    )
                    ignored += 1
                    current_event = None
                    continue
                bindings = await self.repository.active_bindings_for_participants(
                    connection.id, message.participants
                )
                if not bindings:
                    await self.repository.complete_event(
                        event, matched_user_count=0, ignored=True
                    )
                    ignored += 1
                    current_event = None
                    continue
                for binding in bindings:
                    await self._project_message(connection, binding, message)
                    await self.repository.mark_binding_matched(binding)
                    projected_users += 1
                await self.repository.complete_event(
                    event, matched_user_count=len(bindings)
                )
                processed += 1
                current_event = None
            await self.repository.finish_poll(
                connection=connection,
                cursor=cursor,
                last_sequence=last_sequence,
                batch_size=len(messages),
                verified=verifying,
            )
            return WeComArchiveSyncResult(
                fetched=len(messages),
                processed=processed,
                ignored=ignored,
                projected_users=projected_users,
                last_sequence=last_sequence,
            )
        except Exception as exc:
            if current_event is not None:
                await self.repository.fail_event(current_event, exc.__class__.__name__)
            await self.repository.release_poll_failure(
                connection, cursor, f"WeCom archive sync failed: {exc.__class__.__name__}"
            )
            raise

    async def _project_message(
        self,
        connection: WeComArchiveConnection,
        binding: WeComArchiveMemberBinding,
        message: WeComArchiveMessage,
    ) -> None:
        source_type, conversation_key, display_name = self._conversation_for(
            binding, message
        )
        group = source_type != WeComSourceType.PRIVATE
        existing_source = await self.repository.get_archive_source(
            connection_id=connection.id,
            owner_user_id=binding.user_id,
            external_conversation_id=conversation_key,
        )
        quota_paused = False
        quota_reason = None
        if group and not existing_source:
            snapshot = await self.subscription_repository.get_snapshot(binding.user_id)
            telegram_groups, wecom_groups = await self.repository.active_group_counts(
                binding.user_id
            )
            try:
                ensure_group_quota(
                    entitlements=snapshot.entitlements,
                    telegram_groups=telegram_groups,
                    wecom_groups=wecom_groups + 1,
                )
            except GroupQuotaExceeded as exc:
                quota_paused = True
                quota_reason = str(exc)
        source = await self.repository.ensure_archive_source(
            connection_id=connection.id,
            owner_user_id=binding.user_id,
            external_conversation_id=conversation_key,
            display_name=display_name,
            source_type=source_type,
            quota_paused=quota_paused,
            quota_reason=quota_reason,
        )
        if not source.enabled or source.quota_paused:
            return
        external_message_id = (
            f"wecom-archive:{connection.id}:{binding.user_id}:{message.message_id}"
        )
        conversation_id = (
            f"wecom-archive:{connection.id}:{binding.user_id}:{conversation_key}"
        )
        raw_payload = {
            "archive": True,
            "message_type": message.message_type,
            "room_id": message.room_id,
        }
        if message.sender_id == binding.wecom_user_id:
            existing = await self.message_repository.get_by_external_id(
                IMChannel.WECOM, external_message_id
            )
            if not existing:
                await self.message_repository.create_outgoing(
                    owner_user_id=binding.user_id,
                    channel=IMChannel.WECOM,
                    conversation_id=conversation_id,
                    text=message.text or "",
                    source=MessageSource.HUMAN,
                    opportunity_id=None,
                    external_message_id=external_message_id,
                    raw_payload=raw_payload,
                    sender_display_name=binding.display_name,
                )
            return
        await self.ingest_message.execute(
            InboundMessage(
                owner_user_id=binding.user_id,
                channel=IMChannel.WECOM,
                external_message_id=external_message_id,
                conversation_id=conversation_id,
                sender_external_id=message.sender_id or None,
                sender_display_name=message.sender_id or "企业微信成员",
                text=message.text,
                source_type="group" if group else "private",
                group_name=display_name if group else None,
                raw_payload=raw_payload,
                force_human_review=True,
            )
        )

    @staticmethod
    def _conversation_for(
        binding: WeComArchiveMemberBinding,
        message: WeComArchiveMessage,
    ) -> tuple[WeComSourceType, str, str]:
        if message.room_id:
            source_type = (
                WeComSourceType.EXTERNAL_GROUP
                if message.is_external_group
                else WeComSourceType.INTERNAL_GROUP
            )
            return source_type, f"room:{message.room_id}", message.room_id
        counterpart = next(
            (participant for participant in sorted(message.participants) if participant != binding.wecom_user_id),
            message.sender_id or "unknown",
        )
        return WeComSourceType.PRIVATE, f"private:{counterpart}", counterpart
