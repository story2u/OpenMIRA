from app.application.dto import (
    AgentActionRead,
    AuthUserRead,
    ChatMessageRead,
    DetectionSettingsRead,
    NotificationSettingsRead,
    OpportunityDetailRead,
    OpportunityRead,
    ReplyTemplateRead,
    TelegramConnectionAttemptRead,
    TelegramConnectionRead,
    TelegramMonitorRead,
    TelegramSourceRead,
    TelegramUserConfigRead,
    WeComConnectionRead,
    WeComArchiveConnectionRead,
    WeComArchiveMemberBindingRead,
    WeComSourceRead,
    WorkScheduleRead,
    WorkScheduleSlot,
)
from app.domain.enums import (
    FrontendOpportunityStatus,
    MessageDirection,
    OpportunityStatus,
    TelegramConnectionType,
    TelegramSourceType,
)
from app.infrastructure.db.models import (
    Message,
    Opportunity,
    ReplyTemplate,
    TelegramConnection,
    TelegramConnectionAttempt,
    TelegramMonitor,
    TelegramSource,
    TelegramUserConfig,
    User,
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
    WeComConnection,
    WeComArchiveConnection,
    WeComArchiveCursor,
    WeComArchiveMemberBinding,
    WeComSource,
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
        archivedAt=opportunity.archived_at,
        archivedByUserId=opportunity.archived_by_user_id,
        archiveReason=opportunity.archive_reason,
    )


def to_opportunity_detail(opportunity: Opportunity) -> OpportunityDetailRead:
    base = to_opportunity_read(opportunity)
    return OpportunityDetailRead(
        **base.model_dump(),
        aiReplyDraft=opportunity.ai_reply_draft,
        finalReply=opportunity.final_reply,
        detectionReason=opportunity.detection_reason,
    )


def to_detection_settings_read(pref: UserDetectionPreference | None) -> DetectionSettingsRead:
    if not pref:
        return DetectionSettingsRead(keywords=[], aiSemanticsEnabled=True)
    return DetectionSettingsRead(
        keywords=list(pref.keywords),
        aiSemanticsEnabled=pref.ai_semantics_enabled,
    )


def to_work_schedule_read(
    schedule: UserWorkSchedule | None,
    *,
    default_timezone: str,
) -> WorkScheduleRead:
    if not schedule:
        return WorkScheduleRead(timezone=default_timezone, slots=[], isDefault=True)
    return WorkScheduleRead(
        timezone=schedule.timezone,
        slots=[WorkScheduleSlot(**slot) for slot in schedule.slots],
        autoReplyOutsideHours=schedule.auto_reply_outside_hours,
        isDefault=False,
    )


def to_notification_settings_read(
    pref: UserNotificationPreference | None,
) -> NotificationSettingsRead:
    if not pref:
        return NotificationSettingsRead()
    return NotificationSettingsRead(
        newOpportunityEnabled=pref.new_opportunity_enabled,
        aiRepliedEnabled=pref.ai_replied_enabled,
        dailyDigestEnabled=pref.daily_digest_enabled,
        urgentOnly=pref.urgent_only,
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
        hasPassword=bool(user.password_hash),
    )


def to_telegram_monitor_read(monitor: TelegramMonitor) -> TelegramMonitorRead:
    return TelegramMonitorRead(
        id=monitor.id,
        enabled=monitor.enabled,
        name=monitor.name,
        chatId=monitor.chat_id,
        chatTitle=monitor.chat_title,
        backfillLimit=monitor.backfill_limit,
        quotaPaused=monitor.quota_paused,
        quotaReason=monitor.quota_reason,
        lastError=monitor.last_error,
        updatedAt=monitor.updated_at,
    )


def to_telegram_user_config_read(
    config: TelegramUserConfig | None,
    monitors: list[TelegramMonitor] | None = None,
    monitor_limit: int = 1,
) -> TelegramUserConfigRead:
    monitor_reads = [to_telegram_monitor_read(monitor) for monitor in monitors or []]
    enabled_monitors = [monitor for monitor in monitors or [] if monitor.enabled]
    active_monitor_count = sum(not monitor.quota_paused for monitor in enabled_monitors)
    retention_selection_required = len(enabled_monitors) > monitor_limit and (
        not config or config.retention_limit != monitor_limit
    )
    if not config:
        return TelegramUserConfigRead(
            monitors=monitor_reads,
            monitorLimit=monitor_limit,
            canCreateMore=len(enabled_monitors) < monitor_limit,
            activeMonitorCount=active_monitor_count,
            storedMonitorCount=len(monitor_reads),
            retentionSelectionRequired=retention_selection_required,
        )
    return TelegramUserConfigRead(
        apiId=config.api_id,
        apiHashConfigured=bool(config.api_hash_encrypted),
        sessionConfigured=bool(config.session_encrypted),
        monitors=monitor_reads,
        monitorLimit=monitor_limit,
        canCreateMore=len(enabled_monitors) < monitor_limit,
        activeMonitorCount=active_monitor_count,
        storedMonitorCount=len(monitor_reads),
        retentionSelectionRequired=retention_selection_required,
        retentionSelectedAt=config.retention_selected_at,
        updatedAt=config.updated_at,
    )


