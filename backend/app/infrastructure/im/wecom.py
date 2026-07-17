import base64
import hashlib
import hmac
import json
import time
from dataclasses import dataclass
from typing import Any
from uuid import UUID

import httpx
import xmltodict
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from redis.asyncio import Redis

from app.core.config import Settings
from app.core.security import decrypt_secret
from app.domain.enums import (
    IMChannel,
    WeComConnectionStatus,
    WeComDeliveryStatus,
    WeComSendCapability,
)
from app.domain.ports import InboundMessage, SendReceipt
from app.infrastructure.db.models import WeComConnection
from app.infrastructure.db.repositories import WeComConnectionRepository, WeComDeliveryRepository
from app.infrastructure.im.base import IMSendDisabledError


class WeComCryptoError(ValueError):
    pass


class WeComProviderError(RuntimeError):
    pass


@dataclass(frozen=True, slots=True)
class WeComCredentials:
    corp_id: str
    agent_id: str
    secret: str
    token: str
    aes_key: str

    @classmethod
    def from_connection(cls, connection: WeComConnection, settings: Settings) -> "WeComCredentials":
        return cls(
            corp_id=connection.corp_id,
            agent_id=connection.agent_id,
            secret=decrypt_secret(connection.secret_encrypted, settings),
            token=decrypt_secret(connection.token_encrypted, settings),
            aes_key=decrypt_secret(connection.aes_key_encrypted, settings),
        )


def parse_xml_envelope(raw_body: bytes) -> dict[str, Any]:
    upper = raw_body.upper()
    if b"<!DOCTYPE" in upper or b"<!ENTITY" in upper:
        raise WeComCryptoError("unsafe wecom xml")
    try:
        payload = xmltodict.parse(raw_body).get("xml", {})
    except Exception as exc:
        raise WeComCryptoError("invalid wecom xml") from exc
    if not isinstance(payload, dict):
        raise WeComCryptoError("invalid wecom xml envelope")
    return dict(payload)


class WeComCrypto:
    def __init__(self, token: str, encoding_aes_key: str, receive_id: str) -> None:
        self.token = token
        self.receive_id = receive_id
        try:
            self.key = base64.b64decode(f"{encoding_aes_key}=", validate=True)
        except (ValueError, TypeError) as exc:
            raise WeComCryptoError("invalid wecom aes key") from exc
        if len(self.key) != 32:
            raise WeComCryptoError("invalid wecom aes key length")
        self.iv = self.key[:16]

    def verify_signature(self, signature: str, timestamp: str, nonce: str, encrypted: str) -> None:
        values = [self.token, timestamp, nonce, encrypted]
        digest = hashlib.sha1("".join(sorted(values)).encode("utf-8")).hexdigest()
        if not hmac.compare_digest(digest, signature):
            raise WeComCryptoError("invalid wecom signature")

    def decrypt(self, encrypted: str) -> str:
        try:
            encrypted_bytes = base64.b64decode(encrypted, validate=True)
            cipher = Cipher(algorithms.AES(self.key), modes.CBC(self.iv))
            decryptor = cipher.decryptor()
            padded = decryptor.update(encrypted_bytes) + decryptor.finalize()
        except Exception as exc:
            raise WeComCryptoError("invalid wecom encrypted payload") from exc
        content = self._pkcs7_unpad(padded)
        if len(content) < 20:
            raise WeComCryptoError("invalid wecom payload length")
        msg_len = int.from_bytes(content[16:20], byteorder="big")
        if msg_len < 0 or 20 + msg_len > len(content):
            raise WeComCryptoError("invalid wecom message length")
        message = content[20 : 20 + msg_len]
        try:
            receive_id = content[20 + msg_len :].decode("utf-8")
            decoded = message.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise WeComCryptoError("invalid wecom payload encoding") from exc
        if self.receive_id and receive_id not in {self.receive_id, self.receive_id.lower()}:
            raise WeComCryptoError("invalid wecom receive id")
        return decoded

    def _pkcs7_unpad(self, value: bytes) -> bytes:
        if not value:
            raise WeComCryptoError("invalid pkcs7 padding")
        pad = value[-1]
        if pad < 1 or pad > 32 or len(value) < pad or value[-pad:] != bytes([pad]) * pad:
            raise WeComCryptoError("invalid pkcs7 padding")
        return value[:-pad]


