import sys

import pytest

from app.domain.enums import IMChannel
from app.domain.ports import AgentAnalysisRequest
from app.infrastructure.agent.pi_client import PiAgentClient, PiAgentError


def valid_result() -> dict:
    return {
        "is_opportunity": True,
        "confidence": 0.91,
        "title": "采购需求",
        "summary": "客户寻找供应商",
        "priority": "high",
        "trust_score": 80,
        "attention_required": False,
        "link_status": "unverified",
        "link_summary": None,
        "risk_flags": [],
        "contacts": {
            "email": None,
            "phone": None,
            "telegram_handle": None,
            "wecom_id": None,
            "extraction_source": None,
        },
        "actions": [],
    }


def request() -> AgentAnalysisRequest:
    return AgentAnalysisRequest(
        message_id="00000000-0000-0000-0000-000000000001",
        channel=IMChannel.TELEGRAM,
        text="采购需求",
    )


async def test_pi_agent_client_validates_subprocess_json_contract(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    runner.write_text(
        "import json, sys\n"
        "json.load(sys.stdin)\n"
        f"json.dump({valid_result()!r}, sys.stdout)\n",
        encoding="utf-8",
    )
    client = PiAgentClient(
        node_binary=sys.executable,
        runner_path=str(runner),
        provider="fake",
        model="fake",
        api_key="secret",
        timeout_seconds=2,
    )

    result = await client.analyze(request())

    assert result.title == "采购需求"
    assert result.confidence == 0.91


async def test_pi_agent_client_rejects_invalid_contract_without_echoing_output(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    runner.write_text("print('{\"api_key\": \"leaked\"}')\n", encoding="utf-8")
    client = PiAgentClient(
        node_binary=sys.executable,
        runner_path=str(runner),
        provider="fake",
        model="fake",
        api_key="secret",
        timeout_seconds=2,
    )

    with pytest.raises(PiAgentError, match="invalid analysis contract") as exc_info:
        await client.analyze(request())

    assert "leaked" not in str(exc_info.value)


async def test_pi_agent_client_kills_timed_out_runner(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    runner.write_text("import time\ntime.sleep(10)\n", encoding="utf-8")
    client = PiAgentClient(
        node_binary=sys.executable,
        runner_path=str(runner),
        provider="fake",
        model="fake",
        api_key="secret",
        timeout_seconds=0.05,
    )

    with pytest.raises(PiAgentError, match="timed out"):
        await client.analyze(request())
