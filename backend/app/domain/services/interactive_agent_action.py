from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from uuid import UUID


class InteractiveAgentActionError(Exception):
    pass


class InteractiveAgentActionUnavailableError(InteractiveAgentActionError):
    pass


class InteractiveAgentActionRejectedError(InteractiveAgentActionError):
    pass


class InteractiveAgentActionConflictError(InteractiveAgentActionError):
    pass


class InteractiveAgentActionExpiredError(InteractiveAgentActionError):
    pass


class InteractiveAgentActionUncertainError(InteractiveAgentActionError):
    pass


class InteractiveAgentActionProjectionError(InteractiveAgentActionError):
    pass


@dataclass(frozen=True, slots=True)
class CanonicalSendReplyArguments:
    opportunity_id: UUID
    text: str
    mark_following: bool = True

    def as_json_object(self) -> dict[str, bool | str]:
        return {
            "mark_following": self.mark_following,
            "opportunity_id": str(self.opportunity_id),
            "text": self.text,
        }


def canonical_send_reply_arguments_hash(arguments: CanonicalSendReplyArguments) -> str:
    encoded = json.dumps(
        arguments.as_json_object(),
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()
