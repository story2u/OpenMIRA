from __future__ import annotations

from uuid import UUID

from app.domain.services.job_discovery import PROFILE_TTL, redact_source_sample
from app.infrastructure.agent.pi_client import PiAgentClient
from app.infrastructure.db.models import SourceFunctionalProfile, utc_now
from app.infrastructure.db.repositories import (
    MessageRepository,
    SourceFunctionalProfileRepository,
    SubscriptionRepository,
)


class SourceProfileQuotaExceeded(RuntimeError):
    pass


class ReprofileSourceFunctionUseCase:
    def __init__(
        self,
        *,
        agent: PiAgentClient,
        message_repo: MessageRepository,
        profile_repo: SourceFunctionalProfileRepository,
        subscription_repo: SubscriptionRepository,
    ) -> None:
        self.agent = agent
        self.message_repo = message_repo
        self.profile_repo = profile_repo
        self.subscription_repo = subscription_repo

    async def execute(
        self,
        *,
        profile: SourceFunctionalProfile,
        owner_user_id: UUID,
    ) -> SourceFunctionalProfile:
        samples = await self.message_repo.list_recent_source_samples(
            owner_user_id=owner_user_id,
            channel=profile.channel,
            conversation_id=profile.external_source_id,
            limit=20,
        )
        redacted_samples = [sample for value in samples if (sample := redact_source_sample(value))][
            :20
        ]
        idempotency_key = f"source-profile:{profile.id}:{profile.profiled_at.isoformat()}"
        reservation = await self.subscription_repo.reserve_agent_analysis(
            user_id=owner_user_id,
            message_id=None,
            idempotency_key=idempotency_key,
        )
        if not reservation.allowed or not reservation.ledger:
            raise SourceProfileQuotaExceeded("monthly pi agent analysis quota exceeded")
        if not reservation.created:
            return profile
        try:
            assessment = await self.agent.profile_source_function(
                {
                    "name": profile.source_display_name,
                    "description": profile.source_description,
                    "username": profile.source_username,
                    "recent_samples": redacted_samples,
                }
            )
            saved = await self.profile_repo.save_generated(
                owner_user_id=owner_user_id,
                channel=profile.channel,
                external_source_id=profile.external_source_id,
                source_display_name=profile.source_display_name,
                source_description=profile.source_description,
                source_username=profile.source_username,
                source_fingerprint=profile.source_fingerprint,
                primary_function=assessment.primary_function,
                secondary_functions=[item.value for item in assessment.secondary_functions],
                industry_tags=assessment.industry_tags,
                region_tags=assessment.region_tags,
                language_tags=assessment.language_tags,
                job_signal_prior=assessment.job_signal_prior,
                estimated_noise_level=assessment.estimated_noise_level,
                reliability_score=assessment.reliability_score,
                confidence=assessment.confidence,
                evidence=assessment.evidence,
                sampled_message_count=len(redacted_samples),
                expires_at=utc_now() + PROFILE_TTL,
            )
            await self.subscription_repo.consume_usage(reservation.ledger.id)
            return saved
        except Exception:
            await self.subscription_repo.release_usage(
                reservation.ledger.id,
                "source function profiling failed",
            )
            raise
