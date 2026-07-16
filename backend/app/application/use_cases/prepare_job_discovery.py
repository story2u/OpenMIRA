from __future__ import annotations

from dataclasses import dataclass

from app.domain.services.job_discovery import (
    prefilter_job_message,
    profile_source,
    source_fingerprint,
)
from app.infrastructure.db.models import JobMessageAudit, Message, SourceFunctionalProfile, utc_now
from app.infrastructure.db.repositories import (
    JobMessageAuditRepository,
    MessageRepository,
    SourceFunctionalProfileRepository,
)


@dataclass(frozen=True, slots=True)
class PreparedJobDiscovery:
    profile: SourceFunctionalProfile
    audit: JobMessageAudit


class PrepareJobDiscoveryUseCase:
    """Build a bounded source profile and persist the cheap prefilter decision."""

    def __init__(
        self,
        *,
        message_repo: MessageRepository,
        profile_repo: SourceFunctionalProfileRepository,
        audit_repo: JobMessageAuditRepository,
    ) -> None:
        self.message_repo = message_repo
        self.profile_repo = profile_repo
        self.audit_repo = audit_repo

    async def execute(self, message: Message) -> PreparedJobDiscovery | None:
        if not message.owner_user_id or not message.text:
            return None
        metadata = self._source_metadata(message)
        fingerprint = source_fingerprint(
            metadata["name"], metadata["description"], metadata["username"]
        )
        profile = await self.profile_repo.get(
            owner_user_id=message.owner_user_id,
            channel=message.channel,
            external_source_id=message.conversation_id,
        )
        now = utc_now()
        if (
            profile is None
            or profile.expires_at <= now
            or profile.source_fingerprint != fingerprint
        ):
            samples = await self.message_repo.list_recent_source_samples(
                owner_user_id=message.owner_user_id,
                channel=message.channel,
                conversation_id=message.conversation_id,
                limit=20,
            )
            generated = profile_source(
                name=metadata["name"],
                description=metadata["description"],
                username=metadata["username"],
                samples=samples,
                now=now,
            )
            profile = await self.profile_repo.save_generated(
                owner_user_id=message.owner_user_id,
                channel=message.channel,
                external_source_id=message.conversation_id,
                source_display_name=metadata["name"],
                source_description=metadata["description"],
                source_username=metadata["username"],
                source_fingerprint=fingerprint,
                primary_function=generated.primary_function,
                secondary_functions=generated.secondary_functions,
                industry_tags=generated.industry_tags,
                region_tags=generated.region_tags,
                language_tags=generated.language_tags,
                job_signal_prior=generated.job_signal_prior,
                estimated_noise_level=generated.estimated_noise_level,
                reliability_score=generated.reliability_score,
                confidence=generated.confidence,
                evidence=generated.evidence,
                sampled_message_count=generated.sampled_message_count,
                expires_at=generated.expires_at,
            )
        effective_function = profile.manual_override or profile.primary_function
        decision = prefilter_job_message(
            message.text,
            primary_function=effective_function,
            job_signal_prior=profile.job_signal_prior,
        )
        audit, _ = await self.audit_repo.record(
            owner_user_id=message.owner_user_id,
            message_id=message.id,
            source_profile_id=profile.id,
            classification=decision.classification,
            confidence=decision.score if not decision.should_analyze else 0.0,
            filter_reason=decision.reason,
            prefilter_score=decision.score,
            agent_required=decision.should_analyze,
        )
        return PreparedJobDiscovery(profile=profile, audit=audit)

    @staticmethod
    def _source_metadata(message: Message) -> dict[str, str | None]:
        payload = message.raw_payload if isinstance(message.raw_payload, dict) else {}
        chat = payload.get("chat") if isinstance(payload.get("chat"), dict) else {}
        description = chat.get("description") or payload.get("source_description")
        username = chat.get("username") or payload.get("source_username")
        return {
            "name": (message.group_name or str(chat.get("title") or "") or "Private chat")[:500],
            "description": str(description)[:2000] if description else None,
            "username": str(username)[:255] if username else None,
        }
