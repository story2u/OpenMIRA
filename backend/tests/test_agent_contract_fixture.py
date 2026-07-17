import json
from pathlib import Path

import pytest
from pydantic import ValidationError

from app.domain.ports import AgentAnalysisResult

FIXTURE_PATH = (
    Path(__file__).resolve().parents[2]
    / "packages"
    / "radar-agent"
    / "fixtures"
    / "analysis-valid.json"
)


def load_fixture() -> dict:
    return json.loads(FIXTURE_PATH.read_text())


def test_shared_agent_result_fixture_matches_python_policy_model() -> None:
    fixture = load_fixture()
    assert AgentAnalysisResult.model_validate(fixture).model_dump(mode="json") == fixture


@pytest.mark.parametrize(
    "mutation",
    [
        {"link_status": "verifying"},
        {"unexpected": True},
    ],
)
def test_python_model_rejects_values_outside_shared_agent_schema(mutation: dict) -> None:
    with pytest.raises(ValidationError):
        AgentAnalysisResult.model_validate({**load_fixture(), **mutation})
