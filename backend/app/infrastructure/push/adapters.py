import base64
import json
import re
from datetime import UTC, datetime, timedelta

import httpx
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, padding, rsa, utils

from app.core.config import Settings
from app.domain.enums import PushEnvironment
from app.domain.services.push_delivery import (
    PushDeliveryResult,
    PushDeliveryStatus,
)

FCM_SCOPE = "https://www.googleapis.com/auth/firebase.messaging"
FCM_TOKEN_URI = "https://oauth2.googleapis.com/token"
ERROR_CODE_PATTERN = re.compile(r"[^A-Za-z0-9_.-]+")


def _b64url(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _json_segment(value: dict) -> str:
    return _b64url(json.dumps(value, separators=(",", ":")).encode("utf-8"))


def _safe_error_code(value: object, fallback: str) -> str:
    candidate = ERROR_CODE_PATTERN.sub("_", str(value))[:64].strip("_")
    return candidate or fallback


def _cursor_data(cursor: int) -> dict[str, str]:
    return {"type": "sync_cursor", "schemaVersion": "1", "cursor": str(cursor)}


class APNsPushAdapter:
    def __init__(self, settings: Settings, client: httpx.AsyncClient) -> None:
        self.settings = settings
        self.client = client

    def _authorization_token(self) -> str:
        now = datetime.now(UTC)
        header = {"alg": "ES256", "kid": self.settings.apns_key_id}
        payload = {"iss": self.settings.apns_team_id, "iat": int(now.timestamp())}
        signing_input = f"{_json_segment(header)}.{_json_segment(payload)}"
        private_key = serialization.load_pem_private_key(
            self.settings.apns_private_key.replace("\\n", "\n").encode("utf-8"),
            password=None,
        )
        if not isinstance(private_key, ec.EllipticCurvePrivateKey):
            raise ValueError("APNs private key must be an EC private key")
        der = private_key.sign(signing_input.encode("ascii"), ec.ECDSA(hashes.SHA256()))
        r, s = utils.decode_dss_signature(der)
        signature = r.to_bytes(32, "big") + s.to_bytes(32, "big")
        return f"{signing_input}.{_b64url(signature)}"

    async def send(
        self,
        *,
        token: str,
        cursor: int,
        environment: PushEnvironment,
    ) -> PushDeliveryResult:
        host = (
            "https://api.sandbox.push.apple.com"
            if environment == PushEnvironment.SANDBOX
            else "https://api.push.apple.com"
        )
        topic = (
            self.settings.apns_sandbox_topic
            if environment == PushEnvironment.SANDBOX
            else self.settings.apns_topic
        )
        if not topic:
            return PushDeliveryResult(PushDeliveryStatus.RETRY, "apns_topic_missing")
        try:
            response = await self.client.post(
                f"{host}/3/device/{token}",
                headers={
                    "authorization": f"bearer {self._authorization_token()}",
                    "apns-topic": topic,
                    "apns-push-type": "background",
                    "apns-priority": "5",
                    "apns-expiration": "0",
                    "apns-collapse-id": "radar-sync-cursor",
                },
                json={
                    "aps": {"content-available": 1},
                    "radar": _cursor_data(cursor),
                },
            )
        except (httpx.HTTPError, ValueError, TypeError):
            return PushDeliveryResult(PushDeliveryStatus.RETRY, "apns_transport")
        if response.status_code == 200:
            return PushDeliveryResult(PushDeliveryStatus.SUCCESS)
        try:
            reason = response.json().get("reason", "apns_rejected")
        except (ValueError, AttributeError):
            reason = "apns_rejected"
        error_code = _safe_error_code(reason, "apns_rejected")
        if response.status_code in {400, 410} and reason in {
            "BadDeviceToken",
            "DeviceTokenNotForTopic",
            "Unregistered",
        }:
            return PushDeliveryResult(PushDeliveryStatus.INVALID_TOKEN, error_code)
        return PushDeliveryResult(PushDeliveryStatus.RETRY, error_code)


class FCMPushAdapter:
    def __init__(self, settings: Settings, client: httpx.AsyncClient) -> None:
        self.settings = settings
        self.client = client
        self._access_token: str | None = None
        self._access_token_expires_at = datetime.min.replace(tzinfo=UTC)

    def _service_account_assertion(self) -> str:
        now = datetime.now(UTC)
        signing_input = ".".join(
            [
                _json_segment({"alg": "RS256", "typ": "JWT"}),
                _json_segment(
                    {
                        "iss": self.settings.fcm_client_email,
                        "scope": FCM_SCOPE,
                        "aud": FCM_TOKEN_URI,
                        "iat": int(now.timestamp()),
                        "exp": int((now + timedelta(minutes=55)).timestamp()),
                    }
                ),
            ]
        )
        private_key = serialization.load_pem_private_key(
            self.settings.fcm_private_key.replace("\\n", "\n").encode("utf-8"),
            password=None,
        )
        if not isinstance(private_key, rsa.RSAPrivateKey):
            raise ValueError("FCM private key must be an RSA private key")
        signature = private_key.sign(
            signing_input.encode("ascii"),
            padding.PKCS1v15(),
            hashes.SHA256(),
        )
        return f"{signing_input}.{_b64url(signature)}"

    async def _get_access_token(self) -> str:
        now = datetime.now(UTC)
        if self._access_token and now < self._access_token_expires_at:
            return self._access_token
        response = await self.client.post(
            FCM_TOKEN_URI,
            data={
                "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
                "assertion": self._service_account_assertion(),
            },
        )
        response.raise_for_status()
        payload = response.json()
        token = payload.get("access_token")
        if not isinstance(token, str) or not token:
            raise ValueError("FCM access token is missing")
        expires_in = payload.get("expires_in", 3600)
        lifetime = int(expires_in) if isinstance(expires_in, (int, float, str)) else 3600
        self._access_token = token
        self._access_token_expires_at = now + timedelta(seconds=max(60, lifetime - 60))
        return token

    async def send(
        self,
        *,
        token: str,
        cursor: int,
        environment: PushEnvironment,
    ) -> PushDeliveryResult:
        del environment
        try:
            access_token = await self._get_access_token()
            response = await self.client.post(
                "https://fcm.googleapis.com/v1/"
                f"projects/{self.settings.fcm_project_id}/messages:send",
                headers={"authorization": f"Bearer {access_token}"},
                json={
                    "message": {
                        "token": token,
                        "data": _cursor_data(cursor),
                        "android": {"priority": "high", "ttl": "60s"},
                    }
                },
            )
        except (httpx.HTTPError, ValueError, TypeError):
            return PushDeliveryResult(PushDeliveryStatus.RETRY, "fcm_transport")
        if response.status_code == 200:
            return PushDeliveryResult(PushDeliveryStatus.SUCCESS)
        try:
            error = response.json().get("error", {})
            details = error.get("details", []) if isinstance(error, dict) else []
            provider_codes = [
                item.get("errorCode")
                for item in details
                if isinstance(item, dict) and isinstance(item.get("errorCode"), str)
            ]
            status = error.get("status") if isinstance(error, dict) else None
        except (ValueError, AttributeError):
            provider_codes = []
            status = None
        code = provider_codes[0] if provider_codes else status or "fcm_rejected"
        error_code = _safe_error_code(code, "fcm_rejected")
        if "UNREGISTERED" in provider_codes or status == "UNREGISTERED":
            return PushDeliveryResult(PushDeliveryStatus.INVALID_TOKEN, error_code)
        return PushDeliveryResult(PushDeliveryStatus.RETRY, error_code)
