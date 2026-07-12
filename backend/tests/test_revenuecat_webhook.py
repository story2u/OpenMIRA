import hashlib
import hmac
import json
import os
from types import SimpleNamespace
from uuid import uuid4

import pytest
from fastapi import HTTPException
from starlette.requests import Request

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:password@localhost:5432/im")
os.environ.setdefault("ADMIN_API_TOKEN", "test-admin-token")

from app.api.v1.routes import webhooks_revenuecat
from app.core.config import Settings
from app.infrastructure.billing.webhook_security import verify_revenuecat_signature


def signed_header(raw_body: bytes, secret: str, timestamp: int) -> str:
    signature = hmac.new(secret.encode(), str(timestamp).encode() + b"." + raw_body, hashlib.sha256).hexdigest()
    return f"t={timestamp},v1={signature}"


def request_for(raw_body: bytes, *, authorization: str, signature: str) -> Request:
    consumed = False

    async def receive():
        nonlocal consumed
        if consumed:
            return {"type": "http.disconnect"}
        consumed = True
        return {"type": "http.request", "body": raw_body, "more_body": False}

    return Request(
        {
            "type": "http",
            "method": "POST",
            "path": "/api/v1/webhooks/revenuecat",
            "headers": [
                (b"authorization", authorization.encode()),
                (b"x-revenuecat-webhook-signature", signature.encode()),
            ],
        },
        receive,
    )


def webhook_settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost:5432/im",
        admin_api_token="test",
        revenuecat_enabled=True,
        revenuecat_secret_api_key="server-secret",
        revenuecat_webhook_auth_token="Bearer webhook-token",
        revenuecat_webhook_hmac_secret="hmac-secret",
    )


def test_hmac_uses_exact_raw_body_and_rejects_expired_timestamp() -> None:
    raw = b'{"event":{"id":"event-1", "type":"FUTURE_EVENT"}}'
    signature = signed_header(raw, "hmac-secret", 1_000)

    assert verify_revenuecat_signature(
        raw_body=raw,
        signature_header=signature,
        secret="hmac-secret",
        tolerance_seconds=300,
        now_timestamp=1_100,
    )
    assert not verify_revenuecat_signature(
        raw_body=json.dumps(json.loads(raw)).encode(),
        signature_header=signature,
        secret="hmac-secret",
        tolerance_seconds=300,
        now_timestamp=1_100,
    )
    assert not verify_revenuecat_signature(
        raw_body=raw,
        signature_header=signature,
        secret="hmac-secret",
        tolerance_seconds=300,
        now_timestamp=1_301,
    )


class FakeEventRepo:
    def __init__(self):
        self.reserved = None

    async def reserve_revenuecat_event(self, **kwargs):
        self.reserved = kwargs
        return SimpleNamespace(
            event=SimpleNamespace(id=uuid4()),
            should_enqueue=True,
            duplicate=False,
        )

    async def fail(self, *_):
        return None


@pytest.mark.asyncio
async def test_unknown_webhook_event_is_accepted_and_only_hash_is_persisted(monkeypatch) -> None:
    user_id = uuid4()
    raw = json.dumps(
        {
            "event": {
                "id": "event-unknown",
                "type": "A_FUTURE_EVENT",
                "app_user_id": str(user_id),
                "aliases": ["not-a-uuid"],
                "card": "must-not-be-persisted",
            }
        },
        separators=(",", ":"),
    ).encode()
    timestamp = 2_000_000_000
    settings = webhook_settings()
    repo = FakeEventRepo()
    sent: list[tuple] = []
    monkeypatch.setattr(
        "app.infrastructure.billing.webhook_security.time.time",
        lambda: timestamp,
    )
    monkeypatch.setattr(webhooks_revenuecat.celery_app, "send_task", lambda *args, **kwargs: sent.append((args, kwargs)))

    response = await webhooks_revenuecat.receive_revenuecat_webhook(
        request_for(
            raw,
            authorization="Bearer webhook-token",
            signature=signed_header(raw, "hmac-secret", timestamp),
        ),
        settings=settings,
        event_repo=repo,
    )

    assert response["status"] == "accepted"
    assert repo.reserved["event_type"] == "A_FUTURE_EVENT"
    assert repo.reserved["app_user_ids"] == [user_id]
    assert repo.reserved["payload_hash"] == hashlib.sha256(raw).hexdigest()
    assert "card" not in repo.reserved
    assert len(sent) == 1


@pytest.mark.asyncio
async def test_webhook_rejects_wrong_authorization_before_json_parse() -> None:
    raw = b"not-json"
    with pytest.raises(HTTPException) as raised:
        await webhooks_revenuecat.receive_revenuecat_webhook(
            request_for(raw, authorization="wrong", signature="wrong"),
            settings=webhook_settings(),
            event_repo=FakeEventRepo(),
        )

    assert raised.value.status_code == 401
