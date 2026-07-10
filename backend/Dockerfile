FROM node:22.22.0-slim AS pi-agent-runtime

WORKDIR /pi-agent-runtime

COPY pi-agent-runtime/package.json pi-agent-runtime/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts

COPY pi-agent-runtime/src ./src


FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

COPY --from=ghcr.io/astral-sh/uv:0.11.28 /uv /uvx /bin/

WORKDIR /app

RUN adduser --disabled-password --gecos "" appuser

COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev --no-install-project

COPY . .
COPY --from=pi-agent-runtime /usr/local/ /usr/local/
COPY --from=pi-agent-runtime /pi-agent-runtime /app/pi-agent-runtime

ENV PATH="/app/.venv/bin:$PATH"

USER appuser
