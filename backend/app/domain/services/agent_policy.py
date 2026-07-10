from __future__ import annotations

from datetime import datetime

from app.domain.enums import AgentActionType, LinkSafetyStatus, Priority
from app.domain.ports import (
    AgentAnalysisProjection,
    AgentAnalysisResult,
    LinkInspection,
)

_EXTERNAL_ACTIONS = {
    AgentActionType.SEND_EMAIL,
    AgentActionType.ADD_FRIEND,
    AgentActionType.PRIVATE_MESSAGE,
}


def _unique(values: list[str], *, limit: int = 20) -> list[str]:
    result: list[str] = []
    for value in values:
        normalized = value.strip()
        if normalized and normalized not in result:
            result.append(normalized)
        if len(result) >= limit:
            break
    return result


def _link_status(
    result: AgentAnalysisResult,
    inspections: list[LinkInspection],
) -> LinkSafetyStatus:
    if not inspections:
        return LinkSafetyStatus.UNVERIFIED
    if result.link_status == LinkSafetyStatus.MALICIOUS:
        return LinkSafetyStatus.MALICIOUS
    if result.link_status == LinkSafetyStatus.SUSPICIOUS or any(
        inspection.status in {LinkSafetyStatus.SUSPICIOUS, LinkSafetyStatus.MALICIOUS}
        for inspection in inspections
    ):
        return LinkSafetyStatus.SUSPICIOUS
    return LinkSafetyStatus.SAFE


def project_agent_result(
    result: AgentAnalysisResult,
    inspections: list[LinkInspection],
    *,
    analyzed_at: datetime,
) -> AgentAnalysisProjection:
    status = _link_status(result, inspections)
    risk_reasons = _unique(
        [reason for inspection in inspections for reason in inspection.risk_reasons]
        + result.risk_flags
    )
    inspected_emails = _unique(
        [email for inspection in inspections for email in inspection.emails],
        limit=20,
    )
    contacts = result.contacts.model_dump(mode="json")
    if not contacts["email"] and inspected_emails:
        contacts["email"] = inspected_emails[0]
        contacts["extraction_source"] = "link_content"

    actions: list[dict] = []
    notify_recommended = False
    for action in result.actions:
        payload = action.model_dump(mode="json")
        if action.action_type in _EXTERNAL_ACTIONS:
            payload["requires_approval"] = True
        elif action.action_type == AgentActionType.NOTIFY_USER:
            payload["requires_approval"] = False
            notify_recommended = True
        actions.append(payload)

    attention_required = (
        result.attention_required
        or result.priority == Priority.URGENT
        or notify_recommended
    )
    return AgentAnalysisProjection(
        result=result,
        link_verification={
            "status": status.value,
            "verifiedAt": analyzed_at.isoformat() if inspections else None,
            "riskReasons": risk_reasons,
            "resolvedInfo": result.link_summary or result.summary,
        },
        extracted_contacts={
            "phone": contacts["phone"],
            "email": contacts["email"],
            "telegramHandle": contacts["telegram_handle"],
            "wecomId": contacts["wecom_id"],
            "extractionSource": contacts["extraction_source"],
        },
        actions=actions,
        attention_required=attention_required,
        analyzed_at=analyzed_at,
    )
