from __future__ import annotations

import hmac
import math
from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID, uuid4

from app.application.use_cases.analyze_message import message_links
from app.application.use_cases.schedule_agent_analysis import ScheduleAgentAnalysisUseCase
from app.core.config import Settings
from app.core.security import (
    create_analysis_run_token,
    derive_analysis_run_nonce,
    hash_analysis_run_nonce,
)
from app.domain.enums import (
    AgentAnalysisStatus,
    AnalysisRunExecutor,
    AnalysisRunMode,
    AnalysisRunStatus,
    DeviceStatus,
    OpportunityStatus,
    UsageStatus,
)
from app.domain.ports import (
    AgentAnalysisResult,
    AgentExecutionMetadata,
    LinkInspection,
    LinkInspector,
    TaskQueue,
)
from app.domain.services.agent_policy import project_agent_result
from app.domain.services.analysis_run import (
    ACTIVE_ANALYSIS_RUN_STATUSES,
    AnalysisRunConflictError,
    AnalysisRunLeaseExpiredError,
    AnalysisRunNotFoundError,
    AnalysisRunQuotaExceededError,
    AnalysisRunTokenRejectedError,
    AnalysisRunUnavailableError,
    AnalysisRunVersionConflictError,
    AnalysisRolloutEvidence,
    AnalysisRolloutReadiness,
    count_shadow_differences,
    device_in_analysis_rollout,
    evaluate_analysis_rollout_readiness,
    message_can_be_claimed,
    supports_device_analysis,
)
from app.infrastructure.db.analysis_run_repository import AnalysisRunRepository
from app.infrastructure.db.models import AnalysisRun, Device, Message, Opportunity, utc_now
from app.infrastructure.db.repositories import (
    MessageRepository,
    OpportunityRepository,
    SubscriptionRepository,
)


@dataclass(frozen=True, slots=True)
class AnalysisRunClaim:
    run: AnalysisRun
    run_token: str
    message: Message


@dataclass(frozen=True, slots=True)
class AnalysisRunTokenPrincipal:
    run_id: UUID
    owner_user_id: UUID
    device_id: UUID
    nonce: str


