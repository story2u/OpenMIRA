from uuid import UUID, uuid4

from app.domain.enums import IMChannel, MessageSource, OpportunityStatus
from app.domain.services.opportunity_state import ensure_transition_allowed
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import MessageRepository, OpportunityRepository
from app.infrastructure.im.base import AdapterRegistry


class ManualReplyUseCase:
    def __init__(
        self,
        *,
        opportunity_repo: OpportunityRepository,
        message_repo: MessageRepository,
        adapters: AdapterRegistry,
    ) -> None:
        self.opportunity_repo = opportunity_repo
        self.message_repo = message_repo
        self.adapters = adapters

    async def execute(
        self,
        *,
        opportunity: Opportunity,
        text: str,
        operator_id: str,
        mark_following: bool,
        idempotency_key: str | None = None,
    ) -> Opportunity:
        target_status = OpportunityStatus.FOLLOWING if mark_following else OpportunityStatus.REPLIED
        ensure_transition_allowed(opportunity.status, target_status)

        adapter = self.adapters.get(opportunity.channel)
        if opportunity.channel == IMChannel.WECOM and opportunity.conversation_id.startswith(
            "wecom:"
        ):
            receipt = await adapter.send_message(
                opportunity.conversation_id,
                text,
                idempotency_key=idempotency_key,
                opportunity_id=opportunity.id,
                owner_user_id=opportunity.owner_user_id,
            )
            external_message_id = (
                receipt.provider_message_id or f"manual-{opportunity.id}-{idempotency_key}"
            )
        else:
            receipt = await adapter.send_message(opportunity.conversation_id, text)
            external_message_id = receipt.provider_message_id or (
                f"manual-{opportunity.id}-{operator_id}-{idempotency_key or uuid4()}"
            )

        existing = await self.message_repo.get_by_external_id(
            opportunity.channel,
            external_message_id,
        )
        if not existing:
            await self.message_repo.create_outgoing(
                channel=opportunity.channel,
                owner_user_id=opportunity.owner_user_id,
                conversation_id=opportunity.conversation_id,
                text=text,
                source=MessageSource.HUMAN,
                opportunity_id=opportunity.id,
                external_message_id=external_message_id,
                raw_payload=receipt.raw_response,
            )
        return await self.opportunity_repo.update_status(
            opportunity,
            target_status,
            final_reply=text,
            assigned_to=operator_id,
        )


async def get_or_404(repo: OpportunityRepository, opportunity_id: UUID) -> Opportunity:
    opportunity = await repo.get(opportunity_id)
    if not opportunity:
        from fastapi import HTTPException, status

        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="opportunity not found")
    return opportunity
