from __future__ import annotations

from uuid import UUID

from sqlalchemy import func
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import DeviceStatus
from app.infrastructure.db.models import AnalysisProviderRequest, AnalysisRun, Device


class AnalysisGatewayRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def lock_run_owned(
        self,
        *,
        run_id: UUID,
        owner_user_id: UUID,
        device_id: UUID,
    ) -> AnalysisRun | None:
        result = await self.session.exec(
            select(AnalysisRun)
            .where(
                AnalysisRun.id == run_id,
                AnalysisRun.owner_user_id == owner_user_id,
                AnalysisRun.device_id == device_id,
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

    async def request_count(self, run_id: UUID) -> int:
        result = await self.session.exec(
            select(func.count()).select_from(AnalysisProviderRequest).where(
                AnalysisProviderRequest.run_id == run_id
            )
        )
        return int(result.one())

    async def add(self, request: AnalysisProviderRequest) -> None:
        self.session.add(request)
        await self.session.commit()
        await self.session.refresh(request)

    async def commit(self, request: AnalysisProviderRequest) -> None:
        self.session.add(request)
        await self.session.commit()
        await self.session.refresh(request)

    async def rollback(self) -> None:
        await self.session.rollback()