class DeviceAgentRoutingService:
    """Fail-closed rollout policy shared by capability, scheduling, and claim paths."""

    def __init__(self, *, run_repo: AnalysisRunRepository, settings: Settings) -> None:
        self.run_repo = run_repo
        self.settings = settings

    @property
    def primary_claim_window_seconds(self) -> int:
        return self.settings.device_agent_primary_claim_window_seconds

    def transport_available(self, device: Device) -> bool:
        return (
            self.settings.device_agent_gateway_enabled
            and bool(self.settings.device_agent_gateway_api_key)
            and device.status == DeviceStatus.ACTIVE
            and device.revoked_at is None
            and supports_device_analysis(
                device.capabilities,
                runtime_version=self.settings.device_agent_runtime_version,
                schema_version=self.settings.device_agent_schema_version,
            )
        )

    def shadow_available(self, device: Device) -> bool:
        return self.settings.device_agent_shadow_enabled and self.transport_available(device)

    def device_selected(self, device: Device) -> bool:
        return device_in_analysis_rollout(
            owner_user_id=device.owner_user_id,
            device_id=device.id,
            percentage=self.settings.rn_device_agent_rollout_percentage,
            allowlist=self.settings.device_agent_rollout_device_ids,
        )

    async def rollout_readiness(self) -> AnalysisRolloutReadiness:
        now = utc_now()
        samples = await self.run_repo.shadow_rollout_samples(
            claimed_after=now
            - timedelta(hours=self.settings.device_agent_rollout_lookback_hours),
            runtime_version=self.settings.device_agent_runtime_version,
            schema_version=self.settings.device_agent_schema_version,
            model_alias=self.settings.device_agent_model_alias,
            policy_version=self.settings.device_agent_policy_version,
            limit=10_000,
        )
        durations: list[float] = []
        completed = 0
        matched = 0
        for run in samples:
            terminal_at = run.completed_at or run.failed_at or run.expired_at
            if terminal_at is not None:
                durations.append(max(0.0, (terminal_at - run.claimed_at).total_seconds()))
            if run.status == AnalysisRunStatus.COMPLETED:
                completed += 1
                if run.shadow_match is True:
                    matched += 1
        p95_seconds: float | None = None
        if durations:
            durations.sort()
            p95_seconds = durations[max(0, math.ceil(len(durations) * 0.95) - 1)]
        evidence = AnalysisRolloutEvidence(
            terminal_samples=len(samples),
            completed_samples=completed,
            matched_samples=matched,
            p95_seconds=p95_seconds,
        )
        return evaluate_analysis_rollout_readiness(
            evidence,
            minimum_samples=self.settings.device_agent_rollout_min_shadow_samples,
            minimum_success_rate=self.settings.device_agent_rollout_min_shadow_success_rate,
            minimum_match_rate=self.settings.device_agent_rollout_min_shadow_match_rate,
            maximum_p95_seconds=self.settings.device_agent_rollout_max_p95_seconds,
        )

    def primary_configuration_reasons(self) -> tuple[str, ...]:
        reasons: list[str] = []
        if not self.settings.rn_device_agent_rollout_enabled:
            reasons.append("primary_rollout_disabled")
        if not self.settings.rn_sync_rollout_enabled:
            reasons.append("sync_rollout_disabled")
        if not (
            self.settings.rn_device_agent_rollout_percentage > 0
            or bool(self.settings.device_agent_rollout_device_ids)
        ):
            reasons.append("no_rollout_cohort")
        if not self.settings.device_agent_fallback_enabled:
            reasons.append("server_fallback_disabled")
        if not (
            self.settings.pi_agent_enabled and bool(self.settings.effective_pi_agent_api_key)
        ):
            reasons.append("server_pi_unavailable")
        if not (
            self.settings.device_agent_gateway_enabled
            and bool(self.settings.device_agent_gateway_api_key)
        ):
            reasons.append("device_gateway_unavailable")
        return tuple(reasons)

    def primary_gate_reasons(
        self,
        readiness: AnalysisRolloutReadiness,
    ) -> tuple[str, ...]:
        reasons = list(self.primary_configuration_reasons())
        if self.settings.device_agent_rollout_require_shadow_ready:
            reasons.extend(readiness.reasons)
        return tuple(reasons)

    async def primary_globally_ready(
        self,
        *,
        readiness: AnalysisRolloutReadiness | None = None,
    ) -> bool:
        if self.primary_configuration_reasons():
            return False
        if not self.settings.device_agent_rollout_require_shadow_ready:
            return True
        return (readiness or await self.rollout_readiness()).ready

    async def primary_available(self, device: Device) -> bool:
        return (
            self.transport_available(device)
            and self.device_selected(device)
            and await self.primary_globally_ready()
        )

    async def capability_available(self, device: Device) -> bool:
        return self.shadow_available(device) or await self.primary_available(device)

    async def has_primary_device(self, owner_user_id: UUID) -> bool:
        if not await self.primary_globally_ready():
            return False
        devices = await self.run_repo.active_analysis_devices(
            owner_user_id,
            seen_after=utc_now()
            - timedelta(minutes=self.settings.device_agent_recent_device_minutes),
            limit=self.settings.device_max_active_per_user,
        )
        return any(
            self.transport_available(device) and self.device_selected(device)
            for device in devices
        )


