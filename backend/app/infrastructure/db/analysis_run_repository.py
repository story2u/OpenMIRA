from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import and_, exists
from sqlalchemy.orm import aliased
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    AgentAnalysisStatus,
    AnalysisRunMode,
    AnalysisRunStatus,
    DeviceStatus,
    MessageDirection,
    UsageFeature,
    UsageStatus,
)
from app.infrastructure.db.models import AnalysisRun, Device, Message, UsageLedger, User

ACTIVE_ANALYSIS_RUN_STATUSES = (
    AnalysisRunStatus.CLAIMED,
    AnalysisRunStatus.RUNNING,
)


class AnalysisRunRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def lock_message_owned(self, owner_user_id: UUID, message_id: UUID) -> Message | None:
        result = await self.session.exec(
            select(Message)
            .where(
                Message.id == message_id,
                Message.owner_user_id == owner_user_id,
            )
            .with_for_update()
        )
        return result.first()

    async def lock_active_for_message(
        self,
        owner_user_id: UUID,
        message_id: UUID,
    ) -> AnalysisRun | None:
        result = await self.session.exec(
            select(AnalysisRun)
            .where(
                AnalysisRun.owner_user_id == owner_user_id,
                AnalysisRun.message_id == message_id,
                AnalysisRun.status.in_(ACTIVE_ANALYSIS_RUN_STATUSES),
            )
            .with_for_update()
        )
        return result.first()

    async def lock_shadow_candidate(
        self,
        owner_user_id: UUID,
        *,
        analyzed_after: datetime,
    ) -> tuple[Message, UsageLedger] | None:
        owner = await self.session.get(User, owner_user_id, with_for_update=True)
        if not owner or not owner.is_active:
            return None
        existing_shadow = aliased(AnalysisRun)
        ledger_run = aliased(AnalysisRun)
        result = await self.session.exec(
            select(Message, UsageLedger)
            .join(
                UsageLedger,
                and_(
                    UsageLedger.user_id == Message.owner_user_id,
                    UsageLedger.source_message_id == Message.id,
                    UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
                    UsageLedger.status == UsageStatus.CONSUMED,
                ),
            )
            .outerjoin(
                existing_shadow,
                and_(
                    existing_shadow.message_id == Message.id,
                    existing_shadow.mode == AnalysisRunMode.SHADOW,
                ),
            )
            .where(
                Message.owner_user_id == owner_user_id,
                Message.direction == MessageDirection.INCOMING,
                Message.agent_analysis_status == AgentAnalysisStatus.COMPLETED,
                Message.agent_analyzed_at.is_not(None),
                Message.agent_analyzed_at >= analyzed_after,
                existing_shadow.id.is_(None),
                ~exists(
                    select(ledger_run.id).where(
                        ledger_run.usage_ledger_id == UsageLedger.id
                    )
                ),
            )
            .order_by(Message.agent_analyzed_at, UsageLedger.created_at.desc())
            .limit(1)
            .with_for_update(of=Message, skip_locked=True)
        )
        return result.first()

    async def lock_next_primary_candidate(self, owner_user_id: UUID) -> Message | None:
        bound_run = aliased(AnalysisRun)
        result = await self.session.exec(
            select(Message)
            .join(
                UsageLedger,
                and_(
                    UsageLedger.user_id == Message.owner_user_id,
                    UsageLedger.source_message_id == Message.id,
                    UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
                    UsageLedger.status == UsageStatus.RESERVED,
                ),
            )
            .where(
                Message.owner_user_id == owner_user_id,
                Message.direction == MessageDirection.INCOMING,
                Message.agent_analysis_status == AgentAnalysisStatus.QUEUED,
                ~exists(
                    select(bound_run.id).where(
                        bound_run.usage_ledger_id == UsageLedger.id,
                    )
                ),
            )
            .order_by(Message.updated_at, Message.id)
            .limit(1)
            .with_for_update(of=Message, skip_locked=True)
        )
        return result.first()

    async def lock_reserved_usage_for_message(
        self,
        owner_user_id: UUID,
        message_id: UUID,
    ) -> UsageLedger | None:
        result = await self.session.exec(
            select(UsageLedger)
            .where(
                UsageLedger.user_id == owner_user_id,
                UsageLedger.source_message_id == message_id,
                UsageLedger.feature == UsageFeature.PI_AGENT_ANALYSIS,
                UsageLedger.status == UsageStatus.RESERVED,
            )
            .order_by(UsageLedger.created_at, UsageLedger.id)
            .limit(1)
            .with_for_update(skip_locked=True)
        )
        return result.first()

    async def active_analysis_devices(
        self,
        owner_user_id: UUID,
        *,
        seen_after: datetime,
        limit: int,
    ) -> list[Device]:
        result = await self.session.exec(
            select(Device)
            .where(
                Device.owner_user_id == owner_user_id,
                Device.status == DeviceStatus.ACTIVE,
                Device.revoked_at.is_(None),
                Device.last_seen_at >= seen_after,
            )
            .order_by(Device.last_seen_at.desc(), Device.id)
            .limit(limit)
        )
        return list(result.all())

    async def shadow_rollout_samples(
        self,
        *,
        claimed_after: datetime,
        runtime_version: str,
        schema_version: int,
        model_alias: str,
        policy_version: str,
        limit: int,
    ) -> list[AnalysisRun]:
        result = await self.session.exec(
            select(AnalysisRun)
            .where(
                AnalysisRun.mode == AnalysisRunMode.SHADOW,
                AnalysisRun.claimed_at >= claimed_after,
                AnalysisRun.runtime_version == runtime_version,
                AnalysisRun.schema_version == schema_version,
                AnalysisRun.model_alias == model_alias,
                AnalysisRun.policy_version == policy_version,
                AnalysisRun.status.in_(
                    (
                        AnalysisRunStatus.COMPLETED,
                        AnalysisRunStatus.FAILED,
                        AnalysisRunStatus.EXPIRED,
                    )
                ),
            )
            .order_by(AnalysisRun.claimed_at.desc(), AnalysisRun.id)
            .limit(limit)
        )
        return list(result.all())

    async def lock_expired_active(
        self,
        *,
        now: datetime,
        limit: int,
    ) -> list[AnalysisRun]:
        result = await self.session.exec(
            select(AnalysisRun)
            .where(
                AnalysisRun.status.in_(ACTIVE_ANALYSIS_RUN_STATUSES),
                AnalysisRun.lease_expires_at <= now,
            )
            .order_by(AnalysisRun.lease_expires_at, AnalysisRun.id)
            .limit(limit)
            .with_for_update(skip_locked=True)
        )
        return list(result.all())

    async def lock_usage_ledger(self, ledger_id: UUID) -> UsageLedger | None:
        return await self.session.get(UsageLedger, ledger_id, with_for_update=True)

    async def lock_owned(
        self,
        *,
        run_id: UUID,
        owner_user_id: UUID,
        device_id: UUID | None = None,
    ) -> AnalysisRun | None:
        statement = select(AnalysisRun).where(
            AnalysisRun.id == run_id,
            AnalysisRun.owner_user_id == owner_user_id,
        )
        if device_id is not None:
            statement = statement.where(AnalysisRun.device_id == device_id)
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

    async def add(self, run: AnalysisRun) -> AnalysisRun:
        self.session.add(run)
        await self.session.flush()
        return run

    def stage(self, *objects: object) -> None:
        self.session.add_all(objects)

    async def commit(self, *objects: object) -> None:
        await self.session.commit()
        for item in objects:
            await self.session.refresh(item)

    async def refresh(self, *objects: object) -> None:
        for item in objects:
            await self.session.refresh(item)

    async def rollback(self) -> None:
        await self.session.rollback()
