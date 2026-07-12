import hashlib
import json
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Request, status

from app.api.deps import get_billing_event_repo
from app.core.config import Settings, get_settings
from app.core.security import constant_time_equals
from app.infrastructure.billing.webhook_security import verify_revenuecat_signature
from app.infrastructure.db.repositories import BillingEventRepository
from app.worker.celery_app import celery_app

router = APIRouter()


def extract_local_user_ids(event: dict) -> list[UUID]:
    candidates: list[object] = [
        event.get("app_user_id"),
        event.get("original_app_user_id"),
    ]
    for key in ("aliases", "transferred_from", "transferred_to", "redeemed_from", "redeemed_by"):
        value = event.get(key)
        candidates.extend(value if isinstance(value, list) else [])
    result: list[UUID] = []
    seen: set[UUID] = set()
    for candidate in candidates:
        try:
            user_id = UUID(str(candidate))
        except (ValueError, TypeError, AttributeError):
            continue
        if user_id not in seen:
            result.append(user_id)
            seen.add(user_id)
    return result


@router.post("")
async def receive_revenuecat_webhook(
    request: Request,
    settings: Settings = Depends(get_settings),
    event_repo: BillingEventRepository = Depends(get_billing_event_repo),
) -> dict[str, bool | str]:
    if not settings.revenuecat_webhook_available:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="RevenueCat webhook is not configured")
    raw_body = await request.body()
    authorization = request.headers.get("Authorization", "")
    if not constant_time_equals(authorization, settings.revenuecat_webhook_auth_token):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid webhook authorization")
    if not verify_revenuecat_signature(
        raw_body=raw_body,
        signature_header=request.headers.get("X-RevenueCat-Webhook-Signature", ""),
        secret=settings.revenuecat_webhook_hmac_secret,
        tolerance_seconds=settings.revenuecat_webhook_tolerance_seconds,
    ):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid webhook signature")
    try:
        payload = json.loads(raw_body)
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="invalid webhook JSON") from exc
    event = payload.get("event") if isinstance(payload, dict) else None
    if not isinstance(event, dict):
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="missing webhook event")
    provider_event_id = str(event.get("id") or "").strip()
    if not provider_event_id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="missing webhook event ID")
    event_type = str(event.get("type") or "UNKNOWN")
    environment = str(event.get("environment") or "").lower() or None
    try:
        reservation = await event_repo.reserve_revenuecat_event(
            provider_event_id=provider_event_id,
            event_type=event_type,
            app_user_ids=extract_local_user_ids(event),
            environment=environment,
            payload_hash=hashlib.sha256(raw_body).hexdigest(),
        )
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    if reservation.should_enqueue:
        try:
            celery_app.send_task("billing.process_revenuecat_event", args=[str(reservation.event.id)])
        except Exception as exc:
            await event_repo.fail(reservation.event.id, "billing event could not be queued")
            raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="billing event queue unavailable") from exc
    return {
        "status": "accepted",
        "duplicate": reservation.duplicate,
        "eventId": provider_event_id,
    }
