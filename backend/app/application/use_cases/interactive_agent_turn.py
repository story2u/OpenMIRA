from __future__ import annotations

import hmac
from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID, uuid4

from app.core.config import Settings
from app.core.security import (
    create_interactive_agent_turn_token,
    derive_interactive_agent_turn_nonce,
    hash_interactive_agent_turn_nonce,
)
from app.domain.enums import InteractiveAgentTurnStatus, UsageStatus
from app.domain.services.interactive_agent import (
    ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES,
    INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
    InteractiveAgentTurnConflictError,
    InteractiveAgentTurnLeaseExpiredError,
    InteractiveAgentTurnNotFoundError,
    InteractiveAgentTurnQuotaExceededError,
    InteractiveAgentTurnTokenRejectedError,
    InteractiveAgentTurnUnavailableError,
    InteractiveAgentTurnVersionConflictError,
    supports_interactive_agent,
    supports_interactive_agent_contract,
)
from app.infrastructure.db.interactive_agent_repository import (
    InteractiveAgentTurnRepository,
)
from app.infrastructure.db.models import Device, InteractiveAgentTurn, utc_now
from app.infrastructure.db.repositories import SubscriptionRepository


@dataclass(frozen=True, slots=True)
class InteractiveAgentTurnClaim:
    turn: InteractiveAgentTurn
    turn_token: str
    created: bool


@dataclass(frozen=True, slots=True)
class InteractiveAgentTurnTokenPrincipal:
    turn_id: UUID
    owner_user_id: UUID
    device_id: UUID
    nonce: str


class InteractiveAgentRoutingService:
    def __init__(self, *, settings: Settings) -> None:
        self.settings = settings

    def capability_available(self, device: Device) -> bool:
        external_action_gate = (
            self.settings.interactive_agent_schema_version
            < INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION
            or (
                self.settings.interactive_agent_external_actions_enabled
                and self.settings.im_send_enabled
            )
        )
        return bool(
            self.settings.interactive_agent_beta_enabled
            and self.settings.interactive_agent_gateway_enabled
            and self.settings.interactive_agent_beta_monthly_turn_limit > 0
            and self.settings.device_agent_gateway_api_key
            and str(device.id).lower() in self.settings.interactive_agent_device_ids
            and external_action_gate
            and supports_interactive_agent_contract(
                schema_version=self.settings.interactive_agent_schema_version,
                policy_version=self.settings.interactive_agent_policy_version,
            )
            and supports_interactive_agent(
                device.capabilities,
                runtime_version=self.settings.interactive_agent_runtime_version,
                schema_version=self.settings.interactive_agent_schema_version,
            )
        )


