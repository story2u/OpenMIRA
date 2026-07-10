from app.application.dto import (
    AgentActionRead,
    AuthUserRead,
    ChatMessageRead,
    OpportunityDetailRead,
    OpportunityRead,
    ReplyTemplateRead,
    TelegramMonitorRead,
    TelegramUserConfigRead,
)
from app.domain.enums import (
    FrontendOpportunityStatus,
    MessageDirection,
    OpportunityStatus,
)
from app.infrastructure.db.models import (
    Message,
    Opportunity,
    ReplyTemplate,
    TelegramMonitor,
    TelegramUserConfig,
    User,
)


def to_agent_action_read(action: dict) -> AgentActionRead:
    return AgentActionRead(
        actionType=action["action_type"],
        reason=action["reason"],
        target=action.get("target"),
        draft=action.get("draft"),
        requiresApproval=bool(action.get("requires_approval", True)),
    )


def frontend_status(status: OpportunityStatus) -> FrontendOpportunityStatus:
    if status in {OpportunityStatus.PENDING_HUMAN, OpportunityStatus.AI_AUTO_REPLY}:
        return FrontendOpportunityStatus.PENDING
    if status in {OpportunityStatus.REPLIED, OpportunityStatus.FOLLOWING}:
        return FrontendOpportunityStatus.REPLIED
    return FrontendOpportunityStatus.IGNORED


def to_opportunity_read(opportunity: Opportunity) -> OpportunityRead:
    return OpportunityRead(
        id=opportunity.id,
        platform=opportunity.channel,
        contactName=opportunity.contact_name,
        contactAvatar=opportunity.contact_avatar,
        summary=opportunity.summary or opportunity.title,
        matchedKeywords=opportunity.matched_keywords,
        confidenceScore=opportunity.confidence,
        status=frontend_status(opportunity.status),
        internalStatus=opportunity.status,
        priority=opportunity.priority,
        lastMessagePreview=opportunity.last_message_preview,
        createdAt=opportunity.created_at,
        updatedAt=opportunity.updated_at,
        sourceType=opportunity.source_type,
        groupName=opportunity.group_name,
        groupMemberRole="member",
        rawMessageLinks=opportunity.raw_message_links,
        linkVerification=opportunity.link_verification,
        extractedContacts=opportunity.extracted_contacts,
        friendRequestStatus=opportunity.friend_request_status,
        sopStage=opportunity.sop_stage,
        trustScore=opportunity.trust_score,
        agentActions=[to_agent_action_read(action) for action in opportunity.agent_actions],
        agentAnalysisStatus=opportunity.agent_analysis_status,
        agentAnalysisError=opportunity.agent_analysis_error,
        agentAnalyzedAt=opportunity.agent_analyzed_at,
        attentionRequired=opportunity.attention_required,
    )


def to_opportunity_detail(opportunity: Opportunity) -> OpportunityDetailRead:
    base = to_opportunity_read(opportunity)
    return OpportunityDetailRead(
        **base.model_dump(),
        aiReplyDraft=opportunity.ai_reply_draft,
        finalReply=opportunity.final_reply,
        detectionReason=opportunity.detection_reason,
    )


def to_chat_message_read(message: Message) -> ChatMessageRead:
    return ChatMessageRead(
        id=message.id,
        senderName=message.sender_display_name or "客户",
        content=message.text or "",
        isFromContact=message.direction == MessageDirection.INCOMING,
        sentAt=message.sent_at,
        source=message.source,
    )


def to_reply_template_read(template: ReplyTemplate) -> ReplyTemplateRead:
    return ReplyTemplateRead(
        id=template.id,
        title=template.title,
        content=template.content,
        category=template.category,
    )


def to_auth_user_read(user: User) -> AuthUserRead:
    return AuthUserRead(
        id=user.id,
        email=user.email,
        displayName=user.display_name,
        avatarUrl=user.avatar_url,
        isAdmin=user.is_admin,
    )


def to_telegram_monitor_read(monitor: TelegramMonitor) -> TelegramMonitorRead:
    return TelegramMonitorRead(
        id=monitor.id,
        enabled=monitor.enabled,
        name=monitor.name,
        chatId=monitor.chat_id,
        chatTitle=monitor.chat_title,
        backfillLimit=monitor.backfill_limit,
        lastError=monitor.last_error,
        updatedAt=monitor.updated_at,
    )


def to_telegram_user_config_read(
    config: TelegramUserConfig | None,
    monitors: list[TelegramMonitor] | None = None,
    monitor_limit: int = 1,
) -> TelegramUserConfigRead:
    monitor_reads = [to_telegram_monitor_read(monitor) for monitor in monitors or []]
    if not config:
        return TelegramUserConfigRead(
            monitors=monitor_reads,
            monitorLimit=monitor_limit,
            canCreateMore=len(monitor_reads) < monitor_limit,
        )
    return TelegramUserConfigRead(
        apiId=config.api_id,
        apiHashConfigured=bool(config.api_hash_encrypted),
        sessionConfigured=bool(config.session_encrypted),
        monitors=monitor_reads,
        monitorLimit=monitor_limit,
        canCreateMore=len(monitor_reads) < monitor_limit,
        updatedAt=config.updated_at,
    )
