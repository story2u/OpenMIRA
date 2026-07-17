from __future__ import annotations

from json import JSONDecodeError

from fastapi import APIRouter, Depends, HTTPException, Request, status
from fastapi.responses import StreamingResponse
from pydantic import ValidationError

from app.api.deps import (
    get_interactive_agent_gateway_service,
    require_interactive_agent_turn_principal,
)
from app.application.dto import InteractiveGatewayRequest
from app.application.use_cases.interactive_agent_gateway import (
    InteractiveAgentGatewayService,
)
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentTurnTokenPrincipal,
)
from app.core.config import Settings, get_settings
from app.domain.services.interactive_agent_gateway import (
    InteractiveAgentGatewayConflictError,
    InteractiveAgentGatewayContractError,
    InteractiveAgentGatewayProviderError,
    InteractiveAgentGatewayRateLimitError,
    InteractiveAgentGatewayRejectedError,
    InteractiveAgentGatewayUnavailableError,
)

router = APIRouter()


def _inline_gateway_schema() -> dict:
    schema = InteractiveGatewayRequest.model_json_schema()
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
        # Discriminator mappings retain #/$defs references even after Pydantic's
        # concrete oneOf branches are inlined. The const role fields already make
        # the union unambiguous, so omit that generator-only metadata here.
        return {
            key: resolve(item)
            for key, item in value.items()
            if key != "discriminator"
        }

    return resolve(schema)


INTERACTIVE_GATEWAY_OPENAPI_SCHEMA = _inline_gateway_schema()


async def _bounded_payload(request: Request, settings: Settings) -> dict:
    maximum = settings.interactive_agent_gateway_max_request_bytes
    content_length = request.headers.get("content-length")
    if content_length:
        try:
            if int(content_length) > maximum:
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
        if len(body) > maximum:
            raise HTTPException(
                status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                detail="gateway request too large",
            )
    try:
        parsed = InteractiveGatewayRequest.model_validate_json(body)
    except (ValidationError, JSONDecodeError, UnicodeDecodeError) as exc:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail="invalid gateway request",
        ) from exc
    return parsed.model_dump(exclude_none=True)


def _raise_gateway_error(exc: Exception) -> None:
    if isinstance(exc, InteractiveAgentGatewayUnavailableError):
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="interactive Agent gateway is unavailable",
        ) from exc
    if isinstance(exc, InteractiveAgentGatewayContractError):
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail="invalid gateway request",
        ) from exc
    if isinstance(exc, InteractiveAgentGatewayRejectedError):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    if isinstance(exc, InteractiveAgentGatewayConflictError):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="interactive Agent gateway request conflict",
        ) from exc
    if isinstance(exc, InteractiveAgentGatewayRateLimitError):
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail="interactive Agent gateway rate limit exceeded",
        ) from exc
    if isinstance(exc, InteractiveAgentGatewayProviderError):
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="interactive Agent provider is unavailable",
        ) from exc
    raise exc


@router.post(
    "/chat/completions",
    openapi_extra={
        "requestBody": {
            "required": True,
            "content": {"application/json": {"schema": INTERACTIVE_GATEWAY_OPENAPI_SCHEMA}},
        }
    },
)
async def create_interactive_chat_completion(
    request: Request,
    principal: InteractiveAgentTurnTokenPrincipal = Depends(
        require_interactive_agent_turn_principal
    ),
    service: InteractiveAgentGatewayService = Depends(get_interactive_agent_gateway_service),
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
