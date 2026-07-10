from __future__ import annotations

import asyncio
import json
import os
from pathlib import Path

from pydantic import ValidationError

from app.domain.ports import AgentAnalysisRequest, AgentAnalysisResult


class PiAgentError(RuntimeError):
    pass


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
        process = await asyncio.create_subprocess_exec(
            self.node_binary,
            str(self.runner_path),
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env=env,
        )
        payload = json.dumps(request.model_dump(mode="json"), ensure_ascii=False).encode()
        try:
            stdout, _stderr = await asyncio.wait_for(
                process.communicate(payload),
                timeout=self.timeout_seconds,
            )
        except TimeoutError as exc:
            process.kill()
            await process.wait()
            raise PiAgentError("pi agent analysis timed out") from exc
        except BaseException:
            if process.returncode is None:
                process.kill()
                await process.wait()
            raise

        if process.returncode != 0:
            raise PiAgentError(f"pi agent runner failed with exit code {process.returncode}")
        if not stdout or len(stdout) > self.max_output_bytes:
            raise PiAgentError("pi agent returned an empty or oversized response")
        try:
            raw_result = json.loads(stdout)
            return AgentAnalysisResult.model_validate(raw_result)
        except (json.JSONDecodeError, ValidationError, TypeError) as exc:
            raise PiAgentError("pi agent returned an invalid analysis contract") from exc
