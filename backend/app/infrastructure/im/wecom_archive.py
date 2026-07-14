from __future__ import annotations

import base64
import ctypes
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from pydantic import BaseModel, ConfigDict, Field, field_validator


class WeComArchiveProviderError(RuntimeError):
    """A sanitized provider failure that never includes credentials or response bodies."""


class WeComArchiveCredentials(BaseModel):
    corp_id: str
    archive_secret: str
    private_key_pem: str
    public_key_version: int = Field(ge=1)


class WeComArchiveMessage(BaseModel):
    model_config = ConfigDict(extra="ignore")

    sequence: int = Field(ge=0)
    message_id: str = Field(min_length=1, max_length=255)
    message_type: str = Field(default="unknown", max_length=64)
    sender_id: str = Field(default="", max_length=128)
    recipient_ids: list[str] = Field(default_factory=list, max_length=500)
    room_id: str | None = Field(default=None, max_length=255)
    sent_at_ms: int | None = None
    text: str | None = Field(default=None, max_length=100_000)
    payload_hash: str = Field(min_length=64, max_length=64)

    @property
    def participants(self) -> set[str]:
        return {value for value in [self.sender_id, *self.recipient_ids] if value}

    @property
    def is_text(self) -> bool:
        return self.message_type == "text" and self.text is not None

    @property
    def is_external_group(self) -> bool:
        return bool(self.room_id and any(item.startswith(("wm", "wo")) for item in self.participants))

    @classmethod
    def from_decrypted_payload(cls, *, sequence: int, payload: dict[str, Any]) -> "WeComArchiveMessage":
        canonical = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        text_payload = payload.get("text")
        text = text_payload.get("content") if isinstance(text_payload, dict) else None
        recipients = payload.get("tolist")
        if not isinstance(recipients, list):
            recipients = []
        return cls(
            sequence=sequence,
            message_id=str(payload.get("msgid") or ""),
            message_type=str(payload.get("msgtype") or "unknown"),
            sender_id=str(payload.get("from") or ""),
            recipient_ids=[str(value) for value in recipients if value],
            room_id=str(payload["roomid"]) if payload.get("roomid") else None,
            sent_at_ms=int(payload["msgtime"]) if payload.get("msgtime") is not None else None,
            text=str(text) if text is not None else None,
            payload_hash=hashlib.sha256(canonical.encode("utf-8")).hexdigest(),
        )

    @field_validator("sender_id", "room_id")
    @classmethod
    def reject_control_characters(cls, value: str | None) -> str | None:
        if value is not None and any(ord(char) < 32 for char in value):
            raise ValueError("identifier contains control characters")
        return value


class WeComArchiveProvider(Protocol):
    def fetch_messages(
        self,
        credentials: WeComArchiveCredentials,
        *,
        sequence: int,
        limit: int,
        timeout_seconds: int,
    ) -> list[WeComArchiveMessage]: ...


@dataclass(frozen=True, slots=True)
class _EncryptedRecord:
    sequence: int
    public_key_version: int
    encrypted_random_key: str
    encrypted_message: str


