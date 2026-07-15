from __future__ import annotations

import re
from dataclasses import dataclass

from app.domain.enums import (
    AgentAnalysisStatus,
    AutoReplyDecisionReason,
    IMChannel,
    OpportunityStatus,
    Priority,
)


URL_PATTERN = re.compile(r"https?://|www\.", re.IGNORECASE)
AMOUNT_PATTERN = re.compile(
    r"(?:[¥￥$€£]\s*\d|\d[\d,.]*\s*(?:元|万|万元|美元|美金|人民币|rmb|usd|eur))",
    re.IGNORECASE,
)
SENSITIVE_INTENT_PATTERN = re.compile(
    r"报价|价格|费用|折扣|合同|签约|付款|支付|收款|发票|退款|赔偿|违约|法律|律师|"
    r"保证|承诺|交付日期|上线日期|payment|refund|contract|invoice|legal|guarantee",
    re.IGNORECASE,
)
COMMITMENT_PATTERN = re.compile(
    r"(?:我方|我们|本公司)?(?:保证|承诺|确认可以|一定可以|确保|负责赔偿|同意退款)|"
    r"(?:guarantee|we promise|will refund|legally binding)",
    re.IGNORECASE,
)


@dataclass(frozen=True, slots=True)
class AutoReplyPolicyInput:
    feature_enabled: bool
    send_enabled: bool
    user_enabled: bool
    is_working_time: bool
    source_eligible: bool
    source_enabled: bool
    channel: IMChannel
    source_type: str
    opportunity_status: OpportunityStatus
    archived: bool
    assigned_to: str | None
    agent_status: AgentAnalysisStatus
    confidence: float
    min_confidence: float
    priority: Priority
    attention_required: bool
    link_status: str
    has_links: bool
    message_text: str
    recent_sent_count: int
    window_sent_count: int
    max_per_window: int


@dataclass(frozen=True, slots=True)
class AutoReplyPolicyDecision:
    allowed: bool
    reason: AutoReplyDecisionReason


def evaluate_auto_reply(input: AutoReplyPolicyInput) -> AutoReplyPolicyDecision:
    if not input.feature_enabled:
        return _deny(AutoReplyDecisionReason.FEATURE_DISABLED)
    if not input.send_enabled:
        return _deny(AutoReplyDecisionReason.SEND_DISABLED)
    if not input.user_enabled:
        return _deny(AutoReplyDecisionReason.USER_DISABLED)
    if input.is_working_time:
        return _deny(AutoReplyDecisionReason.WORKING_HOURS)
    if (
        not input.source_eligible
        or input.channel != IMChannel.TELEGRAM
        or input.source_type != "private"
    ):
        return _deny(AutoReplyDecisionReason.SOURCE_NOT_ELIGIBLE)
    if not input.source_enabled:
        return _deny(AutoReplyDecisionReason.SOURCE_DISABLED)
    if (
        input.archived
        or input.assigned_to
        or input.opportunity_status != OpportunityStatus.AI_AUTO_REPLY
    ):
        return _deny(AutoReplyDecisionReason.OPPORTUNITY_INACTIVE)
    if input.agent_status != AgentAnalysisStatus.COMPLETED:
        return _deny(AutoReplyDecisionReason.AGENT_NOT_COMPLETED)
    if input.confidence < input.min_confidence:
        return _deny(AutoReplyDecisionReason.LOW_CONFIDENCE)
    if input.attention_required or input.priority in {Priority.HIGH, Priority.URGENT}:
        return _deny(AutoReplyDecisionReason.ATTENTION_REQUIRED)
    if input.has_links and input.link_status != "safe":
        return _deny(AutoReplyDecisionReason.UNSAFE_LINK)
    if input.link_status in {"suspicious", "malicious"}:
        return _deny(AutoReplyDecisionReason.UNSAFE_LINK)
    if SENSITIVE_INTENT_PATTERN.search(input.message_text):
        return _deny(AutoReplyDecisionReason.SENSITIVE_INTENT)
    if input.recent_sent_count > 0:
        return _deny(AutoReplyDecisionReason.COOLDOWN_ACTIVE)
    if input.window_sent_count >= input.max_per_window:
        return _deny(AutoReplyDecisionReason.WINDOW_LIMIT_REACHED)
    return AutoReplyPolicyDecision(True, AutoReplyDecisionReason.ELIGIBLE)


def validate_auto_reply_draft(text: str, *, max_chars: int) -> AutoReplyPolicyDecision:
    normalized = text.strip()
    if not normalized or len(normalized) > max_chars:
        return _deny(AutoReplyDecisionReason.DRAFT_UNSAFE)
    if URL_PATTERN.search(normalized) or AMOUNT_PATTERN.search(normalized):
        return _deny(AutoReplyDecisionReason.DRAFT_UNSAFE)
    if COMMITMENT_PATTERN.search(normalized):
        return _deny(AutoReplyDecisionReason.DRAFT_UNSAFE)
    return AutoReplyPolicyDecision(True, AutoReplyDecisionReason.ELIGIBLE)


def _deny(reason: AutoReplyDecisionReason) -> AutoReplyPolicyDecision:
    return AutoReplyPolicyDecision(False, reason)
