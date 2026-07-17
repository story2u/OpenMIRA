from sqlalchemy import CheckConstraint, ForeignKeyConstraint, Index, UniqueConstraint

from app.infrastructure.db.models import (
    AnalysisProviderRequest,
    AnalysisRun,
    InteractiveAgentActionApproval,
    InteractiveAgentProviderRequest,
    InteractiveAgentTurn,
    Message,
    UsageLedger,
)


def named_constraints(model, constraint_type) -> set[str]:
    return {
        constraint.name
        for constraint in model.__table__.constraints
        if isinstance(constraint, constraint_type) and constraint.name
    }


def test_analysis_run_has_owner_binding_and_lifecycle_constraints() -> None:
    assert {
        "uq_analysis_runs_owner_id",
        "uq_analysis_runs_usage_ledger",
    }.issubset(named_constraints(AnalysisRun, UniqueConstraint))
    assert {
        "fk_analysis_runs_owner_device",
        "fk_analysis_runs_owner_message",
        "fk_analysis_runs_owner_usage_ledger",
    }.issubset(named_constraints(AnalysisRun, ForeignKeyConstraint))
    assert {
        "ck_analysis_runs_positive_versions",
        "ck_analysis_runs_nonce_hash_sha256",
        "ck_analysis_runs_lease_after_claim",
        "ck_analysis_runs_result_bounded_object",
        "ck_analysis_runs_link_evidence_bounded_array",
        "ck_analysis_runs_shadow_observation",
        "ck_analysis_runs_lifecycle_state",
    }.issubset(named_constraints(AnalysisRun, CheckConstraint))
    assert {
        "ix_analysis_runs_owner_status_lease",
        "uq_analysis_runs_message_active",
        "uq_analysis_runs_message_shadow",
    }.issubset(
        {index.name for index in AnalysisRun.__table__.indexes if isinstance(index, Index)}
    )


def test_analysis_run_never_persists_a_plaintext_bearer() -> None:
    columns = set(AnalysisRun.__table__.columns.keys())

    assert "token_nonce_hash" in columns
    assert "run_token" not in columns
    assert "token" not in columns
    assert "provider_key" not in columns
    assert "provider_model" not in columns


def test_message_execution_observation_is_content_free_and_bounded() -> None:
    assert "ck_messages_agent_execution_bounded_object" in named_constraints(
        Message,
        CheckConstraint,
    )


def test_owner_composite_keys_exist_on_run_parents() -> None:
    assert "uq_messages_owner_id" in named_constraints(Message, UniqueConstraint)
    assert "uq_usage_ledger_user_id" in named_constraints(UsageLedger, UniqueConstraint)


def test_provider_request_audit_is_owner_bound_content_free_and_single_active() -> None:
    assert {
        "fk_analysis_provider_requests_owner_run",
        "fk_analysis_provider_requests_owner_device",
    }.issubset(named_constraints(AnalysisProviderRequest, ForeignKeyConstraint))
    assert {
        "ck_analysis_provider_requests_cost_nonnegative",
        "ck_analysis_provider_requests_id_hash_sha256",
        "ck_analysis_provider_requests_lifecycle_state",
    }.issubset(named_constraints(AnalysisProviderRequest, CheckConstraint))
    assert "uq_analysis_provider_requests_run_active" in {
        index.name
        for index in AnalysisProviderRequest.__table__.indexes
        if isinstance(index, Index)
    }
    columns = set(AnalysisProviderRequest.__table__.columns.keys())
    assert {"prompt_tokens", "completion_tokens", "estimated_cost_micros"} <= columns
    assert {"prompt", "request_body", "response", "provider_key"}.isdisjoint(columns)


def test_interactive_turn_is_owner_bound_content_free_and_single_active_per_session() -> None:
    assert {
        "uq_interactive_agent_turns_owner_id",
        "uq_interactive_agent_turns_owner_idempotency",
        "uq_interactive_agent_turns_usage_ledger",
    }.issubset(named_constraints(InteractiveAgentTurn, UniqueConstraint))
    assert {
        "fk_interactive_agent_turns_owner_device",
        "fk_interactive_agent_turns_owner_usage_ledger",
    }.issubset(named_constraints(InteractiveAgentTurn, ForeignKeyConstraint))
    assert {
        "ck_interactive_agent_turns_versions_and_count",
        "ck_interactive_agent_turns_nonce_hash_sha256",
        "ck_interactive_agent_turns_lease_after_claim",
        "ck_interactive_agent_turns_lifecycle_state",
    }.issubset(named_constraints(InteractiveAgentTurn, CheckConstraint))
    assert "uq_interactive_agent_turns_session_active" in {
        index.name
        for index in InteractiveAgentTurn.__table__.indexes
        if isinstance(index, Index)
    }
    columns = set(InteractiveAgentTurn.__table__.columns.keys())
    assert "token_nonce_hash" in columns
    assert {
        "turn_token",
        "prompt",
        "messages",
        "tool_args",
        "tool_result",
        "response",
        "provider_key",
    }.isdisjoint(columns)


def test_interactive_provider_audit_is_content_free_and_sequence_bound() -> None:
    assert {
        "fk_interactive_agent_provider_requests_owner_turn",
        "fk_interactive_agent_provider_requests_owner_device",
    }.issubset(
        named_constraints(InteractiveAgentProviderRequest, ForeignKeyConstraint)
    )
    assert {
        "uq_interactive_agent_provider_requests_turn_sequence",
    }.issubset(named_constraints(InteractiveAgentProviderRequest, UniqueConstraint))
    assert {
        "ck_iapr_sequence_positive",
        "ck_iapr_cost_nonnegative",
        "ck_iapr_id_hash_sha256",
        "ck_iapr_lifecycle_state",
    }.issubset(named_constraints(InteractiveAgentProviderRequest, CheckConstraint))
    assert "uq_interactive_agent_provider_requests_turn_active" in {
        index.name
        for index in InteractiveAgentProviderRequest.__table__.indexes
        if isinstance(index, Index)
    }
    columns = set(InteractiveAgentProviderRequest.__table__.columns.keys())
    assert {"prompt", "request_body", "response", "tool_args", "provider_key"}.isdisjoint(
        columns
    )


def test_interactive_action_approval_is_owner_bound_content_free_and_one_time() -> None:
    assert "uq_iaaa_owner_turn_tool_call" in named_constraints(
        InteractiveAgentActionApproval,
        UniqueConstraint,
    )
    assert {
        "fk_iaaa_owner_device",
        "fk_iaaa_owner_turn",
    }.issubset(named_constraints(InteractiveAgentActionApproval, ForeignKeyConstraint))
    assert {
        "ck_iaaa_expected_version_positive",
        "ck_iaaa_arguments_hash_sha256",
        "ck_iaaa_nonce_hash_sha256",
        "ck_iaaa_lifecycle_state",
    }.issubset(named_constraints(InteractiveAgentActionApproval, CheckConstraint))
    columns = set(InteractiveAgentActionApproval.__table__.columns.keys())
    assert {"arguments_hash", "token_nonce_hash", "expected_version"}.issubset(columns)
    assert {
        "text",
        "prompt",
        "messages",
        "tool_args",
        "approval_token",
        "provider_key",
    }.isdisjoint(columns)
