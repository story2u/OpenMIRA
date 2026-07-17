from __future__ import annotations

from uuid import UUID

from sqlalchemy import func
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.infrastructure.db.models import (
    Device,
    InteractiveAgentActionApproval,
    InteractiveAgentTurn,
    Opportunity,
)


class InteractiveAgentActionRepository:
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
        from app.domain.enums import DeviceStatus

        result = await self.session.exec(
            select(Device).where(
                Device.id == device_id,
                Device.owner_user_id == owner_user_id,
                Device.status == DeviceStatus.ACTIVE,
            )
        )
        return result.first()

    async def lock_opportunity_owned(
        self,
        *,
        opportunity_id: UUID,
        owner_user_id: UUID,
    ) -> Opportunity | None:
        result = await self.session.exec(
            select(Opportunity)
            .where(
                Opportunity.id == opportunity_id,
                Opportunity.owner_user_id == owner_user_id,
            )
            .with_for_update()
        )
        return result.first()

    async def lock_by_tool_call(
        self,
        *,
        owner_user_id: UUID,
        turn_id: UUID,
        tool_call_id: str,
    ) -> InteractiveAgentActionApproval | None:
        result = await self.session.exec(
            select(InteractiveAgentActionApproval)
            .where(
                InteractiveAgentActionApproval.owner_user_id == owner_user_id,
                InteractiveAgentActionApproval.turn_id == turn_id,
                InteractiveAgentActionApproval.tool_call_id == tool_call_id,
            )
            .with_for_update()
        )
        return result.first()

    async def lock_approval_owned(
        self,
        *,
        approval_id: UUID,
        owner_user_id: UUID,
        device_id: UUID,
        turn_id: UUID,
    ) -> InteractiveAgentActionApproval | None:
        result = await self.session.exec(
            select(InteractiveAgentActionApproval)
            .where(
                InteractiveAgentActionApproval.id == approval_id,
                InteractiveAgentActionApproval.owner_user_id == owner_user_id,
                InteractiveAgentActionApproval.device_id == device_id,
                InteractiveAgentActionApproval.turn_id == turn_id,
            )
            .with_for_update()
        )
        return result.first()

    async def lock_approval(self, approval_id: UUID) -> InteractiveAgentActionApproval | None:
        result = await self.session.exec(
            select(InteractiveAgentActionApproval)
            .where(InteractiveAgentActionApproval.id == approval_id)
            .with_for_update()
        )
        return result.first()

    async def count_for_turn(self, turn_id: UUID) -> int:
        value = await self.session.scalar(
            select(func.count())
            .select_from(InteractiveAgentActionApproval)
            .where(InteractiveAgentActionApproval.turn_id == turn_id)
        )
        return int(value or 0)

    async def add_and_commit(
        self,
        approval: InteractiveAgentActionApproval,
    ) -> InteractiveAgentActionApproval:
        self.session.add(approval)
        await self.session.commit()
        await self.session.refresh(approval)
        return approval

    async def commit(
        self,
        approval: InteractiveAgentActionApproval,
    ) -> InteractiveAgentActionApproval:
        self.session.add(approval)
        await self.session.commit()
        await self.session.refresh(approval)
        return approval

    async def rollback(self) -> None:
        await self.session.rollback()
