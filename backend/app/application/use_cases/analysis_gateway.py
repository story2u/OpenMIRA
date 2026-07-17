from __future__ import annotations

import asyncio
import hashlib
import hmac
import json
import math
import time
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

import httpx
from sqlalchemy.exc import IntegrityError

from app.application.use_cases.analysis_run import AnalysisRunTokenPrincipal
from app.core.config import Settings
from app.core.security import hash_analysis_run_nonce
from app.domain.enums import AnalysisProviderRequestStatus, AnalysisRunMode
from app.domain.services.analysis_gateway import (
    AnalysisGatewayConflictError,
    AnalysisGatewayProviderError,
    AnalysisGatewayRateLimitError,
    AnalysisGatewayRejectedError,
    AnalysisGatewayUnavailableError,
    build_provider_payload,
    validate_gateway_contract,
)
from app.domain.services.analysis_run import ACTIVE_ANALYSIS_RUN_STATUSES
from app.infrastructure.ai.analysis_gateway import (
    OpenAICompatibleGatewayClient,
    OpenAICompatibleProviderStream,
)
from app.infrastructure.db.analysis_gateway_repository import AnalysisGatewayRepository
from app.infrastructure.db.models import AnalysisProviderRequest, AnalysisRun, utc_now


def _nonnegative_int(value: Any) -> int | None:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        return None
    return min(value, 2_147_483_647)


def _safe_usage(value: Any) -> tuple[dict[str, Any] | None, tuple[int, int, int] | None]:
    if not isinstance(value, dict):
        return None, None
    prompt = _nonnegative_int(value.get("prompt_tokens"))
    completion = _nonnegative_int(value.get("completion_tokens"))
    total = _nonnegative_int(value.get("total_tokens"))
    if prompt is None or completion is None or total is None:
        return None, None
    safe: dict[str, Any] = {
        "prompt_tokens": prompt,
        "completion_tokens": completion,
        "total_tokens": total,
    }
    prompt_details = value.get("prompt_tokens_details")
    if isinstance(prompt_details, dict):
        cached = _nonnegative_int(prompt_details.get("cached_tokens"))
        if cached is not None:
            safe["prompt_tokens_details"] = {"cached_tokens": cached}
    completion_details = value.get("completion_tokens_details")
    if isinstance(completion_details, dict):
        reasoning = _nonnegative_int(completion_details.get("reasoning_tokens"))
        if reasoning is not None:
            safe["completion_tokens_details"] = {"reasoning_tokens": reasoning}
    return safe, (prompt, completion, total)


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
            raise AnalysisGatewayProviderError()
        buffer.extend(chunk)
        while True:
            lf_end = buffer.find(b"\n\n")
            crlf_end = buffer.find(b"\r\n\r\n")
            candidates = [position for position in (lf_end, crlf_end) if position >= 0]
            if not candidates:
                break
            event_end = min(candidates)
            separator_length = 4 if crlf_end == event_end else 2
            event = bytes(buffer[:event_end])
            del buffer[: event_end + separator_length]
            data_lines = [
                line[5:].lstrip()
                for line in event.splitlines()
                if line.startswith(b"data:")
            ]
            if data_lines:
                try:
                    yield b"\n".join(data_lines).decode("utf-8")
                except UnicodeDecodeError as exc:
                    raise AnalysisGatewayProviderError() from exc
    if buffer.strip():
        raise AnalysisGatewayProviderError()


