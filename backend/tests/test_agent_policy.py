from datetime import datetime, timezone

from app.domain.enums import AgentActionType, LinkSafetyStatus, Priority
from app.domain.ports import (
    AgentActionRecommendation,
    AgentAnalysisResult,
    AgentContactExtraction,
    LinkInspection,
)
from app.domain.services.agent_policy import project_agent_result


def test_deterministic_link_risk_cannot_be_downgraded_by_agent() -> None:
    result = AgentAnalysisResult(
        is_opportunity=True,
        confidence=0.9,
        title="采购需求",
        summary="客户寻找供应商",
        priority=Priority.HIGH,
        trust_score=85,
        link_status=LinkSafetyStatus.SAFE,
        contacts=AgentContactExtraction(),
        actions=[],
    )
    inspection = LinkInspection(
        url="http://example.com",
        status=LinkSafetyStatus.SUSPICIOUS,
        emails=["buyer@example.com"],
        risk_reasons=["链接使用未加密的 HTTP"],
    )

    projection = project_agent_result(
        result,
        [inspection],
        analyzed_at=datetime(2026, 7, 10, tzinfo=timezone.utc),
    )

    assert projection.link_verification["status"] == "suspicious"
    assert projection.extracted_contacts["email"] == "buyer@example.com"
    assert projection.extracted_contacts["extractionSource"] == "link_content"


def test_external_actions_always_require_approval_and_internal_alert_does_not() -> None:
    result = AgentAnalysisResult(
        is_opportunity=True,
        confidence=0.99,
        title="重大采购",
        summary="采购窗口即将关闭",
        priority=Priority.URGENT,
        trust_score=90,
        attention_required=False,
        contacts=AgentContactExtraction(email="buyer@example.com"),
        actions=[
            AgentActionRecommendation(
                action_type=AgentActionType.SEND_EMAIL,
                reason="截止时间临近",
                target="buyer@example.com",
                draft="您好",
                requires_approval=False,
            ),
            AgentActionRecommendation(
                action_type=AgentActionType.NOTIFY_USER,
                reason="重大商机",
                requires_approval=True,
            ),
        ],
    )

    projection = project_agent_result(
        result,
        [],
        analyzed_at=datetime(2026, 7, 10, tzinfo=timezone.utc),
    )

    assert projection.actions[0]["requires_approval"] is True
    assert projection.actions[1]["requires_approval"] is False
    assert projection.attention_required is True
