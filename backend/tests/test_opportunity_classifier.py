import json
from types import SimpleNamespace

import pytest

from app.core.config import Settings
from app.domain.enums import MessageDirection, Priority
from app.domain.ports import ConversationTurn, OpportunityClassificationRequest
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier


def settings(*, ai_enabled: bool) -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://test:test@localhost/test",
        admin_api_token="test-admin-token",
        ai_enabled=ai_enabled,
    )


@pytest.mark.asyncio
async def test_classifier_sends_normalized_context_and_validates_result(monkeypatch) -> None:
    captured_messages = []

    class FakeChatLiteLLM:
        def __init__(self, **kwargs) -> None:
            assert kwargs["temperature"] == 0.1
            assert kwargs["max_tokens"] == 600

        async def ainvoke(self, messages):
            captured_messages.extend(messages)
            return SimpleNamespace(
                content=json.dumps(
                    {
                        "is_opportunity": True,
                        "confidence": 0.86,
                        "title": "内部系统选型",
                        "summary": "团队希望本月确定内部系统",
                        "matched_keywords": [],
                        "priority": "high",
                        "reason": "上文包含团队规模和明确时间",
                    },
                    ensure_ascii=False,
                )
            )

    monkeypatch.setattr(
        "app.infrastructure.ai.litellm_client.ChatLiteLLM",
        FakeChatLiteLLM,
    )
    classifier = LiteLLMOpportunityClassifier(settings(ai_enabled=True))

    result = await classifier.classify(
        OpportunityClassificationRequest(
            text="有类似方案吗？",
            rule_score=0.0,
            ai_hints=["内部工具选型"],
            conversation=[
                ConversationTurn(
                    sender_display_name="Alice",
                    direction=MessageDirection.INCOMING,
                    text="我们 200 人，本月要确定。",
                )
            ],
            source_type="group",
            group_name="产品交流群",
        )
    )

    assert result is not None
    assert result.is_opportunity
    assert result.priority == Priority.HIGH
    prompt = captured_messages[1].content
    assert "我们 200 人，本月要确定" in prompt
    assert "内部工具选型" in prompt
    assert "产品交流群" in prompt


@pytest.mark.asyncio
async def test_classifier_returns_none_for_invalid_or_out_of_range_json(monkeypatch) -> None:
    class FakeChatLiteLLM:
        def __init__(self, **kwargs) -> None:
            pass

        async def ainvoke(self, messages):
            return SimpleNamespace(
                content=(
                    '{"is_opportunity":true,"confidence":2,"title":"x",'
                    '"summary":"x","matched_keywords":[],"priority":"high","reason":"x"}'
                )
            )

    monkeypatch.setattr(
        "app.infrastructure.ai.litellm_client.ChatLiteLLM",
        FakeChatLiteLLM,
    )
    classifier = LiteLLMOpportunityClassifier(settings(ai_enabled=True))

    result = await classifier.classify(
        OpportunityClassificationRequest(text="test", rule_score=0.0)
    )

    assert result is None


@pytest.mark.asyncio
async def test_classifier_does_not_construct_model_when_ai_is_disabled(monkeypatch) -> None:
    def fail_if_called(**kwargs):
        raise AssertionError("model must not be constructed when AI is disabled")

    monkeypatch.setattr(
        "app.infrastructure.ai.litellm_client.ChatLiteLLM",
        fail_if_called,
    )
    classifier = LiteLLMOpportunityClassifier(settings(ai_enabled=False))

    result = await classifier.classify(
        OpportunityClassificationRequest(text="需要一套内部方案", rule_score=0.0)
    )

    assert result is None
