from datetime import datetime, time
from zoneinfo import ZoneInfo

from pydantic import BaseModel, Field

from app.core.config import Settings


class WorkTimeConfig(BaseModel):
    timezone: str = "Asia/Shanghai"
    weekdays: list[int] = Field(default_factory=lambda: [1, 2, 3, 4, 5])
    start: str = "09:00"
    end: str = "18:30"
    auto_reply_after_hours: bool = True

    @classmethod
    def from_settings(cls, settings: Settings) -> "WorkTimeConfig":
        return cls(
            timezone=settings.default_timezone,
            weekdays=settings.default_workdays,
            start=settings.default_work_start,
            end=settings.default_work_end,
        )


class WorkScheduleConfig(BaseModel):
    """用户级工作时间：任意人工审核时段列表；未落入任一时段则视为非工作时间。"""

    timezone: str = "Asia/Shanghai"
    # 每个元素 {"weekday": 1-7, "start": "HH:MM", "end": "HH:MM"}
    slots: list[dict] = Field(default_factory=list)
    auto_reply_outside_hours: bool = True


def parse_hhmm(value: str) -> time:
    hour, minute = value.split(":", maxsplit=1)
    return time(hour=int(hour), minute=int(minute))


class WorkTimeService:
    def __init__(self, config: WorkTimeConfig) -> None:
        self.config = config

    def now(self) -> datetime:
        return datetime.now(ZoneInfo(self.config.timezone))

    def is_working_time(self, at: datetime | None = None) -> bool:
        current = at.astimezone(ZoneInfo(self.config.timezone)) if at else self.now()
        iso_weekday = current.isoweekday()
        if iso_weekday not in self.config.weekdays:
            return False

        start = parse_hhmm(self.config.start)
        end = parse_hhmm(self.config.end)
        current_time = current.time()
        if start <= end:
            return start <= current_time <= end

        # Overnight window, for example 22:00-06:00.
        return current_time >= start or current_time <= end


class WorkScheduleService:
    """基于用户自定义时段判断"当前是否人工审核时间"。空时段=始终非工作时间。"""

    def __init__(self, config: WorkScheduleConfig) -> None:
        self.config = config

    def is_working_time(self, at: datetime | None = None) -> bool:
        tz = ZoneInfo(self.config.timezone)
        current = at.astimezone(tz) if at else datetime.now(tz)
        iso_weekday = current.isoweekday()
        current_time = current.time()
        for slot in self.config.slots:
            if slot.get("weekday") != iso_weekday:
                continue
            start = parse_hhmm(slot["start"])
            end = parse_hhmm(slot["end"])
            if start <= current_time <= end:
                return True
        return False
