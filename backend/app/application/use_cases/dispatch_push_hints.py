import httpx
import structlog

from app.core.config import Settings
from app.core.security import decrypt_secret
from app.domain.enums import PushProvider
from app.domain.services.push_delivery import PushDeliveryStatus
from app.infrastructure.db.repositories import PushRegistrationRepository
from app.infrastructure.push.adapters import APNsPushAdapter, FCMPushAdapter

logger = structlog.get_logger(__name__)


async def dispatch_pending_push_hints(
    repository: PushRegistrationRepository,
    settings: Settings,
) -> int:
    if not settings.push_dispatch_enabled:
        return 0
    deliveries = await repository.claim_pending(
        limit=settings.push_dispatch_batch_size,
        lease_seconds=settings.push_dispatch_lease_seconds,
    )
    if not deliveries:
        return 0
    delivered = 0
    async with httpx.AsyncClient(
        http2=True,
        timeout=httpx.Timeout(15.0, connect=5.0),
    ) as client:
        apns_adapter = APNsPushAdapter(settings, client)
        fcm_adapter = FCMPushAdapter(settings, client)
        for delivery in deliveries:
            try:
                token = decrypt_secret(delivery.token_encrypted, settings)
                adapter = (
                    apns_adapter
                    if delivery.provider == PushProvider.APNS
                    else fcm_adapter
                )
                result = await adapter.send(
                    token=token,
                    cursor=delivery.cursor,
                    environment=delivery.environment,
                )
            except Exception as exc:
                logger.warning(
                    "push.delivery_failed",
                    registration_id=str(delivery.registration_id),
                    provider=delivery.provider.value,
                    error_class=exc.__class__.__name__,
                )
                await repository.mark_failure(delivery.registration_id, "adapter_failure")
                continue
            if result.status == PushDeliveryStatus.SUCCESS:
                await repository.mark_success(delivery.registration_id, delivery.cursor)
                delivered += 1
            elif result.status == PushDeliveryStatus.INVALID_TOKEN:
                await repository.mark_invalid(
                    delivery.registration_id,
                    result.error_code or "invalid_token",
                )
            else:
                await repository.mark_failure(
                    delivery.registration_id,
                    result.error_code or "provider_retry",
                )
    return delivered
