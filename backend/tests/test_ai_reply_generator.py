from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.infrastructure.ai.litellm_client import (
    AIReplyGenerationError,
    AIReplyUnavailableError,
    LiteLLMReplyGenerator,
)


class FakeOpportunityRepository:
    async def get(self, opportunity_id):
        return SimpleNamespace(
            id=opportunity_id,
            channel="telegram",
            conversation_id="conversation-1",
            owner_user_id=uuid4(),
            contact_name="联系人",
            summary="需要企业方案",
            matched_keywords=["企业方案"],
        )


class FakeMessageRepository:
    async def list_by_conversation(self, *args, **kwargs):
        return []


def generator(*, ai_enabled: bool):
    return LiteLLMReplyGenerator(
        settings=SimpleNamespace(ai_enabled=ai_enabled, litellm_model="test-model"),
        opportunity_repo=FakeOpportunityRepository(),
        message_repo=FakeMessageRepository(),
    )


@pytest.mark.asyncio
async def test_disabled_ai_does_not_return_a_fixed_draft_as_provider_truth() -> None:
    with pytest.raises(AIReplyUnavailableError):
        await generator(ai_enabled=False).generate_reply(uuid4())


@pytest.mark.asyncio
async def test_ai_rejects_empty_provider_output(monkeypatch) -> None:
    class FakeChatLiteLLM:
        def __init__(self, **kwargs):
            pass

        async def ainvoke(self, messages):
            return SimpleNamespace(content="   ")

    monkeypatch.setattr(
        "app.infrastructure.ai.litellm_client.ChatLiteLLM",
        FakeChatLiteLLM,
    )
    with pytest.raises(AIReplyGenerationError):
        await generator(ai_enabled=True).generate_reply(uuid4())
