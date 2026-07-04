const DEFAULT_VERSION = "0.1.0";
const UNKNOWN = "unknown";
const defaultEnv = typeof process !== "undefined" && process.env ? process.env : {};

function clean(value) {
  return String(value || "").trim();
}

export function normalizeBuildVersion(value, fallback = DEFAULT_VERSION) {
  const version = clean(value);
  return version || fallback;
}

export function shortCommit(value, fallback = UNKNOWN) {
  const commit = clean(value);
  if (!commit || commit === UNKNOWN) {
    return fallback;
  }
  return commit.length > 12 ? commit.slice(0, 12) : commit;
}

export function getAppVersionInfo(env = defaultEnv) {
  const version = normalizeBuildVersion(env.NEXT_PUBLIC_GO_WEB_VERSION);
  const commit = shortCommit(env.NEXT_PUBLIC_GO_WEB_COMMIT);
  const buildTime = clean(env.NEXT_PUBLIC_GO_WEB_BUILD_TIME) || UNKNOWN;
  return { version, commit, buildTime };
}

export function comparableBuildVersion(info = getAppVersionInfo()) {
  const commit = shortCommit(info.commit, "");
  if (commit && commit !== UNKNOWN) {
    return commit;
  }
  return normalizeBuildVersion(info.version);
}

export function buildVersionLabel(info = getAppVersionInfo()) {
  const version = normalizeBuildVersion(info.version);
  const commit = shortCommit(info.commit);
  if (!commit || commit === UNKNOWN) {
    return `v${version}`;
  }
  return `v${version}+${commit}`;
}
