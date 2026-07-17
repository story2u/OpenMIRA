from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import DeviceStatus, InteractiveAgentTurnStatus
from app.infrastructure.db.models import Device, InteractiveAgentTurn, User

ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES = (
    InteractiveAgentTurnStatus.CLAIMED,
    InteractiveAgentTurnStatus.RUNNING,
)


class InteractiveAgentTurnRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def lock_owner(self, owner_user_id: UUID) -> User | None:
        return await self.session.get(User, owner_user_id, with_for_update=True)

    async def lock_by_idempotency(
        self,
        *,
        owner_user_id: UUID,
        idempotency_key: str,
    ) -> InteractiveAgentTurn | None:
        result = await self.session.exec(
            select(InteractiveAgentTurn)
            .where(
                InteractiveAgentTurn.owner_user_id == owner_user_id,
                InteractiveAgentTurn.idempotency_key == idempotency_key,
            )
            .with_for_update()
        )
        return result.first()

    async def lock_active_session(
        self,
        *,
        owner_user_id: UUID,
        local_session_id: UUID,
    ) -> InteractiveAgentTurn | None:
        result = await self.session.exec(
            select(InteractiveAgentTurn)
            .where(
                InteractiveAgentTurn.owner_user_id == owner_user_id,
                InteractiveAgentTurn.local_session_id == local_session_id,
                InteractiveAgentTurn.status.in_(ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES),
            )
            .with_for_update()
        )
        return result.first()

    async def lock_owned(
        self,
        *,
        turn_id: UUID,
        owner_user_id: UUID,
        device_id: UUID | None = None,
    ) -> InteractiveAgentTurn | None:
        statement = select(InteractiveAgentTurn).where(
            InteractiveAgentTurn.id == turn_id,
            InteractiveAgentTurn.owner_user_id == owner_user_id,
        )
        if device_id is not None:
            statement = statement.where(InteractiveAgentTurn.device_id == device_id)
        result = await self.session.exec(statement.with_for_update())
        return result.first()

    async def active_device_owned(
        self,
        owner_user_id: UUID,
        device_id: UUID,
    ) -> Device | None:
        result = await self.session.exec(
            select(Device).where(
                Device.id == device_id,
                Device.owner_user_id == owner_user_id,
                Device.status == DeviceStatus.ACTIVE,
                Device.revoked_at.is_(None),
            )
        )
        return result.first()

    async def lock_expired_active(
        self,
        *,
        now: datetime,
        limit: int,
    ) -> list[InteractiveAgentTurn]:
        result = await self.session.exec(
            select(InteractiveAgentTurn)
            .where(
                InteractiveAgentTurn.status.in_(ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES),
                InteractiveAgentTurn.lease_expires_at <= now,
            )
            .order_by(InteractiveAgentTurn.lease_expires_at, InteractiveAgentTurn.id)
            .limit(limit)
            .with_for_update(skip_locked=True)
        )
        return list(result.all())

    async def add(self, turn: InteractiveAgentTurn) -> InteractiveAgentTurn:
        self.session.add(turn)
        await self.session.flush()
        return turn

    def stage(self, *objects: object) -> None:
        self.session.add_all(objects)

    async def commit(self, *objects: object) -> None:
        await self.session.commit()
        for item in objects:
            await self.session.refresh(item)

    async def rollback(self) -> None:
        await self.session.rollback()
