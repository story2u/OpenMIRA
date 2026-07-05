const loginPageConfigs = {
  admin: {
    mode: "admin",
    kind: "admin",
    title: "管理中心登录",
    identifierLabel: "用户名",
    identifierParam: "username",
    passwordLabel: "密码",
    requiresPassword: true,
    requiresConfirmation: true,
    submitLabel: "登录",
    defaultRedirect: "/admin",
  },
  cs: {
    mode: "cs",
    kind: "cs",
    title: "客服工作台登录",
    identifierLabel: "客服 ID",
    identifierParam: "cs_id",
    passwordLabel: "密码",
    requiresPassword: true,
    requiresConfirmation: true,
    submitLabel: "登录",
    defaultRedirect: "/",
  },
  passwordless: {
    mode: "passwordless",
    kind: "cs",
    title: "客服免密登录",
    identifierLabel: "客服 ID",
    identifierParam: "cs_id",
    passwordLabel: "",
    requiresPassword: false,
    requiresConfirmation: false,
    submitLabel: "进入工作台",
    defaultRedirect: "/",
  },
};

export function loginPageConfig(mode = "cs") {
  return loginPageConfigs[normalizeLoginPageMode(mode)];
}

export function loginModeFromPath(pathname = "") {
  const normalized = cleanPath(pathname);
  if (normalized === "/admin-login") return "admin";
  if (normalized === "/login") return "passwordless";
  return "cs";
}

export function normalizeLoginPageMode(mode = "") {
  const normalized = String(mode || "").trim().toLowerCase();
  return Object.prototype.hasOwnProperty.call(loginPageConfigs, normalized) ? normalized : "cs";
}

export function resolvePostLoginRedirect(mode = "cs", search = "") {
  const config = loginPageConfig(mode);
  const params = new URLSearchParams(String(search || "").startsWith("?") ? String(search || "") : `?${search || ""}`);
  const redirect = String(params.get("redirect") || params.get("next") || "").trim();
  return isSafeInternalRedirect(redirect) ? redirect : config.defaultRedirect;
}

export function loginPageInitialIdentifier(mode = "cs", search = "") {
  const config = loginPageConfig(mode);
  const params = new URLSearchParams(String(search || "").startsWith("?") ? String(search || "") : `?${search || ""}`);
  return String(params.get(config.identifierParam) || params.get("assignee_id") || (config.mode === "admin" ? "root" : "")).trim();
}

export function loginConfirmation(mode = "cs", identifier = "") {
  const config = loginPageConfig(mode);
  const normalizedIdentifier = String(identifier || "").trim();
  if (!config.requiresConfirmation || !normalizedIdentifier) {
    return { required: false, title: "", message: "", text: "" };
  }
  const title = config.mode === "admin" ? "管理员登录" : "客服登录";
  const message = `登录账号：${normalizedIdentifier}`;
  return {
    required: true,
    title,
    message,
    text: `${title}\n${message}`,
  };
}

function isSafeInternalRedirect(value = "") {
  const target = String(value || "").trim();
  if (!target || !target.startsWith("/") || target.startsWith("//")) return false;
  if (/^\/\\/.test(target)) return false;
  if (/[\u0000-\u001F]/.test(target)) return false;
  return true;
}

function cleanPath(pathname = "") {
  const text = String(pathname || "").trim();
  if (!text) return "/";
  try {
    return new URL(text, "http://localhost").pathname;
  } catch {
    return text.startsWith("/") ? text : `/${text}`;
  }
}
