from app.domain.enums import OpportunityStatus


ALLOWED_TRANSITIONS: dict[OpportunityStatus, set[OpportunityStatus]] = {
    OpportunityStatus.PENDING_HUMAN: {
        OpportunityStatus.AI_AUTO_REPLY,
        OpportunityStatus.REPLIED,
        OpportunityStatus.FOLLOWING,
        OpportunityStatus.IGNORED,
        OpportunityStatus.CLOSED,
    },
    OpportunityStatus.AI_AUTO_REPLY: {
        OpportunityStatus.PENDING_HUMAN,
        OpportunityStatus.REPLIED,
        OpportunityStatus.FOLLOWING,
        OpportunityStatus.CLOSED,
    },
    OpportunityStatus.REPLIED: {
        OpportunityStatus.FOLLOWING,
        OpportunityStatus.CLOSED,
        OpportunityStatus.IGNORED,
    },
    OpportunityStatus.FOLLOWING: {
        OpportunityStatus.REPLIED,
        OpportunityStatus.CLOSED,
        OpportunityStatus.IGNORED,
    },
    OpportunityStatus.IGNORED: {OpportunityStatus.PENDING_HUMAN, OpportunityStatus.CLOSED},
    OpportunityStatus.CLOSED: set(),
}


class InvalidOpportunityTransition(ValueError):
    pass


class OpportunityClaimConflict(ValueError):
    pass


class OpportunityVersionConflict(ValueError):
    pass


class InternalCommandIdempotencyConflict(ValueError):
    pass


def ensure_transition_allowed(current: OpportunityStatus, target: OpportunityStatus) -> None:
    if current == target:
        return
    if target not in ALLOWED_TRANSITIONS[current]:
        raise InvalidOpportunityTransition(f"cannot transition from {current} to {target}")