@dataclass(slots=True)
class AnalysisGatewayStream:
    service: AnalysisGatewayService
    audit: AnalysisProviderRequest
    provider_stream: OpenAICompatibleProviderStream
    started_monotonic: float

    async def events(self) -> AsyncIterator[bytes]:
        usage: tuple[int, int, int] | None = None
        finalized = False
        emitted_bytes = 0
        gateway_completion_id = f"chatcmpl_{self.audit.id.hex}"
        try:
            async for data in _iter_sse_data(
                self.provider_stream.response,
                max_bytes=self.service.settings.device_agent_gateway_max_output_bytes,
            ):
                if data == "[DONE]":
                    yield b"data: [DONE]\n\n"
                    continue
                try:
                    chunk = json.loads(data)
                except (json.JSONDecodeError, UnicodeDecodeError) as exc:
                    raise AnalysisGatewayProviderError() from exc
                if not isinstance(chunk, dict) or "error" in chunk:
                    raise AnalysisGatewayProviderError()
                choices = chunk.get("choices")
                if not isinstance(choices, list):
                    raise AnalysisGatewayProviderError()
                safe_chunk: dict[str, Any] = {
                    "id": gateway_completion_id,
                    "object": "chat.completion.chunk",
                    "created": _nonnegative_int(chunk.get("created")) or int(time.time()),
                    "model": self.audit.model_alias,
                    "choices": choices,
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
                if emitted_bytes > self.service.settings.device_agent_gateway_max_output_bytes:
                    raise AnalysisGatewayProviderError()
                yield b"data: " + encoded + b"\n\n"
            await self.service.finalize(
                self.audit,
                status=AnalysisProviderRequestStatus.COMPLETED,
                failure_code=None,
                usage=usage,
                started_monotonic=self.started_monotonic,
            )
            finalized = True
        except asyncio.CancelledError:
            await asyncio.shield(
                self.service.finalize(
                    self.audit,
                    status=AnalysisProviderRequestStatus.CANCELLED,
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
                    status=AnalysisProviderRequestStatus.CANCELLED,
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
                    status=AnalysisProviderRequestStatus.FAILED,
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
                        status=AnalysisProviderRequestStatus.CANCELLED,
                        failure_code="client_cancelled",
                        usage=usage,
                        started_monotonic=self.started_monotonic,
                    )
                )


class AnalysisGatewayService:
    def __init__(
        self,
        *,
        repository: AnalysisGatewayRepository,
        provider_client: OpenAICompatibleGatewayClient,
        settings: Settings,
    ) -> None:
        self.repository = repository
        self.provider_client = provider_client
        self.settings = settings

    async def open_stream(
        self,
        principal: AnalysisRunTokenPrincipal,
        payload: dict[str, Any],
    ) -> AnalysisGatewayStream:
        if (
            not self.settings.device_agent_gateway_enabled
            or not self.settings.device_agent_gateway_api_key
        ):
            raise AnalysisGatewayUnavailableError()
        validate_gateway_contract(
            payload,
            expected_model_alias=self.settings.device_agent_model_alias,
            max_prompt_chars=self.settings.device_agent_gateway_max_prompt_chars,
            max_completion_tokens=self.settings.device_agent_gateway_max_completion_tokens,
        )
        run = await self.repository.lock_run_owned(
            run_id=principal.run_id,
            owner_user_id=principal.owner_user_id,
            device_id=principal.device_id,
        )
        if not run:
            raise AnalysisGatewayRejectedError()
        try:
            self._authorize_run(run, principal)
            device = await self.repository.active_device_owned(
                owner_user_id=principal.owner_user_id,
                device_id=principal.device_id,
            )
            if not device:
                raise AnalysisGatewayRejectedError()
            if run.mode == AnalysisRunMode.SHADOW:
                if not self.settings.device_agent_shadow_enabled:
                    raise AnalysisGatewayRejectedError()
            elif not self.settings.rn_device_agent_rollout_enabled:
                raise AnalysisGatewayRejectedError()
            if await self.repository.request_count(run.id) >= (
                self.settings.device_agent_gateway_max_requests_per_run
            ):
                raise AnalysisGatewayRateLimitError()
            audit = AnalysisProviderRequest(
                owner_user_id=run.owner_user_id,
                run_id=run.id,
                device_id=run.device_id,
                status=AnalysisProviderRequestStatus.STARTED,
                provider=self.settings.device_agent_gateway_provider,
                provider_model=self.settings.device_agent_gateway_model,
                model_alias=run.model_alias,
            )
            await self.repository.add(audit)
        except IntegrityError as exc:
            await self.repository.rollback()
            raise AnalysisGatewayConflictError() from exc
        except BaseException:
            await self.repository.rollback()
            raise

        provider_payload = build_provider_payload(
            payload,
            provider_model=self.settings.device_agent_gateway_model,
            max_completion_tokens=self.settings.device_agent_gateway_max_completion_tokens,
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
                status=AnalysisProviderRequestStatus.FAILED,
                failure_code="provider_connect_failed",
                usage=None,
                started_monotonic=started_monotonic,
            )
            raise AnalysisGatewayProviderError() from exc
        content_type = provider_stream.response.headers.get("content-type", "")
        if provider_stream.response.status_code != 200 or not content_type.startswith(
            "text/event-stream"
        ):
            await provider_stream.close()
            await self.finalize(
                audit,
                status=AnalysisProviderRequestStatus.FAILED,
                failure_code="provider_rejected",
                usage=None,
                started_monotonic=started_monotonic,
            )
            raise AnalysisGatewayProviderError()
        provider_request_id = provider_stream.response.headers.get("x-request-id")
        if provider_request_id:
            audit.provider_request_id_hash = hashlib.sha256(
                provider_request_id.encode("utf-8")
            ).hexdigest()
            await self.repository.commit(audit)
        return AnalysisGatewayStream(
            service=self,
            audit=audit,
            provider_stream=provider_stream,
            started_monotonic=started_monotonic,
        )

    def _authorize_run(
        self,
        run: AnalysisRun,
        principal: AnalysisRunTokenPrincipal,
    ) -> None:
        if (
            run.status not in ACTIVE_ANALYSIS_RUN_STATUSES
            or run.lease_expires_at <= utc_now()
            or run.model_alias != self.settings.device_agent_model_alias
            or not hmac.compare_digest(
                run.token_nonce_hash,
                hash_analysis_run_nonce(principal.nonce),
            )
        ):
            raise AnalysisGatewayRejectedError()

    async def finalize(
        self,
        audit: AnalysisProviderRequest,
        *,
        status: AnalysisProviderRequestStatus,
        failure_code: str | None,
        usage: tuple[int, int, int] | None,
        started_monotonic: float,
    ) -> None:
        if audit.status != AnalysisProviderRequestStatus.STARTED:
            return
        finished_at = utc_now()
        audit.status = status
        audit.finished_at = finished_at
        audit.updated_at = finished_at
        audit.failure_code = failure_code
        audit.latency_ms = max(0, math.ceil((time.monotonic() - started_monotonic) * 1000))
        if usage:
            prompt, completion, total = usage
            audit.prompt_tokens = prompt
            audit.completion_tokens = completion
            audit.total_tokens = total
            cost_numerator = (
                prompt * self.settings.device_agent_gateway_input_cost_micros_per_million
                + completion
                * self.settings.device_agent_gateway_output_cost_micros_per_million
            )
            audit.estimated_cost_micros = math.ceil(cost_numerator / 1_000_000)
        await self.repository.commit(audit)
