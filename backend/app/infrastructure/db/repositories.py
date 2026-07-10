from __future__ import annotations

from datetime import datetime, timedelta, timezone
from uuid import UUID

import structlog
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
from sqlmodel import col, select
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import (
    AgentAnalysisStatus,
    FrontendOpportunityStatus,
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityStatus,
    Priority,
)
from app.domain.ports import AgentAnalysisProjection, DetectionRule, InboundMessage
from app.infrastructure.db.models import (
    AppConfig,
    AuthAccount,
    Message,
    Opportunity,
    ReplyTemplate,
    Rule,
    TelegramMonitor,
    TelegramUserConfig,
    User,
    utc_now,
)

logger = structlog.get_logger(__name__)


FRONTEND_STATUS_MAP: dict[FrontendOpportunityStatus, set[OpportunityStatus]] = {
    FrontendOpportunityStatus.PENDING: {
        OpportunityStatus.PENDING_HUMAN,
        OpportunityStatus.AI_AUTO_REPLY,
    },
    FrontendOpportunityStatus.REPLIED: {
        OpportunityStatus.REPLIED,
        OpportunityStatus.FOLLOWING,
    },
    FrontendOpportunityStatus.IGNORED: {
        OpportunityStatus.IGNORED,
        OpportunityStatus.CLOSED,
    },
}


class UserRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def count(self) -> int:
        result = await self.session.exec(select(func.count()).select_from(User))
        return int(result.one())

    async def get(self, user_id: UUID) -> User | None:
        return await self.session.get(User, user_id)

    async def get_by_email(self, email: str) -> User | None:
        statement = select(User).where(User.email == email.lower().strip())
        result = await self.session.exec(statement)
        return result.first()

    async def get_by_auth_account(self, provider: str, provider_subject: str) -> User | None:
        statement = (
            select(User)
            .join(AuthAccount)
            .where(
                AuthAccount.provider == provider,
                AuthAccount.provider_subject == provider_subject,
            )
        )
        result = await self.session.exec(statement)
        return result.first()

    async def list(self) -> list[User]:
        result = await self.session.exec(select(User).order_by(col(User.created_at).asc()))
        return list(result.all())

    async def create_oauth_user(
        self,
        *,
        email: str,
        display_name: str,
        avatar_url: str = "",
    ) -> User:
        user = User(
            email=email.lower().strip(),
            display_name=display_name.strip() or email.lower().strip(),
            avatar_url=avatar_url,
        )
        self.session.add(user)
        await self.session.commit()
        await self.session.refresh(user)
        return user

    async def link_auth_account(
        self,
        *,
        user: User,
        provider: str,
        provider_subject: str,
        email: str | None,
    ) -> AuthAccount:
        account = AuthAccount(
            user_id=user.id,
            provider=provider,
            provider_subject=provider_subject,
            email=email.lower().strip() if email else None,
        )
        self.session.add(account)
        await self.session.commit()
        await self.session.refresh(account)
        return account

    async def get_or_create_oauth_user(
        self,
        *,
        provider: str,
        provider_subject: str,
        email: str,
        display_name: str,
        avatar_url: str = "",
    ) -> User:
        auth_account_available = True
        try:
            user = await self.get_by_auth_account(provider, provider_subject)
        except SQLAlchemyError as exc:
            await self.session.rollback()
            auth_account_available = False
            logger.warning(
                "oauth.auth_account_lookup_failed",
                provider=provider,
                error_class=exc.__class__.__name__,
            )
            user = None
        if user:
            return await self.mark_login(user)

        user = await self.get_by_email(email)
        if not user:
            try:
                user = await self.create_oauth_user(
                    email=email,
                    display_name=display_name,
                    avatar_url=avatar_url,
                )
            except IntegrityError:
                await self.session.rollback()
                user = await self.get_by_email(email)
                if not user:
                    raise

        if auth_account_available:
            try:
                await self.link_auth_account(
                    user=user,
                    provider=provider,
                    provider_subject=provider_subject,
                    email=email,
                )
            except IntegrityError:
                await self.session.rollback()
                linked_user = await self.get_by_auth_account(provider, provider_subject)
                if linked_user:
                    return await self.mark_login(linked_user)
                raise
            except SQLAlchemyError as exc:
                await self.session.rollback()
                logger.warning(
                    "oauth.auth_account_link_failed",
                    provider=provider,
                    user_id=str(user.id),
                    error_class=exc.__class__.__name__,
                )
        return await self.mark_login(user)

    async def mark_login(self, user: User) -> User:
        user.last_login_at = utc_now()
        user.updated_at = utc_now()
        self.session.add(user)
        await self.session.commit()
        await self.session.refresh(user)
        return user


class MessageRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_by_external_id(self, channel: IMChannel, external_message_id: str) -> Message | None:
        statement = select(Message).where(
            Message.channel == channel,
            Message.external_message_id == external_message_id,
        )
        result = await self.session.exec(statement)
        return result.first()

    async def get(self, message_id: UUID) -> Message | None:
        return await self.session.get(Message, message_id)

    async def create_incoming(self, inbound: InboundMessage) -> Message:
        message = Message(
            owner_user_id=inbound.owner_user_id,
            channel=inbound.channel,
            external_message_id=inbound.external_message_id,
            conversation_id=inbound.conversation_id,
            sender_external_id=inbound.sender_external_id,
            sender_display_name=inbound.sender_display_name,
            direction=MessageDirection.INCOMING,
            text=inbound.text,
            source_type=inbound.source_type,
            group_name=inbound.group_name,
            raw_message_links=inbound.raw_message_links,
            raw_payload=inbound.raw_payload,
        )
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def mark_agent_queued(self, message_id: UUID, *, force: bool = False) -> Message | None:
        message = await self.session.get(Message, message_id)
        if not message or message.agent_analysis_status == AgentAnalysisStatus.RUNNING:
            return message
        if message.agent_analysis_status == AgentAnalysisStatus.COMPLETED and not force:
            return message
        message.agent_analysis_status = AgentAnalysisStatus.QUEUED
        message.agent_error = None
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def claim_agent_analysis(self, message_id: UUID, *, force: bool = False) -> Message | None:
        message = await self.session.get(Message, message_id, with_for_update=True)
        if not message:
            return None
        if (
            message.agent_analysis_status == AgentAnalysisStatus.RUNNING
            and message.agent_started_at
            and message.agent_started_at > utc_now() - timedelta(minutes=10)
        ):
            return None
        if message.agent_analysis_status == AgentAnalysisStatus.COMPLETED and not force:
            return None
        now = utc_now()
        message.agent_analysis_status = AgentAnalysisStatus.RUNNING
        message.agent_started_at = now
        message.agent_error = None
        message.updated_at = now
        self.session.add(message)
        if message.opportunity_id:
            opportunity = await self.session.get(Opportunity, message.opportunity_id)
            if opportunity:
                opportunity.agent_analysis_status = AgentAnalysisStatus.RUNNING
                opportunity.agent_analysis_error = None
                opportunity.sop_stage = "analyzing"
                opportunity.updated_at = now
                self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def complete_agent_analysis(
        self,
        message: Message,
        projection: AgentAnalysisProjection,
    ) -> Message:
        message.agent_analysis_status = AgentAnalysisStatus.COMPLETED
        message.agent_result = projection.model_dump(mode="json")
        message.agent_error = None
        message.agent_analyzed_at = projection.analyzed_at
        message.updated_at = projection.analyzed_at
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def fail_agent_analysis(self, message_id: UUID, error: str) -> None:
        # Recover from provider or database exceptions before writing the durable failure state.
        await self.session.rollback()
        message = await self.session.get(Message, message_id)
        if not message:
            return
        now = utc_now()
        safe_error = error[:1000]
        message.agent_analysis_status = AgentAnalysisStatus.FAILED
        message.agent_error = safe_error
        message.updated_at = now
        self.session.add(message)
        if message.opportunity_id:
            opportunity = await self.session.get(Opportunity, message.opportunity_id)
            if opportunity:
                opportunity.agent_analysis_status = AgentAnalysisStatus.FAILED
                opportunity.agent_analysis_error = safe_error
                opportunity.updated_at = now
                self.session.add(opportunity)
        await self.session.commit()

    async def create_outgoing(
        self,
        *,
        channel: IMChannel,
        conversation_id: str,
        text: str,
        source: MessageSource,
        opportunity_id: UUID,
        external_message_id: str,
        raw_payload: dict,
        owner_user_id: UUID | None = None,
    ) -> Message:
        message = Message(
            owner_user_id=owner_user_id,
            channel=channel,
            external_message_id=external_message_id,
            conversation_id=conversation_id,
            sender_display_name="商机助手",
            direction=MessageDirection.OUTGOING,
            source=source,
            text=text,
            raw_payload=raw_payload,
            opportunity_id=opportunity_id,
        )
        self.session.add(message)
        await self.session.commit()
        await self.session.refresh(message)
        return message

    async def attach_opportunity(self, message_id: UUID, opportunity_id: UUID) -> None:
        message = await self.session.get(Message, message_id)
        if not message:
            return
        message.opportunity_id = opportunity_id
        message.processed_at = utc_now()
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()

    async def mark_processed(self, message_id: UUID) -> None:
        message = await self.session.get(Message, message_id)
        if not message:
            return
        message.processed_at = utc_now()
        message.updated_at = utc_now()
        self.session.add(message)
        await self.session.commit()

    async def list_by_opportunity(self, opportunity_id: UUID) -> list[Message]:
        statement = (
            select(Message)
            .where(Message.opportunity_id == opportunity_id)
            .order_by(col(Message.sent_at).asc(), col(Message.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def list_by_conversation(
        self,
        channel: IMChannel,
        conversation_id: str,
        limit: int = 20,
    ) -> list[Message]:
        statement = (
            select(Message)
            .where(Message.channel == channel, Message.conversation_id == conversation_id)
            .order_by(col(Message.sent_at).desc())
            .limit(limit)
        )
        result = await self.session.exec(statement)
        return list(reversed(result.all()))


class OpportunityRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create(
        self,
        *,
        channel: IMChannel,
        owner_user_id: UUID | None,
        conversation_id: str,
        customer_external_id: str | None,
        contact_name: str | None,
        source_type: str,
        group_name: str | None,
        source_message_id: UUID,
        title: str,
        summary: str | None,
        matched_keywords: list[str],
        raw_message_links: list[str],
        confidence: float,
        priority: Priority,
        detection_reason: str | None,
        status: OpportunityStatus,
        last_message_preview: str,
    ) -> Opportunity:
        opportunity = Opportunity(
            owner_user_id=owner_user_id,
            channel=channel,
            conversation_id=conversation_id,
            customer_external_id=customer_external_id,
            contact_name=contact_name or customer_external_id or "未知联系人",
            source_type=source_type,
            group_name=group_name,
            source_message_id=source_message_id,
            title=title,
            summary=summary,
            matched_keywords=matched_keywords,
            raw_message_links=raw_message_links,
            trust_score=80 if not raw_message_links else 65,
            confidence=confidence,
            priority=priority,
            detection_reason=detection_reason,
            status=status,
            last_message_preview=last_message_preview,
            friend_request_status="not_sent" if source_type == "group" else "n/a",
        )
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def get(self, opportunity_id: UUID) -> Opportunity | None:
        return await self.session.get(Opportunity, opportunity_id)

    async def get_by_source_message(self, message_id: UUID) -> Opportunity | None:
        statement = select(Opportunity).where(Opportunity.source_message_id == message_id)
        result = await self.session.exec(statement)
        return result.first()

    async def apply_agent_projection(
        self,
        opportunity: Opportunity,
        projection: AgentAnalysisProjection,
    ) -> Opportunity:
        priority_order = {
            Priority.LOW: 0,
            Priority.NORMAL: 1,
            Priority.HIGH: 2,
            Priority.URGENT: 3,
        }
        opportunity.link_verification = projection.link_verification
        opportunity.extracted_contacts = projection.extracted_contacts
        opportunity.agent_actions = projection.actions
        opportunity.agent_analysis_status = AgentAnalysisStatus.COMPLETED
        opportunity.agent_analysis_error = None
        opportunity.agent_analyzed_at = projection.analyzed_at
        opportunity.attention_required = projection.attention_required
        opportunity.trust_score = projection.result.trust_score
        opportunity.confidence = max(opportunity.confidence, projection.result.confidence)
        if priority_order[projection.result.priority] > priority_order[opportunity.priority]:
            opportunity.priority = projection.result.priority

        link_status = projection.link_verification.get("status")
        has_contacts = any(
            projection.extracted_contacts.get(key)
            for key in ("phone", "email", "telegramHandle", "wecomId")
        )
        if link_status in {"suspicious", "malicious"}:
            opportunity.sop_stage = "analyzing"
        elif has_contacts:
            opportunity.sop_stage = (
                "contact_extracted" if opportunity.source_type == "group" else "ready_to_chat"
            )
        elif opportunity.raw_message_links:
            opportunity.sop_stage = "verified"
        opportunity.updated_at = projection.analyzed_at
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def list(
        self,
        *,
        frontend_status: FrontendOpportunityStatus | None = None,
        channel: IMChannel | None = None,
        owner_user_id: UUID | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[Opportunity]:
        statement = select(Opportunity)
        if frontend_status:
            statement = statement.where(Opportunity.status.in_(FRONTEND_STATUS_MAP[frontend_status]))
        if channel:
            statement = statement.where(Opportunity.channel == channel)
        if owner_user_id:
            statement = statement.where(Opportunity.owner_user_id == owner_user_id)
        statement = statement.order_by(col(Opportunity.last_message_at).desc()).offset(offset).limit(limit)
        result = await self.session.exec(statement)
        return list(result.all())

    async def update_status(
        self,
        opportunity: Opportunity,
        status: OpportunityStatus,
        *,
        final_reply: str | None = None,
        assigned_to: str | None = None,
    ) -> Opportunity:
        opportunity.status = status
        if final_reply is not None:
            opportunity.final_reply = final_reply
            opportunity.last_message_preview = final_reply
            opportunity.last_message_at = utc_now()
        if assigned_to is not None:
            opportunity.assigned_to = assigned_to
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def save_ai_draft(self, opportunity: Opportunity, draft: str) -> Opportunity:
        opportunity.ai_reply_draft = draft
        opportunity.updated_at = utc_now()
        self.session.add(opportunity)
        await self.session.commit()
        await self.session.refresh(opportunity)
        return opportunity

    async def pending_human_older_than(self, minutes: int) -> list[Opportunity]:
        cutoff = datetime.now(timezone.utc) - timedelta(minutes=minutes)
        statement = select(Opportunity).where(
            Opportunity.status == OpportunityStatus.PENDING_HUMAN,
            Opportunity.created_at <= cutoff,
        )
        result = await self.session.exec(statement)
        return list(result.all())


class RuleRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def enabled_detection_rules(self) -> list[DetectionRule]:
        statement = select(Rule).where(Rule.enabled.is_(True)).order_by(col(Rule.priority).asc())
        result = await self.session.exec(statement)
        return [
            DetectionRule(
                id=rule.id,
                name=rule.name,
                rule_type=rule.rule_type,
                pattern=rule.pattern,
                score=rule.score,
                priority=rule.priority,
            )
            for rule in result.all()
        ]

    async def list(self) -> list[Rule]:
        result = await self.session.exec(select(Rule).order_by(col(Rule.priority).asc()))
        return list(result.all())

    async def create(self, rule: Rule) -> Rule:
        self.session.add(rule)
        await self.session.commit()
        await self.session.refresh(rule)
        return rule

    async def get(self, rule_id: UUID) -> Rule | None:
        return await self.session.get(Rule, rule_id)

    async def save(self, rule: Rule) -> Rule:
        rule.updated_at = utc_now()
        self.session.add(rule)
        await self.session.commit()
        await self.session.refresh(rule)
        return rule

    async def delete(self, rule: Rule) -> None:
        await self.session.delete(rule)
        await self.session.commit()


class ConfigRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_value(self, key: str) -> dict | None:
        config = await self.session.get(AppConfig, key)
        return config.value if config else None

    async def set_value(self, key: str, value: dict, description: str | None = None) -> AppConfig:
        config = await self.session.get(AppConfig, key)
        if config:
            config.value = value
            config.description = description or config.description
            config.updated_at = utc_now()
        else:
            config = AppConfig(key=key, value=value, description=description)
        self.session.add(config)
        await self.session.commit()
        await self.session.refresh(config)
        return config


class TelegramUserConfigRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_by_user(self, user_id: UUID) -> TelegramUserConfig | None:
        statement = select(TelegramUserConfig).where(TelegramUserConfig.user_id == user_id)
        result = await self.session.exec(statement)
        return result.first()

    async def list_monitors_by_user(self, user_id: UUID) -> list[TelegramMonitor]:
        statement = (
            select(TelegramMonitor)
            .where(TelegramMonitor.user_id == user_id)
            .order_by(col(TelegramMonitor.created_at).asc())
        )
        result = await self.session.exec(statement)
        return list(result.all())

    async def list_enabled_monitors(self) -> list[tuple[TelegramUserConfig, TelegramMonitor]]:
        statement = (
            select(TelegramUserConfig, TelegramMonitor)
            .join(TelegramMonitor, TelegramMonitor.telegram_config_id == TelegramUserConfig.id)
            .where(TelegramMonitor.enabled.is_(True))
            .order_by(col(TelegramMonitor.updated_at).asc())
        )
        result = await self.session.exec(statement)
        return [(row[0], row[1]) for row in result.all()]

    async def save_account_for_user(
        self,
        *,
        user_id: UUID,
        api_id: int | None,
        api_hash_encrypted: str | None = None,
        session_encrypted: str | None = None,
    ) -> TelegramUserConfig:
        config = await self.get_by_user(user_id)
        if not config:
            config = TelegramUserConfig(user_id=user_id)

        config.api_id = api_id
        if api_hash_encrypted is not None:
            config.api_hash_encrypted = api_hash_encrypted
        if session_encrypted is not None:
            config.session_encrypted = session_encrypted
        config.updated_at = utc_now()

        self.session.add(config)
        await self.session.commit()
        await self.session.refresh(config)
        return config

    async def replace_monitors_for_user(
        self,
        *,
        user_id: UUID,
        telegram_config_id: UUID,
        chats: list[str | int],
        enabled: bool,
        backfill_limit: int,
    ) -> list[TelegramMonitor]:
        existing = {monitor.chat_id: monitor for monitor in await self.list_monitors_by_user(user_id)}
        desired_chat_ids = list(dict.fromkeys(str(chat) for chat in chats))
        config = await self.session.get(TelegramUserConfig, telegram_config_id)
        if config:
            config.enabled = enabled
            config.updated_at = utc_now()
            self.session.add(config)

        for chat_id, monitor in list(existing.items()):
            if chat_id not in desired_chat_ids:
                await self.session.delete(monitor)

        monitors: list[TelegramMonitor] = []
        for chat in desired_chat_ids:
            monitor = existing.get(chat)
            if not monitor:
                monitor = TelegramMonitor(
                    user_id=user_id,
                    telegram_config_id=telegram_config_id,
                    chat_id=chat,
                )
            monitor.enabled = enabled
            monitor.backfill_limit = backfill_limit
            monitor.updated_at = utc_now()
            self.session.add(monitor)
            monitors.append(monitor)

        await self.session.commit()
        for monitor in monitors:
            await self.session.refresh(monitor)
        return monitors

    async def record_monitor_error(self, monitor_id: UUID, error: str | None) -> None:
        monitor = await self.session.get(TelegramMonitor, monitor_id)
        if not monitor:
            return
        monitor.last_error = error[:1000] if error else None
        monitor.updated_at = utc_now()
        self.session.add(monitor)
        await self.session.commit()


class ReplyTemplateRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def list(self, enabled_only: bool = True) -> list[ReplyTemplate]:
        statement = select(ReplyTemplate).order_by(col(ReplyTemplate.created_at).desc())
        if enabled_only:
            statement = statement.where(ReplyTemplate.enabled.is_(True))
        result = await self.session.exec(statement)
        return list(result.all())

    async def create(self, template: ReplyTemplate) -> ReplyTemplate:
        self.session.add(template)
        await self.session.commit()
        await self.session.refresh(template)
        return template

    async def get(self, template_id: UUID) -> ReplyTemplate | None:
        return await self.session.get(ReplyTemplate, template_id)

    async def save(self, template: ReplyTemplate) -> ReplyTemplate:
        template.updated_at = utc_now()
        self.session.add(template)
        await self.session.commit()
        await self.session.refresh(template)
        return template
