"""One-time token helpers for Telegram connection handshakes."""

import hashlib
import secrets


def create_connection_token() -> str:
    return secrets.token_urlsafe(32)


def hash_connection_token(token: str) -> str:
    # Tokens have 256 bits of entropy. Persisting only a digest limits the impact of a DB read.
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def create_chat_request_ids() -> tuple[int, int]:
    """Return two distinct, positive signed 32-bit request IDs for one Bot API message."""
    group_request_id = secrets.randbelow(1_000_000_000) + 1
    return group_request_id, group_request_id + 1_000_000_000
