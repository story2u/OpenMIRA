from __future__ import annotations

import asyncio
import hashlib
import hmac
import json
import math
import re
import time
from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from typing import Any

import httpx
from sqlalchemy.exc import IntegrityError

from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.core.config import Settings
from app.core.security import hash_interactive_agent_turn_nonce
from app.domain.enums import InteractiveAgentProviderRequestStatus
from app.domain.services.interactive_agent import (
    ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES,
)
from app.domain.services.interactive_agent_gateway import (
    InteractiveAgentGatewayConflictError,
    InteractiveAgentGatewayContract,
    InteractiveAgentGatewayProviderError,
    InteractiveAgentGatewayRateLimitError,
    InteractiveAgentGatewayRejectedError,
    InteractiveAgentGatewayUnavailableError,
    build_interactive_provider_payload,
    interactive_agent_gateway_contract,
    validate_interactive_gateway_contract,
)
from app.infrastructure.ai.analysis_gateway import (
    OpenAICompatibleGatewayClient,
    OpenAICompatibleProviderStream,
)
from app.infrastructure.db.interactive_agent_gateway_repository import (
    InteractiveAgentGatewayRepository,
)
from app.infrastructure.db.models import (
    InteractiveAgentProviderRequest,
    InteractiveAgentTurn,
    utc_now,
)

_provider_identifier = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")


def _nonnegative_int(value: Any) -> int | None:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        return None
    return min(value, 2_147_483_647)


def _safe_usage(value: Any) -> tuple[dict[str, int] | None, tuple[int, int, int] | None]:
    if not isinstance(value, dict):
        return None, None
    prompt = _nonnegative_int(value.get("prompt_tokens"))
    completion = _nonnegative_int(value.get("completion_tokens"))
    total = _nonnegative_int(value.get("total_tokens"))
    if prompt is None or completion is None or total is None:
        return None, None
    return (
        {
            "prompt_tokens": prompt,
            "completion_tokens": completion,
            "total_tokens": total,
        },
        (prompt, completion, total),
    )


async def _iter_sse_data(
    response: httpx.Response,
    *,
    max_bytes: int,
) -> AsyncIterator[str]:
    buffer = bytearray()
    received_bytes = 0
    async for chunk in response.aiter_bytes():
        received_bytes += len(chunk)
        if received_bytes > max_bytes:
            raise InteractiveAgentGatewayProviderError()
        buffer.extend(chunk)
        while True:
            lf_end = buffer.find(b"\n\n")
            crlf_end = buffer.find(b"\r\n\r\n")
            positions = [position for position in (lf_end, crlf_end) if position >= 0]
            if not positions:
                break
            event_end = min(positions)
            separator_length = 4 if crlf_end == event_end else 2
            event = bytes(buffer[:event_end])
            del buffer[: event_end + separator_length]
            data_lines = [
                line[5:].lstrip() for line in event.splitlines() if line.startswith(b"data:")
            ]
            if data_lines:
                try:
                    yield b"\n".join(data_lines).decode("utf-8")
                except UnicodeDecodeError as exc:
                    raise InteractiveAgentGatewayProviderError() from exc
    if buffer.strip():
        raise InteractiveAgentGatewayProviderError()


@dataclass(slots=True)
class _StreamToolState:
    names: dict[int, str] = field(default_factory=dict)
    ids: dict[int, str] = field(default_factory=dict)


