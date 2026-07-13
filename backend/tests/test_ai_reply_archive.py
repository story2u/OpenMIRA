from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.application.use_cases.ai_reply import AIAutoReplyUseCase, transition_pending_to_ai
from app.domain.enums import OpportunityStatus


class ArchivedOpportunityRepository:
    def __init__(self, opportunity) -> None:
        self.opportunity = opportunity

    async def get(self, _):
        return self.opportunity

    async def update_status(self, *_args, **_kwargs):
        raise AssertionError("archived opportunity must not transition")


@pytest.mark.asyncio
async def test_archived_opportunity_never_executes_queued_auto_reply() -> None:
    opportunity = SimpleNamespace(
        id=uuid4(),
        status=OpportunityStatus.AI_AUTO_REPLY,
        archived_at=object(),
    )
    use_case = AIAutoReplyUseCase(
        opportunity_repo=SimpleNamespace(),
        message_repo=SimpleNamespace(),
        adapters=SimpleNamespace(),
        reply_generator=SimpleNamespace(),
    )

    assert await use_case.execute(opportunity) is opportunity


@pytest.mark.asyncio
async def test_archived_pending_opportunity_never_transitions_to_auto_reply() -> None:
    opportunity = SimpleNamespace(
        id=uuid4(),
        status=OpportunityStatus.PENDING_HUMAN,
        archived_at=object(),
    )
    repo = ArchivedOpportunityRepository(opportunity)

    assert await transition_pending_to_ai(repo, opportunity.id) is opportunity
