"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { changeAdminPassword, loginAdminWithPassword, loginCSWithPassword, loginCSWithoutPassword, sessionLoginErrorMessage } from "../lib/sessionLogin.js";
import { loginConfirmation, loginPageConfig, loginPageInitialIdentifier, resolvePostLoginRedirect } from "../lib/sessionLoginPage.js";

export function LoginPageClient({ mode = "cs" }) {
  const config = useMemo(() => loginPageConfig(mode), [mode]);
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [changeToken, setChangeToken] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") return;
    setIdentifier(loginPageInitialIdentifier(config.mode, window.location.search));
  }, [config.mode]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    if (!identifier.trim()) {
      setError(`请输入${config.identifierLabel}`);
      return;
    }
    if (config.requiresPassword && !password.trim()) {
      setError("请输入密码");
      return;
    }
    const confirmation = loginConfirmation(config.mode, identifier);
    if (
      confirmation.required
      && typeof window !== "undefined"
      && typeof window.confirm === "function"
      && !window.confirm(confirmation.text)
    ) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      let response;
      if (config.mode === "admin") {
        response = await loginAdminWithPassword(identifier, password);
        if (response.password_change_required) {
          setChangeToken(response.token);
          setCurrentPassword("");
          setNewPassword("");
          setConfirmPassword("");
          return;
        }
      } else if (config.mode === "passwordless") {
        await loginCSWithoutPassword(identifier);
      } else {
        await loginCSWithPassword(identifier, password);
      }
      const target = typeof window === "undefined"
        ? config.defaultRedirect
        : resolvePostLoginRedirect(config.mode, window.location.search);
      window.location.assign(target);
    } catch (err) {
      setError(sessionLoginErrorMessage(config.kind, err));
    } finally {
      setLoading(false);
    }
  }, [config, identifier, password]);

  const handlePasswordChange = useCallback(async (event) => {
    event.preventDefault();
    if (!currentPassword.trim()) {
      setError("请输入当前密码");
      return;
    }
    if (newPassword.trim().length < 10) {
      setError("新密码至少 10 位");
      return;
    }
    if (newPassword.trim() !== confirmPassword.trim()) {
      setError("两次输入的新密码不一致");
      return;
    }
    setLoading(true);
    setError("");
    try {
      await changeAdminPassword(currentPassword, newPassword, { token: changeToken });
      const target = typeof window === "undefined"
        ? config.defaultRedirect
        : resolvePostLoginRedirect(config.mode, window.location.search);
      window.location.assign(target);
    } catch (err) {
      setError(sessionLoginErrorMessage(config.kind, err));
    } finally {
      setLoading(false);
    }
  }, [changeToken, config, currentPassword, newPassword, confirmPassword]);

  if (config.mode === "admin" && changeToken) {
    return (
      <div className="mx-auto grid max-w-7xl px-4 py-4 lg:px-6">
        <section className="grid min-h-[640px] items-center border border-[#d8dde8] bg-white p-4 md:p-8">
          <form className="mx-auto grid w-full max-w-sm gap-4" onSubmit={handlePasswordChange}>
            <div>
              <h1 className="text-lg font-semibold text-[#172033]">修改管理员密码</h1>
              <p className="mt-1 text-xs text-[#697386]">/api/v1/session/admin/change-password</p>
            </div>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">当前密码</span>
              <input
                className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                type="password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
                placeholder="current password"
                autoComplete="current-password"
                autoFocus
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">新密码</span>
              <input
                className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                type="password"
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
                placeholder="new password"
                autoComplete="new-password"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">确认新密码</span>
              <input
                className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                placeholder="confirm password"
                autoComplete="new-password"
              />
            </label>
            {error && <div className="border border-[#f2b8b5] bg-[#fff4f2] px-3 py-2 text-sm text-[#b42318]">{error}</div>}
            <button
              className="h-10 border border-[#172033] bg-[#172033] px-4 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={loading}
            >
              {loading ? "更新中" : "更新密码"}
            </button>
          </form>
        </section>
      </div>
    );
  }

  return (
    <div className="mx-auto grid max-w-7xl px-4 py-4 lg:px-6">
      <section className="grid min-h-[640px] items-center border border-[#d8dde8] bg-white p-4 md:p-8">
        <form className="mx-auto grid w-full max-w-sm gap-4" onSubmit={handleSubmit}>
          <div>
            <h1 className="text-lg font-semibold text-[#172033]">{config.title}</h1>
            <p className="mt-1 text-xs text-[#697386]">{loginEndpointLabel(config.mode)}</p>
          </div>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">{config.identifierLabel}</span>
            <input
              className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={identifier}
              onChange={(event) => setIdentifier(event.target.value)}
              placeholder={config.identifierParam}
              autoComplete={config.mode === "admin" ? "username" : "off"}
              autoFocus
            />
          </label>
          {config.requiresPassword && (
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">{config.passwordLabel}</span>
              <input
                className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="password"
                autoComplete="current-password"
              />
            </label>
          )}
          {error && <div className="border border-[#f2b8b5] bg-[#fff4f2] px-3 py-2 text-sm text-[#b42318]">{error}</div>}
          <button
            className="h-10 border border-[#172033] bg-[#172033] px-4 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={loading}
          >
            {loading ? "登录中" : config.submitLabel}
          </button>
        </form>
      </section>
    </div>
  );
}

function loginEndpointLabel(mode) {
  if (mode === "admin") return "/api/v1/session/admin-login";
  if (mode === "passwordless") return "/api/v1/session/login";
  return "/api/v1/session/cs-login";
}
