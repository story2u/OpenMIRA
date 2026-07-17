from datetime import datetime
from typing import Any, Protocol, Sequence
from uuid import UUID

from sqlalchemy import func
from sqlalchemy.dialects.postgresql import insert
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.infrastructure.db.models import SignalAppetiteEvent


class SignalAppetiteEventInput(Protocol):
    @property
    def eventId(self) -> UUID: ...

    @property
    def eventType(self) -> str: ...

    @property
    def aggregateId(self) -> UUID: ...

    @property
    def aggregateVersion(self) -> int: ...

    @property
    def schemaVersion(self) -> int: ...

    @property
    def payload(self) -> dict[str, Any]: ...

    @property
    def occurredAt(self) -> datetime: ...


class SignalAppetiteEventConflictError(ValueError):
    pass


class SignalAppetiteRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def latest_cursor(self, owner_user_id: UUID) -> int:
        value = await self.session.scalar(
            select(func.max(SignalAppetiteEvent.cursor)).where(
                SignalAppetiteEvent.owner_user_id == owner_user_id
            )
        )
        return int(value or 0)

    async def append(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        events: Sequence[SignalAppetiteEventInput],
    ) -> list[SignalAppetiteEvent]:
        stored: list[SignalAppetiteEvent] = []
        for event in events:
            values = {
                "owner_user_id": owner_user_id,
                "device_id": device_id,
                "event_id": event.eventId,
                "event_type": event.eventType,
                "aggregate_id": event.aggregateId,
                "aggregate_version": event.aggregateVersion,
                "schema_version": event.schemaVersion,
                "payload": event.payload,
                "occurred_at": event.occurredAt,
            }
            cursor = await self.session.scalar(
                insert(SignalAppetiteEvent)
                .values(**values)
                .on_conflict_do_nothing(
                    index_elements=["owner_user_id", "event_id"]
                )
                .returning(SignalAppetiteEvent.cursor)
            )
            row = (
                await self.session.exec(
                    select(SignalAppetiteEvent).where(
                        SignalAppetiteEvent.owner_user_id == owner_user_id,
                        SignalAppetiteEvent.event_id == event.eventId,
                    )
                )
            ).one()
            if cursor is None and not self._matches(row, device_id=device_id, event=event):
                await self.session.rollback()
                raise SignalAppetiteEventConflictError(str(event.eventId))
            stored.append(row)
        await self.session.commit()
        for row in stored:
            await self.session.refresh(row)
        return stored

    async def list_events(
        self,
        owner_user_id: UUID,
        *,
        after: int,
        limit: int,
    ) -> list[SignalAppetiteEvent]:
        result = await self.session.exec(
            select(SignalAppetiteEvent)
            .where(
                SignalAppetiteEvent.owner_user_id == owner_user_id,
                SignalAppetiteEvent.cursor > after,
            )
            .order_by(SignalAppetiteEvent.cursor)
            .limit(limit)
        )
        return list(result.all())

    @staticmethod
    def _matches(
        row: SignalAppetiteEvent,
        *,
        device_id: UUID,
        event: SignalAppetiteEventInput,
    ) -> bool:
        return (
            row.device_id == device_id
            and row.event_type == event.eventType
            and row.aggregate_id == event.aggregateId
            and row.aggregate_version == event.aggregateVersion
            and row.schema_version == event.schemaVersion
            and row.payload == event.payload
            and row.occurred_at == event.occurredAt
        )
