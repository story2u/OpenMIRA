import hashlib
import hmac
import time


def verify_revenuecat_signature(
    *,
    raw_body: bytes,
    signature_header: str,
    secret: str,
    tolerance_seconds: int,
    now_timestamp: int | None = None,
) -> bool:
    if not signature_header or not secret:
        return False
    parts: dict[str, str] = {}
    for part in signature_header.split(","):
        key, separator, value = part.strip().partition("=")
        if separator and key and value:
            parts[key] = value
    timestamp = parts.get("t")
    supplied = parts.get("v1")
    if not timestamp or not supplied:
        return False
    try:
        timestamp_value = int(timestamp)
    except ValueError:
        return False
    current = int(time.time()) if now_timestamp is None else now_timestamp
    if abs(current - timestamp_value) > tolerance_seconds:
        return False
    signed = timestamp.encode("ascii") + b"." + raw_body
    expected = hmac.new(secret.encode(), signed, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, supplied)
