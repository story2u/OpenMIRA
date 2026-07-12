from celery import Celery

from app.core.config import get_settings

settings = get_settings()

celery_app = Celery(
    "opportunity_im_assistant",
    broker=settings.celery_broker_url,
    backend=settings.celery_result_backend,
    include=["app.worker.tasks"],
)

celery_app.conf.update(
    task_serializer="json",
    accept_content=["json"],
    result_serializer="json",
    timezone=settings.default_timezone,
    enable_utc=True,
    task_track_started=True,
    worker_prefetch_multiplier=1,
    task_routes={
        "ai.generate_and_send_reply": {"queue": "ai"},
        "agent.analyze_message": {"queue": "agent"},
        "opportunity.sweep_pending_for_ai": {"queue": "default"},
        "billing.process_revenuecat_event": {"queue": "default"},
        "billing.sync_revenuecat_customer": {"queue": "default"},
        "billing.reconcile_revenuecat_subscriptions": {"queue": "default"},
    },
    beat_schedule={
        "sweep-pending-human-every-5-minutes": {
            "task": "opportunity.sweep_pending_for_ai",
            "schedule": 300.0,
        },
        "reconcile-revenuecat-subscriptions": {
            "task": "billing.reconcile_revenuecat_subscriptions",
            "schedule": float(settings.revenuecat_reconcile_interval_hours * 3600),
        },
    },
)
