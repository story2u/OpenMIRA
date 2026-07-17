from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from sqlalchemy import func
from sqlmodel import col, select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import DeviceStatus, SyncAggregateType
from app.infrastructure.db.models import (
    Device,
    Message,
    Opportunity,
    SyncChange,
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
    utc_now,
)
from app.infrastructure.db.sync_feed import (
    detection_payload,
    message_payload,
    notification_payload,
    opportunity_payload,
    work_schedule_payload,
)

SYNC_SNAPSHOT_TYPE_ORDER = (
    SyncAggregateType.USER_DETECTION_PREFERENCE,
    SyncAggregateType.USER_WORK_SCHEDULE,
    SyncAggregateType.USER_NOTIFICATION_PREFERENCE,
    SyncAggregateType.OPPORTUNITY,
    SyncAggregateType.MESSAGE,
)


@dataclass(frozen=True)
class SyncSnapshotRecord:
    aggregate_type: SyncAggregateType
    aggregate_id: UUID
    aggregate_version: int
    payload: dict


class SyncCursorAheadError(ValueError):
    pass


class SyncDeviceUnavailableError(ValueError):
    pass


class SyncFeedRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def latest_cursor(self, owner_user_id: UUID) -> int:
        value = await self.session.scalar(
            select(func.max(SyncChange.cursor)).where(
                SyncChange.owner_user_id == owner_user_id
            )
        )
        return int(value or 0)

    async def expired_through_cursor(
        self,
        owner_user_id: UUID,
        *,
        retention_cutoff: datetime,
    ) -> int:
        value = await self.session.scalar(
            select(func.max(SyncChange.cursor)).where(
                SyncChange.owner_user_id == owner_user_id,
                SyncChange.created_at < retention_cutoff,
            )
        )
        return int(value or 0)

    async def list_changes(
        self,
        owner_user_id: UUID,
        *,
        after: int,
        retention_cutoff: datetime,
        limit: int,
    ) -> list[SyncChange]:
        result = await self.session.exec(
            select(SyncChange)
            .where(
                SyncChange.owner_user_id == owner_user_id,
                SyncChange.cursor > after,
                SyncChange.created_at >= retention_cutoff,
            )
            .order_by(SyncChange.cursor)
            .limit(limit)
        )
        return list(result.all())

    async def snapshot_page(
        self,
        owner_user_id: UUID,
        *,
        default_timezone: str,
        after_type: SyncAggregateType | None,
        after_id: UUID | None,
        limit: int,
    ) -> list[SyncSnapshotRecord]:
        if (after_type is None) != (after_id is None):
            raise ValueError("snapshot continuation is incomplete")
        start_index = (
            SYNC_SNAPSHOT_TYPE_ORDER.index(after_type) if after_type is not None else 0
        )
        records: list[SyncSnapshotRecord] = []
        for aggregate_type in SYNC_SNAPSHOT_TYPE_ORDER[start_index:]:
            remaining = limit - len(records)
            if remaining <= 0:
                break
            type_after_id = after_id if aggregate_type == after_type else None
            records.extend(
                await self._snapshot_type(
                    owner_user_id,
                    aggregate_type=aggregate_type,
                    default_timezone=default_timezone,
                    after_id=type_after_id,
                    limit=remaining,
                )
            )
        return records

    async def _snapshot_type(
        self,
        owner_user_id: UUID,
        *,
        aggregate_type: SyncAggregateType,
        default_timezone: str,
        after_id: UUID | None,
        limit: int,
    ) -> list[SyncSnapshotRecord]:
        if aggregate_type == SyncAggregateType.USER_DETECTION_PREFERENCE:
            if after_id is not None and owner_user_id.int <= after_id.int:
                return []
            row = (
                await self.session.exec(
                    select(UserDetectionPreference).where(
                        UserDetectionPreference.user_id == owner_user_id
                    )
                )
            ).first()
            return [
                SyncSnapshotRecord(
                    aggregate_type=aggregate_type,
                    aggregate_id=owner_user_id,
                    aggregate_version=row.aggregate_version if row else 0,
                    payload=(
                        detection_payload(row)
                        if row
                        else {"keywords": [], "aiSemanticsEnabled": True}
                    ),
                )
            ]
        if aggregate_type == SyncAggregateType.USER_WORK_SCHEDULE:
            if after_id is not None and owner_user_id.int <= after_id.int:
                return []
            row = (
                await self.session.exec(
                    select(UserWorkSchedule).where(UserWorkSchedule.user_id == owner_user_id)
                )
            ).first()
            return [
                SyncSnapshotRecord(
                    aggregate_type=aggregate_type,
                    aggregate_id=owner_user_id,
                    aggregate_version=row.aggregate_version if row else 0,
                    payload=(
                        work_schedule_payload(row)
                        if row
                        else {
                            "timezone": default_timezone,
                            "slots": [],
                            "autoReplyOutsideHours": True,
                            "isDefault": True,
                        }
                    ),
                )
            ]
        if aggregate_type == SyncAggregateType.USER_NOTIFICATION_PREFERENCE:
            if after_id is not None and owner_user_id.int <= after_id.int:
                return []
            row = (
                await self.session.exec(
                    select(UserNotificationPreference).where(
                        UserNotificationPreference.user_id == owner_user_id
                    )
                )
            ).first()
            return [
                SyncSnapshotRecord(
                    aggregate_type=aggregate_type,
                    aggregate_id=owner_user_id,
                    aggregate_version=row.aggregate_version if row else 0,
                    payload=(
                        notification_payload(row)
                        if row
                        else {
                            "newOpportunityEnabled": True,
                            "aiRepliedEnabled": True,
                            "dailyDigestEnabled": False,
                            "urgentOnly": False,
                        }
                    ),
                )
            ]

        model = Opportunity if aggregate_type == SyncAggregateType.OPPORTUNITY else Message
        owner_column = model.owner_user_id
        statement = select(model).where(owner_column == owner_user_id)
        if after_id is not None:
            statement = statement.where(col(model.id) > after_id)
        rows = list(
            (
                await self.session.exec(
                    statement.order_by(col(model.id)).limit(limit)
                )
            ).all()
        )
        serializer = (
            opportunity_payload
            if aggregate_type == SyncAggregateType.OPPORTUNITY
            else message_payload
        )
        return [
            SyncSnapshotRecord(
                aggregate_type=aggregate_type,
                aggregate_id=row.id,
                aggregate_version=row.aggregate_version,
                payload=serializer(row),
            )
            for row in rows
        ]

    async def acknowledge(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        cursor: int,
        error_code: str | None,
    ) -> Device:
        latest = await self.latest_cursor(owner_user_id)
        if cursor > latest:
            raise SyncCursorAheadError
        result = await self.session.exec(
            select(Device)
            .where(
                Device.id == device_id,
                Device.owner_user_id == owner_user_id,
                Device.status == DeviceStatus.ACTIVE,
            )
            .with_for_update()
        )
        device = result.first()
        if not device:
            await self.session.rollback()
            raise SyncDeviceUnavailableError
        device.last_sync_cursor = max(device.last_sync_cursor, cursor)
        device.last_sync_at = utc_now()
        device.last_sync_error_code = error_code
        device.updated_at = utc_now()
        self.session.add(device)
        await self.session.commit()
        await self.session.refresh(device)
        return device
