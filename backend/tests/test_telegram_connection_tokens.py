from app.core.telegram_connection_tokens import (
    create_chat_request_ids,
    create_connection_token,
    hash_connection_token,
)


def test_connection_token_is_random_and_only_its_digest_is_stable() -> None:
    first = create_connection_token()
    second = create_connection_token()

    assert first != second
    assert len(first) >= 40
    assert hash_connection_token(first) == hash_connection_token(first)
    assert hash_connection_token(first) != hash_connection_token(second)
    assert first not in hash_connection_token(first)


def test_chat_request_ids_are_distinct_positive_signed_32_bit_values() -> None:
    group_request_id, channel_request_id = create_chat_request_ids()

    assert group_request_id != channel_request_id
    assert 0 < group_request_id <= 2_147_483_647
    assert 0 < channel_request_id <= 2_147_483_647
