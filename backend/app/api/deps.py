from collections.abc import AsyncGenerator
from dataclasses import dataclass
from uuid import UUID

from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from redis.asyncio import Redis
from sqlmodel.ext.asyncio.session import AsyncSession

from app.application.use_cases.analysis_gateway import AnalysisGatewayService
from app.application.use_cases.analysis_run import (
    AnalysisRunService,
    AnalysisRunTokenPrincipal,
    DeviceAgentRoutingService,
)
from app.application.use_cases.device_session import DeviceSessionService
from app.application.use_cases.interactive_agent_action import (
    InteractiveAgentActionService,
    InteractiveAgentApprovalTokenPrincipal,
)
from app.application.use_cases.interactive_agent_gateway import (
    InteractiveAgentGatewayService,
)
from app.application.use_cases.interactive_agent_turn import (
    InteractiveAgentRoutingService,
    InteractiveAgentTurnService,
    InteractiveAgentTurnTokenPrincipal,
)
from app.application.use_cases.manual_reply import ManualReplyUseCase
from app.application.use_cases.push_registration import PushRegistrationService
from app.application.use_cases.signal_appetite_sync import SignalAppetiteSyncService
from app.application.use_cases.sync_feed import SyncFeedService
from app.core.config import Settings, get_settings
from app.core.security import (
    constant_time_equals,
    decode_access_token,
    decode_analysis_run_token,
    decode_interactive_agent_approval_token,
    decode_interactive_agent_turn_token,
    hash_analysis_run_nonce,
    hash_interactive_agent_approval_nonce,
    hash_interactive_agent_turn_nonce,
)
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.agent.link_inspector import SafeLinkInspector
from app.infrastructure.ai.analysis_gateway import OpenAICompatibleGatewayClient
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier, LiteLLMReplyGenerator
from app.infrastructure.db.analysis_gateway_repository import AnalysisGatewayRepository
from app.infrastructure.db.analysis_run_repository import AnalysisRunRepository
from app.infrastructure.db.interactive_agent_action_repository import (
    InteractiveAgentActionRepository,
)
from app.infrastructure.db.interactive_agent_gateway_repository import (
    InteractiveAgentGatewayRepository,
)
from app.infrastructure.db.interactive_agent_repository import (
    InteractiveAgentTurnRepository,
)
from app.infrastructure.db.models import Device, Opportunity, User
from app.infrastructure.db.repositories import (
    BillingEventRepository,
    ConfigRepository,
    DeviceRepository,
    ManualReplyDeliveryRepository,
    MessageRepository,
    JobMessageAuditRepository,
    JobOpportunityMatchRepository,
    JobOpportunityRepository,
    JobSearchProfileRepository,
    OpportunityRepository,
    PushRegistrationRepository,
    ReplyTemplateRepository,
    RuleRepository,
    SourceFunctionalProfileRepository,
    SubscriptionRepository,
    TelegramConnectionRepository,
    TelegramUserConfigRepository,
    UserRepository,
    UserSettingsRepository,
    WeComConnectionRepository,
    WeComArchiveRepository,
    WeComDeliveryRepository,
    WeComEventRepository,
)
from app.infrastructure.db.session import get_session
from app.infrastructure.db.signal_appetite_repository import SignalAppetiteRepository
from app.infrastructure.db.sync_repository import SyncFeedRepository
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.telegram import TelegramAdapter
from app.infrastructure.im.wecom import WeComAdapter
from app.worker.queue import CeleryTaskQueue

bearer = HTTPBearer(auto_error=False)


@dataclass(frozen=True)
class DevicePrincipal:
    user: User
    device: Device | None


async def require_analysis_run_principal(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
) -> AnalysisRunTokenPrincipal:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    payload = decode_analysis_run_token(credentials.credentials, settings)
    try:
        owner_user_id = UUID(str(payload["sub"]))
        device_id = UUID(str(payload["did"]))
        run_id = UUID(str(payload["rid"]))
        nonce = str(payload["nonce"])
        hash_analysis_run_nonce(nonce)
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    return AnalysisRunTokenPrincipal(
        run_id=run_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        nonce=nonce,
    )


async def require_interactive_agent_turn_principal(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
) -> InteractiveAgentTurnTokenPrincipal:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    payload = decode_interactive_agent_turn_token(credentials.credentials, settings)
    try:
        owner_user_id = UUID(str(payload["sub"]))
        device_id = UUID(str(payload["did"]))
        turn_id = UUID(str(payload["tid"]))
        nonce = str(payload["nonce"])
        hash_interactive_agent_turn_nonce(nonce)
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    return InteractiveAgentTurnTokenPrincipal(
        turn_id=turn_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        nonce=nonce,
    )


