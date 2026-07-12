"""用户级设置与看板逻辑的纯单元测试（无需数据库，CI 与本地都能跑）。"""

from datetime import datetime
from zoneinfo import ZoneInfo

import pytest
from pydantic import ValidationError

from app.application.dto import (
    DetectionSettingsUpdate,
    WorkScheduleSlot,
    WorkScheduleUpdate,
    _normalize_keywords,
)
from app.core.time_window import WorkScheduleConfig, WorkScheduleService


class TestKeywordNormalization:
    def test_trims_dedupes_preserves_order(self) -> None:
        assert _normalize_keywords(["  报价 ", "报价", "API", "api"]) == ["报价", "API"]

    def test_drops_empty(self) -> None:
        assert _normalize_keywords(["", "   ", "采购"]) == ["采购"]

    def test_rejects_too_long_keyword(self) -> None:
        with pytest.raises(ValueError):
            _normalize_keywords(["x" * 65])

    def test_rejects_too_many(self) -> None:
        with pytest.raises(ValueError):
            _normalize_keywords([f"kw{i}" for i in range(51)])

    def test_update_dto_applies_normalization(self) -> None:
        dto = DetectionSettingsUpdate(keywords=[" 报价 ", "报价"], aiSemanticsEnabled=False)
        assert dto.keywords == ["报价"]
        assert dto.aiSemanticsEnabled is False


class TestWorkScheduleSlot:
    def test_end_must_be_after_start(self) -> None:
        with pytest.raises(ValidationError):
            WorkScheduleSlot(weekday=1, start="18:00", end="09:00")

    def test_valid_slot(self) -> None:
        slot = WorkScheduleSlot(weekday=3, start="09:00", end="18:00")
        assert slot.weekday == 3

    def test_bad_time_format_rejected(self) -> None:
        with pytest.raises(ValidationError):
            WorkScheduleSlot(weekday=1, start="9:00", end="18:00")

    def test_weekday_range(self) -> None:
        with pytest.raises(ValidationError):
            WorkScheduleSlot(weekday=8, start="09:00", end="18:00")


class TestWorkScheduleUpdate:
    def test_rejects_invalid_timezone(self) -> None:
        with pytest.raises(ValidationError):
            WorkScheduleUpdate(timezone="Not/AZone", slots=[])

    def test_accepts_iana_timezone(self) -> None:
        dto = WorkScheduleUpdate(
            timezone="Asia/Shanghai",
            slots=[WorkScheduleSlot(weekday=1, start="09:00", end="18:00")],
        )
        assert dto.timezone == "Asia/Shanghai"


class TestWorkScheduleService:
    def _config(self) -> WorkScheduleConfig:
        return WorkScheduleConfig(
            timezone="Asia/Shanghai",
            slots=[
                {"weekday": 1, "start": "09:00", "end": "18:00"},
                {"weekday": 6, "start": "10:00", "end": "12:00"},
            ],
        )

    def test_inside_slot_is_working_time(self) -> None:
        service = WorkScheduleService(self._config())
        # 周一 14:00 上海时间落在 09:00-18:00
        at = datetime(2026, 7, 13, 14, 0, tzinfo=ZoneInfo("Asia/Shanghai"))
        assert service.is_working_time(at) is True

    def test_outside_slot_is_not_working_time(self) -> None:
        service = WorkScheduleService(self._config())
        at = datetime(2026, 7, 13, 20, 0, tzinfo=ZoneInfo("Asia/Shanghai"))
        assert service.is_working_time(at) is False

    def test_wrong_weekday_is_not_working_time(self) -> None:
        service = WorkScheduleService(self._config())
        # 周日不在任何时段
        at = datetime(2026, 7, 12, 11, 0, tzinfo=ZoneInfo("Asia/Shanghai"))
        assert service.is_working_time(at) is False

    def test_timezone_conversion(self) -> None:
        service = WorkScheduleService(self._config())
        # 周一 09:30 UTC = 周一 17:30 上海，仍在时段内
        at = datetime(2026, 7, 13, 9, 30, tzinfo=ZoneInfo("UTC"))
        assert service.is_working_time(at) is True

    def test_empty_schedule_never_working(self) -> None:
        service = WorkScheduleService(WorkScheduleConfig(timezone="UTC", slots=[]))
        at = datetime(2026, 7, 13, 14, 0, tzinfo=ZoneInfo("UTC"))
        assert service.is_working_time(at) is False
