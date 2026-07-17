import json

import httpx
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec, rsa

from app.core.config import Settings
from app.domain.enums import PushEnvironment
from app.domain.services.push_delivery import PushDeliveryStatus
from app.infrastructure.push.adapters import APNsPushAdapter, FCMPushAdapter


def pem_private_key(key) -> str:
    return key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")


def push_settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        push_dispatch_enabled=True,
        apns_team_id="TEAM123",
        apns_key_id="KEY123",
        apns_private_key=pem_private_key(ec.generate_private_key(ec.SECP256R1())),
        apns_topic="com.codeiy.im",
        apns_sandbox_topic="com.codeiy.im.dev",
        fcm_project_id="radar-project",
        fcm_client_email="firebase@example.test",
        fcm_private_key=pem_private_key(rsa.generate_private_key(public_exponent=65537, key_size=2048)),
    )


async def test_apns_adapter_sends_background_cursor_hint_without_user_content() -> None:
    captured: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json={})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        result = await APNsPushAdapter(push_settings(), client).send(
            token="a" * 64,
            cursor=42,
            environment=PushEnvironment.SANDBOX,
        )

    payload = json.loads(captured[0].content)
    assert result.status == PushDeliveryStatus.SUCCESS
    assert captured[0].url.host == "api.sandbox.push.apple.com"
    assert captured[0].headers["apns-push-type"] == "background"
    assert payload == {
        "aps": {"content-available": 1},
        "radar": {"type": "sync_cursor", "schemaVersion": "1", "cursor": "42"},
    }
    assert "alert" not in captured[0].content.decode("utf-8")


async def test_fcm_adapter_uses_v1_data_only_payload_and_reuses_oauth_token() -> None:
    messages: list[dict] = []
    token_requests = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal token_requests
        if request.url.host == "oauth2.googleapis.com":
            token_requests += 1
            assert b"assertion=" in request.content
            return httpx.Response(200, json={"access_token": "oauth-token", "expires_in": 3600})
        assert request.headers["authorization"] == "Bearer oauth-token"
        messages.append(json.loads(request.content))
        return httpx.Response(200, json={"name": "projects/radar/messages/1"})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        adapter = FCMPushAdapter(push_settings(), client)
        first = await adapter.send(
            token="fcm-native-token-value",
            cursor=51,
            environment=PushEnvironment.PRODUCTION,
        )
        second = await adapter.send(
            token="fcm-native-token-value",
            cursor=52,
            environment=PushEnvironment.PRODUCTION,
        )

    assert first.status == PushDeliveryStatus.SUCCESS
    assert second.status == PushDeliveryStatus.SUCCESS
    assert token_requests == 1
    assert [message["message"]["data"]["cursor"] for message in messages] == ["51", "52"]
    assert all("notification" not in message["message"] for message in messages)


async def test_provider_invalid_token_responses_are_terminal_without_exposing_token() -> None:
    def apns_handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(410, json={"reason": "Unregistered"})

    async with httpx.AsyncClient(transport=httpx.MockTransport(apns_handler)) as client:
        result = await APNsPushAdapter(push_settings(), client).send(
            token="b" * 64,
            cursor=70,
            environment=PushEnvironment.PRODUCTION,
        )

    assert result.status == PushDeliveryStatus.INVALID_TOKEN
    assert result.error_code == "Unregistered"

    def fcm_handler(request: httpx.Request) -> httpx.Response:
        if request.url.host == "oauth2.googleapis.com":
            return httpx.Response(200, json={"access_token": "oauth-token", "expires_in": 3600})
        return httpx.Response(
            404,
            json={
                "error": {
                    "status": "NOT_FOUND",
                    "details": [{"errorCode": "UNREGISTERED"}],
                }
            },
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(fcm_handler)) as client:
        fcm_result = await FCMPushAdapter(push_settings(), client).send(
            token="fcm-invalid-token-value",
            cursor=71,
            environment=PushEnvironment.PRODUCTION,
        )

    assert fcm_result.status == PushDeliveryStatus.INVALID_TOKEN
    assert fcm_result.error_code == "UNREGISTERED"
