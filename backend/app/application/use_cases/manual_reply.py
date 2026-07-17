from dataclasses import dataclass
import hashlib
from uuid import UUID

import structlog

from app.domain.enums import (
    ManualReplyDeliveryStatus,
    MessageSource,
    OpportunityStatus,
)
from app.domain.services.opportunity_state import (
    OpportunityVersionConflict,
    ensure_transition_allowed,
)
from app.infrastructure.db.models import ManualReplyDelivery, Message, Opportunity
from app.infrastructure.db.repositories import (
    ManualReplyDeliveryRepository,
    MessageRepository,
    OpportunityRepository,
)
from app.infrastructure.im.base import AdapterRegistry, IMSendDisabledError

logger = structlog.get_logger(__name__)


class ManualReplyIdempotencyConflict(ValueError):
    pass


class ManualReplyInProgress(RuntimeError):
    pass


class ManualReplyOutcomeUncertain(RuntimeError):
    pass


class ManualReplyDeliveryError(RuntimeError):
    pass


class ManualReplyProjectionError(RuntimeError):
    pass


@dataclass(frozen=True, slots=True)
class ManualReplyResult:
    delivery_id: UUID
    opportunity: Opportunity
    message: Message


class ManualReplyUseCase:
    def __init__(
        self,
        *,
        opportunity_repo: OpportunityRepository,
        message_repo: MessageRepository,
        delivery_repo: ManualReplyDeliveryRepository,
        adapters: AdapterRegistry,
    ) -> None:
        self.opportunity_repo = opportunity_repo
        self.message_repo = message_repo
        self.delivery_repo = delivery_repo
        self.adapters = adapters

    async def execute(
        self,
        *,
        opportunity: Opportunity,
        text: str,
        operator_id: str,
        mark_following: bool,
        idempotency_key: str,
        expected_version: int | None = None,
    ) -> ManualReplyResult:
        if opportunity.owner_user_id is None:
            raise ManualReplyDeliveryError("manual reply owner is unavailable")
        opportunity_id = opportunity.id
        target_status = OpportunityStatus.FOLLOWING if mark_following else OpportunityStatus.REPLIED
        content_hash = hashlib.sha256(f"{target_status.value}\0{text}".encode("utf-8")).hexdigest()
        delivery = await self.delivery_repo.reserve(
            owner_user_id=opportunity.owner_user_id,
            opportunity_id=opportunity.id,
            idempotency_key=idempotency_key,
            content_hash=content_hash,
        )
        if delivery.opportunity_id != opportunity.id or delivery.content_hash != content_hash:
            raise ManualReplyIdempotencyConflict(
                "Idempotency-Key was already used for a different manual reply"
            )
        if delivery.status in {
            ManualReplyDeliveryStatus.DELIVERED,
            ManualReplyDeliveryStatus.COMPLETED,
        }:
            return await self._finish_projection(
                delivery=delivery,
                opportunity=opportunity,
                text=text,
                operator_id=operator_id,
                target_status=target_status,
            )
        if delivery.status == ManualReplyDeliveryStatus.UNCERTAIN:
            raise ManualReplyOutcomeUncertain(
                "manual reply delivery outcome is uncertain and must be reviewed"
            )

        # Do not contact the provider after a concurrent status change made this reply illegal.
        current = await self.opportunity_repo.get(opportunity.id)
        if not current:
            raise ManualReplyDeliveryError("opportunity is unavailable")
        if current.archived_at is not None:
            raise ManualReplyDeliveryError("opportunity is archived")
        ensure_transition_allowed(current.status, target_status)

        try:
            claimed = (
                await self.delivery_repo.claim_send_attempt(delivery.id)
                if expected_version is None
                else await self.delivery_repo.claim_send_attempt(
                    delivery.id,
                    expected_opportunity_version=expected_version,
                    opportunity_id=opportunity.id,
                    owner_user_id=opportunity.owner_user_id,
                    target_status=target_status,
                )
            )
        except OpportunityVersionConflict:
            await self.delivery_repo.mark_failed(delivery.id, "OpportunityVersionConflict")
            raise
        if not claimed:
            raise ManualReplyInProgress("manual reply delivery is already in progress")
        delivery = claimed

        adapter = self.adapters.get(opportunity.channel)
        try:
            receipt = await adapter.send_message(
                opportunity.conversation_id,
                text,
                idempotency_key=idempotency_key,
                opportunity_id=opportunity.id,
                owner_user_id=opportunity.owner_user_id,
            )
        except IMSendDisabledError as exc:
            await self.delivery_repo.mark_failed(delivery.id, exc.__class__.__name__)
            logger.warning(
                "manual_reply.delivery_disabled",
                opportunity_id=str(opportunity_id),
                delivery_id=str(delivery.id),
            )
            raise ManualReplyDeliveryError("IM sending is disabled") from exc
        except Exception as exc:
            # Provider timeouts can happen after acceptance. Fail closed: this key may
            # never send again until an operator reviews the uncertain delivery.
            await self.delivery_repo.mark_uncertain(delivery.id, exc.__class__.__name__)
            logger.warning(
                "manual_reply.delivery_uncertain",
                opportunity_id=str(opportunity_id),
                delivery_id=str(delivery.id),
                error_class=exc.__class__.__name__,
            )
            raise ManualReplyOutcomeUncertain(
                "manual reply delivery outcome is uncertain and must be reviewed"
            ) from exc

        try:
            delivery = await self.delivery_repo.mark_delivered(
                delivery,
                receipt.provider_message_id,
            )
        except Exception as exc:
            logger.error(
                "manual_reply.delivery_projection_unknown",
                opportunity_id=str(opportunity_id),
                delivery_id=str(delivery.id),
                error_class=exc.__class__.__name__,
            )
            raise ManualReplyOutcomeUncertain(
                "manual reply was accepted but its delivery record could not be confirmed"
            ) from exc

        return await self._finish_projection(
            delivery=delivery,
            opportunity=opportunity,
            text=text,
            operator_id=operator_id,
            target_status=target_status,
        )

    async def _finish_projection(
        self,
        *,
        delivery: ManualReplyDelivery,
        opportunity: Opportunity,
        text: str,
        operator_id: str,
        target_status: OpportunityStatus,
    ) -> ManualReplyResult:
        # Provider message IDs are not guaranteed globally unique (Telegram IDs are
        # scoped to a chat). The delivery UUID is stable across every retry.
        external_message_id = f"manual-{delivery.id}"
        try:
            message = await self.message_repo.create_outgoing_idempotent(
                channel=opportunity.channel,
                owner_user_id=opportunity.owner_user_id,
                conversation_id=opportunity.conversation_id,
                text=text,
                source=MessageSource.HUMAN,
                opportunity_id=opportunity.id,
                external_message_id=external_message_id,
                raw_payload={
                    "manual_reply_delivery_id": str(delivery.id),
                    "provider_message_id": delivery.provider_message_id,
                },
            )
            updated = await self.opportunity_repo.finalize_manual_reply(
                opportunity_id=opportunity.id,
                target_status=target_status,
                text=text,
                operator_id=operator_id,
            )
            await self.delivery_repo.mark_completed(delivery.id)
        except Exception as exc:
            logger.error(
                "manual_reply.projection_failed",
                opportunity_id=str(opportunity.id),
                delivery_id=str(delivery.id),
                error_class=exc.__class__.__name__,
            )
            raise ManualReplyProjectionError(
                "manual reply was delivered but local projection is incomplete; retry the same key"
            ) from exc
        return ManualReplyResult(
            delivery_id=delivery.id,
            opportunity=updated,
            message=message,
        )


async def get_or_404(repo: OpportunityRepository, opportunity_id: UUID) -> Opportunity:
    opportunity = await repo.get(opportunity_id)
    if not opportunity:
        from fastapi import HTTPException, status

        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="opportunity not found")
    return opportunity
