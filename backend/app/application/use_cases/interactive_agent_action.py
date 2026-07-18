from __future__ import annotations

import hmac
from dataclasses import dataclass
from datetime import timedelta
from uuid import UUID

from sqlalchemy.exc import IntegrityError

from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.application.use_cases.manual_reply import (
    ManualReplyDeliveryError,
    ManualReplyIdempotencyConflict,
    ManualReplyInProgress,
    ManualReplyOutcomeUncertain,
    ManualReplyProjectionError,
    ManualReplyResult,
    ManualReplyUseCase,
)
from app.core.config import Settings
from app.core.security import (
    create_interactive_agent_approval_token,
    derive_interactive_agent_approval_nonce,
    hash_interactive_agent_approval_nonce,
    hash_interactive_agent_turn_nonce,
)
from app.domain.enums import (
    InteractiveAgentApprovalStatus,
    InteractiveAgentTurnStatus,
    OpportunityStatus,
)
from app.domain.services.interactive_agent import (
    INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
    INTERACTIVE_AGENT_SIGNAL_APPETITE_SCHEMA_VERSION,
    supports_interactive_agent_contract,
)
from app.domain.services.interactive_agent_action import (
    CanonicalSendReplyArguments,
    InteractiveAgentActionConflictError,
    InteractiveAgentActionExpiredError,
    InteractiveAgentActionProjectionError,
    InteractiveAgentActionRejectedError,
    InteractiveAgentActionUnavailableError,
    InteractiveAgentActionUncertainError,
    canonical_send_reply_arguments_hash,
)
from app.domain.services.opportunity_state import (
    InvalidOpportunityTransition,
    OpportunityVersionConflict,
    ensure_transition_allowed,
)
from app.infrastructure.db.interactive_agent_action_repository import (
    InteractiveAgentActionRepository,
)
from app.infrastructure.db.models import (
    InteractiveAgentActionApproval,
    InteractiveAgentTurn,
    Opportunity,
    utc_now,
)
from app.infrastructure.im.base import AdapterRegistry


@dataclass(frozen=True, slots=True)
class InteractiveAgentApprovalTokenPrincipal:
    approval_id: UUID
    turn_id: UUID
    owner_user_id: UUID
    device_id: UUID
    nonce: str


@dataclass(frozen=True, slots=True)
class InteractiveAgentApprovalDecision:
    approval: InteractiveAgentActionApproval
    approval_token: str | None


def _same_decision(
    approval: InteractiveAgentActionApproval,
    *,
    arguments_hash: str,
    expected_version: int,
    idempotency_key: str,
    opportunity_id: UUID,
    device_id: UUID,
) -> bool:
    return (
        approval.device_id == device_id
        and approval.tool_name == "send_reply"
        and approval.opportunity_id == opportunity_id
        and approval.expected_version == expected_version
        and approval.idempotency_key == idempotency_key
        and approval.arguments_hash == arguments_hash
    )


