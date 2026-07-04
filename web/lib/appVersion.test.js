import assert from "node:assert/strict";
import test from "node:test";
import {
  buildVersionLabel,
  comparableBuildVersion,
  getAppVersionInfo,
  normalizeBuildVersion,
  shortCommit,
} from "./appVersion.js";

test("normalizeBuildVersion falls back when value is empty", () => {
  assert.equal(normalizeBuildVersion(""), "0.1.0");
  assert.equal(normalizeBuildVersion(" 0.2.0-phase2 "), "0.2.0-phase2");
});

test("shortCommit trims long commits and preserves unknown fallback", () => {
  assert.equal(shortCommit("abcdef1234567890"), "abcdef123456");
  assert.equal(shortCommit("unknown"), "unknown");
  assert.equal(shortCommit(""), "unknown");
});

test("getAppVersionInfo reads public build env", () => {
  assert.deepEqual(
    getAppVersionInfo({
      NEXT_PUBLIC_GO_WEB_VERSION: "0.3.0",
      NEXT_PUBLIC_GO_WEB_COMMIT: "0123456789abcdef",
      NEXT_PUBLIC_GO_WEB_BUILD_TIME: "2026-07-01T00:00:00Z",
    }),
    {
      version: "0.3.0",
      commit: "0123456789ab",
      buildTime: "2026-07-01T00:00:00Z",
    },
  );
});

test("buildVersionLabel includes commit only when known", () => {
  assert.equal(buildVersionLabel({ version: "0.3.0", commit: "0123456789" }), "v0.3.0+0123456789");
  assert.equal(buildVersionLabel({ version: "0.3.0", commit: "unknown" }), "v0.3.0");
});

test("comparableBuildVersion prefers commit and falls back to version", () => {
  assert.equal(comparableBuildVersion({ version: "0.3.0", commit: "0123456789abcdef" }), "0123456789ab");
  assert.equal(comparableBuildVersion({ version: "0.3.0", commit: "unknown" }), "0.3.0");
});
