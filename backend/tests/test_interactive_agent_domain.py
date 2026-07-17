from uuid import uuid4

import pytest
from pydantic import ValidationError

from app.core.config import Settings
from app.domain.services.interactive_agent import (
    supports_interactive_agent,
    supports_interactive_agent_contract,
)


def test_interactive_agent_requires_exact_runtime_and_at_least_requested_schema() -> None:
    capabilities = {
        "client.reactNative": True,
        "sqlite.schema": 5,
        "agent.streaming": True,
        "agent.runtime": "pi-0.80.6",
        "agent.interactive": True,
        "agent.interactiveSchema": 2,
    }

    assert supports_interactive_agent(
        capabilities,
        runtime_version="pi-0.80.6",
        schema_version=1,
    )
    for key, invalid in (
        ("client.reactNative", False),
        ("sqlite.schema", 4),
        ("sqlite.schema", True),
        ("agent.streaming", False),
        ("agent.runtime", "pi-0.80.7"),
        ("agent.interactive", False),
        ("agent.interactiveSchema", 0),
        ("agent.interactiveSchema", True),
    ):
        candidate = {**capabilities, key: invalid}
        assert not supports_interactive_agent(
            candidate,
            runtime_version="pi-0.80.6",
            schema_version=1,
        )
    assert supports_interactive_agent(
        {**capabilities, "agent.interactiveSchema": 1},
        runtime_version="pi-0.80.6",
        schema_version=1,
    )
    assert not supports_interactive_agent(
        {**capabilities, "agent.interactiveSchema": 1},
        runtime_version="pi-0.80.6",
        schema_version=2,
    )


def test_interactive_agent_accepts_only_reviewed_schema_policy_pairs() -> None:
    assert supports_interactive_agent_contract(
        schema_version=1,
        policy_version="interactive-read-only-v1",
    )
    assert supports_interactive_agent_contract(
        schema_version=2,
        policy_version="interactive-internal-v2",
    )
    assert supports_interactive_agent_contract(
        schema_version=3,
        policy_version="interactive-approved-send-v3",
    )
    assert not supports_interactive_agent_contract(
        schema_version=2,
        policy_version="interactive-read-only-v1",
    )
    assert not supports_interactive_agent_contract(
        schema_version=3,
        policy_version="interactive-internal-v3",
    )


def test_interactive_agent_beta_defaults_closed_and_normalizes_allowlist() -> None:
    device_id = uuid4()
    settings = Settings(
        database_url="postgresql+asyncpg://user:password@localhost/test",
        admin_api_token="test-admin-token",
        interactive_agent_device_allowlist=f" {device_id}, {device_id} ",
    )

    assert settings.interactive_agent_beta_enabled is False
    assert settings.interactive_agent_gateway_enabled is False
    assert settings.interactive_agent_external_actions_enabled is False
    assert settings.interactive_agent_approval_token_seconds == 120
    assert settings.interactive_agent_beta_monthly_turn_limit == 0
    assert settings.interactive_agent_device_ids == frozenset({str(device_id)})


def test_interactive_agent_allowlist_rejects_invalid_or_unbounded_values() -> None:
    with pytest.raises(ValidationError, match="device allowlist must contain UUIDs"):
        Settings(
            database_url="postgresql+asyncpg://user:password@localhost/test",
            admin_api_token="test-admin-token",
            interactive_agent_device_allowlist="not-a-uuid",
        )
    with pytest.raises(ValidationError, match="device allowlist must not exceed 100"):
        Settings(
            database_url="postgresql+asyncpg://user:password@localhost/test",
            admin_api_token="test-admin-token",
            interactive_agent_device_allowlist=",".join(str(uuid4()) for _ in range(101)),
        )
