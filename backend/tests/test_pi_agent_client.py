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
        "job_analysis": None,
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
        f"import json, sys\njson.load(sys.stdin)\njson.dump({valid_result()!r}, sys.stdout)\n",
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


async def test_pi_agent_client_accepts_runtime_metrics_envelope(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    envelope = {
        "result": valid_result(),
        "runtime_meta": {
            "prompt_version": "opportunity-job-discovery-v1",
            "token_usage": {"input": 12, "output": 7, "cacheRead": 0, "cacheWrite": 0},
        },
    }
    runner.write_text(
        f"import json, sys\njson.load(sys.stdin)\njson.dump({envelope!r}, sys.stdout)\n",
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


async def test_pi_agent_client_rejects_invalid_contract_without_echoing_output(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    runner.write_text('print(\'{"api_key": "leaked"}\')\n', encoding="utf-8")
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


async def test_pi_agent_client_validates_job_profile_preview(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    preview = {
        "name": "远程后端",
        "target_roles": ["Python Backend Engineer"],
        "excluded_roles": [],
        "target_industries": [],
        "preferred_seniority": ["mid"],
        "candidate_skills": ["Python", "FastAPI"],
        "years_experience": 3,
        "education_level": None,
        "english_level": None,
        "other_languages": [],
        "preferred_countries": [],
        "preferred_cities": [],
        "preferred_timezones": ["Europe/Berlin"],
        "work_modes": ["remote"],
        "employment_types": ["full_time"],
        "minimum_salary": 80000,
        "salary_currency": "USD",
        "salary_period": "annual",
        "visa_sponsorship_required": True,
        "relocation_acceptable": None,
        "required_keywords": [],
        "preferred_keywords": [],
        "excluded_keywords": [],
        "require_salary_disclosed": False,
        "minimum_match_score": 60,
        "notification_enabled": False,
        "requires_confirmation": True,
    }
    runner.write_text(
        f"import json, sys\njson.load(sys.stdin)\njson.dump({preview!r}, sys.stdout)\n",
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

    result = await client.parse_job_search_profile("远程 Python 后端，欧洲时区")

    assert result.requires_confirmation is True
    assert result.work_modes == ["remote"]


async def test_pi_agent_client_validates_source_profile_assessment(tmp_path) -> None:
    runner = tmp_path / "runner.py"
    assessment = {
        "primary_function": "recruitment",
        "secondary_functions": ["career_networking"],
        "industry_tags": ["software"],
        "region_tags": ["europe"],
        "language_tags": ["zh"],
        "job_signal_prior": 0.9,
        "estimated_noise_level": 0.2,
        "reliability_score": 0.8,
        "confidence": 0.88,
        "evidence": ["supplied samples contain hiring signals"],
    }
    runner.write_text(
        f"import json, sys\njson.load(sys.stdin)\njson.dump({assessment!r}, sys.stdout)\n",
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

    result = await client.profile_source_function(
        {"name": "Example Jobs", "recent_samples": ["Hiring Python Engineer"]}
    )

    assert result.primary_function.value == "recruitment"
    assert result.confidence == 0.88
