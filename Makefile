.PHONY: harness-check backend-sync backend-check pi-agent-sync pi-agent-check frontend-check ios-check android-check check

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

# 需要 macOS + Xcode + xcodegen（brew install xcodegen）；CI 用 macOS runner。
ios-check:
	cd mobile/ios && xcodegen generate
	cd mobile/ios && xcodebuild -project OpportunityRadar.xcodeproj -scheme OpportunityRadar \
		-destination 'generic/platform=iOS Simulator' -derivedDataPath .build/DerivedData \
		CODE_SIGNING_ALLOWED=NO build

# 需要 JDK 17 + Android SDK；首次运行 `cd mobile/android && gradle wrapper` 生成 wrapper。
android-check:
	cd mobile/android && ./gradlew --no-daemon lintDebug testDebugUnitTest assembleDebug

check: harness-check backend-check pi-agent-check frontend-check
