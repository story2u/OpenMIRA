from dataclasses import dataclass
from datetime import timedelta
from uuid import UUID

from fastapi import HTTPException

from app.application.dto import (
    SyncBootstrapRead,
    SyncChangeRead,
    SyncChangesRead,
    SyncSnapshotItemRead,
)
from app.core.config import Settings
from app.core.security import create_signed_token, decode_signed_token
from app.domain.enums import SyncAggregateType
from app.infrastructure.db.models import utc_now
from app.infrastructure.db.sync_repository import (
    SYNC_SNAPSHOT_TYPE_ORDER,
    SyncFeedRepository,
)


class InvalidSyncPageToken(ValueError):
    pass


@dataclass(frozen=True)
class BootstrapContinuation:
    watermark_cursor: int
    after_type: SyncAggregateType
    after_id: UUID


class SyncFeedService:
    def __init__(self, repository: SyncFeedRepository, settings: Settings) -> None:
        self.repository = repository
        self.settings = settings

    def _decode_bootstrap_token(
        self,
        token: str,
        *,
        owner_user_id: UUID,
    ) -> BootstrapContinuation:
        try:
            payload = decode_signed_token(token, self.settings)
            if payload.get("kind") != "sync-bootstrap-v1":
                raise ValueError("wrong token kind")
            if UUID(str(payload["ownerId"])) != owner_user_id:
                raise ValueError("wrong owner")
            watermark = int(payload["watermarkCursor"])
            after_type = SyncAggregateType(str(payload["afterType"]))
            after_id = UUID(str(payload["afterId"]))
            if watermark < 0 or after_type not in SYNC_SNAPSHOT_TYPE_ORDER:
                raise ValueError("invalid continuation")
            return BootstrapContinuation(watermark, after_type, after_id)
        except (HTTPException, KeyError, TypeError, ValueError) as exc:
            raise InvalidSyncPageToken from exc

    def _encode_bootstrap_token(
        self,
        *,
        owner_user_id: UUID,
        watermark_cursor: int,
        after_type: SyncAggregateType,
        after_id: UUID,
    ) -> str:
        return create_signed_token(
            {
                "kind": "sync-bootstrap-v1",
                "ownerId": str(owner_user_id),
                "watermarkCursor": watermark_cursor,
                "afterType": after_type.value,
                "afterId": str(after_id),
            },
            settings=self.settings,
            expires_delta=timedelta(minutes=self.settings.sync_bootstrap_token_minutes),
        )

    async def bootstrap(
        self,
        *,
        owner_user_id: UUID,
        limit: int,
        page_token: str | None,
    ) -> SyncBootstrapRead:
        if page_token:
            continuation = self._decode_bootstrap_token(
                page_token,
                owner_user_id=owner_user_id,
            )
            watermark = continuation.watermark_cursor
            after_type = continuation.after_type
            after_id = continuation.after_id
        else:
            watermark = await self.repository.latest_cursor(owner_user_id)
            after_type = None
            after_id = None

        records = await self.repository.snapshot_page(
            owner_user_id,
            default_timezone=self.settings.default_timezone,
            after_type=after_type,
            after_id=after_id,
            limit=limit + 1,
        )
        has_more = len(records) > limit
        visible = records[:limit]
        next_page_token = None
        if has_more and visible:
            last = visible[-1]
            next_page_token = self._encode_bootstrap_token(
                owner_user_id=owner_user_id,
                watermark_cursor=watermark,
                after_type=last.aggregate_type,
                after_id=last.aggregate_id,
            )
        return SyncBootstrapRead(
            watermarkCursor=watermark,
            items=[
                SyncSnapshotItemRead(
                    aggregateType=record.aggregate_type,
                    aggregateId=record.aggregate_id,
                    aggregateVersion=record.aggregate_version,
                    schemaVersion=1,
                    payload=record.payload,
                )
                for record in visible
            ],
            nextPageToken=next_page_token,
            hasMore=has_more,
        )

    async def changes(
        self,
        *,
        owner_user_id: UUID,
        after: int,
        limit: int,
    ) -> SyncChangesRead:
        retention_cutoff = utc_now() - timedelta(
            days=self.settings.sync_change_retention_days
        )
        latest = await self.repository.latest_cursor(owner_user_id)
        expired_through = await self.repository.expired_through_cursor(
            owner_user_id,
            retention_cutoff=retention_cutoff,
        )
        reset_reason = None
        if after > latest:
            reset_reason = "cursor_ahead"
        elif after < expired_through:
            reset_reason = "cursor_expired"
        if reset_reason:
            return SyncChangesRead(
                changes=[],
                nextCursor=after,
                serverCursor=latest,
                hasMore=False,
                resetRequired=True,
                resetReason=reset_reason,
            )

        changes = await self.repository.list_changes(
            owner_user_id,
            after=after,
            retention_cutoff=retention_cutoff,
            limit=limit + 1,
        )
        has_more = len(changes) > limit
        visible = changes[:limit]
        next_cursor = int(visible[-1].cursor) if visible else after
        return SyncChangesRead(
            changes=[
                SyncChangeRead(
                    eventId=change.id,
                    cursor=int(change.cursor),
                    aggregateType=change.aggregate_type,
                    aggregateId=change.aggregate_id,
                    aggregateVersion=change.aggregate_version,
                    operation=change.operation,
                    schemaVersion=change.schema_version,
                    createdAt=change.created_at,
                    payload=change.payload,
                )
                for change in visible
            ],
            nextCursor=next_cursor,
            serverCursor=latest,
            hasMore=has_more,
        )

    async def acknowledge(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        cursor: int,
        error_code: str | None,
    ):
        return await self.repository.acknowledge(
            owner_user_id=owner_user_id,
            device_id=device_id,
            cursor=cursor,
            error_code=error_code,
        )
