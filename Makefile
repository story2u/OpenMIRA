.PHONY: harness-check backend-sync backend-check frontend-check check

PYTHON ?= python3
UV ?= uv

harness-check:
	$(PYTHON) scripts/harness_check.py

backend-sync:
	cd backend && $(UV) sync --locked --dev

backend-check: backend-sync
	cd backend && $(UV) run --locked python -m compileall -q app tests scripts alembic
	cd backend && $(UV) run --locked ruff check app tests scripts alembic --select E,F,ASYNC --ignore E501
	cd backend && $(UV) run --locked pytest -q

frontend-check:
	cd frontend && pnpm lint
	cd frontend && pnpm typecheck
	cd frontend && pnpm build

check: harness-check backend-check frontend-check
