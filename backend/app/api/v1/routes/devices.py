from uuid import UUID

import structlog
from fastapi import APIRouter, Depends, HTTPException, Response, status
from fastapi.security import HTTPAuthorizationCredentials
from sqlalchemy.exc import SQLAlchemyError

from app.api.deps import (
    DevicePrincipal,
    bearer,
    get_device_agent_routing_service,
    get_device_session_service,
    get_interactive_agent_routing_service,
    get_push_registration_service,
    require_device_principal,
    require_user,
)
from app.application.dto import (
    ClientCapabilitiesRead,
    DeviceRead,
    DeviceRegistrationRequest,
    DeviceSessionRead,
    PushRegistrationRead,
    PushRegistrationRequest,
)
from app.application.mappers import to_auth_user_read, to_device_read
from app.application.use_cases.analysis_run import DeviceAgentRoutingService
from app.application.use_cases.device_session import DeviceSessionIssue, DeviceSessionService
from app.application.use_cases.interactive_agent_turn import InteractiveAgentRoutingService
from app.application.use_cases.push_registration import PushRegistrationService
from app.core.config import Settings, get_settings
from app.domain.enums import PushEnvironment, PushProvider
from app.domain.services.device_session import (
    DeviceCredentialRejectedError,
    DeviceCredentialReuseDetectedError,
    DeviceLimitReachedError,
    DeviceNotFoundError,
    DeviceRevokedError,
)
from app.domain.services.push_delivery import (
    PushRegistrationConflictError,
    PushRegistrationUnavailableError,
)
from app.infrastructure.db.models import User

router = APIRouter()
logger = structlog.get_logger(__name__)


def prevent_credential_caching(response: Response) -> None:
    response.headers["Cache-Control"] = "no-store"
    response.headers["Pragma"] = "no-cache"


def to_device_session_read(issue: DeviceSessionIssue) -> DeviceSessionRead:
    return DeviceSessionRead(
        accessToken=issue.access_token,
        deviceRefreshToken=issue.refresh_token,
        deviceRefreshTokenExpiresAt=issue.refresh_token_expires_at,
        device=to_device_read(issue.device),
        user=to_auth_user_read(issue.user),
    )


@router.post("/register", response_model=DeviceSessionRead, status_code=status.HTTP_201_CREATED)
async def register_device(
    payload: DeviceRegistrationRequest,
    response: Response,
    user: User = Depends(require_user),
    service: DeviceSessionService = Depends(get_device_session_service),
) -> DeviceSessionRead:
    prevent_credential_caching(response)
    try:
        issue = await service.register(user=user, request=payload)
    except DeviceRevokedError as exc:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="device installation is revoked",
        ) from exc
    except DeviceLimitReachedError as exc:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="active device limit reached",
        ) from exc
    except DeviceCredentialRejectedError as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="device registration is not authorized",
        ) from exc
    except SQLAlchemyError as exc:
        logger.warning(
            "device.registration_failed",
            user_id=str(user.id),
            error_class=exc.__class__.__name__,
        )
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="device service is unavailable",
        ) from exc
    return to_device_session_read(issue)


@router.get("", response_model=list[DeviceRead])
async def list_devices(
    user: User = Depends(require_user),
    service: DeviceSessionService = Depends(get_device_session_service),
) -> list[DeviceRead]:
    devices = await service.list_devices(user.id)
    return [to_device_read(device) for device in devices]


