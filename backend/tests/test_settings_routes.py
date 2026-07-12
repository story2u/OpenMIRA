"""用户级设置 API 的契约测试：owner 由登录用户确定，客户端不能提交 user_id。

用 TestClient + dependency_overrides + 内存假仓储，无需数据库。
"""

from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.deps import get_user_settings_repo, get_settings as get_settings_dep, require_user
from app.api.v1.routes import settings as settings_route
from app.core.config import Settings
from app.infrastructure.db.models import (
    User,
    UserDetectionPreference,
    UserNotificationPreference,
    UserWorkSchedule,
)


class FakeSettingsRepo:
    def __init__(self) -> None:
        self.detection: dict = {}
        self.schedule: dict = {}
        self.notifications: dict = {}

    async def get_detection(self, user_id):
        return self.detection.get(user_id)

    async def upsert_detection(self, *, user_id, keywords, ai_semantics_enabled):
        pref = UserDetectionPreference(
            user_id=user_id, keywords=keywords, ai_semantics_enabled=ai_semantics_enabled
        )
        self.detection[user_id] = pref
        return pref

    async def get_work_schedule(self, user_id):
        return self.schedule.get(user_id)

    async def upsert_work_schedule(self, *, user_id, timezone, slots, auto_reply_outside_hours):
        sched = UserWorkSchedule(
            user_id=user_id,
            timezone=timezone,
            slots=slots,
            auto_reply_outside_hours=auto_reply_outside_hours,
        )
        self.schedule[user_id] = sched
        return sched

    async def get_notifications(self, user_id):
        return self.notifications.get(user_id)

    async def upsert_notifications(
        self, *, user_id, new_opportunity_enabled, ai_replied_enabled, daily_digest_enabled, urgent_only
    ):
        pref = UserNotificationPreference(
            user_id=user_id,
            new_opportunity_enabled=new_opportunity_enabled,
            ai_replied_enabled=ai_replied_enabled,
            daily_digest_enabled=daily_digest_enabled,
            urgent_only=urgent_only,
        )
        self.notifications[user_id] = pref
        return pref


def make_settings() -> Settings:
    return Settings(
        database_url="postgresql+asyncpg://u:p@localhost/db",
        admin_api_token="tok",
        telegram_webhook_secret="sec",
        default_timezone="Asia/Shanghai",
    )


def build_client(repo: FakeSettingsRepo, user: User) -> TestClient:
    app = FastAPI()
    app.include_router(settings_route.router, prefix="/settings")
    app.dependency_overrides[require_user] = lambda: user
    app.dependency_overrides[get_user_settings_repo] = lambda: repo
    app.dependency_overrides[get_settings_dep] = make_settings
    return TestClient(app)


def _user() -> User:
    return User(id=uuid4(), email="u@example.test", display_name="U")


def test_get_me_returns_defaults_when_unset() -> None:
    repo = FakeSettingsRepo()
    client = build_client(repo, _user())
    resp = client.get("/settings/me")
    assert resp.status_code == 200
    body = resp.json()
    assert body["detection"] == {"keywords": [], "aiSemanticsEnabled": True}
    assert body["workSchedule"]["isDefault"] is True
    assert body["workSchedule"]["timezone"] == "Asia/Shanghai"
    assert body["capabilities"] == {
        "pushAvailable": False,
        "wecomUserBindingAvailable": False,
    }


def test_patch_detection_normalizes_and_persists() -> None:
    repo = FakeSettingsRepo()
    user = _user()
    client = build_client(repo, user)
    resp = client.patch(
        "/settings/detection",
        json={"keywords": [" 报价 ", "报价", "API"], "aiSemanticsEnabled": False},
    )
    assert resp.status_code == 200
    assert resp.json() == {"keywords": ["报价", "API"], "aiSemanticsEnabled": False}
    assert repo.detection[user.id].keywords == ["报价", "API"]


def test_patch_detection_rejects_too_long_keyword() -> None:
    client = build_client(FakeSettingsRepo(), _user())
    resp = client.patch(
        "/settings/detection",
        json={"keywords": ["x" * 65], "aiSemanticsEnabled": True},
    )
    assert resp.status_code == 422


def test_patch_work_schedule_validates_timezone() -> None:
    client = build_client(FakeSettingsRepo(), _user())
    resp = client.patch(
        "/settings/work-schedule",
        json={"timezone": "Not/AZone", "slots": [], "autoReplyOutsideHours": True},
    )
    assert resp.status_code == 422


def test_patch_work_schedule_persists() -> None:
    repo = FakeSettingsRepo()
    user = _user()
    client = build_client(repo, user)
    resp = client.patch(
        "/settings/work-schedule",
        json={
            "timezone": "Asia/Shanghai",
            "slots": [{"weekday": 1, "start": "09:00", "end": "18:00"}],
            "autoReplyOutsideHours": True,
        },
    )
    assert resp.status_code == 200
    assert resp.json()["isDefault"] is False
    assert repo.schedule[user.id].timezone == "Asia/Shanghai"


def test_patch_work_schedule_rejects_end_before_start() -> None:
    client = build_client(FakeSettingsRepo(), _user())
    resp = client.patch(
        "/settings/work-schedule",
        json={
            "timezone": "Asia/Shanghai",
            "slots": [{"weekday": 1, "start": "18:00", "end": "09:00"}],
            "autoReplyOutsideHours": True,
        },
    )
    assert resp.status_code == 422


def test_patch_notifications_persists() -> None:
    repo = FakeSettingsRepo()
    user = _user()
    client = build_client(repo, user)
    resp = client.patch(
        "/settings/notifications",
        json={
            "newOpportunityEnabled": False,
            "aiRepliedEnabled": True,
            "dailyDigestEnabled": True,
            "urgentOnly": True,
        },
    )
    assert resp.status_code == 200
    assert repo.notifications[user.id].urgent_only is True


def test_client_cannot_inject_user_id() -> None:
    # 即便请求体带 user_id，也不会被采用：owner 来自 require_user。
    repo = FakeSettingsRepo()
    user = _user()
    client = build_client(repo, user)
    resp = client.patch(
        "/settings/detection",
        json={"keywords": ["报价"], "aiSemanticsEnabled": True, "user_id": str(uuid4())},
    )
    assert resp.status_code == 200
    assert user.id in repo.detection
