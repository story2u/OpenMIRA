"""Transactional aggregate change capture for the bounded mobile sync feed.

The current repositories commit in several independent transactions. Capturing at
``before_flush`` keeps every emitted aggregate upsert/tombstone in the exact
transaction that changed its source row without pretending those transactions are
one domain event.
"""

from collections.abc import Callable
from datetime import datetime
from typing import Any
from uuid import UUID

from sqlalchemy import event
from sqlalchemy.orm import Session

from app.domain.enums import (
    MessageDirection,
    OpportunityStatus,
    SyncAggregateType,
    SyncOperation,
)
from app.infrastructure.db.models import (
    Message,
    Opportunity,
    SyncChange,
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
    utc_now,
)

TrackedAggregate = (
    Opportunity
    | Message
    | UserDetectionPreference
    | UserWorkSchedule
    | UserNotificationPreference
)


def _iso(value: datetime | None) -> str | None:
    return value.isoformat() if value is not None else None


def _frontend_status(status: OpportunityStatus) -> str:
    if status in {OpportunityStatus.PENDING_HUMAN, OpportunityStatus.AI_AUTO_REPLY}:
        return "pending"
    if status in {OpportunityStatus.REPLIED, OpportunityStatus.FOLLOWING}:
        return "replied"
    return "ignored"


def _agent_action_payload(action: dict[str, Any]) -> dict[str, Any]:
    return {
        "actionType": action.get("action_type", action.get("actionType")),
        "reason": action.get("reason", ""),
        "target": action.get("target"),
        "draft": action.get("draft"),
        "requiresApproval": bool(
            action.get("requires_approval", action.get("requiresApproval", True))
        ),
    }


def opportunity_payload(opportunity: Opportunity) -> dict[str, Any]:
    """Serialize only the public detail projection; provider payloads never enter the feed."""
    return {
        "id": str(opportunity.id),
        "platform": opportunity.channel.value,
        "contactName": opportunity.contact_name,
        "contactAvatar": opportunity.contact_avatar,
        "summary": opportunity.summary or opportunity.title,
        "matchedKeywords": list(opportunity.matched_keywords),
        "confidenceScore": opportunity.confidence,
        "status": _frontend_status(opportunity.status),
        "internalStatus": opportunity.status.value,
        "priority": opportunity.priority.value,
        "lastMessagePreview": opportunity.last_message_preview,
        "createdAt": _iso(opportunity.created_at),
        "updatedAt": _iso(opportunity.updated_at),
        "sourceType": opportunity.source_type,
        "groupName": opportunity.group_name,
        "groupMemberRole": "member",
        "rawMessageLinks": list(opportunity.raw_message_links),
        "linkVerification": opportunity.link_verification,
        "extractedContacts": opportunity.extracted_contacts,
        "friendRequestStatus": opportunity.friend_request_status,
        "sopStage": opportunity.sop_stage,
        "trustScore": opportunity.trust_score,
        "agentActions": [_agent_action_payload(action) for action in opportunity.agent_actions],
        "agentAnalysisStatus": opportunity.agent_analysis_status.value,
        "agentAnalysisError": opportunity.agent_analysis_error,
        "agentAnalyzedAt": _iso(opportunity.agent_analyzed_at),
        "attentionRequired": opportunity.attention_required,
        "archivedAt": _iso(opportunity.archived_at),
        "archivedByUserId": (
            str(opportunity.archived_by_user_id)
            if opportunity.archived_by_user_id is not None
            else None
        ),
        "archiveReason": opportunity.archive_reason,
        "aiReplyDraft": opportunity.ai_reply_draft,
        "finalReply": opportunity.final_reply,
        "detectionReason": opportunity.detection_reason,
        "assignedTo": opportunity.assigned_to,
    }


