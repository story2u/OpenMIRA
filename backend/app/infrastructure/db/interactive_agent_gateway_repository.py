from __future__ import annotations

from uuid import UUID

from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import DeviceStatus
from app.infrastructure.db.models import (
    Device,
    InteractiveAgentProviderRequest,
    InteractiveAgentTurn,
)


class InteractiveAgentGatewayRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def lock_turn_owned(
        self,
        *,
        turn_id: UUID,
        owner_user_id: UUID,
        device_id: UUID,
    ) -> InteractiveAgentTurn | None:
        result = await self.session.exec(
            select(InteractiveAgentTurn)
            .where(
                InteractiveAgentTurn.id == turn_id,
                InteractiveAgentTurn.owner_user_id == owner_user_id,
                InteractiveAgentTurn.device_id == device_id,
            )
            .with_for_update()
        )
        return result.first()

    async def active_device_owned(
        self,
        *,
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

    async def add(
        self,
        turn: InteractiveAgentTurn,
        request: InteractiveAgentProviderRequest,
    ) -> None:
        self.session.add_all((turn, request))
        await self.session.commit()
        await self.session.refresh(turn)
        await self.session.refresh(request)

    async def commit(self, request: InteractiveAgentProviderRequest) -> None:
        self.session.add(request)
        await self.session.commit()
        await self.session.refresh(request)

    async def rollback(self) -> None:
        await self.session.rollback()
