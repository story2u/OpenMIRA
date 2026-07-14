import base64
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa

from app.application.use_cases.sync_wecom_archive import SyncWeComArchive
from app.domain.enums import PlanCode
from app.domain.services.subscription_policy import get_plan_entitlements
from app.infrastructure.im.wecom_archive import (
    CtypesWeComFinanceProvider,
    WeComArchiveCredentials,
    WeComArchiveMessage,
    WeComArchiveProviderError,
    validate_wecom_archive_private_key,
)


class FakeProvider:
    def __init__(self, messages=None, error: Exception | None = None) -> None:
        self.messages = messages or []
        self.error = error

    def fetch_messages(self, credentials, *, sequence, limit, timeout_seconds):
        if self.error:
            raise self.error
        return list(self.messages)


def archive_message(
    *, sender: str, recipients: list[str], room_id: str | None = None
) -> WeComArchiveMessage:
    return WeComArchiveMessage.from_decrypted_payload(
        sequence=7,
        payload={
            "msgid": "provider-message-7",
            "msgtype": "text",
            "from": sender,
            "tolist": recipients,
            "roomid": room_id,
            "msgtime": 1_700_000_000_000,
            "text": {"content": "我们准备采购 50 套设备，请安排演示。"},
        },
    )


def subject(message: WeComArchiveMessage, *, bindings: list | None = None):
    connection = SimpleNamespace(id=uuid4(), enabled=True)
    cursor = SimpleNamespace(last_seq=0)
    owner_id = uuid4()
    binding = SimpleNamespace(
        id=uuid4(),
        user_id=owner_id,
        wecom_user_id="member-a",
        display_name="成员 A",
    )
    repository = SimpleNamespace(
        acquire_poll_lease=AsyncMock(return_value=cursor),
        reserve_event=AsyncMock(
            return_value=(SimpleNamespace(id=uuid4()), True)
        ),
        active_bindings_for_participants=AsyncMock(
            return_value=bindings if bindings is not None else [binding]
        ),
        complete_event=AsyncMock(),
        fail_event=AsyncMock(),
        mark_binding_matched=AsyncMock(),
        get_archive_source=AsyncMock(return_value=None),
        active_group_counts=AsyncMock(return_value=(0, 0)),
        ensure_archive_source=AsyncMock(
            return_value=SimpleNamespace(enabled=True, quota_paused=False)
        ),
        finish_poll=AsyncMock(),
        release_poll_failure=AsyncMock(),
    )
    message_repository = SimpleNamespace(
        get_by_external_id=AsyncMock(return_value=None),
        create_outgoing=AsyncMock(),
    )
    ingest = SimpleNamespace(execute=AsyncMock())
    subscription = SimpleNamespace(
        get_snapshot=AsyncMock(
            return_value=SimpleNamespace(entitlements=get_plan_entitlements(PlanCode.FREE))
        )
    )
    use_case = SyncWeComArchive(
        repository=repository,
        provider=FakeProvider([message]),
        ingest_message=ingest,
        message_repository=message_repository,
        subscription_repository=subscription,
        batch_size=100,
        timeout_seconds=5,
        lease_seconds=120,
    )
    credentials = WeComArchiveCredentials(
        corp_id="corp-id",
        archive_secret="archive-secret",
        private_key_pem="unused-by-fake",
        public_key_version=1,
    )
    return use_case, connection, credentials, repository, binding, ingest, message_repository


@pytest.mark.asyncio
async def test_incoming_archive_message_is_projected_only_to_matching_member() -> None:
    values = subject(archive_message(sender="customer-x", recipients=["member-a"]))
    use_case, connection, credentials, repository, binding, ingest, message_repository = values

    result = await use_case.execute(connection, credentials)

    assert result and result.processed == 1
    inbound = ingest.execute.await_args.args[0]
    assert inbound.owner_user_id == binding.user_id
    assert inbound.force_human_review is True
    assert str(binding.user_id) in inbound.external_message_id
    message_repository.create_outgoing.assert_not_awaited()
    repository.finish_poll.assert_awaited_once()
    assert repository.finish_poll.await_args.kwargs["last_sequence"] == 7


@pytest.mark.asyncio
async def test_message_sent_by_bound_member_is_context_not_opportunity_input() -> None:
    values = subject(archive_message(sender="member-a", recipients=["customer-x"]))
    use_case, connection, credentials, _, _, ingest, message_repository = values

    await use_case.execute(connection, credentials)

    ingest.execute.assert_not_awaited()
    message_repository.create_outgoing.assert_awaited_once()
    assert message_repository.create_outgoing.await_args.kwargs["opportunity_id"] is None


@pytest.mark.asyncio
async def test_message_without_member_binding_is_ignored() -> None:
    values = subject(
        archive_message(sender="member-b", recipients=["customer-x"]), bindings=[]
    )
    use_case, connection, credentials, repository, _, ingest, _ = values

    result = await use_case.execute(connection, credentials)

    assert result and result.ignored == 1
    ingest.execute.assert_not_awaited()
    assert repository.complete_event.await_args.kwargs["ignored"] is True


