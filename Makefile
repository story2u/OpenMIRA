.PHONY: harness-check backend-sync backend-check pi-agent-sync pi-agent-check js-sync contracts-check shared-check frontend-check rn-check ios-check android-check check

PYTHON ?= python3
UV ?= uv
PNPM ?= corepack pnpm@10.25.0

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

js-sync:
	$(PNPM) install --frozen-lockfile

contracts-check:
	cd backend && $(UV) run --locked python scripts/export_openapi.py ../packages/radar-contracts/openapi.json --check
	$(PNPM) --dir packages/radar-contracts check

shared-check:
	$(PNPM) --dir packages/radar-core check
	$(PNPM) --dir packages/radar-api check
	$(PNPM) --dir packages/radar-agent check

frontend-check:
	cd frontend && $(PNPM) lint
	cd frontend && $(PNPM) typecheck
	cd frontend && $(PNPM) test
	cd frontend && $(PNPM) build

rn-check: js-sync contracts-check shared-check
	$(PNPM) check:radar
	$(PNPM) --dir mobile/radar export:ios
	$(PNPM) --dir mobile/radar export:android

# 需要 macOS + Xcode + xcodegen（brew install xcodegen）；CI 用 macOS runner。
ios-check:
	cd mobile/ios && xcodegen generate
	cd mobile/ios && xcodebuild test -project OpportunityRadar.xcodeproj -scheme OpportunityRadar \
		-destination 'platform=iOS Simulator,OS=latest,name=iPhone 16' \
		-derivedDataPath .build/DerivedData CODE_SIGNING_ALLOWED=NO

# 需要 JDK 17 + Android SDK；首次运行 `cd mobile/android && gradle wrapper` 生成 wrapper。
android-check:
	cd mobile/android && ./gradlew --no-daemon lintDebug testDebugUnitTest assembleDebug

check: harness-check backend-check pi-agent-check frontend-check rn-check
