from uuid import uuid4

import pytest

from app.domain.enums import MessageDirection, Priority, RuleType
from app.domain.ports import (
    ConversationTurn,
    DetectionResult,
    DetectionRule,
    OpportunityClassificationRequest,
)
from app.domain.services.detection_policy import OpportunityDetector


class RecordingClassifier:
    def __init__(self, result: DetectionResult | None) -> None:
        self.result = result
        self.requests: list[OpportunityClassificationRequest] = []

    async def classify(
        self,
        request: OpportunityClassificationRequest,
    ) -> DetectionResult | None:
        self.requests.append(request)
        return self.result


class FailingClassifier:
    async def classify(self, request: OpportunityClassificationRequest) -> DetectionResult | None:
        raise RuntimeError("provider unavailable")


@pytest.mark.asyncio
async def test_detector_marks_high_intent_message_as_opportunity() -> None:
    detector = OpportunityDetector()
    result = await detector.detect(
        "我们想了解企业版 API 接入和 200 人批量采购报价",
        [
            DetectionRule(
                id=uuid4(),
                name="报价",
                rule_type=RuleType.KEYWORD,
                pattern="报价,批量采购",
                score=0.5,
                priority=10,
            ),
            DetectionRule(
                id=uuid4(),
                name="企业能力",
                rule_type=RuleType.KEYWORD,
                pattern="企业版,API",
                score=0.4,
                priority=20,
            ),
        ],
    )

    assert result.is_opportunity
    assert result.confidence >= 0.75
    assert "报价" in result.matched_keywords


@pytest.mark.asyncio
async def test_detector_marks_recruiting_group_message_as_opportunity() -> None:
    detector = OpportunityDetector()
    result = await detector.detect(
        "招聘 Python 后端工程师，远程全职，薪资 25k-35k，简历发 @hr_jobs",
        [],
    )

    assert result.is_opportunity
    assert result.confidence >= 0.75
    assert "招聘" in result.matched_keywords


@pytest.mark.asyncio
async def test_detector_semantically_reviews_zero_rule_score_with_context_and_hints() -> None:
    classifier = RecordingClassifier(
        DetectionResult(
            is_opportunity=True,
            confidence=0.88,
            title="团队正在比较内部知识库方案",
            summary="200 人团队希望月底前确定内部知识库方案",
            reason="结合上文的团队规模和时间要求，存在明确方案选型需求",
            priority=Priority.HIGH,
        )
    )
    detector = OpportunityDetector(ai_classifier=classifier)
    context = [
        ConversationTurn(
            sender_display_name="Alice",
            direction=MessageDirection.INCOMING,
            text="我们大约 200 人，月底前要确定。",
        )
    ]

    result = await detector.detect(
        "你们有类似方案吗？",
        [
            DetectionRule(
                id=uuid4(),
                name="知识库选型",
                rule_type=RuleType.AI_HINT,
                pattern="团队正在比较内部知识库方案",
                score=0.1,
                priority=10,
            )
        ],
        conversation=context,
        source_type="group",
        group_name="AI 产品交流群",
    )

    assert result.is_opportunity
    assert result.requires_human_review
    assert classifier.requests[0].rule_score == 0.0
    assert classifier.requests[0].conversation == context
    assert classifier.requests[0].ai_hints == ["团队正在比较内部知识库方案"]
    assert classifier.requests[0].group_name == "AI 产品交流群"


@pytest.mark.asyncio
async def test_detector_keeps_rule_result_when_ai_does_not_return_valid_result() -> None:
    classifier = RecordingClassifier(None)
    detector = OpportunityDetector(ai_classifier=classifier)

    result = await detector.detect("只是随便聊聊", [])

    assert not result.is_opportunity
    assert result.confidence == 0.0
    assert len(classifier.requests) == 1


@pytest.mark.asyncio
async def test_detector_does_not_call_ai_for_high_confidence_rule_match() -> None:
    classifier = RecordingClassifier(None)
    detector = OpportunityDetector(ai_classifier=classifier)

    result = await detector.detect(
        "需要采购报价",
        [
            DetectionRule(
                id=uuid4(),
                name="采购",
                rule_type=RuleType.KEYWORD,
                pattern="采购",
                score=0.75,
                priority=10,
            )
        ],
    )

    assert result.is_opportunity
    assert classifier.requests == []


@pytest.mark.asyncio
async def test_detector_falls_back_to_rules_when_provider_fails() -> None:
    detector = OpportunityDetector(ai_classifier=FailingClassifier())

    result = await detector.detect("需要报价", [])

    assert not result.is_opportunity
    assert result.confidence == 0.12
    assert result.matched_keywords == ["报价"]


@pytest.mark.asyncio
async def test_detector_accepts_ai_rejection_for_low_confidence_keyword_match() -> None:
    classifier = RecordingClassifier(
        DetectionResult(
            is_opportunity=False,
            confidence=0.93,
            title="供应商广告",
            summary="发送者正在推广自己的报价服务",
            reason="消息是自我推广，没有表达采购需求",
            priority=Priority.LOW,
        )
    )
    detector = OpportunityDetector(ai_classifier=classifier)

    result = await detector.detect("我们可以提供企业版报价，欢迎联系", [])

    assert not result.is_opportunity
    assert result.reason == "消息是自我推广，没有表达采购需求"
    assert "报价" in result.matched_keywords