def message_payload(message: Message) -> dict[str, Any]:
    return {
        "id": str(message.id),
        "opportunityId": str(message.opportunity_id) if message.opportunity_id else None,
        "senderName": message.sender_display_name or "客户",
        "content": message.text or "",
        "isFromContact": message.direction == MessageDirection.INCOMING,
        "sentAt": _iso(message.sent_at),
        "source": message.source.value if message.source else None,
    }


def detection_payload(preference: UserDetectionPreference) -> dict[str, Any]:
    return {
        "keywords": list(preference.keywords),
        "aiSemanticsEnabled": preference.ai_semantics_enabled,
    }


def work_schedule_payload(schedule: UserWorkSchedule) -> dict[str, Any]:
    return {
        "timezone": schedule.timezone,
        "slots": list(schedule.slots),
        "autoReplyOutsideHours": schedule.auto_reply_outside_hours,
        "isDefault": False,
    }


def notification_payload(preference: UserNotificationPreference) -> dict[str, Any]:
    return {
        "newOpportunityEnabled": preference.new_opportunity_enabled,
        "aiRepliedEnabled": preference.ai_replied_enabled,
        "dailyDigestEnabled": preference.daily_digest_enabled,
        "urgentOnly": preference.urgent_only,
    }


SERIALIZERS: dict[type, Callable[[Any], dict[str, Any]]] = {
    Opportunity: opportunity_payload,
    Message: message_payload,
    UserDetectionPreference: detection_payload,
    UserWorkSchedule: work_schedule_payload,
    UserNotificationPreference: notification_payload,
}

AGGREGATE_TYPES: dict[type, SyncAggregateType] = {
    Opportunity: SyncAggregateType.OPPORTUNITY,
    Message: SyncAggregateType.MESSAGE,
    UserDetectionPreference: SyncAggregateType.USER_DETECTION_PREFERENCE,
    UserWorkSchedule: SyncAggregateType.USER_WORK_SCHEDULE,
    UserNotificationPreference: SyncAggregateType.USER_NOTIFICATION_PREFERENCE,
}


def aggregate_owner_id(aggregate: TrackedAggregate) -> UUID | None:
    if isinstance(aggregate, (Opportunity, Message)):
        return aggregate.owner_user_id
    return aggregate.user_id


def aggregate_identity(aggregate: TrackedAggregate) -> UUID:
    if isinstance(
        aggregate,
        (UserDetectionPreference, UserWorkSchedule, UserNotificationPreference),
    ):
        return aggregate.user_id
    return aggregate.id


def _append_sync_changes(session: Session, _flush_context: object, _instances: object) -> None:
    tracked_types = tuple(SERIALIZERS)
    new_aggregates = [item for item in session.new if isinstance(item, tracked_types)]
    dirty_aggregates = [
        item
        for item in session.dirty
        if isinstance(item, tracked_types) and session.is_modified(item, include_collections=True)
    ]
    deleted_aggregates = [item for item in session.deleted if isinstance(item, tracked_types)]

    for aggregate in [*new_aggregates, *dirty_aggregates, *deleted_aggregates]:
        is_new = aggregate in session.new
        is_deleted = aggregate in session.deleted
        aggregate.aggregate_version = (
            max(1, aggregate.aggregate_version)
            if is_new
            else aggregate.aggregate_version + 1
        )
        owner_user_id = aggregate_owner_id(aggregate)
        if owner_user_id is None:
            continue
        aggregate_type = AGGREGATE_TYPES[type(aggregate)]
        session.add(
            SyncChange(
                owner_user_id=owner_user_id,
                aggregate_type=aggregate_type,
                aggregate_id=aggregate_identity(aggregate),
                aggregate_version=aggregate.aggregate_version,
                operation=SyncOperation.DELETE if is_deleted else SyncOperation.UPSERT,
                schema_version=1,
                payload=None if is_deleted else SERIALIZERS[type(aggregate)](aggregate),
                created_at=utc_now(),
            )
        )


def install_sync_change_tracking() -> None:
    if not event.contains(Session, "before_flush", _append_sync_changes):
        event.listen(Session, "before_flush", _append_sync_changes)