class CtypesWeComFinanceProvider:
    """Thin lifecycle-safe wrapper around Tencent's official Linux Finance SDK."""

    def __init__(self, sdk_path: str) -> None:
        path = Path(sdk_path)
        if not path.is_file():
            raise WeComArchiveProviderError("WeCom Finance SDK is not installed")
        try:
            self._library = ctypes.CDLL(str(path))
            self._configure_signatures()
        except (OSError, AttributeError) as exc:
            raise WeComArchiveProviderError("WeCom Finance SDK cannot be loaded") from exc

    def fetch_messages(
        self,
        credentials: WeComArchiveCredentials,
        *,
        sequence: int,
        limit: int,
        timeout_seconds: int,
    ) -> list[WeComArchiveMessage]:
        sdk = self._library.NewSdk()
        if not sdk:
            raise WeComArchiveProviderError("WeCom Finance SDK initialization failed")
        try:
            result = self._library.Init(
                sdk,
                credentials.corp_id.encode("utf-8"),
                credentials.archive_secret.encode("utf-8"),
            )
            if result != 0:
                raise WeComArchiveProviderError(f"WeCom Finance SDK init failed ({result})")
            records = self._get_encrypted_records(
                sdk, sequence=sequence, limit=limit, timeout_seconds=timeout_seconds
            )
            return [self._decrypt_record(record, credentials) for record in records]
        finally:
            self._library.DestroySdk(sdk)

    def _get_encrypted_records(
        self, sdk: int, *, sequence: int, limit: int, timeout_seconds: int
    ) -> list[_EncryptedRecord]:
        output = self._library.NewSlice()
        if not output:
            raise WeComArchiveProviderError("WeCom Finance SDK output allocation failed")
        try:
            result = self._library.GetChatData(
                sdk,
                ctypes.c_ulonglong(sequence),
                ctypes.c_uint(limit),
                b"",
                b"",
                ctypes.c_int(timeout_seconds),
                output,
            )
            if result != 0:
                raise WeComArchiveProviderError(f"WeCom Finance SDK fetch failed ({result})")
            payload = self._slice_json(output, operation="fetch")
        finally:
            self._library.FreeSlice(output)
        if int(payload.get("errcode", 0)) != 0:
            raise WeComArchiveProviderError("WeCom Finance API returned an error")
        chat_data = payload.get("chatdata", [])
        if not isinstance(chat_data, list):
            raise WeComArchiveProviderError("WeCom Finance API returned invalid data")
        records: list[_EncryptedRecord] = []
        for item in chat_data:
            try:
                records.append(
                    _EncryptedRecord(
                        sequence=int(item["seq"]),
                        public_key_version=int(item["publickey_ver"]),
                        encrypted_random_key=str(item["encrypt_random_key"]),
                        encrypted_message=str(item["encrypt_chat_msg"]),
                    )
                )
            except (KeyError, TypeError, ValueError) as exc:
                raise WeComArchiveProviderError("WeCom Finance record is invalid") from exc
        return records

    def _decrypt_record(
        self, record: _EncryptedRecord, credentials: WeComArchiveCredentials
    ) -> WeComArchiveMessage:
        try:
            if record.public_key_version != credentials.public_key_version:
                raise WeComArchiveProviderError("WeCom archive public key version does not match")
            private_key = serialization.load_pem_private_key(
                credentials.private_key_pem.encode("utf-8"), password=None
            )
            if not isinstance(private_key, rsa.RSAPrivateKey):
                raise TypeError("not RSA")
            random_key = private_key.decrypt(
                base64.b64decode(record.encrypted_random_key, validate=True),
                padding.PKCS1v15(),
            )
        except (TypeError, ValueError) as exc:
            raise WeComArchiveProviderError("WeCom archive key decryption failed") from exc
        output = self._library.NewSlice()
        if not output:
            raise WeComArchiveProviderError("WeCom Finance SDK output allocation failed")
        try:
            result = self._library.DecryptData(
                random_key,
                record.encrypted_message.encode("utf-8"),
                output,
            )
            if result != 0:
                raise WeComArchiveProviderError(f"WeCom Finance SDK decrypt failed ({result})")
            payload = self._slice_json(output, operation="decrypt")
        finally:
            self._library.FreeSlice(output)
        return WeComArchiveMessage.from_decrypted_payload(
            sequence=record.sequence, payload=payload
        )

    def _slice_json(self, output: int, *, operation: str) -> dict[str, Any]:
        content = self._library.GetContentFromSlice(output)
        if not content:
            raise WeComArchiveProviderError(f"WeCom Finance SDK {operation} returned no data")
        try:
            payload = json.loads(content.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise WeComArchiveProviderError(
                f"WeCom Finance SDK {operation} returned invalid data"
            ) from exc
        if not isinstance(payload, dict):
            raise WeComArchiveProviderError(
                f"WeCom Finance SDK {operation} returned invalid data"
            )
        return payload

    def _configure_signatures(self) -> None:
        library = self._library
        library.NewSdk.restype = ctypes.c_void_p
        library.Init.argtypes = [ctypes.c_void_p, ctypes.c_char_p, ctypes.c_char_p]
        library.Init.restype = ctypes.c_int
        library.DestroySdk.argtypes = [ctypes.c_void_p]
        library.NewSlice.restype = ctypes.c_void_p
        library.FreeSlice.argtypes = [ctypes.c_void_p]
        library.GetContentFromSlice.argtypes = [ctypes.c_void_p]
        library.GetContentFromSlice.restype = ctypes.c_char_p
        library.GetChatData.argtypes = [
            ctypes.c_void_p,
            ctypes.c_ulonglong,
            ctypes.c_uint,
            ctypes.c_char_p,
            ctypes.c_char_p,
            ctypes.c_int,
            ctypes.c_void_p,
        ]
        library.GetChatData.restype = ctypes.c_int
        library.DecryptData.argtypes = [ctypes.c_char_p, ctypes.c_char_p, ctypes.c_void_p]
        library.DecryptData.restype = ctypes.c_int


def validate_wecom_archive_private_key(private_key_pem: str) -> None:
    try:
        key = serialization.load_pem_private_key(private_key_pem.encode("utf-8"), password=None)
    except (TypeError, ValueError) as exc:
        raise ValueError("invalid RSA private key") from exc
    if not isinstance(key, rsa.RSAPrivateKey) or key.key_size < 2048:
        raise ValueError("WeCom archive private key must be RSA with at least 2048 bits")
