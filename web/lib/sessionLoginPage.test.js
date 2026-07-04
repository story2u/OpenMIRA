import assert from "node:assert/strict";
import test from "node:test";

import {
  loginModeFromPath,
  loginConfirmation,
  loginPageConfig,
  loginPageInitialIdentifier,
  normalizeLoginPageMode,
  resolvePostLoginRedirect,
} from "./sessionLoginPage.js";

test("loginModeFromPath maps legacy login routes", () => {
  assert.equal(loginModeFromPath("/admin-login"), "admin");
  assert.equal(loginModeFromPath("https://example.test/login?cs_id=cs-1"), "passwordless");
  assert.equal(loginModeFromPath("/cs-login"), "cs");
  assert.equal(loginModeFromPath("/unknown"), "cs");
});

test("loginPageConfig exposes stable form metadata", () => {
  assert.equal(loginPageConfig("admin").defaultRedirect, "/admin");
  assert.equal(loginPageConfig("admin").identifierLabel, "用户名");
  assert.equal(loginPageConfig("cs").requiresPassword, true);
  assert.equal(loginPageConfig("cs").requiresConfirmation, true);
  assert.equal(loginPageConfig("passwordless").requiresPassword, false);
  assert.equal(loginPageConfig("passwordless").requiresConfirmation, false);
  assert.equal(normalizeLoginPageMode("bad"), "cs");
});

test("resolvePostLoginRedirect keeps safe internal targets only", () => {
  assert.equal(resolvePostLoginRedirect("admin", "?redirect=/admin?tab=users"), "/admin?tab=users");
  assert.equal(resolvePostLoginRedirect("cs", "?next=/"), "/");
  assert.equal(resolvePostLoginRedirect("admin", "?redirect=https://evil.example"), "/admin");
  assert.equal(resolvePostLoginRedirect("cs", "?redirect=//evil.example"), "/");
  assert.equal(resolvePostLoginRedirect("passwordless", "?redirect=javascript:alert(1)"), "/");
});

test("loginPageInitialIdentifier reads legacy query params", () => {
  assert.equal(loginPageInitialIdentifier("admin", "?username=admin"), "admin");
  assert.equal(loginPageInitialIdentifier("cs", "?cs_id=cs-001"), "cs-001");
  assert.equal(loginPageInitialIdentifier("passwordless", "?assignee_id=cs-002"), "cs-002");
});

test("loginConfirmation mirrors legacy password login confirmation", () => {
  assert.deepEqual(loginConfirmation("admin", " admin "), {
    required: true,
    title: "管理员登录",
    message: "登录账号：admin",
    text: "管理员登录\n登录账号：admin",
  });
  assert.deepEqual(loginConfirmation("cs", "cs-001"), {
    required: true,
    title: "客服登录",
    message: "登录账号：cs-001",
    text: "客服登录\n登录账号：cs-001",
  });
  assert.equal(loginConfirmation("passwordless", "cs-002").required, false);
  assert.equal(loginConfirmation("admin", "").required, false);
});
