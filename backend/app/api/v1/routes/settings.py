from fastapi import APIRouter, Depends

from app.api.deps import get_user_settings_repo, require_user
from app.application.dto import (
    DetectionSettingsRead,
    DetectionSettingsUpdate,
    NotificationSettingsRead,
    NotificationSettingsUpdate,
    SettingsBundleRead,
    SettingsCapabilitiesRead,
    WorkScheduleRead,
    WorkScheduleUpdate,
)
from app.application.mappers import (
    to_detection_settings_read,
    to_notification_settings_read,
    to_work_schedule_read,
)
from app.core.config import Settings, get_settings
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import UserSettingsRepository

router = APIRouter()


def _capabilities() -> SettingsCapabilitiesRead:
    # 诚实的能力位：推送通道与企业微信用户级绑定均未开发。写死为 False，
    # 待对应能力落地后由后端翻转，客户端据此展示"将在启用后生效/由管理员配置"。
    return SettingsCapabilitiesRead(pushAvailable=False, wecomUserBindingAvailable=False)


@router.get("/me", response_model=SettingsBundleRead)
async def get_my_settings(
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repo: UserSettingsRepository = Depends(get_user_settings_repo),
) -> SettingsBundleRead:
    detection = await repo.get_detection(current_user.id)
    schedule = await repo.get_work_schedule(current_user.id)
    notifications = await repo.get_notifications(current_user.id)
    return SettingsBundleRead(
        detection=to_detection_settings_read(detection),
        workSchedule=to_work_schedule_read(schedule, default_timezone=settings.default_timezone),
        notifications=to_notification_settings_read(notifications),
        capabilities=_capabilities(),
    )


@router.patch("/detection", response_model=DetectionSettingsRead)
async def update_detection(
    payload: DetectionSettingsUpdate,
    current_user: User = Depends(require_user),
    repo: UserSettingsRepository = Depends(get_user_settings_repo),
) -> DetectionSettingsRead:
    pref = await repo.upsert_detection(
        user_id=current_user.id,
        keywords=payload.keywords,
        ai_semantics_enabled=payload.aiSemanticsEnabled,
    )
    return to_detection_settings_read(pref)


@router.patch("/work-schedule", response_model=WorkScheduleRead)
async def update_work_schedule(
    payload: WorkScheduleUpdate,
    current_user: User = Depends(require_user),
    settings: Settings = Depends(get_settings),
    repo: UserSettingsRepository = Depends(get_user_settings_repo),
) -> WorkScheduleRead:
    schedule = await repo.upsert_work_schedule(
        user_id=current_user.id,
        timezone=payload.timezone,
        slots=[slot.model_dump() for slot in payload.slots],
        auto_reply_outside_hours=payload.autoReplyOutsideHours,
    )
    return to_work_schedule_read(schedule, default_timezone=settings.default_timezone)


@router.patch("/notifications", response_model=NotificationSettingsRead)
async def update_notifications(
    payload: NotificationSettingsUpdate,
    current_user: User = Depends(require_user),
    repo: UserSettingsRepository = Depends(get_user_settings_repo),
) -> NotificationSettingsRead:
    pref = await repo.upsert_notifications(
        user_id=current_user.id,
        new_opportunity_enabled=payload.newOpportunityEnabled,
        ai_replied_enabled=payload.aiRepliedEnabled,
        daily_digest_enabled=payload.dailyDigestEnabled,
        urgent_only=payload.urgentOnly,
    )
    return to_notification_settings_read(pref)