@router.get("/current/capabilities", response_model=ClientCapabilitiesRead)
async def get_current_device_capabilities(
    principal: DevicePrincipal = Depends(require_device_principal),
    settings: Settings = Depends(get_settings),
    agent_routing: DeviceAgentRoutingService = Depends(get_device_agent_routing_service),
    interactive_routing: InteractiveAgentRoutingService = Depends(
        get_interactive_agent_routing_service
    ),
) -> ClientCapabilitiesRead:
    device = principal.device
    assert device is not None
    reported = device.capabilities
    react_native = reported.get("client.reactNative") is True
    sqlite_schema = reported.get("sqlite.schema")
    supports_sync_schema = (
        isinstance(sqlite_schema, int)
        and not isinstance(sqlite_schema, bool)
        and sqlite_schema >= 2
    )
    supports_push_schema = (
        isinstance(sqlite_schema, int)
        and not isinstance(sqlite_schema, bool)
        and sqlite_schema >= 3
    )
    push_environment = reported.get("push.environment")
    supports_push_environment = isinstance(push_environment, str) and push_environment in {
        PushEnvironment.SANDBOX.value,
        PushEnvironment.PRODUCTION.value,
    }
    push_provider_available = False
    if device.platform.value == "ios" and supports_push_environment:
        assert isinstance(push_environment, str)
        push_provider_available = settings.apns_available_for(push_environment)
    elif device.platform.value == "android":
        push_provider_available = settings.fcm_push_available
    sync_available = settings.rn_sync_rollout_enabled and react_native and supports_sync_schema
    return ClientCapabilitiesRead(
        agentToolsAvailable=interactive_routing.capability_available(device),
        rnClientSupported=react_native,
        deviceAgentAvailable=await agent_routing.capability_available(device),
        syncAvailable=sync_available,
        pushAvailable=(
            sync_available
            and settings.rn_push_rollout_enabled
            and supports_push_schema
            and supports_push_environment
            and push_provider_available
        ),
    )


def to_push_registration_read(registration) -> PushRegistrationRead:
    return PushRegistrationRead(
        id=registration.id,
        provider=registration.provider,
        environment=registration.environment,
        status=registration.status,
        tokenFingerprint=registration.token_hash[-12:],
        lastRegisteredAt=registration.last_registered_at,
        lastSuccessAt=registration.last_success_at,
        lastNotifiedCursor=registration.last_notified_cursor,
    )


@router.put("/current/push-registration", response_model=PushRegistrationRead)
async def register_current_push_token(
    payload: PushRegistrationRequest,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: PushRegistrationService = Depends(get_push_registration_service),
) -> PushRegistrationRead:
    device = principal.device
    assert device is not None
    try:
        registration = await service.register(
            user=principal.user,
            device=device,
            request=payload,
        )
    except PushRegistrationConflictError as exc:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="push token does not match the authenticated device",
        ) from exc
    except PushRegistrationUnavailableError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="push delivery is unavailable",
        ) from exc
    except SQLAlchemyError as exc:
        logger.warning(
            "push.registration_failed",
            device_id=str(device.id),
            error_class=exc.__class__.__name__,
        )
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="push registration is unavailable",
        ) from exc
    return to_push_registration_read(registration)


@router.delete(
    "/current/push-registration/{provider}/{environment}",
    status_code=status.HTTP_204_NO_CONTENT,
)
async def revoke_current_push_token(
    provider: PushProvider,
    environment: PushEnvironment,
    principal: DevicePrincipal = Depends(require_device_principal),
    service: PushRegistrationService = Depends(get_push_registration_service),
) -> Response:
    device = principal.device
    assert device is not None
    try:
        await service.revoke(
            user=principal.user,
            device=device,
            provider=provider,
            environment=environment,
        )
    except PushRegistrationConflictError as exc:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="push provider does not match the authenticated device",
        ) from exc
    return Response(status_code=status.HTTP_204_NO_CONTENT)


@router.post("/credentials/rotate", response_model=DeviceSessionRead)
async def rotate_device_credential(
    response: Response,
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    service: DeviceSessionService = Depends(get_device_session_service),
) -> DeviceSessionRead:
    prevent_credential_caching(response)
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid device credential",
            headers={"WWW-Authenticate": "Bearer"},
        )
    try:
        issue = await service.rotate(credentials.credentials)
    except DeviceCredentialReuseDetectedError as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid device credential",
            headers={"WWW-Authenticate": "Bearer"},
        ) from exc
    except DeviceCredentialRejectedError as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid device credential",
            headers={"WWW-Authenticate": "Bearer"},
        ) from exc
    except SQLAlchemyError as exc:
        logger.warning(
            "device.credential_rotation_failed",
            error_class=exc.__class__.__name__,
        )
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="device service is unavailable",
        ) from exc
    return to_device_session_read(issue)


@router.post("/{device_id}/revoke", response_model=DeviceRead)
async def revoke_device(
    device_id: UUID,
    user: User = Depends(require_user),
    service: DeviceSessionService = Depends(get_device_session_service),
) -> DeviceRead:
    try:
        device = await service.revoke(owner_user_id=user.id, device_id=device_id)
    except DeviceNotFoundError as exc:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND, detail="device not found"
        ) from exc
    return to_device_read(device)
