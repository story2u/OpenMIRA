from fastapi import APIRouter

from app.api.v1.routes import (
    auth,
    configs,
    health,
    messages,
    opportunities,
    rules,
    settings,
    stats,
    subscriptions,
    telegram_connections,
    telegram_user_configs,
    templates,
    webhooks_revenuecat,
    webhooks_telegram,
    webhooks_wecom,
    wecom_connections,
    wecom_archive_connections,
)

api_router = APIRouter()
api_router.include_router(health.router, tags=["health"])
api_router.include_router(auth.router, prefix="/auth", tags=["auth"])
api_router.include_router(webhooks_telegram.router, prefix="/webhooks/telegram", tags=["webhooks"])
api_router.include_router(webhooks_wecom.router, prefix="/webhooks/wecom", tags=["webhooks"])
api_router.include_router(
    webhooks_revenuecat.router, prefix="/webhooks/revenuecat", tags=["webhooks"]
)
api_router.include_router(opportunities.router, prefix="/opportunities", tags=["opportunities"])
api_router.include_router(messages.router, prefix="/messages", tags=["messages"])
api_router.include_router(rules.router, prefix="/rules", tags=["rules"])
api_router.include_router(configs.router, prefix="/configs", tags=["configs"])
api_router.include_router(templates.router, prefix="/templates", tags=["templates"])
api_router.include_router(stats.router, prefix="/stats", tags=["stats"])
api_router.include_router(subscriptions.router, prefix="/subscriptions", tags=["subscriptions"])
api_router.include_router(settings.router, prefix="/settings", tags=["settings"])
api_router.include_router(
    telegram_connections.router,
    prefix="/integrations/telegram",
    tags=["integrations"],
)
api_router.include_router(
    telegram_user_configs.router,
    prefix="/integrations/telegram-user",
    tags=["integrations"],
)
api_router.include_router(
    wecom_connections.router,
    prefix="/integrations/wecom",
    tags=["integrations"],
)
api_router.include_router(
    wecom_archive_connections.router,
    prefix="/integrations/wecom",
    tags=["integrations"],
)