class InteractiveAgentTurnService:
    def __init__(
        self,
        *,
        turn_repo: InteractiveAgentTurnRepository,
        subscription_repo: SubscriptionRepository,
        settings: Settings,
        routing_service: InteractiveAgentRoutingService,
    ) -> None:
        self.turn_repo = turn_repo
        self.subscription_repo = subscription_repo
        self.settings = settings
        self.routing_service = routing_service

    async def claim(
        self,
        *,
        owner_user_id: UUID,
        device: Device,
        local_session_id: UUID,
        idempotency_key: str,
    ) -> InteractiveAgentTurnClaim:
        if device.owner_user_id != owner_user_id or not self.routing_service.capability_available(
            device
        ):
            raise InteractiveAgentTurnUnavailableError()
        normalized_key = idempotency_key.strip()
        owner = await self.turn_repo.lock_owner(owner_user_id)
        if not owner or not owner.is_active:
            raise InteractiveAgentTurnUnavailableError()

        existing = await self.turn_repo.lock_by_idempotency(
            owner_user_id=owner_user_id,
            idempotency_key=normalized_key,
        )
        if existing:
            if (
                existing.local_session_id != local_session_id
                or existing.device_id != device.id
                or existing.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES
            ):
                await self.turn_repo.rollback()
                raise InteractiveAgentTurnConflictError()
            if existing.lease_expires_at <= utc_now():
                await self._expire_locked(existing, now=utc_now())
                raise InteractiveAgentTurnLeaseExpiredError()
            await self.turn_repo.commit(existing)
            return InteractiveAgentTurnClaim(
                turn=existing,
                turn_token=self._token_for(existing),
                created=False,
            )

        active = await self.turn_repo.lock_active_session(
            owner_user_id=owner_user_id,
            local_session_id=local_session_id,
        )
        if active:
            if active.lease_expires_at <= utc_now():
                await self._expire_locked(active, now=utc_now())
                raise InteractiveAgentTurnLeaseExpiredError()
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnConflictError()

        now = utc_now()
        reservation = await self.subscription_repo.reserve_interactive_agent_turn_in_transaction(
            user_id=owner_user_id,
            idempotency_key=normalized_key,
            limit=self.settings.interactive_agent_beta_monthly_turn_limit,
            now=now,
        )
        if not reservation.allowed or not reservation.ledger:
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnQuotaExceededError(
                limit=reservation.limit,
                allocated=reservation.allocated,
            )
        if not reservation.created:
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnConflictError()

        turn_id = uuid4()
        nonce = derive_interactive_agent_turn_nonce(
            turn_id=turn_id,
            owner_user_id=owner_user_id,
            device_id=device.id,
            settings=self.settings,
        )
        turn = InteractiveAgentTurn(
            id=turn_id,
            owner_user_id=owner_user_id,
            device_id=device.id,
            local_session_id=local_session_id,
            idempotency_key=normalized_key,
            usage_ledger_id=reservation.ledger.id,
            status=InteractiveAgentTurnStatus.CLAIMED,
            runtime_version=self.settings.interactive_agent_runtime_version,
            schema_version=self.settings.interactive_agent_schema_version,
            model_alias=self.settings.interactive_agent_model_alias,
            policy_version=self.settings.interactive_agent_policy_version,
            lock_version=1,
            request_count=0,
            token_nonce_hash=hash_interactive_agent_turn_nonce(nonce),
            lease_expires_at=now + timedelta(seconds=self.settings.interactive_agent_lease_seconds),
            claimed_at=now,
        )
        await self.turn_repo.add(turn)
        await self.turn_repo.commit(turn, reservation.ledger)
        return InteractiveAgentTurnClaim(
            turn=turn,
            turn_token=self._token_for(turn),
            created=True,
        )

    async def heartbeat(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
        *,
        expected_lock_version: int,
    ) -> InteractiveAgentTurn:
        turn = await self._lock_token_turn(principal)
        if turn.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES:
            raise InteractiveAgentTurnConflictError()
        now = utc_now()
        if turn.lease_expires_at <= now:
            await self._expire_locked(turn, now=now)
            raise InteractiveAgentTurnLeaseExpiredError()
        if expected_lock_version < turn.lock_version:
            await self.turn_repo.commit(turn)
            return turn
        if expected_lock_version != turn.lock_version:
            raise InteractiveAgentTurnVersionConflictError()
        turn.status = InteractiveAgentTurnStatus.RUNNING
        turn.heartbeat_at = now
        turn.lease_expires_at = now + timedelta(
            seconds=self.settings.interactive_agent_lease_seconds
        )
        turn.lock_version += 1
        turn.updated_at = now
        self.turn_repo.stage(turn)
        await self.turn_repo.commit(turn)
        return turn

    async def complete(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
        *,
        expected_lock_version: int,
    ) -> InteractiveAgentTurn:
        turn = await self._lock_token_turn(principal)
        if turn.status == InteractiveAgentTurnStatus.COMPLETED:
            await self.turn_repo.commit(turn)
            return turn
        if turn.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES:
            raise InteractiveAgentTurnConflictError()
        now = utc_now()
        if turn.lease_expires_at <= now:
            await self._expire_locked(turn, now=now)
            raise InteractiveAgentTurnLeaseExpiredError()
        if expected_lock_version != turn.lock_version:
            raise InteractiveAgentTurnVersionConflictError()
        ledger = await self.subscription_repo.consume_usage_in_transaction(turn.usage_ledger_id)
        if not ledger or ledger.status != UsageStatus.CONSUMED:
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnConflictError()
        turn.status = InteractiveAgentTurnStatus.COMPLETED
        turn.completed_at = now
        turn.lock_version += 1
        turn.updated_at = now
        self.turn_repo.stage(turn)
        await self.turn_repo.commit(turn, ledger)
        return turn

    async def fail(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
        *,
        expected_lock_version: int,
        failure_code: str,
    ) -> InteractiveAgentTurn:
        turn = await self._lock_token_turn(principal)
        if turn.status == InteractiveAgentTurnStatus.FAILED:
            if turn.failure_code != failure_code:
                raise InteractiveAgentTurnConflictError()
            await self.turn_repo.commit(turn)
            return turn
        if turn.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES:
            raise InteractiveAgentTurnConflictError()
        now = utc_now()
        if turn.lease_expires_at <= now:
            await self._expire_locked(turn, now=now)
            raise InteractiveAgentTurnLeaseExpiredError()
        if expected_lock_version != turn.lock_version:
            raise InteractiveAgentTurnVersionConflictError()
        ledger = await self.subscription_repo.release_usage_in_transaction(
            turn.usage_ledger_id,
            f"interactive Agent turn failed: {failure_code}",
        )
        if not ledger or ledger.status != UsageStatus.RELEASED:
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnConflictError()
        turn.status = InteractiveAgentTurnStatus.FAILED
        turn.failed_at = now
        turn.failure_code = failure_code
        turn.lock_version += 1
        turn.updated_at = now
        self.turn_repo.stage(turn)
        await self.turn_repo.commit(turn, ledger)
        return turn

    async def expire(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        turn_id: UUID,
    ) -> InteractiveAgentTurn:
        turn = await self.turn_repo.lock_owned(
            turn_id=turn_id,
            owner_user_id=owner_user_id,
            device_id=device_id,
        )
        if not turn:
            raise InteractiveAgentTurnNotFoundError()
        if turn.status == InteractiveAgentTurnStatus.EXPIRED:
            await self.turn_repo.commit(turn)
            return turn
        if turn.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES:
            raise InteractiveAgentTurnConflictError()
        now = utc_now()
        if turn.lease_expires_at > now:
            raise InteractiveAgentTurnConflictError()
        await self._expire_locked(turn, now=now)
        return turn

    async def expire_stale(self, *, limit: int | None = None) -> int:
        maximum = limit or self.settings.interactive_agent_expire_batch_size
        expired = 0
        while expired < maximum:
            now = utc_now()
            turns = await self.turn_repo.lock_expired_active(now=now, limit=1)
            if not turns:
                await self.turn_repo.rollback()
                break
            await self._expire_locked(turns[0], now=now)
            expired += 1
        return expired

    async def _lock_token_turn(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
    ) -> InteractiveAgentTurn:
        turn = await self.turn_repo.lock_owned(
            turn_id=principal.turn_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not turn:
            raise InteractiveAgentTurnNotFoundError()
        try:
            nonce_hash = hash_interactive_agent_turn_nonce(principal.nonce)
        except ValueError as exc:
            raise InteractiveAgentTurnTokenRejectedError() from exc
        if not hmac.compare_digest(turn.token_nonce_hash, nonce_hash):
            raise InteractiveAgentTurnTokenRejectedError()
        device = await self.turn_repo.active_device_owned(
            principal.owner_user_id,
            principal.device_id,
        )
        if not device:
            raise InteractiveAgentTurnTokenRejectedError()
        if not self.routing_service.capability_available(device):
            raise InteractiveAgentTurnUnavailableError()
        return turn

    async def _expire_locked(
        self,
        turn: InteractiveAgentTurn,
        *,
        now: datetime,
    ) -> None:
        ledger = await self.subscription_repo.release_usage_in_transaction(
            turn.usage_ledger_id,
            "interactive Agent turn lease expired",
        )
        if not ledger or ledger.status != UsageStatus.RELEASED:
            await self.turn_repo.rollback()
            raise InteractiveAgentTurnConflictError()
        turn.status = InteractiveAgentTurnStatus.EXPIRED
        turn.expired_at = now
        turn.lock_version += 1
        turn.updated_at = now
        self.turn_repo.stage(turn)
        await self.turn_repo.commit(turn, ledger)

    def _token_for(self, turn: InteractiveAgentTurn) -> str:
        return create_interactive_agent_turn_token(
            turn_id=turn.id,
            owner_user_id=turn.owner_user_id,
            device_id=turn.device_id,
            settings=self.settings,
        )
