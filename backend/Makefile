.PHONY: dev up down migrate test lint format

dev:
	uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

up:
	docker compose up api celery_worker celery_beat

down:
	docker compose down

migrate:
	alembic upgrade head

test:
	pytest

lint:
	ruff check app tests
	mypy app

format:
	ruff format app tests scripts
