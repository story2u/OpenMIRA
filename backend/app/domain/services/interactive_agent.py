from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from app.domain.enums import InteractiveAgentTurnStatus


class InteractiveAgentTurnError(Exception):
    pass


class InteractiveAgentTurnUnavailableError(InteractiveAgentTurnError):
    pass


class InteractiveAgentTurnNotFoundError(InteractiveAgentTurnError):
    pass


class InteractiveAgentTurnConflictError(InteractiveAgentTurnError):
    pass


class InteractiveAgentTurnLeaseExpiredError(InteractiveAgentTurnError):
    pass


class InteractiveAgentTurnQuotaExceededError(InteractiveAgentTurnError):
    def __init__(self, *, limit: int, allocated: int) -> None:
        super().__init__("interactive Agent turn quota exceeded")
        self.limit = limit
        self.allocated = allocated


class InteractiveAgentTurnTokenRejectedError(InteractiveAgentTurnError):
    pass


class InteractiveAgentTurnVersionConflictError(InteractiveAgentTurnError):
    pass


ACTIVE_INTERACTIVE_AGENT_TURN_STATUSES = {
    InteractiveAgentTurnStatus.CLAIMED,
    InteractiveAgentTurnStatus.RUNNING,
}

INTERACTIVE_AGENT_READ_ONLY_SCHEMA_VERSION = 1
INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION = 2
INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION = 3
INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION = "interactive-read-only-v1"
INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION = "interactive-internal-v2"
INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION = "interactive-approved-send-v3"
INTERACTIVE_AGENT_POLICIES = {
    INTERACTIVE_AGENT_READ_ONLY_SCHEMA_VERSION: INTERACTIVE_AGENT_READ_ONLY_POLICY_VERSION,
    INTERACTIVE_AGENT_INTERNAL_TOOLS_SCHEMA_VERSION: INTERACTIVE_AGENT_INTERNAL_POLICY_VERSION,
    INTERACTIVE_AGENT_APPROVED_SEND_SCHEMA_VERSION: INTERACTIVE_AGENT_APPROVED_SEND_POLICY_VERSION,
}


def supports_interactive_agent_contract(*, schema_version: int, policy_version: str) -> bool:
    return INTERACTIVE_AGENT_POLICIES.get(schema_version) == policy_version


def supports_interactive_agent(
    capabilities: Mapping[str, Any],
    *,
    runtime_version: str,
    schema_version: int,
) -> bool:
    sqlite_schema = capabilities.get("sqlite.schema")
    interactive_schema = capabilities.get("agent.interactiveSchema")
    return (
        capabilities.get("client.reactNative") is True
        and isinstance(sqlite_schema, int)
        and not isinstance(sqlite_schema, bool)
        and sqlite_schema >= 5
        and capabilities.get("agent.streaming") is True
        and capabilities.get("agent.runtime") == runtime_version
        and capabilities.get("agent.interactive") is True
        and isinstance(interactive_schema, int)
        and not isinstance(interactive_schema, bool)
        and interactive_schema >= schema_version
    )
