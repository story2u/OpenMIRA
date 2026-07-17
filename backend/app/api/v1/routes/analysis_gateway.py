from __future__ import annotations

from json import JSONDecodeError

from fastapi import APIRouter, Depends, HTTPException, Request, status
from fastapi.responses import StreamingResponse
from pydantic import ValidationError

from app.api.deps import get_analysis_gateway_service, require_analysis_run_principal
from app.application.dto import AnalysisGatewayRequest
from app.application.use_cases.analysis_gateway import AnalysisGatewayService
from app.application.use_cases.analysis_run import AnalysisRunTokenPrincipal
from app.core.config import Settings, get_settings
from app.domain.services.analysis_gateway import (
    AnalysisGatewayConflictError,
    AnalysisGatewayContractError,
    AnalysisGatewayProviderError,
    AnalysisGatewayRateLimitError,
    AnalysisGatewayRejectedError,
    AnalysisGatewayUnavailableError,
)

router = APIRouter()


def _inline_gateway_schema() -> dict:
    schema = AnalysisGatewayRequest.model_json_schema()
    definitions = schema.pop("$defs", {})

    def resolve(value):
        if isinstance(value, list):
            return [resolve(item) for item in value]
        if not isinstance(value, dict):
            return value
        reference = value.get("$ref")
        if isinstance(reference, str) and reference.startswith("#/$defs/"):
            name = reference.removeprefix("#/$defs/")
            return resolve(definitions[name])
        return {key: resolve(item) for key, item in value.items()}

    return resolve(schema)


ANALYSIS_GATEWAY_OPENAPI_SCHEMA = _inline_gateway_schema()


async def _bounded_payload(request: Request, settings: Settings) -> dict:
    content_length = request.headers.get("content-length")
    if content_length:
        try:
            if int(content_length) > settings.device_agent_gateway_max_request_bytes:
                raise HTTPException(
                    status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                    detail="gateway request too large",
                )
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="invalid gateway request",
            ) from exc
    body = bytearray()
    async for chunk in request.stream():
        body.extend(chunk)
        if len(body) > settings.device_agent_gateway_max_request_bytes:
            raise HTTPException(
                status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                detail="gateway request too large",
            )
    try:
        parsed = AnalysisGatewayRequest.model_validate_json(body)
    except (ValidationError, JSONDecodeError, UnicodeDecodeError) as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail="invalid gateway request",
        ) from exc
    return parsed.model_dump(exclude_none=True)


def _raise_gateway_error(exc: Exception) -> None:
    if isinstance(exc, AnalysisGatewayUnavailableError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="analysis gateway is unavailable",
        ) from exc
    if isinstance(exc, AnalysisGatewayContractError):
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail="invalid gateway request",
        ) from exc
    if isinstance(exc, AnalysisGatewayRejectedError):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    if isinstance(exc, AnalysisGatewayConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="analysis gateway request conflict",
        ) from exc
    if isinstance(exc, AnalysisGatewayRateLimitError):
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail="analysis gateway rate limit exceeded",
        ) from exc
    if isinstance(exc, AnalysisGatewayProviderError):
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="analysis provider is unavailable",
        ) from exc
    raise exc


@router.post(
    "/chat/completions",
    openapi_extra={
        "requestBody": {
            "required": True,
            "content": {
                "application/json": {"schema": ANALYSIS_GATEWAY_OPENAPI_SCHEMA}
            },
        }
    },
)
async def create_analysis_chat_completion(
    request: Request,
    principal: AnalysisRunTokenPrincipal = Depends(require_analysis_run_principal),
    service: AnalysisGatewayService = Depends(get_analysis_gateway_service),
    settings: Settings = Depends(get_settings),
) -> StreamingResponse:
    payload = await _bounded_payload(request, settings)
    try:
        stream = await service.open_stream(principal, payload)
    except Exception as exc:
        _raise_gateway_error(exc)
    return StreamingResponse(
        stream.events(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-store",
            "X-Accel-Buffering": "no",
            "X-Content-Type-Options": "nosniff",
        },
    )
