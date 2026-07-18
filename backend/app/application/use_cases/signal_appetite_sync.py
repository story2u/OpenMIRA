from uuid import UUID

from app.application.dto import (
    SignalAppetiteEventRead,
    SignalAppetiteEventsAppendRead,
    SignalAppetiteEventsPageRead,
    SignalAppetiteEventWrite,
)
from app.infrastructure.db.models import SignalAppetiteEvent
from app.infrastructure.db.signal_appetite_repository import SignalAppetiteRepository


def _read(event: SignalAppetiteEvent) -> SignalAppetiteEventRead:
    assert event.cursor is not None
    return SignalAppetiteEventRead(
        eventId=event.event_id,
        eventType=event.event_type,
        aggregateId=event.aggregate_id,
        aggregateVersion=event.aggregate_version,
        schemaVersion=1,
        occurredAt=event.occurred_at,
        payload=event.payload,
        ownerId=event.owner_user_id,
        deviceId=event.device_id,
        cursor=int(event.cursor),
        serverReceivedAt=event.created_at,
    )


class SignalAppetiteSyncService:
    def __init__(self, repository: SignalAppetiteRepository) -> None:
        self.repository = repository

    async def append(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        events: list[SignalAppetiteEventWrite],
    ) -> SignalAppetiteEventsAppendRead:
        rows = await self.repository.append(
            owner_user_id=owner_user_id,
            device_id=device_id,
            events=events,
        )
        return SignalAppetiteEventsAppendRead(
            events=[_read(row) for row in rows],
            serverCursor=await self.repository.latest_cursor(owner_user_id),
        )

    async def list_events(
        self,
        *,
        owner_user_id: UUID,
        after: int,
        limit: int,
    ) -> SignalAppetiteEventsPageRead:
        rows = await self.repository.list_events(
            owner_user_id,
            after=after,
            limit=limit + 1,
        )
        has_more = len(rows) > limit
        visible = rows[:limit]
        server_cursor = await self.repository.latest_cursor(owner_user_id)
        next_cursor = int(visible[-1].cursor) if visible else after
        return SignalAppetiteEventsPageRead(
            events=[_read(row) for row in visible],
            nextCursor=next_cursor,
            serverCursor=server_cursor,
            hasMore=has_more,
        )