async def require_interactive_agent_approval_principal(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
) -> InteractiveAgentApprovalTokenPrincipal:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    payload = decode_interactive_agent_approval_token(credentials.credentials, settings)
    try:
        owner_user_id = UUID(str(payload["sub"]))
        device_id = UUID(str(payload["did"]))
        turn_id = UUID(str(payload["tid"]))
        approval_id = UUID(str(payload["aid"]))
        nonce = str(payload["nonce"])
        hash_interactive_agent_approval_nonce(nonce)
    except (KeyError, TypeError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid token",
        ) from exc
    return InteractiveAgentApprovalTokenPrincipal(
        approval_id=approval_id,
        turn_id=turn_id,
        owner_user_id=owner_user_id,
        device_id=device_id,
        nonce=nonce,
    )


async def require_admin(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
    session: AsyncSession = Depends(get_session),
) -> User | None:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    if not constant_time_equals(credentials.credentials, settings.admin_api_token):
        user = await _user_from_token(credentials.credentials, settings, session)
        if not user.is_admin:
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="admin required")
        return user
    return None


async def _principal_from_token(
    token: str,
    settings: Settings,
    session: AsyncSession,
) -> DevicePrincipal:
    payload = decode_access_token(token, settings)
    try:
        user_id = UUID(str(payload["sub"]))
    except (KeyError, ValueError) as exc:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token"
        ) from exc

    user = await UserRepository(session).get(user_id)
    if not user or not user.is_active:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="inactive user")
    device = None
    device_id_value = payload.get("did")
    if device_id_value is not None:
        try:
            device_id = UUID(str(device_id_value))
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="invalid token",
            ) from exc
        device = await DeviceRepository(session).get_active_owned(user.id, device_id)
        if not device:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="inactive device",
            )
    return DevicePrincipal(user=user, device=device)


async def _user_from_token(token: str, settings: Settings, session: AsyncSession) -> User:
    return (await _principal_from_token(token, settings, session)).user


async def require_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
    session: AsyncSession = Depends(get_session),
) -> User:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    return await _user_from_token(credentials.credentials, settings, session)


async def require_device_principal(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
    session: AsyncSession = Depends(get_session),
) -> DevicePrincipal:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    principal = await _principal_from_token(credentials.credentials, settings, session)
    if principal.device is None:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="device session required",
        )
    return principal


async def get_redis_client(
    settings: Settings = Depends(get_settings),
) -> AsyncGenerator[Redis, None]:
    redis = Redis.from_url(settings.redis_url, decode_responses=True)
    try:
        yield redis
    finally:
        await redis.aclose()


def get_message_repo(session: AsyncSession = Depends(get_session)) -> MessageRepository:
    return MessageRepository(session)


def get_manual_reply_delivery_repo(
    session: AsyncSession = Depends(get_session),
) -> ManualReplyDeliveryRepository:
    return ManualReplyDeliveryRepository(session)


def get_opportunity_repo(session: AsyncSession = Depends(get_session)) -> OpportunityRepository:
    return OpportunityRepository(session)


def get_rule_repo(session: AsyncSession = Depends(get_session)) -> RuleRepository:
    return RuleRepository(session)


def get_template_repo(session: AsyncSession = Depends(get_session)) -> ReplyTemplateRepository:
    return ReplyTemplateRepository(session)


def get_user_repo(session: AsyncSession = Depends(get_session)) -> UserRepository:
    return UserRepository(session)


def get_device_session_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> DeviceSessionService:
    return DeviceSessionService(DeviceRepository(session), settings)


def get_sync_feed_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> SyncFeedService:
    return SyncFeedService(SyncFeedRepository(session), settings)


def get_signal_appetite_sync_service(
    session: AsyncSession = Depends(get_session),
) -> SignalAppetiteSyncService:
    return SignalAppetiteSyncService(SignalAppetiteRepository(session))


def get_push_registration_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> PushRegistrationService:
    return PushRegistrationService(PushRegistrationRepository(session), settings)


def get_subscription_repo(session: AsyncSession = Depends(get_session)) -> SubscriptionRepository:
    return SubscriptionRepository(session)


def get_user_settings_repo(session: AsyncSession = Depends(get_session)) -> UserSettingsRepository:
    return UserSettingsRepository(session)


def get_billing_event_repo(session: AsyncSession = Depends(get_session)) -> BillingEventRepository:
    return BillingEventRepository(session)


def get_telegram_user_config_repo(
    session: AsyncSession = Depends(get_session),
) -> TelegramUserConfigRepository:
    return TelegramUserConfigRepository(session)


def get_telegram_connection_repo(
    session: AsyncSession = Depends(get_session),
) -> TelegramConnectionRepository:
    return TelegramConnectionRepository(session)


def get_wecom_connection_repo(
    session: AsyncSession = Depends(get_session),
) -> WeComConnectionRepository:
    return WeComConnectionRepository(session)


def get_wecom_archive_repo(
    session: AsyncSession = Depends(get_session),
) -> WeComArchiveRepository:
    return WeComArchiveRepository(session)


def get_wecom_event_repo(
    session: AsyncSession = Depends(get_session),
) -> WeComEventRepository:
    return WeComEventRepository(session)


def get_wecom_delivery_repo(
    session: AsyncSession = Depends(get_session),
) -> WeComDeliveryRepository:
    return WeComDeliveryRepository(session)


