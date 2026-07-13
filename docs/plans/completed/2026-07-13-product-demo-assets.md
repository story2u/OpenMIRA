# Product Demo Assets

> Status: completed
> Branch: `features/product-demo-assets`
> Baseline: `release/v2.0.0` at `9e43250`

## Goal

Deliver a truthful unauthenticated product home, deterministic Web demo, repeatable screenshots,
Playwright recording, Remotion presentation, FFmpeg derivatives, mobile screenshot harnesses, and
release documentation without exposing production data or credentials.

## Acceptance

- Unauthenticated `/` is a responsive product home; authenticated `/` remains the real dashboard.
- `/demo/*` is available only when `DEMO_MODE=true` and uses isolated fictional data.
- Playwright produces the documented Web screenshots and a deterministic raw WebM recording.
- Remotion and FFmpeg produce validated MP4, WebM, GIF and cover outputs without narration.
- iOS/Android Debug-only demo routes and screenshot scripts fail clearly without platform tools.
- README and release docs distinguish implemented, Beta, and externally unverified capabilities.

## Work

1. Audit routes, auth, data, mobile projects, workflows and host media tools.
2. Build isolated demo data/routes and unauthenticated home.
3. Add screenshot and recording automation with stable selectors.
4. Add Remotion timeline and FFmpeg post-processing.
5. Add Debug-only mobile demo routes and capture scripts.
6. Update README, docs, workflow and checks; generate and inspect actual Web assets.

## Safety Decisions

- Demo data uses fixed IDs, `example.com` addresses, fictional groups and a fixed clock.
- Demo routes never initialize OAuth, RevenueCat, Telegram or mutation APIs.
- Production builds fail closed because both server and client demo gates default to false.
- Generated binary video is a build artifact; only lightweight screenshots/GIF/cover are candidates
  for normal Git history. Full MP4/WebM are prepared for GitHub Release.

## Verification Log

- `make check`: passed; backend 91 passed / 9 skipped, pi runtime 4 passed, frontend 5 passed,
  harness 49 linked Markdown / 79 backend Python files.
- Playwright screenshot suite: 3 passed; 16 Web screenshots generated and visually inspected.
- Playwright recording: 1920×1080 raw WebM generated after deterministic nine-scene flow.
- Remotion: 4500 frames, 1920×1080, 30fps, 150 seconds, no narration required.
- FFmpeg/ffprobe: H.264 MP4, VP9 WebM, optimized GIF and PNG cover generated; no black-frame
  interval detected; sampled frames at 2/12/45/100/125/148 seconds inspected.
- Lighthouse production home: Performance 96, Accessibility 100, Best Practices 100, SEO 100.
- Production gate: `/` returned 200 and `/demo` returned 404 without Demo environment variables.
- GitHub CI run `29219876642`: backend, frontend + Playwright, harness, pi, iOS and Android passed.
- Local mobile screenshots were not captured: host has no Xcode or Android SDK/adb/JDK. Both scripts
  failed with explicit prerequisite messages; platform source builds passed in CI.

## Remaining External Work

- Upload the ignored MP4/WebM and optional Android demo APK through `Demo Assets` workflow or a
  `demo-v*` GitHub tag. No Release or TestFlight distribution was claimed in this work.
- Run the platform screenshot scripts on macOS and an Android emulator before replacing the honest
  Beta placeholders in the video.