class InteractiveAgentActionService:
    def __init__(
        self,
        *,
        repository: InteractiveAgentActionRepository,
        manual_reply: ManualReplyUseCase,
        adapters: AdapterRegistry,
        settings: Settings,
        routing_service: InteractiveAgentRoutingService,
    ) -> None:
        self.repository = repository
        self.manual_reply = manual_reply
        self.adapters = adapters
        self.settings = settings
        self.routing_service = routing_service

    def _external_actions_available(self) -> bool:
        schema_version = self.settings.interactive_agent_schema_version
        policy_version = self.settings.interactive_agent_policy_version
        return bool(
            self.settings.interactive_agent_beta_enabled
            and self.settings.interactive_agent_gateway_enabled
            and self.settings.interactive_agent_external_actions_enabled
            and self.settings.im_send_enabled
            and schema_version
            in {
                INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
                INTERACTIVE_AGENT_SIGNAL_APPETITE_SCHEMA_VERSION,
            }
            and supports_interactive_agent_contract(
                schema_version=schema_version,
                policy_version=policy_version,
            )
        )

    def _authorize_turn(
        self,
        turn: InteractiveAgentTurn,
        principal: InteractiveAgentTurnTokenPrincipal,
    ) -> None:
        if (
            turn.status
            not in {InteractiveAgentTurnStatus.CLAIMED, InteractiveAgentTurnStatus.RUNNING}
            or turn.lease_expires_at <= utc_now()
            or turn.schema_version
            not in {
                INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
                INTERACTIVE_AGENT_SIGNAL_APPETITE_SCHEMA_VERSION,
            }
            or not supports_interactive_agent_contract(
                schema_version=turn.schema_version,
                policy_version=turn.policy_version,
            )
            or turn.runtime_version != self.settings.interactive_agent_runtime_version
            or turn.model_alias != self.settings.interactive_agent_model_alias
            or not hmac.compare_digest(
                turn.token_nonce_hash,
                hash_interactive_agent_turn_nonce(principal.nonce),
            )
        ):
            raise InteractiveAgentActionRejectedError()

    def _authorize_execution_turn(self, turn: InteractiveAgentTurn) -> None:
        if (
            turn.status
            not in {InteractiveAgentTurnStatus.CLAIMED, InteractiveAgentTurnStatus.RUNNING}
            or turn.lease_expires_at <= utc_now()
            or turn.schema_version
            not in {
                INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION,
                INTERACTIVE_AGENT_SIGNAL_APPETITE_SCHEMA_VERSION,
            }
            or not supports_interactive_agent_contract(
                schema_version=turn.schema_version,
                policy_version=turn.policy_version,
            )
            or turn.runtime_version != self.settings.interactive_agent_runtime_version
            or turn.model_alias != self.settings.interactive_agent_model_alias
        ):
            raise InteractiveAgentActionRejectedError()

    @staticmethod
    def _validate_opportunity(
        opportunity: Opportunity | None,
        *,
        expected_version: int,
    ) -> Opportunity:
        if not opportunity or opportunity.archived_at is not None:
            raise InteractiveAgentActionRejectedError()
        if opportunity.aggregate_version != expected_version:
            raise InteractiveAgentActionConflictError()
        try:
            ensure_transition_allowed(opportunity.status, OpportunityStatus.FOLLOWING)
        except InvalidOpportunityTransition as exc:
            raise InteractiveAgentActionConflictError() from exc
        return opportunity

    async def decide(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
        *,
        approved: bool,
        expected_version: int,
        idempotency_key: str,
        opportunity_id: UUID,
        text: str,
        tool_call_id: str,
    ) -> InteractiveAgentApprovalDecision:
        if not self._external_actions_available():
            raise InteractiveAgentActionUnavailableError()
        arguments_hash = canonical_send_reply_arguments_hash(
            CanonicalSendReplyArguments(opportunity_id=opportunity_id, text=text)
        )
        turn = await self.repository.lock_turn_owned(
            turn_id=principal.turn_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not turn:
            raise InteractiveAgentActionRejectedError()
        self._authorize_turn(turn, principal)
        device = await self.repository.active_device_owned(
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not device or not self.routing_service.capability_available(device):
            raise InteractiveAgentActionRejectedError()
        existing = await self.repository.lock_by_tool_call(
            owner_user_id=principal.owner_user_id,
            turn_id=principal.turn_id,
            tool_call_id=tool_call_id,
        )
        if existing:
            return await self._replay_decision(
                existing,
                approved=approved,
                arguments_hash=arguments_hash,
                expected_version=expected_version,
                idempotency_key=idempotency_key,
                opportunity_id=opportunity_id,
                device_id=principal.device_id,
            )
        opportunity = self._validate_opportunity(
            await self.repository.lock_opportunity_owned(
                opportunity_id=opportunity_id,
                owner_user_id=principal.owner_user_id,
            ),
            expected_version=expected_version,
        )
        try:
            self.adapters.get(opportunity.channel)
        except KeyError as exc:
            raise InteractiveAgentActionRejectedError() from exc
        if (
            await self.repository.count_for_turn(principal.turn_id)
            >= self.settings.interactive_agent_max_approvals_per_turn
        ):
            raise InteractiveAgentActionConflictError()

        now = utc_now()
        approval = InteractiveAgentActionApproval(
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
            turn_id=principal.turn_id,
            tool_call_id=tool_call_id,
            tool_name="send_reply",
            opportunity_id=opportunity_id,
            expected_version=expected_version,
            idempotency_key=idempotency_key,
            arguments_hash=arguments_hash,
            status=(
                InteractiveAgentApprovalStatus.GRANTED
                if approved
                else InteractiveAgentApprovalStatus.DENIED
            ),
            decided_at=now,
        )
        if approved:
            approval.expires_at = min(
                now + timedelta(seconds=self.settings.interactive_agent_approval_token_seconds),
                turn.lease_expires_at,
            )
            if approval.expires_at <= now:
                raise InteractiveAgentActionExpiredError()
            nonce = derive_interactive_agent_approval_nonce(
                approval_id=approval.id,
                turn_id=approval.turn_id,
                owner_user_id=approval.owner_user_id,
                device_id=approval.device_id,
                settings=self.settings,
            )
            approval.token_nonce_hash = hash_interactive_agent_approval_nonce(nonce)
        try:
            approval = await self.repository.add_and_commit(approval)
        except IntegrityError as exc:
            await self.repository.rollback()
            concurrent = await self.repository.lock_by_tool_call(
                owner_user_id=principal.owner_user_id,
                turn_id=principal.turn_id,
                tool_call_id=tool_call_id,
            )
            if not concurrent:
                raise InteractiveAgentActionConflictError() from exc
            return await self._replay_decision(
                concurrent,
                approved=approved,
                arguments_hash=arguments_hash,
                expected_version=expected_version,
                idempotency_key=idempotency_key,
                opportunity_id=opportunity_id,
                device_id=principal.device_id,
            )
        return InteractiveAgentApprovalDecision(
            approval=approval,
            approval_token=(
                create_interactive_agent_approval_token(
                    approval_id=approval.id,
                    turn_id=approval.turn_id,
                    owner_user_id=approval.owner_user_id,
                    device_id=approval.device_id,
                    settings=self.settings,
                )
                if approved
                else None
            ),
        )

    async def _replay_decision(
        self,
        approval: InteractiveAgentActionApproval,
        *,
        approved: bool,
        arguments_hash: str,
        expected_version: int,
        idempotency_key: str,
        opportunity_id: UUID,
        device_id: UUID,
    ) -> InteractiveAgentApprovalDecision:
        if not _same_decision(
            approval,
            arguments_hash=arguments_hash,
            expected_version=expected_version,
            idempotency_key=idempotency_key,
            opportunity_id=opportunity_id,
            device_id=device_id,
        ):
            raise InteractiveAgentActionConflictError()
        expected_status = (
            InteractiveAgentApprovalStatus.GRANTED
            if approved
            else InteractiveAgentApprovalStatus.DENIED
        )
        if approval.status == InteractiveAgentApprovalStatus.EXPIRED:
            raise InteractiveAgentActionExpiredError()
        if approval.status != expected_status:
            raise InteractiveAgentActionConflictError()
        if approval.expires_at and approval.expires_at <= utc_now():
            approval.status = InteractiveAgentApprovalStatus.EXPIRED
            approval.finished_at = utc_now()
            approval = await self.repository.commit(approval)
            raise InteractiveAgentActionExpiredError()
        return InteractiveAgentApprovalDecision(
            approval=approval,
            approval_token=(
                create_interactive_agent_approval_token(
                    approval_id=approval.id,
                    turn_id=approval.turn_id,
                    owner_user_id=approval.owner_user_id,
                    device_id=approval.device_id,
                    settings=self.settings,
                )
                if approved
                else None
            ),
        )

    async def execute_send_reply(
        self,
        principal: InteractiveAgentApprovalTokenPrincipal,
        *,
        expected_version: int,
        idempotency_key: str,
        opportunity_id: UUID,
        text: str,
    ) -> ManualReplyResult:
        if not self._external_actions_available():
            raise InteractiveAgentActionUnavailableError()
        turn = await self.repository.lock_turn_owned(
            turn_id=principal.turn_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not turn:
            raise InteractiveAgentActionRejectedError()
        self._authorize_execution_turn(turn)
        device = await self.repository.active_device_owned(
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not device or not self.routing_service.capability_available(device):
            raise InteractiveAgentActionRejectedError()
        approval = await self.repository.lock_approval_owned(
            approval_id=principal.approval_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
            turn_id=principal.turn_id,
        )
        if not approval:
            raise InteractiveAgentActionRejectedError()
        if approval.status != InteractiveAgentApprovalStatus.GRANTED:
            raise InteractiveAgentActionConflictError()
        now = utc_now()
        if not approval.expires_at or approval.expires_at <= now:
            approval.status = InteractiveAgentApprovalStatus.EXPIRED
            approval.finished_at = now
            await self.repository.commit(approval)
            raise InteractiveAgentActionExpiredError()
        if not approval.token_nonce_hash or not hmac.compare_digest(
            approval.token_nonce_hash,
            hash_interactive_agent_approval_nonce(principal.nonce),
        ):
            raise InteractiveAgentActionRejectedError()
        arguments_hash = canonical_send_reply_arguments_hash(
            CanonicalSendReplyArguments(opportunity_id=opportunity_id, text=text)
        )
        if (
            approval.tool_name != "send_reply"
            or approval.opportunity_id != opportunity_id
            or approval.expected_version != expected_version
            or approval.idempotency_key != idempotency_key
            or not hmac.compare_digest(approval.arguments_hash, arguments_hash)
        ):
            raise InteractiveAgentActionRejectedError()
        opportunity = self._validate_opportunity(
            await self.repository.lock_opportunity_owned(
                opportunity_id=opportunity_id,
                owner_user_id=principal.owner_user_id,
            ),
            expected_version=expected_version,
        )
        try:
            self.adapters.get(opportunity.channel)
        except KeyError as exc:
            raise InteractiveAgentActionRejectedError() from exc
        approval.status = InteractiveAgentApprovalStatus.EXECUTING
        approval.execution_started_at = now
        approval_id = approval.id
        await self.repository.commit(approval)

        try:
            result = await self.manual_reply.execute(
                opportunity=opportunity,
                text=text,
                operator_id=f"agent-approved:{principal.device_id}",
                mark_following=True,
                idempotency_key=idempotency_key,
                expected_version=expected_version,
            )
        except ManualReplyProjectionError as exc:
            await self._finish_execution(
                approval_id,
                status=InteractiveAgentApprovalStatus.CONSUMED,
            )
            raise InteractiveAgentActionProjectionError() from exc
        except (ManualReplyOutcomeUncertain, ManualReplyInProgress) as exc:
            await self._finish_execution(
                approval_id,
                status=InteractiveAgentApprovalStatus.UNCERTAIN,
                failure_code="delivery_outcome_uncertain",
            )
            raise InteractiveAgentActionUncertainError() from exc
        except (
            InvalidOpportunityTransition,
            LookupError,
            ManualReplyDeliveryError,
            ManualReplyIdempotencyConflict,
            OpportunityVersionConflict,
        ) as exc:
            await self._finish_execution(
                approval_id,
                status=InteractiveAgentApprovalStatus.FAILED,
                failure_code="action_rejected_before_delivery",
            )
            raise InteractiveAgentActionConflictError() from exc
        except Exception as exc:
            await self._finish_execution(
                approval_id,
                status=InteractiveAgentApprovalStatus.UNCERTAIN,
                failure_code="action_outcome_unknown",
            )
            raise InteractiveAgentActionUncertainError() from exc
        await self._finish_execution(
            approval_id,
            status=InteractiveAgentApprovalStatus.CONSUMED,
            delivery_id=result.delivery_id,
            reset_session=False,
        )
        return result

    async def _finish_execution(
        self,
        approval_id: UUID,
        *,
        status: InteractiveAgentApprovalStatus,
        delivery_id: UUID | None = None,
        failure_code: str | None = None,
        reset_session: bool = True,
    ) -> None:
        if reset_session:
            await self.repository.rollback()
        approval = await self.repository.lock_approval(approval_id)
        if not approval or approval.status != InteractiveAgentApprovalStatus.EXECUTING:
            raise InteractiveAgentActionUncertainError()
        approval.status = status
        approval.finished_at = utc_now()
        approval.failure_code = failure_code
        approval.manual_reply_delivery_id = delivery_id
        await self.repository.commit(approval)