class WeComAdapter:
    channel = IMChannel.WECOM

    def __init__(
        self,
        settings: Settings,
        redis: Redis | None = None,
        *,
        connection_repo: WeComConnectionRepository | None = None,
        delivery_repo: WeComDeliveryRepository | None = None,
    ) -> None:
        self.settings = settings
        self.redis = redis
        self.connection_repo = connection_repo
        self.delivery_repo = delivery_repo

    async def verify_url(
        self,
        query: dict[str, str],
        *,
        credentials: WeComCredentials | None = None,
    ) -> str:
        encrypted = query.get("echostr", "")
        credentials = credentials or self._global_credentials()
        self._verify_timestamp(query.get("timestamp", ""))
        crypto = self._crypto(credentials)
        crypto.verify_signature(
            signature=query.get("msg_signature", ""),
            timestamp=query.get("timestamp", ""),
            nonce=query.get("nonce", ""),
            encrypted=encrypted,
        )
        return crypto.decrypt(encrypted)

    async def parse_webhook(
        self,
        payload: dict[str, Any],
        headers: dict[str, str],
        query: dict[str, str] | None = None,
        *,
        credentials: WeComCredentials | None = None,
        connection: WeComConnection | None = None,
    ) -> InboundMessage | None:
        del headers
        query = query or {}
        credentials = credentials or self._global_credentials()
        encrypted = self._extract_encrypt(payload)
        self._verify_timestamp(query.get("timestamp", ""))
        crypto = self._crypto(credentials)
        crypto.verify_signature(
            signature=query.get("msg_signature", ""),
            timestamp=query.get("timestamp", ""),
            nonce=query.get("nonce", ""),
            encrypted=encrypted,
        )
        decrypted_xml = crypto.decrypt(encrypted)
        body = parse_xml_envelope(decrypted_xml.encode("utf-8"))

        msg_type = str(body.get("MsgType") or "unknown")
        if msg_type != "text":
            return None
        content = str(body.get("Content") or "").strip()
        if not content:
            return None
        if len(content) > 20_000:
            raise WeComCryptoError("wecom message text is too long")

        sender_id = str(body.get("FromUserName") or "")
        if not sender_id or len(sender_id) > 255:
            raise WeComCryptoError("invalid wecom sender")
        provider_message_id = str(
            body.get("MsgId")
            or f"{sender_id}:{body.get('CreateTime')}:{hashlib.sha256(content.encode()).hexdigest()[:16]}"
        )
        prefix = f"wecom:{connection.id}:" if connection else ""
        return InboundMessage(
            owner_user_id=connection.owner_user_id if connection else None,
            channel=self.channel,
            external_message_id=f"{prefix}{provider_message_id}",
            conversation_id=f"{prefix}{sender_id}",
            sender_external_id=sender_id,
            sender_display_name=sender_id,
            text=content,
            source_type="private",
            raw_payload={
                "connectionId": str(connection.id) if connection else None,
                "msgType": msg_type,
                "agentId": str(body.get("AgentID") or ""),
                "createTime": str(body.get("CreateTime") or ""),
            },
            force_human_review=True,
        )

    async def verify_credentials(
        self,
        connection: WeComConnection,
    ) -> None:
        credentials = WeComCredentials.from_connection(connection, self.settings)
        await self._access_token(credentials, cache_scope=str(connection.id), force_refresh=True)

    async def send_message(
        self,
        conversation_id: str,
        text: str,
        *,
        idempotency_key: str | None = None,
        opportunity_id: UUID | None = None,
        owner_user_id: UUID | None = None,
    ) -> SendReceipt:
        dynamic_target = self._parse_dynamic_conversation(conversation_id)
        if not dynamic_target:
            return await self._send_with_credentials(
                credentials=self._global_credentials(),
                cache_scope="legacy",
                target_user_id=conversation_id,
                text=text,
            )
        if not self.connection_repo or not self.delivery_repo:
            raise WeComProviderError("wecom user connection adapter is unavailable")
        if not idempotency_key or not opportunity_id or not owner_user_id:
            raise WeComProviderError("wecom send requires an idempotency key and owner context")

        connection_id, target_user_id = dynamic_target
        connection = await self.connection_repo.get(connection_id)
        if (
            not connection
            or connection.owner_user_id != owner_user_id
            or not connection.enabled
            or connection.status != WeComConnectionStatus.ACTIVE
        ):
            raise WeComProviderError("wecom connection is not active")
        source = await self.connection_repo.get_source_for_conversation(
            connection.id, target_user_id
        )
        if (
            not source
            or source.owner_user_id != owner_user_id
            or not source.enabled
            or source.quota_paused
            or source.send_capability != WeComSendCapability.APP_MESSAGE
        ):
            raise WeComProviderError("wecom source does not allow application messages")

        content_hash = hashlib.sha256(text.encode("utf-8")).hexdigest()
        delivery, should_send = await self.delivery_repo.reserve(
            owner_user_id=owner_user_id,
            connection_id=connection.id,
            source_id=source.id,
            opportunity_id=opportunity_id,
            idempotency_key=idempotency_key,
            content_hash=content_hash,
        )
        if delivery.content_hash != content_hash or delivery.opportunity_id != opportunity_id:
            raise WeComProviderError("idempotency key was already used for different content")
        if delivery.status == WeComDeliveryStatus.SENT:
            return SendReceipt(
                provider_message_id=delivery.provider_message_id,
                raw_response={"duplicate": True, "delivery_id": str(delivery.id)},
            )
        if not should_send:
            raise WeComProviderError("wecom delivery is already in progress")

        await self.delivery_repo.mark_sending(delivery)
        try:
            receipt = await self._send_with_credentials(
                credentials=WeComCredentials.from_connection(connection, self.settings),
                cache_scope=str(connection.id),
                target_user_id=target_user_id,
                text=text,
            )
        except Exception as exc:
            await self.delivery_repo.mark_failed(delivery, exc.__class__.__name__)
            raise
        await self.delivery_repo.mark_sent(delivery, receipt.provider_message_id)
        receipt.raw_response["delivery_id"] = str(delivery.id)
        return receipt

    async def _send_with_credentials(
        self,
        *,
        credentials: WeComCredentials,
        cache_scope: str,
        target_user_id: str,
        text: str,
    ) -> SendReceipt:
        if not self.settings.im_send_enabled:
            raise IMSendDisabledError("IM sending is disabled")
        token = await self._access_token(credentials, cache_scope=cache_scope)
        try:
            async with httpx.AsyncClient(timeout=httpx.Timeout(10.0, connect=5.0)) as client:
                response = await client.post(
                    f"https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token={token}",
                    json={
                        "touser": target_user_id,
                        "msgtype": "text",
                        "agentid": int(credentials.agent_id),
                        "text": {"content": text},
                        "safe": 0,
                        "enable_duplicate_check": 1,
                        "duplicate_check_interval": 1800,
                    },
                )
                response.raise_for_status()
                data = response.json()
        except (httpx.HTTPError, ValueError) as exc:
            raise WeComProviderError("wecom message provider is unavailable") from exc
        if data.get("errcode") != 0:
            raise WeComProviderError(f"wecom send failed with code {data.get('errcode')}")
        provider_message_id = str(data.get("msgid")) if data.get("msgid") else None
        return SendReceipt(
            provider_message_id=provider_message_id,
            raw_response={"errcode": 0, "msgid": provider_message_id},
        )

    async def _access_token(
        self,
        credentials: WeComCredentials,
        *,
        cache_scope: str,
        force_refresh: bool = False,
    ) -> str:
        cache_key = f"wecom:access_token:{cache_scope}"
        if self.redis and not force_refresh:
            cached = await self.redis.get(cache_key)
            if cached:
                return cached.decode("utf-8") if isinstance(cached, bytes) else str(cached)
        try:
            async with httpx.AsyncClient(timeout=httpx.Timeout(10.0, connect=5.0)) as client:
                response = await client.get(
                    "https://qyapi.weixin.qq.com/cgi-bin/gettoken",
                    params={"corpid": credentials.corp_id, "corpsecret": credentials.secret},
                )
                response.raise_for_status()
                data = response.json()
        except (httpx.HTTPError, ValueError) as exc:
            raise WeComProviderError("wecom credential provider is unavailable") from exc
        if data.get("errcode") != 0 or not data.get("access_token"):
            raise WeComProviderError(f"wecom token failed with code {data.get('errcode')}")
        token = str(data["access_token"])
        if self.redis:
            await self.redis.set(
                cache_key, token, ex=max(int(data.get("expires_in", 7200)) - 300, 60)
            )
        return token

    def _verify_timestamp(self, timestamp: str) -> None:
        try:
            value = int(timestamp)
        except (TypeError, ValueError) as exc:
            raise WeComCryptoError("invalid wecom timestamp") from exc
        if abs(int(time.time()) - value) > self.settings.wecom_webhook_tolerance_seconds:
            raise WeComCryptoError("expired wecom timestamp")

    def _global_credentials(self) -> WeComCredentials:
        return WeComCredentials(
            corp_id=self.settings.wecom_corp_id,
            agent_id=self.settings.wecom_agent_id,
            secret=self.settings.wecom_secret,
            token=self.settings.wecom_token,
            aes_key=self.settings.wecom_aes_key,
        )

    def _crypto(self, credentials: WeComCredentials) -> WeComCrypto:
        return WeComCrypto(
            token=credentials.token,
            encoding_aes_key=credentials.aes_key,
            receive_id=credentials.corp_id,
        )

    def _extract_encrypt(self, payload: dict[str, Any]) -> str:
        if "xml" in payload:
            xml_node = payload["xml"]
            if isinstance(xml_node, dict) and xml_node.get("Encrypt"):
                return str(xml_node["Encrypt"])
        if payload.get("Encrypt"):
            return str(payload["Encrypt"])
        raise WeComCryptoError("missing Encrypt field")

    def _parse_dynamic_conversation(self, value: str) -> tuple[UUID, str] | None:
        if not value.startswith("wecom:"):
            return None
        try:
            _, raw_connection_id, target = value.split(":", 2)
            connection_id = UUID(raw_connection_id)
        except (ValueError, TypeError) as exc:
            raise WeComProviderError("invalid wecom conversation target") from exc
        if not target or len(target) > 255:
            raise WeComProviderError("invalid wecom conversation target")
        return connection_id, target


def serialize_inbound(inbound: InboundMessage) -> str:
    return json.dumps(inbound.model_dump(mode="json"), separators=(",", ":"), ensure_ascii=False)
