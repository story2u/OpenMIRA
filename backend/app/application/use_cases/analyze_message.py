from __future__ import annotations

import re
from uuid import UUID

from app.domain.enums import OpportunityStatus
from app.domain.ports import AgentAnalysisRequest, LinkInspector, MessageAgent, TaskQueue
from app.domain.services.agent_policy import project_agent_result
from app.infrastructure.db.models import Opportunity, utc_now
from app.infrastructure.db.repositories import MessageRepository, OpportunityRepository

URL_PATTERN = re.compile(r"https?://[^\s<>()\[\]{}\"']+", re.IGNORECASE)


def message_links(text: str, provided: list[str], *, limit: int) -> list[str]:
    links: list[str] = []
    for candidate in [*provided, *URL_PATTERN.findall(text)]:
        normalized = candidate.rstrip(".,;:!?，。；：！？")
        if normalized and normalized not in links:
            links.append(normalized)
        if len(links) >= limit:
            break
    return links


class AnalyzeMessageUseCase:
    def __init__(
        self,
        *,
        message_repo: MessageRepository,
        opportunity_repo: OpportunityRepository,
        agent: MessageAgent,
        link_inspector: LinkInspector,
        task_queue: TaskQueue,
        min_opportunity_confidence: float,
        max_links: int,
    ) -> None:
        self.message_repo = message_repo
        self.opportunity_repo = opportunity_repo
        self.agent = agent
        self.link_inspector = link_inspector
        self.task_queue = task_queue
        self.min_opportunity_confidence = min_opportunity_confidence
        self.max_links = max_links

    async def execute(self, message_id: UUID, *, force: bool = False) -> Opportunity | None:
        message = await self.message_repo.claim_agent_analysis(message_id, force=force)
        if not message:
            return None

        opportunity: Opportunity | None = None
        try:
            links = message_links(
                message.text or "",
                message.raw_message_links,
                limit=self.max_links,
            )
            inspections = await self.link_inspector.inspect_many(links)
            result = await self.agent.analyze(
                AgentAnalysisRequest(
                    message_id=message.id,
                    channel=message.channel,
                    sender_display_name=message.sender_display_name,
                    source_type=message.source_type,
                    group_name=message.group_name,
                    text=message.text or "",
                    links=inspections,
                )
            )
            projection = project_agent_result(result, inspections, analyzed_at=utc_now())

            opportunity = (
                await self.opportunity_repo.get(message.opportunity_id)
                if message.opportunity_id
                else await self.opportunity_repo.get_by_source_message(message.id)
            )
            if (
                not opportunity
                and result.is_opportunity
                and result.confidence >= self.min_opportunity_confidence
            ):
                opportunity = await self.opportunity_repo.create(
                    channel=message.channel,
                    owner_user_id=message.owner_user_id,
                    conversation_id=message.conversation_id,
                    customer_external_id=message.sender_external_id,
                    contact_name=message.sender_display_name,
                    source_type=message.source_type,
                    group_name=message.group_name,
                    source_message_id=message.id,
                    title=result.title,
                    summary=result.summary,
                    matched_keywords=[],
                    raw_message_links=links,
                    confidence=result.confidence,
                    priority=result.priority,
                    detection_reason="pi agent post-processing",
                    # Agent-discovered opportunities always enter human review. An untrusted
                    # model result must not opt itself into the automatic reply path.
                    status=OpportunityStatus.PENDING_HUMAN,
                    last_message_preview=message.text or "",
                )
                await self.message_repo.attach_opportunity(message.id, opportunity.id)
                message.opportunity_id = opportunity.id
                self.task_queue.notify_reviewers(opportunity.id)

            if opportunity:
                opportunity = await self.opportunity_repo.apply_agent_projection(
                    opportunity,
                    projection,
                )
            await self.message_repo.complete_agent_analysis(message, projection)
            if opportunity and opportunity.status == OpportunityStatus.AI_AUTO_REPLY:
                self.task_queue.enqueue_ai_reply(opportunity.id)
            return opportunity
        except Exception as exc:
            await self.message_repo.fail_agent_analysis(message.id, str(exc))
            if message.opportunity_id:
                opportunity = await self.opportunity_repo.get(message.opportunity_id)
                if opportunity and opportunity.status == OpportunityStatus.AI_AUTO_REPLY:
                    opportunity = await self.opportunity_repo.update_status(
                        opportunity,
                        OpportunityStatus.PENDING_HUMAN,
                    )
                    self.task_queue.notify_reviewers(opportunity.id)
            raise
