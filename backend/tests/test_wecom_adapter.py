import base64
import hashlib
import os
from time import time
from uuid import uuid4

import pytest
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

from app.core.config import Settings
from app.infrastructure.im.wecom import (
    WeComAdapter,
    WeComCredentials,
    WeComCryptoError,
    parse_xml_envelope,
)
from app.infrastructure.im.base import IMSendDisabledError

TOKEN = "callback-token"
CORP_ID = "ww-test-corp"
AES_KEY = base64.b64encode(bytes(range(32))).decode().rstrip("=")


def _settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://u:p@localhost/db",
        admin_api_token="test-admin-token",
        telegram_webhook_secret="telegram-secret",
        wecom_webhook_tolerance_seconds=300,
    )


def _credentials() -> WeComCredentials:
    return WeComCredentials(
        corp_id=CORP_ID,
        agent_id="1000002",
        secret="application-secret",
        token=TOKEN,
        aes_key=AES_KEY,
    )


def _encrypt(plain_xml: str, receive_id: str = CORP_ID) -> str:
    key = base64.b64decode(f"{AES_KEY}=")
    content = (
        os.urandom(16)
        + len(plain_xml.encode()).to_bytes(4, byteorder="big")
        + plain_xml.encode()
        + receive_id.encode()
    )
    padding = 32 - len(content) % 32
    padded = content + bytes([padding]) * padding
    cipher = Cipher(algorithms.AES(key), modes.CBC(key[:16]))
    encryptor = cipher.encryptor()
    return base64.b64encode(encryptor.update(padded) + encryptor.finalize()).decode()


def _signature(timestamp: str, nonce: str, encrypted: str) -> str:
    values = sorted([TOKEN, timestamp, nonce, encrypted])
    return hashlib.sha1("".join(values).encode()).hexdigest()


@pytest.mark.asyncio
async def test_encrypted_text_callback_is_normalized_for_human_review() -> None:
    connection_id = uuid4()
    owner_id = uuid4()
    plain_xml = """<xml>
      <FromUserName>zhangsan</FromUserName><CreateTime>1784030400</CreateTime>
      <MsgType>text</MsgType><Content>需要采购 50 套设备，请提供报价</Content>
      <MsgId>123456789</MsgId><AgentID>1000002</AgentID>
    </xml>"""
    encrypted = _encrypt(plain_xml)
    timestamp = str(int(time()))
    nonce = "fixed-nonce"
    adapter = WeComAdapter(_settings())
    connection = type(
        "Connection",
        (),
        {"id": connection_id, "owner_user_id": owner_id},
    )()

    inbound = await adapter.parse_webhook(
        {"xml": {"Encrypt": encrypted}},
        {},
        {
            "timestamp": timestamp,
            "nonce": nonce,
            "msg_signature": _signature(timestamp, nonce, encrypted),
        },
        credentials=_credentials(),
        connection=connection,
    )

    assert inbound is not None
    assert inbound.owner_user_id == owner_id
    assert inbound.external_message_id == f"wecom:{connection_id}:123456789"
    assert inbound.conversation_id == f"wecom:{connection_id}:zhangsan"
    assert inbound.force_human_review is True
    assert "Encrypt" not in inbound.raw_payload


@pytest.mark.asyncio
async def test_callback_rejects_wrong_receive_id() -> None:
    encrypted = _encrypt("<xml><MsgType>text</MsgType></xml>", receive_id="other-corp")
    timestamp = str(int(time()))
    nonce = "nonce"
    adapter = WeComAdapter(_settings())

    with pytest.raises(WeComCryptoError, match="receive id"):
        await adapter.parse_webhook(
            {"xml": {"Encrypt": encrypted}},
            {},
            {
                "timestamp": timestamp,
                "nonce": nonce,
                "msg_signature": _signature(timestamp, nonce, encrypted),
            },
            credentials=_credentials(),
        )


def test_xml_parser_rejects_entity_declarations() -> None:
    with pytest.raises(WeComCryptoError, match="unsafe"):
        parse_xml_envelope(b'<!DOCTYPE xml [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><xml/>')


@pytest.mark.asyncio
async def test_wecom_send_is_explicitly_disabled_instead_of_dry_run_success() -> None:
    adapter = WeComAdapter(_settings())

    with pytest.raises(IMSendDisabledError):
        await adapter.send_message("zhangsan", "真实回复")