@pytest.mark.asyncio
async def test_group_over_quota_is_discovered_but_paused() -> None:
    values = subject(
        archive_message(
            sender="customer-x", recipients=["member-a"], room_id="group-1"
        )
    )
    use_case, connection, credentials, repository, _, ingest, _ = values
    repository.active_group_counts.return_value = (0, 1)
    repository.ensure_archive_source.side_effect = lambda **kwargs: SimpleNamespace(
        enabled=True, quota_paused=kwargs["quota_paused"]
    )

    await use_case.execute(connection, credentials)

    assert repository.ensure_archive_source.await_args.kwargs["quota_paused"] is True
    assert "current plan allows" in repository.ensure_archive_source.await_args.kwargs["quota_reason"]
    ingest.execute.assert_not_awaited()


@pytest.mark.asyncio
async def test_provider_failure_releases_lease_without_advancing_cursor() -> None:
    message = archive_message(sender="customer-x", recipients=["member-a"])
    values = subject(message)
    use_case, connection, credentials, repository, *_ = values
    use_case.provider = FakeProvider(error=WeComArchiveProviderError("sanitized"))

    with pytest.raises(WeComArchiveProviderError):
        await use_case.execute(connection, credentials)

    repository.finish_poll.assert_not_awaited()
    repository.release_poll_failure.assert_awaited_once()


def test_decrypted_message_parser_ignores_unknown_fields_and_detects_external_group() -> None:
    message = WeComArchiveMessage.from_decrypted_payload(
        sequence=9,
        payload={
            "msgid": "message-9",
            "msgtype": "text",
            "from": "member-a",
            "tolist": ["wm-external"],
            "roomid": "room-9",
            "text": {"content": "hello"},
            "future_field": {"ignored": True},
        },
    )

    assert message.is_text is True
    assert message.is_external_group is True
    assert message.participants == {"member-a", "wm-external"}


def test_private_key_validation_and_missing_sdk_fail_closed(tmp_path: Path) -> None:
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    ).decode()
    validate_wecom_archive_private_key(pem)

    with pytest.raises(ValueError, match="invalid RSA private key"):
        validate_wecom_archive_private_key("not-a-key")
    with pytest.raises(WeComArchiveProviderError, match="not installed"):
        CtypesWeComFinanceProvider(str(tmp_path / "missing.so"))


def test_finance_sdk_provider_decrypts_and_releases_native_resources() -> None:
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private_key_pem = private_key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    ).decode()
    random_key = b"archive-random-key"
    encrypted_random_key = base64.b64encode(
        private_key.public_key().encrypt(random_key, padding.PKCS1v15())
    ).decode()
    decrypted_payload = {
        "msgid": "provider-message-11",
        "msgtype": "text",
        "from": "customer-x",
        "tolist": ["member-a"],
        "msgtime": 1_700_000_000_000,
        "text": {"content": "需要采购 50 套设备。"},
    }

    class FakeFinanceLibrary:
        def __init__(self) -> None:
            self.slices: dict[int, bytes] = {}
            self.next_slice = 1
            self.destroyed = False

        def NewSdk(self):
            return 99

        def Init(self, sdk, corp_id, secret):
            assert (sdk, corp_id, secret) == (99, b"corp-id", b"archive-secret")
            return 0

        def DestroySdk(self, sdk):
            assert sdk == 99
            self.destroyed = True

        def NewSlice(self):
            value = self.next_slice
            self.next_slice += 1
            return value

        def FreeSlice(self, output):
            self.slices.pop(output, None)

        def GetContentFromSlice(self, output):
            return self.slices[output]

        def GetChatData(
            self, sdk, sequence, limit, proxy, password, timeout, output
        ):
            assert sdk == 99
            assert sequence.value == 0
            assert limit.value == 100
            assert proxy == password == b""
            assert timeout.value == 5
            self.slices[output] = json.dumps(
                {
                    "errcode": 0,
                    "chatdata": [
                        {
                            "seq": 11,
                            "publickey_ver": 1,
                            "encrypt_random_key": encrypted_random_key,
                            "encrypt_chat_msg": "encrypted-message",
                        }
                    ],
                }
            ).encode()
            return 0

        def DecryptData(self, key, encrypted_message, output):
            assert key == random_key
            assert encrypted_message == b"encrypted-message"
            self.slices[output] = json.dumps(
                decrypted_payload, ensure_ascii=False
            ).encode()
            return 0

    library = FakeFinanceLibrary()
    provider = object.__new__(CtypesWeComFinanceProvider)
    provider._library = library

    messages = provider.fetch_messages(
        WeComArchiveCredentials(
            corp_id="corp-id",
            archive_secret="archive-secret",
            private_key_pem=private_key_pem,
            public_key_version=1,
        ),
        sequence=0,
        limit=100,
        timeout_seconds=5,
    )

    assert library.destroyed is True
    assert len(messages) == 1
    assert messages[0].sequence == 11
    assert messages[0].text == "需要采购 50 套设备。"