def _safe_tool_calls(
    value: Any,
    state: _StreamToolState,
    allowed_tool_names: frozenset[str],
) -> list[dict[str, Any]]:
    if not isinstance(value, list) or not 1 <= len(value) <= 4:
        raise InteractiveAgentGatewayProviderError()
    safe: list[dict[str, Any]] = []
    for raw in value:
        if not isinstance(raw, dict):
            raise InteractiveAgentGatewayProviderError()
        index = raw.get("index")
        if not isinstance(index, int) or isinstance(index, bool) or not 0 <= index < 4:
            raise InteractiveAgentGatewayProviderError()
        item: dict[str, Any] = {"index": index}
        call_id = raw.get("id")
        if call_id is not None:
            if not isinstance(call_id, str) or not _provider_identifier.fullmatch(call_id):
                raise InteractiveAgentGatewayProviderError()
            existing_id = state.ids.get(index)
            if existing_id is not None and existing_id != call_id:
                raise InteractiveAgentGatewayProviderError()
            state.ids[index] = call_id
            item["id"] = call_id
        call_type = raw.get("type")
        if call_type is not None:
            if call_type != "function":
                raise InteractiveAgentGatewayProviderError()
            item["type"] = "function"
        function = raw.get("function")
        if function is not None:
            if not isinstance(function, dict):
                raise InteractiveAgentGatewayProviderError()
            safe_function: dict[str, str] = {}
            name = function.get("name")
            if name is not None:
                if name not in allowed_tool_names:
                    raise InteractiveAgentGatewayProviderError()
                existing_name = state.names.get(index)
                if existing_name is not None and existing_name != name:
                    raise InteractiveAgentGatewayProviderError()
                state.names[index] = name
                safe_function["name"] = name
            arguments = function.get("arguments")
            if arguments is not None:
                if not isinstance(arguments, str):
                    raise InteractiveAgentGatewayProviderError()
                safe_function["arguments"] = arguments
            if not safe_function:
                raise InteractiveAgentGatewayProviderError()
            item["function"] = safe_function
        if set(item) == {"index"}:
            raise InteractiveAgentGatewayProviderError()
        safe.append(item)
    return safe


def _safe_choices(
    value: Any,
    state: _StreamToolState,
    allowed_tool_names: frozenset[str],
) -> list[dict[str, Any]]:
    if not isinstance(value, list) or len(value) > 1:
        raise InteractiveAgentGatewayProviderError()
    if not value:
        return []
    raw = value[0]
    if not isinstance(raw, dict) or raw.get("index") != 0:
        raise InteractiveAgentGatewayProviderError()
    finish_reason = raw.get("finish_reason")
    if finish_reason not in {None, "stop", "tool_calls", "length"}:
        raise InteractiveAgentGatewayProviderError()
    safe: dict[str, Any] = {"index": 0, "finish_reason": finish_reason}
    delta = raw.get("delta")
    if delta is not None:
        if not isinstance(delta, dict):
            raise InteractiveAgentGatewayProviderError()
        safe_delta: dict[str, Any] = {}
        if "role" in delta:
            if delta["role"] != "assistant":
                raise InteractiveAgentGatewayProviderError()
            safe_delta["role"] = "assistant"
        if "content" in delta:
            if delta["content"] is not None and not isinstance(delta["content"], str):
                raise InteractiveAgentGatewayProviderError()
            safe_delta["content"] = delta["content"]
        if "tool_calls" in delta:
            safe_delta["tool_calls"] = _safe_tool_calls(
                delta["tool_calls"],
                state,
                allowed_tool_names,
            )
        if not safe_delta and delta:
            raise InteractiveAgentGatewayProviderError()
        safe["delta"] = safe_delta
    return [safe]


