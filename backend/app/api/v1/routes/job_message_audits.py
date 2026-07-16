from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.api.deps import get_job_message_audit_repo, require_user
from app.application.dto import (
    JobMessageAuditCorrectionRequest,
    JobMessageAuditPageRead,
    JobMessageAuditRead,
)
from app.domain.enums import JobMessageClassification
from app.infrastructure.db.models import JobMessageAudit, Message, User
from app.infrastructure.db.repositories import JobMessageAuditRepository

router = APIRouter()


def _to_read(audit: JobMessageAudit, message: Message) -> JobMessageAuditRead:
    text = " ".join((message.text or "").split())
    return JobMessageAuditRead(
        id=audit.id,
        messageId=message.id,
        channel=message.channel,
        sourceName=message.group_name or message.sender_display_name,
        messageExcerpt=text[:280],
        classification=audit.classification,
        confidence=audit.confidence,
        filterReason=audit.filter_reason,
        prefilterScore=audit.prefilter_score,
        agentRequired=audit.agent_required,
        manuallyCorrected=audit.manually_corrected,
        sentAt=message.sent_at,
        updatedAt=audit.updated_at,
    )


@router.get("", response_model=JobMessageAuditPageRead)
async def list_job_message_audits(
    classification: JobMessageClassification | None = None,
    manually_corrected: bool | None = None,
    limit: int = Query(default=20, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: JobMessageAuditRepository = Depends(get_job_message_audit_repo),
) -> JobMessageAuditPageRead:
    rows, total = await repo.list_for_owner(
        owner_user_id=current_user.id,
        classification=classification,
        manually_corrected=manually_corrected,
        limit=limit,
        offset=offset,
    )
    return JobMessageAuditPageRead(
        items=[_to_read(audit, message) for audit, message in rows],
        total=total,
        limit=limit,
        offset=offset,
    )


@router.patch("/{audit_id}/correction", response_model=JobMessageAuditRead)
async def correct_job_message_audit(
    audit_id: UUID,
    payload: JobMessageAuditCorrectionRequest,
    current_user: User = Depends(require_user),
    repo: JobMessageAuditRepository = Depends(get_job_message_audit_repo),
) -> JobMessageAuditRead:
    row = await repo.correct_for_owner(
        audit_id=audit_id,
        owner_user_id=current_user.id,
        is_job=payload.isJob,
        note=payload.note,
    )
    if not row:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="audit not found")
    return _to_read(*row)
