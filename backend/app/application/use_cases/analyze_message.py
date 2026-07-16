from __future__ import annotations

import re
from uuid import UUID

from app.application.use_cases.match_job_opportunity import MatchJobOpportunityUseCase
from app.application.use_cases.persist_job_opportunity import PersistJobOpportunityUseCase
from app.domain.enums import OpportunityStatus
from app.domain.ports import AgentAnalysisRequest, LinkInspector, MessageAgent, TaskQueue
from app.domain.services.agent_policy import project_agent_result
from app.domain.services.job_discovery import PROFILE_TTL, redact_source_sample
from app.infrastructure.db.models import Opportunity, utc_now
from app.infrastructure.db.repositories import (
    JobMessageAuditRepository,
    JobOpportunityMatchRepository,
    JobOpportunityRepository,
    JobSearchProfileRepository,
    MessageRepository,
    OpportunityRepository,
    SourceFunctionalProfileRepository,
)

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
        job_audit_repo: JobMessageAuditRepository | None = None,
        source_profile_repo: SourceFunctionalProfileRepository | None = None,
        job_opportunity_repo: JobOpportunityRepository | None = None,
        job_search_profile_repo: JobSearchProfileRepository | None = None,
        job_match_repo: JobOpportunityMatchRepository | None = None,
    ) -> None:
        self.message_repo = message_repo
        self.opportunity_repo = opportunity_repo
        self.agent = agent
        self.link_inspector = link_inspector
        self.task_queue = task_queue
        self.min_opportunity_confidence = min_opportunity_confidence
        self.max_links = max_links
        self.job_audit_repo = job_audit_repo
        self.source_profile_repo = source_profile_repo
        self.job_opportunity_repo = job_opportunity_repo
        self.job_search_profile_repo = job_search_profile_repo
        self.job_match_repo = job_match_repo

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
            job_audit = (
                await self.job_audit_repo.get_by_message(message.id)
                if self.job_audit_repo
                else None
            )
            job_context = None
            source_profile = None
            if (
                job_audit
                and job_audit.agent_required
                and self.source_profile_repo
                and message.owner_user_id
                and job_audit.source_profile_id
            ):
                source_profile = await self.source_profile_repo.get_by_id_for_owner(
                    job_audit.source_profile_id, message.owner_user_id
                )
                if source_profile:
                    job_context = {
                        "source_function": (
                            source_profile.manual_override or source_profile.primary_function
                        ).value,
                        "job_signal_prior": source_profile.job_signal_prior,
                        "source_reliability": source_profile.reliability_score,
                        "prefilter_score": job_audit.prefilter_score,
                    }
                    if source_profile.manual_override is None and source_profile.confidence < 0.75:
                        recent_samples = await self.message_repo.list_recent_source_samples(
                            owner_user_id=message.owner_user_id,
                            channel=message.channel,
                            conversation_id=message.conversation_id,
                            limit=20,
                        )
                        job_context["source_profile_input"] = {
                            "name": source_profile.source_display_name,
                            "description": source_profile.source_description,
                            "username": source_profile.source_username,
                            "recent_samples": [
                                sample
                                for raw_sample in recent_samples
                                if (sample := redact_source_sample(raw_sample))
                            ][:20],
                        }
            result = await self.agent.analyze(
                AgentAnalysisRequest(
                    message_id=message.id,
                    channel=message.channel,
                    sender_display_name=message.sender_display_name,
                    source_type=message.source_type,
                    group_name=message.group_name,
                    text=message.text or "",
                    links=inspections,
                    job_discovery=job_context,
                )
            )
            if job_audit and result.job_analysis and self.job_audit_repo:
                await self.job_audit_repo.apply_agent_classification(
                    job_audit,
                    classification=result.job_analysis.classification,
                    confidence=result.job_analysis.classification_confidence,
                    reason="; ".join(result.job_analysis.noise_reasons)
                    or "pi agent structured classification",
                )
            if result.source_profile_analysis and source_profile and message.owner_user_id:
                source_assessment = result.source_profile_analysis
                assert job_context is not None
                source_profile = await self.source_profile_repo.save_generated(
                    owner_user_id=message.owner_user_id,
                    channel=source_profile.channel,
                    external_source_id=source_profile.external_source_id,
                    source_display_name=source_profile.source_display_name,
                    source_description=source_profile.source_description,
                    source_username=source_profile.source_username,
                    source_fingerprint=source_profile.source_fingerprint,
                    primary_function=source_assessment.primary_function,
                    secondary_functions=[
                        item.value for item in source_assessment.secondary_functions
                    ],
                    industry_tags=source_assessment.industry_tags,
                    region_tags=source_assessment.region_tags,
                    language_tags=source_assessment.language_tags,
                    job_signal_prior=source_assessment.job_signal_prior,
                    estimated_noise_level=source_assessment.estimated_noise_level,
                    reliability_score=source_assessment.reliability_score,
                    confidence=source_assessment.confidence,
                    evidence=source_assessment.evidence,
                    sampled_message_count=len(
                        job_context["source_profile_input"]["recent_samples"]
                    ),
                    expires_at=utc_now() + PROFILE_TTL,
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
            if result.job_analysis and source_profile and self.job_opportunity_repo:
                persisted_job = await PersistJobOpportunityUseCase(
                    self.job_opportunity_repo
                ).execute(
                    message=message,
                    analysis=result.job_analysis,
                    source_profile=source_profile,
                    existing_opportunity=opportunity,
                )
                if persisted_job.opportunity:
                    opportunity = persisted_job.opportunity
                    if (
                        opportunity.owner_user_id
                        and self.job_search_profile_repo
                        and self.job_match_repo
                    ):
                        await MatchJobOpportunityUseCase(
                            job_repo=self.job_opportunity_repo,
                            profile_repo=self.job_search_profile_repo,
                            match_repo=self.job_match_repo,
                        ).execute(opportunity.id, opportunity.owner_user_id)
                elif persisted_job.rejected_reason and job_audit and self.job_audit_repo:
                    await self.job_audit_repo.apply_agent_classification(
                        job_audit,
                        classification=result.job_analysis.classification,
                        confidence=result.job_analysis.classification_confidence,
                        reason=persisted_job.rejected_reason,
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
