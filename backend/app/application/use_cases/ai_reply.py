from uuid import UUID, uuid4

from app.domain.enums import IMChannel, MessageSource, OpportunityStatus
from app.domain.services.opportunity_state import ensure_transition_allowed
from app.infrastructure.ai.litellm_client import LiteLLMReplyGenerator
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import MessageRepository, OpportunityRepository
from app.infrastructure.im.base import AdapterRegistry


class AIDraftUseCase:
    def __init__(
        self,
        *,
        opportunity_repo: OpportunityRepository,
        reply_generator: LiteLLMReplyGenerator,
    ) -> None:
        self.opportunity_repo = opportunity_repo
        self.reply_generator = reply_generator

    async def execute(self, opportunity: Opportunity) -> str:
        draft = await self.reply_generator.generate_reply(opportunity.id)
        await self.opportunity_repo.save_ai_draft(opportunity, draft)
        return draft


class AIAutoReplyUseCase:
    def __init__(
        self,
        *,
        opportunity_repo: OpportunityRepository,
        message_repo: MessageRepository,
        adapters: AdapterRegistry,
        reply_generator: LiteLLMReplyGenerator,
    ) -> None:
        self.opportunity_repo = opportunity_repo
        self.message_repo = message_repo
        self.adapters = adapters
        self.reply_generator = reply_generator

    async def execute(self, opportunity: Opportunity) -> Opportunity:
        if opportunity.archived_at is not None:
            return opportunity
        if opportunity.status in {
            OpportunityStatus.REPLIED,
            OpportunityStatus.FOLLOWING,
            OpportunityStatus.CLOSED,
            OpportunityStatus.IGNORED,
        }:
            return opportunity
        if _requires_manual_wecom_reply(opportunity):
            return opportunity

        ensure_transition_allowed(opportunity.status, OpportunityStatus.REPLIED)
        draft = opportunity.ai_reply_draft or await self.reply_generator.generate_reply(
            opportunity.id
        )
        await self.opportunity_repo.save_ai_draft(opportunity, draft)

        adapter = self.adapters.get(opportunity.channel)
        receipt = await adapter.send_message(opportunity.conversation_id, draft)
        external_message_id = receipt.provider_message_id or f"ai-{opportunity.id}-{uuid4()}"

        await self.message_repo.create_outgoing(
            channel=opportunity.channel,
            owner_user_id=opportunity.owner_user_id,
            conversation_id=opportunity.conversation_id,
            text=draft,
            source=MessageSource.AI,
            opportunity_id=opportunity.id,
            external_message_id=external_message_id,
            raw_payload=receipt.raw_response,
        )
        return await self.opportunity_repo.update_status(
            opportunity,
            OpportunityStatus.REPLIED,
            final_reply=draft,
        )


async def transition_pending_to_ai(
    opportunity_repo: OpportunityRepository,
    opportunity_id: UUID,
) -> Opportunity | None:
    opportunity = await opportunity_repo.get(opportunity_id)
    if not opportunity:
        return None
    if opportunity.archived_at is not None:
        return opportunity
    if _requires_manual_wecom_reply(opportunity):
        return None
    if opportunity.status == OpportunityStatus.PENDING_HUMAN:
        ensure_transition_allowed(opportunity.status, OpportunityStatus.AI_AUTO_REPLY)
        return await opportunity_repo.update_status(opportunity, OpportunityStatus.AI_AUTO_REPLY)
    return opportunity


def _requires_manual_wecom_reply(opportunity: Opportunity) -> bool:
    return opportunity.channel == IMChannel.WECOM and opportunity.conversation_id.startswith(
        ("wecom:", "wecom-archive:")
    )
