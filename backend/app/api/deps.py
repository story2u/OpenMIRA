from collections.abc import AsyncGenerator
from uuid import UUID

from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from redis.asyncio import Redis
from sqlmodel.ext.asyncio.session import AsyncSession

from app.core.config import Settings, get_settings
from app.core.security import constant_time_equals, decode_access_token
from app.core.time_window import WorkTimeConfig, WorkTimeService
from app.domain.services.detection_policy import OpportunityDetector
from app.infrastructure.ai.litellm_client import LiteLLMOpportunityClassifier, LiteLLMReplyGenerator
from app.infrastructure.db.models import Opportunity, User
from app.infrastructure.db.repositories import (
    ConfigRepository,
    MessageRepository,
    OpportunityRepository,
    ReplyTemplateRepository,
    RuleRepository,
    SubscriptionRepository,
    TelegramUserConfigRepository,
    UserRepository,
)
from app.infrastructure.db.session import get_session
from app.infrastructure.im.base import AdapterRegistry
from app.infrastructure.im.telegram import TelegramAdapter
from app.infrastructure.im.wecom import WeComAdapter
from app.worker.queue import CeleryTaskQueue

bearer = HTTPBearer(auto_error=False)


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


async def _user_from_token(token: str, settings: Settings, session: AsyncSession) -> User:
    payload = decode_access_token(token, settings)
    try:
        user_id = UUID(str(payload["sub"]))
    except (KeyError, ValueError) as exc:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token") from exc

    user = await UserRepository(session).get(user_id)
    if not user or not user.is_active:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="inactive user")
    return user


async def require_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
    session: AsyncSession = Depends(get_session),
) -> User:
    if not credentials or credentials.scheme.lower() != "bearer":
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="missing token")
    return await _user_from_token(credentials.credentials, settings, session)


async def get_redis_client(settings: Settings = Depends(get_settings)) -> AsyncGenerator[Redis, None]:
    redis = Redis.from_url(settings.redis_url, decode_responses=True)
    try:
        yield redis
    finally:
        await redis.aclose()


def get_message_repo(session: AsyncSession = Depends(get_session)) -> MessageRepository:
    return MessageRepository(session)


def get_opportunity_repo(session: AsyncSession = Depends(get_session)) -> OpportunityRepository:
    return OpportunityRepository(session)


def get_rule_repo(session: AsyncSession = Depends(get_session)) -> RuleRepository:
    return RuleRepository(session)


def get_template_repo(session: AsyncSession = Depends(get_session)) -> ReplyTemplateRepository:
    return ReplyTemplateRepository(session)


def get_user_repo(session: AsyncSession = Depends(get_session)) -> UserRepository:
    return UserRepository(session)


def get_subscription_repo(session: AsyncSession = Depends(get_session)) -> SubscriptionRepository:
    return SubscriptionRepository(session)


def get_telegram_user_config_repo(
    session: AsyncSession = Depends(get_session),
) -> TelegramUserConfigRepository:
    return TelegramUserConfigRepository(session)


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
) -> AdapterRegistry:
    return AdapterRegistry(
        [
            TelegramAdapter(settings),
            WeComAdapter(settings, redis=redis),
        ]
    )


def get_task_queue() -> CeleryTaskQueue:
    return CeleryTaskQueue()


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
