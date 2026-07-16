from __future__ import annotations

import hashlib
from uuid import UUID

from app.domain.job_models import JobSearchProfilePreview
from app.infrastructure.agent.pi_client import PiAgentClient
from app.infrastructure.db.repositories import SubscriptionRepository


class JobProfileQuotaExceeded(RuntimeError):
    pass


class ParseJobSearchProfileUseCase:
    def __init__(
        self,
        *,
        agent: PiAgentClient,
        subscription_repo: SubscriptionRepository,
    ) -> None:
        self.agent = agent
        self.subscription_repo = subscription_repo

    async def execute(self, user_id: UUID, text: str) -> JobSearchProfilePreview:
        digest = hashlib.sha256(text.strip().encode()).hexdigest()
        reservation = await self.subscription_repo.reserve_agent_analysis(
            user_id=user_id,
            message_id=None,
            idempotency_key=f"job-profile-parse:{digest}",
        )
        if not reservation.allowed or not reservation.ledger:
            raise JobProfileQuotaExceeded("Pi Agent monthly quota exceeded")
        try:
            preview = await self.agent.parse_job_search_profile(text)
        except Exception:
            if reservation.created:
                await self.subscription_repo.release_usage(
                    reservation.ledger.id, "job search profile parsing failed"
                )
            raise
        if reservation.created:
            await self.subscription_repo.consume_usage(reservation.ledger.id)
        return preview
