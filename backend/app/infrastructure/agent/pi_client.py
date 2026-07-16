from __future__ import annotations

import asyncio
import json
import os
from pathlib import Path
from time import perf_counter

import structlog
from pydantic import ValidationError

from app.domain.job_models import JobSearchProfilePreview, SourceProfileAgentAssessment
from app.domain.ports import AgentAnalysisRequest, AgentAnalysisResult


class PiAgentError(RuntimeError):
    pass


logger = structlog.get_logger(__name__)


class PiAgentClient:
    def __init__(
        self,
        *,
        node_binary: str,
        runner_path: str,
        provider: str,
        model: str,
        api_key: str,
        timeout_seconds: float,
        max_output_bytes: int = 256_000,
    ) -> None:
        self.node_binary = node_binary
        self.runner_path = Path(runner_path)
        self.provider = provider
        self.model = model
        self.api_key = api_key
        self.timeout_seconds = timeout_seconds
        self.max_output_bytes = max_output_bytes

    async def analyze(self, request: AgentAnalysisRequest) -> AgentAnalysisResult:
        raw_result = await self._execute(request.model_dump(mode="json"))
        try:
            return AgentAnalysisResult.model_validate(raw_result)
        except ValidationError as exc:
            raise PiAgentError("pi agent returned an invalid analysis contract") from exc

    async def parse_job_search_profile(self, text: str) -> JobSearchProfilePreview:
        raw_result = await self._execute({"task": "parse_job_search_profile", "text": text})
        try:
            return JobSearchProfilePreview.model_validate(raw_result)
        except ValidationError as exc:
            raise PiAgentError("pi agent returned an invalid profile contract") from exc

    async def profile_source_function(self, source: dict) -> SourceProfileAgentAssessment:
        raw_result = await self._execute({"task": "profile_source_function", "source": source})
        try:
            return SourceProfileAgentAssessment.model_validate(raw_result)
        except ValidationError as exc:
            raise PiAgentError("pi agent returned an invalid source profile contract") from exc

    async def _execute(self, payload: dict) -> dict:
        if not self.api_key:
            raise PiAgentError("pi agent API key is not configured")
        if not self.runner_path.is_file():
            raise PiAgentError("pi agent runner is not installed")

        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": os.environ.get("HOME", "/tmp"),
            "NODE_ENV": "production",
            "PI_OFFLINE": "1",
            "PI_AGENT_PROVIDER": self.provider,
            "PI_AGENT_MODEL": self.model,
            "PI_AGENT_API_KEY": self.api_key,
        }
        started = perf_counter()
        task = str(payload.get("task") or "analyze_message")
        process = await asyncio.create_subprocess_exec(
            self.node_binary,
            str(self.runner_path),
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env=env,
        )
        encoded_payload = json.dumps(payload, ensure_ascii=False).encode()
        try:
            stdout, _stderr = await asyncio.wait_for(
                process.communicate(encoded_payload),
                timeout=self.timeout_seconds,
            )
        except TimeoutError as exc:
            process.kill()
            await process.wait()
            logger.warning(
                "agent.runtime_failed",
                task=task,
                provider=self.provider,
                model=self.model,
                latency_ms=round((perf_counter() - started) * 1000),
                result_status="timeout",
            )
            raise PiAgentError("pi agent analysis timed out") from exc
        except BaseException:
            if process.returncode is None:
                process.kill()
                await process.wait()
            raise

        if process.returncode != 0:
            logger.warning(
                "agent.runtime_failed",
                task=task,
                provider=self.provider,
                model=self.model,
                latency_ms=round((perf_counter() - started) * 1000),
                result_status="failed",
            )
            raise PiAgentError(f"pi agent runner failed with exit code {process.returncode}")
        if not stdout or len(stdout) > self.max_output_bytes:
            raise PiAgentError("pi agent returned an empty or oversized response")
        try:
            envelope = json.loads(stdout)
            if not isinstance(envelope, dict):
                raise TypeError("result must be an object")
            runtime_meta = envelope.get("runtime_meta")
            raw_result = envelope.get("result", envelope)
            if not isinstance(raw_result, dict):
                raise TypeError("result must be an object")
            usage = runtime_meta.get("token_usage", {}) if isinstance(runtime_meta, dict) else {}
            logger.info(
                "agent.runtime_completed",
                task=task,
                provider=self.provider,
                model=self.model,
                prompt_version=(
                    runtime_meta.get("prompt_version")
                    if isinstance(runtime_meta, dict)
                    else "legacy"
                ),
                latency_ms=round((perf_counter() - started) * 1000),
                input_tokens=usage.get("input"),
                output_tokens=usage.get("output"),
                cache_read_tokens=usage.get("cacheRead"),
                cache_write_tokens=usage.get("cacheWrite"),
                result_status="completed",
            )
            return raw_result
        except (json.JSONDecodeError, TypeError) as exc:
            raise PiAgentError("pi agent returned invalid JSON") from exc