async def get_work_time_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> WorkTimeService:
    config_repo = ConfigRepository(session)
    raw_config = await config_repo.get_value("working_hours")
    config = (
        WorkTimeConfig.model_validate(raw_config)
        if raw_config
        else WorkTimeConfig.from_settings(settings)
    )
    return WorkTimeService(config)


def get_detector(settings: Settings = Depends(get_settings)) -> OpportunityDetector:
    classifier = LiteLLMOpportunityClassifier(settings)
    return OpportunityDetector(ai_classifier=classifier)


def get_adapter_registry(
    settings: Settings = Depends(get_settings),
    redis: Redis = Depends(get_redis_client),
    telegram_connection_repo: TelegramConnectionRepository = Depends(get_telegram_connection_repo),
    wecom_connection_repo: WeComConnectionRepository = Depends(get_wecom_connection_repo),
    wecom_delivery_repo: WeComDeliveryRepository = Depends(get_wecom_delivery_repo),
) -> AdapterRegistry:
    return AdapterRegistry(
        [
            TelegramAdapter(settings, connection_repo=telegram_connection_repo),
            WeComAdapter(
                settings,
                redis=redis,
                connection_repo=wecom_connection_repo,
                delivery_repo=wecom_delivery_repo,
            ),
        ]
    )


def get_interactive_agent_action_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
    adapters: AdapterRegistry = Depends(get_adapter_registry),
) -> InteractiveAgentActionService:
    return InteractiveAgentActionService(
        repository=InteractiveAgentActionRepository(session),
        manual_reply=ManualReplyUseCase(
            opportunity_repo=OpportunityRepository(session),
            message_repo=MessageRepository(session),
            delivery_repo=ManualReplyDeliveryRepository(session),
            adapters=adapters,
        ),
        adapters=adapters,
        settings=settings,
        routing_service=InteractiveAgentRoutingService(settings=settings),
    )


def get_task_queue() -> CeleryTaskQueue:
    return CeleryTaskQueue()


def get_analysis_run_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
    task_queue: CeleryTaskQueue = Depends(get_task_queue),
) -> AnalysisRunService:
    run_repo = AnalysisRunRepository(session)
    return AnalysisRunService(
        run_repo=run_repo,
        subscription_repo=SubscriptionRepository(session),
        message_repo=MessageRepository(session),
        opportunity_repo=OpportunityRepository(session),
        task_queue=task_queue,
        settings=settings,
        routing_service=DeviceAgentRoutingService(run_repo=run_repo, settings=settings),
    )


def get_device_agent_routing_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> DeviceAgentRoutingService:
    return DeviceAgentRoutingService(
        run_repo=AnalysisRunRepository(session),
        settings=settings,
    )


def get_interactive_agent_routing_service(
    settings: Settings = Depends(get_settings),
) -> InteractiveAgentRoutingService:
    return InteractiveAgentRoutingService(settings=settings)


def get_interactive_agent_turn_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> InteractiveAgentTurnService:
    routing = InteractiveAgentRoutingService(settings=settings)
    return InteractiveAgentTurnService(
        turn_repo=InteractiveAgentTurnRepository(session),
        subscription_repo=SubscriptionRepository(session),
        settings=settings,
        routing_service=routing,
    )


def get_interactive_agent_gateway_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> InteractiveAgentGatewayService:
    return InteractiveAgentGatewayService(
        repository=InteractiveAgentGatewayRepository(session),
        provider_client=OpenAICompatibleGatewayClient(settings),
        settings=settings,
        routing_service=InteractiveAgentRoutingService(settings=settings),
    )


def get_analysis_gateway_service(
    session: AsyncSession = Depends(get_session),
    settings: Settings = Depends(get_settings),
) -> AnalysisGatewayService:
    return AnalysisGatewayService(
        repository=AnalysisGatewayRepository(session),
        provider_client=OpenAICompatibleGatewayClient(settings),
        settings=settings,
    )


def get_analysis_link_inspector(
    settings: Settings = Depends(get_settings),
) -> SafeLinkInspector:
    return SafeLinkInspector(
        max_links=settings.pi_agent_max_links,
        max_content_bytes=settings.pi_agent_max_content_bytes,
        max_text_chars=settings.pi_agent_max_link_text_chars,
        timeout_seconds=settings.pi_agent_link_timeout_seconds,
    )


def get_reply_generator(
    settings: Settings = Depends(get_settings),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
    message_repo: MessageRepository = Depends(get_message_repo),
) -> LiteLLMReplyGenerator:
    return LiteLLMReplyGenerator(
        settings=settings,
        opportunity_repo=opportunity_repo,
        message_repo=message_repo,
    )


async def get_opportunity_or_404(
    opportunity_id: UUID,
    current_user: User = Depends(require_user),
    repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> Opportunity:
    opportunity = await repo.get(opportunity_id)
    if not opportunity or opportunity.owner_user_id != current_user.id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="opportunity not found")
    return opportunity