class AnalysisRunService:
    def __init__(
        self,
        *,
        run_repo: AnalysisRunRepository,
        subscription_repo: SubscriptionRepository,
        message_repo: MessageRepository,
        opportunity_repo: OpportunityRepository,
        task_queue: TaskQueue,
        settings: Settings,
        routing_service: DeviceAgentRoutingService | None = None,
    ) -> None:
        self.run_repo = run_repo
        self.subscription_repo = subscription_repo
        self.message_repo = message_repo
        self.opportunity_repo = opportunity_repo
        self.task_queue = task_queue
        self.settings = settings
        self.routing_service = routing_service or DeviceAgentRoutingService(
            run_repo=run_repo,
            settings=settings,
        )

    async def device_available(self, device: Device) -> bool:
        return await self.routing_service.primary_available(device)

    def shadow_available(self, device: Device) -> bool:
        return self.routing_service.shadow_available(device)

    async def claim(
        self,
        *,
        owner_user_id: UUID,
        device: Device,
        message_id: UUID,
    ) -> AnalysisRunClaim:
        if device.owner_user_id != owner_user_id or not await self.device_available(device):
            raise AnalysisRunUnavailableError()
        now = utc_now()
        message = await self.run_repo.lock_message_owned(owner_user_id, message_id)
        if not message:
            raise AnalysisRunNotFoundError()
        existing = await self.run_repo.lock_active_for_message(owner_user_id, message_id)
        if existing:
            if existing.lease_expires_at <= now:
                await self._expire_locked(existing, message, now=now)
            elif existing.device_id != device.id:
                raise AnalysisRunConflictError()
            else:
                self._verify_derived_nonce(existing)
                await self.run_repo.commit(existing, message)
                return AnalysisRunClaim(
                    run=existing,
                    run_token=self._token_for(existing),
                    message=message,
                )
        if not message_can_be_claimed(
            direction=message.direction,
            status=message.agent_analysis_status,
        ):
            await self.run_repo.rollback()
            raise AnalysisRunConflictError()

        run_id = uuid4()
        reserved_ledger = await self.run_repo.lock_reserved_usage_for_message(
            owner_user_id,
            message.id,
        )
        if reserved_ledger is None:
            reservation = await self.subscription_repo.reserve_agent_analysis_in_transaction(
                user_id=owner_user_id,
                message_id=message.id,
                idempotency_key=f"analysis-run:{run_id}",
                now=now,
            )
            if not reservation.allowed or not reservation.ledger:
                await self.run_repo.rollback()
                raise AnalysisRunQuotaExceededError(
                    limit=reservation.limit,
                    allocated=reservation.allocated,
                )
            reserved_ledger = reservation.ledger
        nonce = derive_analysis_run_nonce(
            run_id=run_id,
            owner_user_id=owner_user_id,
            device_id=device.id,
            settings=self.settings,
        )
        run = AnalysisRun(
            id=run_id,
            owner_user_id=owner_user_id,
            message_id=message.id,
            device_id=device.id,
            usage_ledger_id=reserved_ledger.id,
            status=AnalysisRunStatus.CLAIMED,
            executor=AnalysisRunExecutor.DEVICE,
            mode=AnalysisRunMode.PRIMARY,
            runtime_version=self.settings.device_agent_runtime_version,
            schema_version=self.settings.device_agent_schema_version,
            model_alias=self.settings.device_agent_model_alias,
            policy_version=self.settings.device_agent_policy_version,
            source_message_version=message.aggregate_version,
            lock_version=1,
            token_nonce_hash=hash_analysis_run_nonce(nonce),
            lease_expires_at=now + timedelta(seconds=self.settings.device_agent_lease_seconds),
            claimed_at=now,
        )
        message.agent_analysis_status = AgentAnalysisStatus.RUNNING
        message.agent_started_at = now
        message.agent_error = None
        message.updated_at = now
        self.run_repo.stage(message)
        if message.opportunity_id:
            opportunity = await self.opportunity_repo.get(message.opportunity_id)
            if opportunity and opportunity.owner_user_id == owner_user_id:
                opportunity.agent_analysis_status = AgentAnalysisStatus.RUNNING
                opportunity.agent_analysis_error = None
                opportunity.sop_stage = "analyzing"
                opportunity.updated_at = now
                self.run_repo.stage(opportunity)
        await self.run_repo.add(run)
        # The claim itself advances the sync-visible Message aggregate version. Bind the
        # run to that committed input version after the flush, not the pre-claim value.
        run.source_message_version = message.aggregate_version
        self.run_repo.stage(run)
        await self.run_repo.commit(run, message, reserved_ledger)
        return AnalysisRunClaim(run=run, run_token=self._token_for(run), message=message)

    async def claim_next(
        self,
        *,
        owner_user_id: UUID,
        device: Device,
    ) -> AnalysisRunClaim | None:
        if device.owner_user_id != owner_user_id or not await self.device_available(device):
            await self.run_repo.rollback()
            return None
        candidate = await self.run_repo.lock_next_primary_candidate(owner_user_id)
        if candidate is None:
            await self.run_repo.rollback()
            return None
        return await self.claim(
            owner_user_id=owner_user_id,
            device=device,
            message_id=candidate.id,
        )

    async def claim_shadow(
        self,
        *,
        owner_user_id: UUID,
        device: Device,
    ) -> AnalysisRunClaim | None:
        if device.owner_user_id != owner_user_id or not self.routing_service.transport_available(
            device
        ):
            raise AnalysisRunUnavailableError()
        if not self.settings.device_agent_shadow_enabled:
            await self.run_repo.rollback()
            return None
        now = utc_now()
        candidate = await self.run_repo.lock_shadow_candidate(
            owner_user_id,
            analyzed_after=now
            - timedelta(hours=self.settings.device_agent_shadow_lookback_hours),
        )
        if not candidate:
            await self.run_repo.rollback()
            return None
        message, ledger = candidate
        if ledger.status != UsageStatus.CONSUMED or not message.agent_result:
            await self.run_repo.rollback()
            raise AnalysisRunConflictError()
        run_id = uuid4()
        nonce = derive_analysis_run_nonce(
            run_id=run_id,
            owner_user_id=owner_user_id,
            device_id=device.id,
            settings=self.settings,
        )
        run = AnalysisRun(
            id=run_id,
            owner_user_id=owner_user_id,
            message_id=message.id,
            device_id=device.id,
            usage_ledger_id=ledger.id,
            status=AnalysisRunStatus.CLAIMED,
            executor=AnalysisRunExecutor.DEVICE,
            mode=AnalysisRunMode.SHADOW,
            runtime_version=self.settings.device_agent_runtime_version,
            schema_version=self.settings.device_agent_schema_version,
            model_alias=self.settings.device_agent_model_alias,
            policy_version=self.settings.device_agent_policy_version,
            source_message_version=message.aggregate_version,
            lock_version=1,
            token_nonce_hash=hash_analysis_run_nonce(nonce),
            lease_expires_at=now + timedelta(seconds=self.settings.device_agent_lease_seconds),
            claimed_at=now,
        )
        await self.run_repo.add(run)
        await self.run_repo.commit(run, message, ledger)
        return AnalysisRunClaim(run=run, run_token=self._token_for(run), message=message)

    async def heartbeat(
        self,
        principal: AnalysisRunTokenPrincipal,
        *,
        expected_lock_version: int,
    ) -> AnalysisRun:
        run = await self._lock_token_run(principal)
        if run.status not in ACTIVE_ANALYSIS_RUN_STATUSES:
            raise AnalysisRunConflictError()
        now = utc_now()
        if run.lease_expires_at <= now:
            message = await self._locked_run_message(run)
            await self._expire_locked(run, message, now=now)
            await self._schedule_fallback(run, message)
            raise AnalysisRunLeaseExpiredError()
        if expected_lock_version < run.lock_version:
            await self.run_repo.commit(run)
            return run
        if expected_lock_version != run.lock_version:
            raise AnalysisRunVersionConflictError()
        run.status = AnalysisRunStatus.RUNNING
        run.heartbeat_at = now
        run.lease_expires_at = now + timedelta(seconds=self.settings.device_agent_lease_seconds)
        run.lock_version += 1
        run.updated_at = now
        self.run_repo.stage(run)
        await self.run_repo.commit(run)
        return run

    async def complete(
        self,
        principal: AnalysisRunTokenPrincipal,
        *,
        expected_lock_version: int,
        result: AgentAnalysisResult,
    ) -> AnalysisRun:
        run = await self._lock_token_run(principal)
        serialized = result.model_dump(mode="json")
        if run.status == AnalysisRunStatus.COMPLETED:
            if run.result != serialized:
                raise AnalysisRunConflictError()
            await self.run_repo.commit(run)
            return run
        if run.status not in ACTIVE_ANALYSIS_RUN_STATUSES:
            raise AnalysisRunConflictError()
        now = utc_now()
        if run.lease_expires_at <= now:
            message = await self._locked_run_message(run)
            await self._expire_locked(run, message, now=now)
            await self._schedule_fallback(run, message)
            raise AnalysisRunLeaseExpiredError()
        if expected_lock_version != run.lock_version:
            raise AnalysisRunVersionConflictError()

        message = await self._locked_run_message(run)
        if message.aggregate_version != run.source_message_version:
            raise AnalysisRunConflictError()
        candidate_links = message_links(
            message.text or "",
            message.raw_message_links,
            limit=self.settings.pi_agent_max_links,
        )
        if candidate_links and run.link_evidence is None:
            raise AnalysisRunConflictError()
        inspections = [
            LinkInspection.model_validate(item) for item in (run.link_evidence or [])
        ]
        projection = project_agent_result(result, inspections, analyzed_at=now)
        if run.mode == AnalysisRunMode.SHADOW:
            ledger = await self.run_repo.lock_usage_ledger(run.usage_ledger_id)
            if (
                not ledger
                or ledger.status != UsageStatus.CONSUMED
                or message.agent_analysis_status != AgentAnalysisStatus.COMPLETED
                or not message.agent_result
            ):
                await self.run_repo.rollback()
                raise AnalysisRunConflictError()
            baseline_time = message.agent_analyzed_at or now
            candidate_projection = project_agent_result(
                result,
                inspections,
                analyzed_at=baseline_time,
            ).model_dump(mode="json")
            difference_count = count_shadow_differences(
                message.agent_result,
                candidate_projection,
            )
            run.status = AnalysisRunStatus.COMPLETED
            run.completed_at = now
            run.result = serialized
            run.shadow_match = difference_count == 0
            run.shadow_difference_count = difference_count
            run.lock_version += 1
            run.updated_at = now
            self.run_repo.stage(run)
            await self.run_repo.commit(run, ledger)
            return run
        opportunity = await self._owned_opportunity(message, run.owner_user_id)
        created_opportunity = False
        if (
            not opportunity
            and result.is_opportunity
            and result.confidence >= self.settings.pi_agent_min_opportunity_confidence
        ):
            opportunity = await self.opportunity_repo.create(
                channel=message.channel,
                owner_user_id=run.owner_user_id,
                conversation_id=message.conversation_id,
                customer_external_id=message.sender_external_id,
                contact_name=message.sender_display_name,
                source_type=message.source_type,
                group_name=message.group_name,
                source_message_id=message.id,
                title=result.title,
                summary=result.summary,
                matched_keywords=[],
                raw_message_links=message.raw_message_links,
                confidence=result.confidence,
                priority=result.priority,
                detection_reason="device pi agent post-processing",
                status=OpportunityStatus.PENDING_HUMAN,
                last_message_preview=message.text or "",
                commit=False,
            )
            message.opportunity_id = opportunity.id
            message.processed_at = now
            created_opportunity = True
        if opportunity:
            await self.opportunity_repo.apply_agent_projection(
                opportunity,
                projection,
                commit=False,
            )
        await self.message_repo.complete_agent_analysis(
            message,
            projection,
            execution=AgentExecutionMetadata(
                executed_by=AnalysisRunExecutor.DEVICE,
                run_id=run.id,
                device_id=run.device_id,
                runtime_version=run.runtime_version,
                schema_version=run.schema_version,
                model_version=run.model_alias,
                policy_version=run.policy_version,
            ),
            commit=False,
        )
        ledger = await self.subscription_repo.consume_usage_in_transaction(
            run.usage_ledger_id
        )
        if not ledger or ledger.status != UsageStatus.CONSUMED:
            await self.run_repo.rollback()
            raise AnalysisRunConflictError()
        run.status = AnalysisRunStatus.COMPLETED
        run.completed_at = now
        run.result = serialized
        run.lock_version += 1
        run.updated_at = now
        self.run_repo.stage(run)
        refresh: list[object] = [run, message, ledger]
        if opportunity:
            refresh.append(opportunity)
        await self.run_repo.commit(*refresh)
        if created_opportunity and opportunity:
            self.task_queue.notify_reviewers(opportunity.id)
        return run

    async def fail(
        self,
        principal: AnalysisRunTokenPrincipal,
        *,
        expected_lock_version: int,
        failure_code: str,
    ) -> AnalysisRun:
        run = await self._lock_token_run(principal)
        if run.status == AnalysisRunStatus.FAILED:
            if run.failure_code != failure_code:
                raise AnalysisRunConflictError()
            await self.run_repo.commit(run)
            return run
        if run.status not in ACTIVE_ANALYSIS_RUN_STATUSES:
            raise AnalysisRunConflictError()
        if expected_lock_version != run.lock_version:
            raise AnalysisRunVersionConflictError()
        now = utc_now()
        if run.lease_expires_at <= now:
            message = await self._locked_run_message(run)
            await self._expire_locked(run, message, now=now)
            await self._schedule_fallback(run, message)
            raise AnalysisRunLeaseExpiredError()
        message = await self._locked_run_message(run)
        if run.mode == AnalysisRunMode.SHADOW:
            run.status = AnalysisRunStatus.FAILED
            run.failed_at = now
            run.failure_code = failure_code
            run.lock_version += 1
            run.updated_at = now
            self.run_repo.stage(run)
            await self.run_repo.commit(run)
            return run
        run.status = AnalysisRunStatus.FAILED
        run.failed_at = now
        run.failure_code = failure_code
        run.lock_version += 1
        run.updated_at = now
        self._mark_message_failed(message, failure_code, now=now)
        await self._mark_opportunity_analysis_status(
            message,
            run.owner_user_id,
            status=AgentAnalysisStatus.FAILED,
            error=f"device analysis failed: {failure_code}",
            now=now,
        )
        ledger = await self.subscription_repo.release_usage_in_transaction(
            run.usage_ledger_id,
            f"device analysis failed: {failure_code}",
        )
        if not ledger or ledger.status != UsageStatus.RELEASED:
            await self.run_repo.rollback()
            raise AnalysisRunConflictError()
        self.run_repo.stage(run, message)
        await self.run_repo.commit(run, message, ledger)
        await self._schedule_fallback(run, message)
        return run

    async def expire(
        self,
        *,
        owner_user_id: UUID,
        device_id: UUID,
        run_id: UUID,
    ) -> AnalysisRun:
        run = await self.run_repo.lock_owned(
            run_id=run_id,
            owner_user_id=owner_user_id,
            device_id=device_id,
        )
        if not run:
            raise AnalysisRunNotFoundError()
        if run.status == AnalysisRunStatus.EXPIRED:
            await self.run_repo.commit(run)
            return run
        if run.status not in ACTIVE_ANALYSIS_RUN_STATUSES:
            raise AnalysisRunConflictError()
        now = utc_now()
        if run.lease_expires_at > now:
            raise AnalysisRunConflictError()
        message = await self._locked_run_message(run)
        await self._expire_locked(run, message, now=now)
        await self._schedule_fallback(run, message)
        return run

    async def expire_stale(self, *, limit: int | None = None) -> int:
        maximum = limit or self.settings.device_agent_expire_batch_size
        expired = 0
        while expired < maximum:
            now = utc_now()
            runs = await self.run_repo.lock_expired_active(now=now, limit=1)
            if not runs:
                await self.run_repo.rollback()
                break
            run = runs[0]
            message = await self._locked_run_message(run)
            await self._expire_locked(run, message, now=now)
            await self._schedule_fallback(run, message)
            expired += 1
        return expired

    async def inspect_links(
        self,
        principal: AnalysisRunTokenPrincipal,
        *,
        inspector: LinkInspector,
    ) -> tuple[AnalysisRun, list[LinkInspection]]:
        run = await self._lock_token_run(principal)
        message = await self._locked_run_message(run)
        self._ensure_active_lease(run)
        if message.aggregate_version != run.source_message_version:
            raise AnalysisRunConflictError()
        if run.link_evidence is not None:
            inspections = [LinkInspection.model_validate(item) for item in run.link_evidence]
            await self.run_repo.commit(run, message)
            return run, inspections
        urls = message_links(
            message.text or "",
            message.raw_message_links,
            limit=self.settings.pi_agent_max_links,
        )
        await self.run_repo.commit(run, message)
        inspections = await inspector.inspect_many(urls)

        run = await self._lock_token_run(principal)
        message = await self._locked_run_message(run)
        self._ensure_active_lease(run)
        if message.aggregate_version != run.source_message_version:
            raise AnalysisRunConflictError()
        if run.link_evidence is None:
            run.link_evidence = [item.model_dump(mode="json") for item in inspections]
            run.link_evidence_fetched_at = utc_now()
            run.updated_at = run.link_evidence_fetched_at
            self.run_repo.stage(run)
            await self.run_repo.commit(run, message)
            return run, inspections
        cached = [LinkInspection.model_validate(item) for item in run.link_evidence]
        await self.run_repo.commit(run, message)
        return run, cached

    async def _lock_token_run(self, principal: AnalysisRunTokenPrincipal) -> AnalysisRun:
        run = await self.run_repo.lock_owned(
            run_id=principal.run_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not run:
            raise AnalysisRunNotFoundError()
        if not await self.run_repo.active_device_owned(
            principal.owner_user_id,
            principal.device_id,
        ):
            raise AnalysisRunTokenRejectedError()
        if not hmac.compare_digest(
            run.token_nonce_hash,
            hash_analysis_run_nonce(principal.nonce),
        ):
            raise AnalysisRunTokenRejectedError()
        return run

    def _ensure_active_lease(self, run: AnalysisRun) -> None:
        if run.status not in ACTIVE_ANALYSIS_RUN_STATUSES:
            raise AnalysisRunConflictError()
        if run.lease_expires_at <= utc_now():
            raise AnalysisRunLeaseExpiredError()

    async def _locked_run_message(self, run: AnalysisRun) -> Message:
        message = await self.run_repo.lock_message_owned(run.owner_user_id, run.message_id)
        if not message:
            raise AnalysisRunNotFoundError()
        return message

    async def _owned_opportunity(
        self,
        message: Message,
        owner_user_id: UUID,
    ) -> Opportunity | None:
        opportunity = (
            await self.opportunity_repo.get(message.opportunity_id)
            if message.opportunity_id
            else await self.opportunity_repo.get_by_source_message(message.id)
        )
        if opportunity and opportunity.owner_user_id != owner_user_id:
            raise AnalysisRunConflictError()
        return opportunity

    async def _expire_locked(
        self,
        run: AnalysisRun,
        message: Message,
        *,
        now: datetime,
    ) -> None:
        run.status = AnalysisRunStatus.EXPIRED
        run.expired_at = now
        run.lock_version += 1
        run.updated_at = now
        if run.mode == AnalysisRunMode.SHADOW:
            self.run_repo.stage(run)
            await self.run_repo.commit(run)
            return
        message.agent_analysis_status = AgentAnalysisStatus.QUEUED
        message.agent_error = "device analysis lease expired"
        message.updated_at = now
        await self._mark_opportunity_analysis_status(
            message,
            run.owner_user_id,
            status=AgentAnalysisStatus.QUEUED,
            error="device analysis lease expired",
            now=now,
        )
        ledger = await self.subscription_repo.release_usage_in_transaction(
            run.usage_ledger_id,
            "device analysis lease expired",
        )
        if not ledger or ledger.status != UsageStatus.RELEASED:
            await self.run_repo.rollback()
            raise AnalysisRunConflictError()
        self.run_repo.stage(run, message)
        await self.run_repo.commit(run, message, ledger)

    async def _schedule_fallback(self, run: AnalysisRun, message: Message) -> None:
        if (
            run.mode != AnalysisRunMode.PRIMARY
            or not self.settings.device_agent_fallback_enabled
        ):
            return
        await ScheduleAgentAnalysisUseCase(
            message_repo=self.message_repo,
            subscription_repo=self.subscription_repo,
            task_queue=self.task_queue,
        ).execute(
            message,
            idempotency_key=f"device-fallback:{run.id}",
            force=True,
        )
        await self.run_repo.refresh(run, message)

    async def _mark_opportunity_analysis_status(
        self,
        message: Message,
        owner_user_id: UUID,
        *,
        status: AgentAnalysisStatus,
        error: str,
        now: datetime,
    ) -> None:
        opportunity = await self._owned_opportunity(message, owner_user_id)
        if not opportunity:
            return
        opportunity.agent_analysis_status = status
        opportunity.agent_analysis_error = error[:1000]
        opportunity.updated_at = now
        self.run_repo.stage(opportunity)

    @staticmethod
    def _mark_message_failed(message: Message, failure_code: str, *, now: datetime) -> None:
        message.agent_analysis_status = AgentAnalysisStatus.FAILED
        message.agent_error = f"device analysis failed: {failure_code}"[:1000]
        message.updated_at = now

    def _verify_derived_nonce(self, run: AnalysisRun) -> None:
        nonce = derive_analysis_run_nonce(
            run_id=run.id,
            owner_user_id=run.owner_user_id,
            device_id=run.device_id,
            settings=self.settings,
        )
        if not hmac.compare_digest(run.token_nonce_hash, hash_analysis_run_nonce(nonce)):
            raise AnalysisRunTokenRejectedError()

    def _token_for(self, run: AnalysisRun) -> str:
        return create_analysis_run_token(
            run_id=run.id,
            owner_user_id=run.owner_user_id,
            device_id=run.device_id,
            settings=self.settings,
        )
