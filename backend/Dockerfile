FROM node:22.22.0-slim AS pi-agent-runtime

WORKDIR /pi-agent-runtime

COPY backend/pi-agent-runtime/package.json backend/pi-agent-runtime/package-lock.json ./
COPY packages/radar-agent/package.json /packages/radar-agent/package.json
COPY packages/radar-agent/src /packages/radar-agent/src
RUN npm ci --omit=dev --ignore-scripts

COPY backend/pi-agent-runtime/src ./src


FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

COPY --from=ghcr.io/astral-sh/uv:0.11.28 /uv /uvx /bin/

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

RUN adduser --disabled-password --gecos "" appuser

COPY backend/pyproject.toml backend/uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev --no-install-project

COPY backend/ .
COPY --from=pi-agent-runtime /usr/local/ /usr/local/
COPY --from=pi-agent-runtime /pi-agent-runtime /app/pi-agent-runtime

ENV PATH="/app/.venv/bin:$PATH"

USER appuser
