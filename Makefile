.PHONY: harness-check backend-sync backend-check pi-agent-sync pi-agent-check frontend-check check

PYTHON ?= python3
UV ?= uv

harness-check:
	$(PYTHON) scripts/harness_check.py
	$(PYTHON) -m unittest discover -s scripts/tests -p 'test_*.py'

backend-sync:
	cd backend && $(UV) sync --locked --dev

backend-check: backend-sync
	cd backend && $(UV) run --locked python -m compileall -q app tests scripts alembic
	cd backend && $(UV) run --locked ruff check app tests scripts alembic --select E,F,ASYNC --ignore E501
	cd backend && $(UV) run --locked pytest -q

pi-agent-sync:
	cd backend/pi-agent-runtime && npm ci --ignore-scripts

pi-agent-check: pi-agent-sync
	cd backend/pi-agent-runtime && npm run check
	cd backend/pi-agent-runtime && npm test

frontend-check:
	cd frontend && pnpm lint
	cd frontend && pnpm typecheck
	cd frontend && pnpm build

check: harness-check backend-check pi-agent-check frontend-check