@dataclass(slots=True)
class InteractiveAgentGatewayStream:
    service: InteractiveAgentGatewayService
    audit: InteractiveAgentProviderRequest
    provider_stream: OpenAICompatibleProviderStream
    started_monotonic: float
    contract: InteractiveAgentGatewayContract

    async def events(self) -> AsyncIterator[bytes]:
        usage: tuple[int, int, int] | None = None
        finalized = False
        emitted_bytes = 0
        saw_done = False
        tool_state = _StreamToolState()
        gateway_completion_id = f"chatcmpl_{self.audit.id.hex}"
        try:
            async for data in _iter_sse_data(
                self.provider_stream.response,
                max_bytes=self.service.settings.interactive_agent_gateway_max_output_bytes,
            ):
                if data == "[DONE]":
                    saw_done = True
                    continue
                try:
                    chunk = json.loads(data)
                except (json.JSONDecodeError, UnicodeDecodeError) as exc:
                    raise InteractiveAgentGatewayProviderError() from exc
                if not isinstance(chunk, dict) or "error" in chunk:
                    raise InteractiveAgentGatewayProviderError()
                safe_choices = _safe_choices(
                    chunk.get("choices"),
                    tool_state,
                    self.contract.tool_names,
                )
                safe_chunk: dict[str, Any] = {
                    "id": gateway_completion_id,
                    "object": "chat.completion.chunk",
                    "created": _nonnegative_int(chunk.get("created")) or int(time.time()),
                    "model": self.audit.model_alias,
                    "choices": safe_choices,
                }
                safe_chunk_usage, parsed_usage = _safe_usage(chunk.get("usage"))
                if safe_chunk_usage is not None and parsed_usage is not None:
                    safe_chunk["usage"] = safe_chunk_usage
                    usage = parsed_usage
                encoded = json.dumps(
                    safe_chunk,
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode("utf-8")
                emitted_bytes += len(encoded)
                if emitted_bytes > self.service.settings.interactive_agent_gateway_max_output_bytes:
                    raise InteractiveAgentGatewayProviderError()
                yield b"data: " + encoded + b"\n\n"
            if not saw_done:
                raise InteractiveAgentGatewayProviderError()
            await self.service.finalize(
                self.audit,
                status=InteractiveAgentProviderRequestStatus.COMPLETED,
                failure_code=None,
                usage=usage,
                started_monotonic=self.started_monotonic,
            )
            finalized = True
            yield b"data: [DONE]\n\n"
        except asyncio.CancelledError:
            await asyncio.shield(
                self.service.finalize(
                    self.audit,
                    status=InteractiveAgentProviderRequestStatus.CANCELLED,
                    failure_code="client_cancelled",
                    usage=usage,
                    started_monotonic=self.started_monotonic,
                )
            )
            finalized = True
            raise
        except GeneratorExit:
            await asyncio.shield(
                self.service.finalize(
                    self.audit,
                    status=InteractiveAgentProviderRequestStatus.CANCELLED,
                    failure_code="client_cancelled",
                    usage=usage,
                    started_monotonic=self.started_monotonic,
                )
            )
            finalized = True
            raise
        except Exception:
            await asyncio.shield(
                self.service.finalize(
                    self.audit,
                    status=InteractiveAgentProviderRequestStatus.FAILED,
                    failure_code="provider_stream_failed",
                    usage=usage,
                    started_monotonic=self.started_monotonic,
                )
            )
            finalized = True
            yield (
                b'data: {"error":{"message":"gateway stream failed",'
                b'"type":"gateway_error","code":"provider_stream_failed"}}\n\n'
            )
            yield b"data: [DONE]\n\n"
        finally:
            await self.provider_stream.close()
            if not finalized:
                await asyncio.shield(
                    self.service.finalize(
                        self.audit,
                        status=InteractiveAgentProviderRequestStatus.CANCELLED,
                        failure_code="client_cancelled",
                        usage=usage,
                        started_monotonic=self.started_monotonic,
                    )
                )


class InteractiveAgentGatewayService:
    def __init__(
        self,
        *,
        repository: InteractiveAgentGatewayRepository,
        provider_client: OpenAICompatibleGatewayClient,
        settings: Settings,
        routing_service: InteractiveAgentRoutingService,
    ) -> None:
        self.repository = repository
        self.provider_client = provider_client
        self.settings = settings
        self.routing_service = routing_service

    async def open_stream(
        self,
        principal: InteractiveAgentTurnTokenPrincipal,
        payload: dict[str, Any],
    ) -> InteractiveAgentGatewayStream:
        if (
            not self.settings.interactive_agent_gateway_enabled
            or not self.settings.interactive_agent_beta_enabled
            or not self.settings.device_agent_gateway_api_key
        ):
            raise InteractiveAgentGatewayUnavailableError()
        contract = interactive_agent_gateway_contract(
            schema_version=self.settings.interactive_agent_schema_version,
            policy_version=self.settings.interactive_agent_policy_version,
        )
        validate_interactive_gateway_contract(
            payload,
            expected_model_alias=self.settings.interactive_agent_model_alias,
            max_prompt_chars=self.settings.interactive_agent_gateway_max_prompt_chars,
            max_completion_tokens=(self.settings.interactive_agent_gateway_max_completion_tokens),
            schema_version=contract.schema_version,
            policy_version=contract.policy_version,
        )
        turn = await self.repository.lock_turn_owned(
            turn_id=principal.turn_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not turn:
            raise InteractiveAgentGatewayRejectedError()
        try:
            self._authorize_turn(turn, principal)
            device = await self.repository.active_device_owned(
                owner_user_id=principal.owner_user_id,
                device_id=principal.device_id,
            )
            if not device or not self.routing_service.capability_available(device):
                raise InteractiveAgentGatewayRejectedError()
            if turn.request_count >= self.settings.interactive_agent_gateway_max_requests_per_turn:
                raise InteractiveAgentGatewayRateLimitError()
            turn.request_count += 1
            turn.updated_at = utc_now()
            audit = InteractiveAgentProviderRequest(
                owner_user_id=turn.owner_user_id,
                turn_id=turn.id,
                device_id=turn.device_id,
                request_sequence=turn.request_count,
                status=InteractiveAgentProviderRequestStatus.STARTED,
                provider=self.settings.device_agent_gateway_provider,
                provider_model=self.settings.device_agent_gateway_model,
                model_alias=turn.model_alias,
            )
            await self.repository.add(turn, audit)
        except IntegrityError as exc:
            await self.repository.rollback()
            raise InteractiveAgentGatewayConflictError() from exc
        except BaseException:
            await self.repository.rollback()
            raise

        provider_payload = build_interactive_provider_payload(
            payload,
            provider_model=self.settings.device_agent_gateway_model,
            max_completion_tokens=(self.settings.interactive_agent_gateway_max_completion_tokens),
            schema_version=contract.schema_version,
            policy_version=contract.policy_version,
        )
        started_monotonic = time.monotonic()
        try:
            provider_stream = await self.provider_client.open_stream(
                provider_payload,
                gateway_request_id=audit.id.hex,
            )
        except (httpx.HTTPError, OSError) as exc:
            await self.finalize(
                audit,
                status=InteractiveAgentProviderRequestStatus.FAILED,
                failure_code="provider_connect_failed",
                usage=None,
                started_monotonic=started_monotonic,
            )
            raise InteractiveAgentGatewayProviderError() from exc
        content_type = provider_stream.response.headers.get("content-type", "")
        if provider_stream.response.status_code != 200 or not content_type.startswith(
            "text/event-stream"
        ):
            await provider_stream.close()
            await self.finalize(
                audit,
                status=InteractiveAgentProviderRequestStatus.FAILED,
                failure_code="provider_rejected",
                usage=None,
                started_monotonic=started_monotonic,
            )
            raise InteractiveAgentGatewayProviderError()
        provider_request_id = provider_stream.response.headers.get("x-request-id")
        if provider_request_id:
            audit.provider_request_id_hash = hashlib.sha256(
                provider_request_id.encode("utf-8")
            ).hexdigest()
            await self.repository.commit(audit)
        return InteractiveAgentGatewayStream(
            service=self,
            audit=audit,
            provider_stream=provider_stream,
            started_monotonic=started_monotonic,
            contract=contract,
        )

    def _authorize_turn(
        self,
        turn: InteractiveAgentTurn,
        principal: InteractiveAgentTurnTokenPrincipal,
    ) -> None:
        if (
            turn.status not in ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES
            or turn.lease_expires_at <= utc_now()
            or turn.model_alias != self.settings.interactive_agent_model_alias
            or turn.runtime_version != self.settings.interactive_agent_runtime_version
            or turn.schema_version != self.settings.interactive_agent_schema_version
            or turn.policy_version != self.settings.interactive_agent_policy_version
            or not hmac.compare_digest(
                turn.token_nonce_hash,
                hash_interactive_agent_turn_nonce(principal.nonce),
            )
        ):
            raise InteractiveAgentGatewayRejectedError()

    async def finalize(
        self,
        audit: InteractiveAgentProviderRequest,
        *,
        status: InteractiveAgentProviderRequestStatus,
        failure_code: str | None,
        usage: tuple[int, int, int] | None,
        started_monotonic: float,
    ) -> None:
        if audit.status != InteractiveAgentProviderRequestStatus.STARTED:
            return
        finished_at = utc_now()
        audit.status = status
        audit.finished_at = finished_at
        audit.updated_at = finished_at
        audit.failure_code = failure_code
        audit.latency_ms = max(
            0,
            math.ceil((time.monotonic() - started_monotonic) * 1000),
        )
        if usage:
            prompt, completion, total = usage
            audit.prompt_tokens = prompt
            audit.completion_tokens = completion
            audit.total_tokens = total
            cost_numerator = (
                prompt * self.settings.device_agent_gateway_input_cost_micros_per_million
                + completion * self.settings.device_agent_gateway_output_cost_micros_per_million
            )
            audit.estimated_cost_micros = math.ceil(cost_numerator / 1_000_000)
        try:
            await self.repository.commit(audit)
        except BaseException:
            await self.repository.rollback()
            # Permit the stream error path to record a terminal failure instead
            # of leaving an in-memory COMPLETED audit that cannot be retried.
            audit.status = InteractiveAgentProviderRequestStatus.STARTED
            raise
