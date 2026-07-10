import os
from types import SimpleNamespace
from uuid import uuid4

os.environ.setdefault("DATABASE_URL", "postgresql+asyncpg://user:pass@localhost/test")
os.environ.setdefault("ADMIN_API_TOKEN", "test-token")

from app.worker import queue as queue_module
from app.worker.queue import CeleryTaskQueue


def test_agent_queue_is_a_noop_when_feature_is_disabled(monkeypatch) -> None:
    monkeypatch.setattr(
        queue_module,
        "get_settings",
        lambda: SimpleNamespace(
            pi_agent_enabled=False,
            effective_pi_agent_api_key="",
            pi_agent_provider="deepseek",
        ),
    )
    monkeypatch.setattr(
        queue_module.celery_app,
        "send_task",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError((args, kwargs))),
    )

    assert CeleryTaskQueue().enqueue_agent_analysis(uuid4()) is False


def test_agent_queue_routes_message_id_and_force_flag(monkeypatch) -> None:
    sent: list[tuple[str, list]] = []
    monkeypatch.setattr(
        queue_module,
        "get_settings",
        lambda: SimpleNamespace(
            pi_agent_enabled=True,
            effective_pi_agent_api_key="secret",
            pi_agent_provider="deepseek",
        ),
    )
    monkeypatch.setattr(
        queue_module.celery_app,
        "send_task",
        lambda name, args: sent.append((name, args)),
    )
    message_id = uuid4()

    assert CeleryTaskQueue().enqueue_agent_analysis(message_id, force=True) is True
    assert sent == [("agent.analyze_message", [str(message_id), True])]


def test_agent_queue_is_a_noop_until_provider_key_is_configured(monkeypatch) -> None:
    monkeypatch.setattr(
        queue_module,
        "get_settings",
        lambda: SimpleNamespace(
            pi_agent_enabled=True,
            effective_pi_agent_api_key="",
            pi_agent_provider="deepseek",
        ),
    )
    monkeypatch.setattr(
        queue_module.celery_app,
        "send_task",
        lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError((args, kwargs))),
    )

    assert CeleryTaskQueue().enqueue_agent_analysis(uuid4()) is False
