from dataclasses import replace

import pytest

from app.domain.enums import (
    AgentAnalysisStatus,
    AutoReplyDecisionReason,
    IMChannel,
    OpportunityStatus,
    Priority,
)
from app.domain.services.auto_reply_policy import (
    AutoReplyPolicyInput,
    evaluate_auto_reply,
    validate_auto_reply_draft,
)


@pytest.fixture
def eligible() -> AutoReplyPolicyInput:
    return AutoReplyPolicyInput(
        feature_enabled=True,
        send_enabled=True,
        user_enabled=True,
        is_working_time=False,
        source_eligible=True,
        source_enabled=True,
        channel=IMChannel.TELEGRAM,
        source_type="private",
        opportunity_status=OpportunityStatus.AI_AUTO_REPLY,
        archived=False,
        assigned_to=None,
        agent_status=AgentAnalysisStatus.COMPLETED,
        confidence=0.91,
        min_confidence=0.85,
        priority=Priority.NORMAL,
        attention_required=False,
        link_status="unverified",
        has_links=False,
        message_text="我们需要采购一批设备，可以先了解使用场景吗？",
        recent_sent_count=0,
        window_sent_count=0,
        max_per_window=1,
    )


def test_eligible_business_private_message_can_pass_policy(
    eligible: AutoReplyPolicyInput,
) -> None:
    decision = evaluate_auto_reply(eligible)

    assert decision.allowed is True
    assert decision.reason == AutoReplyDecisionReason.ELIGIBLE


@pytest.mark.parametrize(
    ("changes", "reason"),
    [
        ({"feature_enabled": False}, AutoReplyDecisionReason.FEATURE_DISABLED),
        ({"send_enabled": False}, AutoReplyDecisionReason.SEND_DISABLED),
        ({"user_enabled": False}, AutoReplyDecisionReason.USER_DISABLED),
        ({"is_working_time": True}, AutoReplyDecisionReason.WORKING_HOURS),
        ({"source_enabled": False}, AutoReplyDecisionReason.SOURCE_DISABLED),
        (
            {"agent_status": AgentAnalysisStatus.FAILED},
            AutoReplyDecisionReason.AGENT_NOT_COMPLETED,
        ),
        ({"confidence": 0.4}, AutoReplyDecisionReason.LOW_CONFIDENCE),
        ({"priority": Priority.HIGH}, AutoReplyDecisionReason.ATTENTION_REQUIRED),
        (
            {"has_links": True, "link_status": "unverified"},
            AutoReplyDecisionReason.UNSAFE_LINK,
        ),
        (
            {"message_text": "请给我报价 10 万并确认合同"},
            AutoReplyDecisionReason.SENSITIVE_INTENT,
        ),
        ({"recent_sent_count": 1}, AutoReplyDecisionReason.COOLDOWN_ACTIVE),
        ({"window_sent_count": 1}, AutoReplyDecisionReason.WINDOW_LIMIT_REACHED),
        ({"archived": True}, AutoReplyDecisionReason.OPPORTUNITY_INACTIVE),
    ],
)
def test_policy_fails_closed(
    eligible: AutoReplyPolicyInput,
    changes: dict,
    reason: AutoReplyDecisionReason,
) -> None:
    decision = evaluate_auto_reply(replace(eligible, **changes))

    assert decision.allowed is False
    assert decision.reason == reason


def test_groups_and_wecom_are_never_eligible(eligible: AutoReplyPolicyInput) -> None:
    group = evaluate_auto_reply(replace(eligible, source_type="group"))
    wecom = evaluate_auto_reply(replace(eligible, channel=IMChannel.WECOM))

    assert group.reason == AutoReplyDecisionReason.SOURCE_NOT_ELIGIBLE
    assert wecom.reason == AutoReplyDecisionReason.SOURCE_NOT_ELIGIBLE


@pytest.mark.parametrize(
    "draft",
    [
        "详情请访问 https://example.com",
        "项目费用为 10 万元。",
        "我们保证下周一定交付。",
        "x" * 241,
        "   ",
    ],
)
def test_unsafe_auto_reply_drafts_are_rejected(draft: str) -> None:
    decision = validate_auto_reply_draft(draft, max_chars=240)

    assert decision.allowed is False
    assert decision.reason == AutoReplyDecisionReason.DRAFT_UNSAFE


def test_safe_acknowledgement_draft_is_allowed() -> None:
    decision = validate_auto_reply_draft(
        "您好，需求已收到。方便补充一下预计使用人数吗？",
        max_chars=240,
    )

    assert decision.allowed is True