def to_telegram_source_read(
    source: TelegramSource,
    *,
    connection_type: TelegramConnectionType,
    connection_capabilities: dict,
) -> TelegramSourceRead:
    auto_reply_eligible = (
        connection_type == TelegramConnectionType.BUSINESS
        and source.source_type == TelegramSourceType.PRIVATE
        and connection_capabilities.get("can_reply") is True
    )
    return TelegramSourceRead(
        id=source.id,
        connectionId=source.connection_id,
        sourceType=source.source_type,
        externalChatId=source.external_chat_id,
        displayName=source.display_name,
        username=source.username,
        enabled=source.enabled,
        autoReplyEnabled=source.auto_reply_enabled,
        autoReplyEligible=auto_reply_eligible,
        quotaPaused=source.quota_paused,
        quotaReason=source.quota_reason,
        lastError=source.last_error,
        updatedAt=source.updated_at,
    )


def to_telegram_connection_read(
    connection: TelegramConnection,
    sources: list[TelegramSource],
) -> TelegramConnectionRead:
    return TelegramConnectionRead(
        id=connection.id,
        connectionType=connection.connection_type,
        status=connection.status,
        enabled=connection.enabled,
        label=connection.label,
        capabilities=connection.capabilities,
        lastError=connection.last_error,
        lastCheckedAt=connection.last_checked_at,
        updatedAt=connection.updated_at,
        sources=[
            to_telegram_source_read(
                source,
                connection_type=connection.connection_type,
                connection_capabilities=connection.capabilities,
            )
            for source in sources
        ],
    )


def to_telegram_connection_attempt_read(
    attempt: TelegramConnectionAttempt,
    *,
    telegram_url: str | None = None,
    qr_code_url: str | None = None,
    instructions: list[str] | None = None,
    local_mock: bool = False,
) -> TelegramConnectionAttemptRead:
    return TelegramConnectionAttemptRead(
        id=attempt.id,
        connectionType=attempt.connection_type,
        status=attempt.status,
        expiresAt=attempt.expires_at,
        connectionId=attempt.connection_id,
        error=attempt.error,
        telegramUrl=telegram_url,
        qrCodeUrl=qr_code_url,
        instructions=instructions or [],
        localMock=local_mock,
    )


def to_wecom_source_read(source: WeComSource) -> WeComSourceRead:
    return WeComSourceRead(
        id=source.id,
        connectionId=source.connection_id,
        archiveConnectionId=source.archive_connection_id,
        sourceType=source.source_type,
        externalConversationId=source.external_conversation_id,
        displayName=source.display_name,
        receiveCapability=source.receive_capability,
        sendCapability=source.send_capability,
        enabled=source.enabled,
        quotaPaused=source.quota_paused,
        quotaReason=source.quota_reason,
        lastMessageAt=source.last_message_at,
        lastError=source.last_error,
    )


def to_wecom_connection_read(
    connection: WeComConnection,
    *,
    callback_url: str,
    sources: list[WeComSource],
) -> WeComConnectionRead:
    return WeComConnectionRead(
        id=connection.id,
        connectionType=connection.connection_type,
        status=connection.status,
        enabled=connection.enabled,
        displayName=connection.display_name,
        corpId=connection.corp_id,
        agentId=connection.agent_id,
        callbackUrl=callback_url,
        lastVerifiedAt=connection.last_verified_at,
        lastError=connection.last_error,
        updatedAt=connection.updated_at,
        sources=[to_wecom_source_read(source) for source in sources],
    )


def to_wecom_archive_connection_read(
    connection: WeComArchiveConnection,
    *,
    binding: WeComArchiveMemberBinding,
    cursor: WeComArchiveCursor,
    sdk_configured: bool,
    sources: list[WeComSource],
) -> WeComArchiveConnectionRead:
    return WeComArchiveConnectionRead(
        id=connection.id,
        status=connection.status,
        enabled=connection.enabled,
        displayName=connection.display_name,
        corpId=connection.corp_id,
        publicKeyVersion=connection.public_key_version,
        sdkConfigured=sdk_configured,
        lastSequence=cursor.last_seq,
        lastVerifiedAt=connection.last_verified_at,
        lastPolledAt=connection.last_polled_at,
        lastError=connection.last_error,
        updatedAt=connection.updated_at,
        member=WeComArchiveMemberBindingRead(
            id=binding.id,
            wecomUserId=binding.wecom_user_id,
            displayName=binding.display_name,
            enabled=binding.enabled,
            lastMatchedAt=binding.last_matched_at,
        ),
        sources=[to_wecom_source_read(source) for source in sources],
    )
