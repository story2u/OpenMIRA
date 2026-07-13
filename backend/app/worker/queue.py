from uuid import UUID

import structlog

from app.core.config import get_settings
from app.worker.celery_app import celery_app

logger = structlog.get_logger(__name__)


class CeleryTaskQueue:
    def enqueue_ai_reply(self, opportunity_id: UUID) -> None:
        celery_app.send_task("ai.generate_and_send_reply", args=[str(opportunity_id)])

    def notify_reviewers(self, opportunity_id: UUID) -> None:
        logger.info("opportunity.pending_review", opportunity_id=str(opportunity_id))

    def enqueue_wecom_event(self, event_id: UUID) -> bool:
        try:
            celery_app.send_task("wecom.process_webhook_event", args=[str(event_id)])
        except Exception:
            logger.exception("wecom.event_enqueue_failed", event_id=str(event_id))
            return False
        return True

    def enqueue_agent_analysis(
        self,
        message_id: UUID,
        *,
        force: bool = False,
        usage_ledger_id: UUID | None = None,
    ) -> bool:
        settings = get_settings()
        if not settings.pi_agent_enabled:
            return False
        if not settings.effective_pi_agent_api_key:
            logger.warning(
                "agent.analysis_not_configured",
                message_id=str(message_id),
                provider=settings.pi_agent_provider,
            )
            return False
        try:
            celery_app.send_task(
                "agent.analyze_message",
                args=[str(message_id), force, str(usage_ledger_id) if usage_ledger_id else None],
            )
        except Exception:
            logger.exception("agent.analysis_enqueue_failed", message_id=str(message_id))
            return False
        return True
