#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const usage = () => {
  console.error(
    "Usage: next-routes.mjs <app-dir> [--markdown] [--check] [--require=<route1,route2,...>]"
  );
  process.exit(2);
};

const args = process.argv.slice(2);
const appDirArg = args.find((arg) => !arg.startsWith("--"));

if (!appDirArg) {
  usage();
}

const appDir = path.resolve(appDirArg);
const wantMarkdown = args.includes("--markdown");
const doCheck = args.includes("--check");
const rawRequiredArg =
  args.find((arg) => arg.startsWith("--require="))?.slice("--require=".length) ??
  "/,/admin,/login,/admin-login,/cs-login,/version.txt";

const requiredRoutes = rawRequiredArg
  .split(",")
  .map((route) => route.trim())
  .filter((route) => route.length > 0)
  .map((route) => (route === "" ? "/" : route));

const routeFileExt = new Set([".js", ".jsx", ".ts", ".tsx"]);
const routeFiles = new Set(["page", "route"]);
const ignoredFiles = new Set([
  "layout",
  "loading",
  "error",
  "not-found",
  "global-error",
  "forbidden",
  "unauthorized",
  "middleware",
  "template",
  "default",
  "icon",
  "apple-icon",
  "manifest",
  "sitemap",
  "robots",
  "opengraph-image",
  "twitter-image",
]);

const routes = [];
const seen = new Set();

const normalizeSegment = (segment) => {
  if (segment.startsWith("[[...") && segment.endsWith("]]")) {
    const key = segment.slice(4, -2);
    return `:${key}*`;
  }
  if (segment.startsWith("[...") && segment.endsWith("]")) {
    const key = segment.slice(4, -1);
    return `:${key}*`;
  }
  if (segment.startsWith("[") && segment.endsWith("]")) {
    return `:${segment.slice(1, -1)}`;
  }
  return segment;
};

const isRouteGroup = (segment) =>
  segment.startsWith("(") && segment.endsWith(")");

const pushRoute = (segments, routeType, sourceFile) => {
  const normalizedSegments = segments.map(normalizeSegment);
  const routePath = `/${normalizedSegments.join("/")}`.replace(/\/+/g, "/").replace(/\/$/, "").replace(/^$/, "/");
  const finalPath = normalizedSegments.length === 0 ? "/" : routePath;
  const key = `${routeType}|${finalPath}`;
  if (seen.has(key)) {
    return;
  }
  seen.add(key);
  routes.push({
    path: finalPath,
    route_type: routeType,
    source: path.relative(appDir, sourceFile).replace(/\\/g, "/"),
    segments: normalizedSegments,
    kind: segments.length === 0 ? "root" : "nested",
  });
};

const walk = (dir, segments = []) => {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name.startsWith(".")) {
      continue;
    }
    const entryPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (isRouteGroup(entry.name)) {
        walk(entryPath, segments);
      } else {
        walk(entryPath, segments.concat([entry.name]));
      }
      continue;
    }

    if (!entry.isFile()) {
      continue;
    }

    const ext = path.extname(entry.name);
    const stem = path.basename(entry.name, ext);
    if (!routeFileExt.has(ext)) {
      continue;
    }
    if (!routeFiles.has(stem) || ignoredFiles.has(stem)) {
      continue;
    }
    pushRoute(segments, stem, entryPath);
  }
};

if (!fs.existsSync(appDir) || !fs.statSync(appDir).isDirectory()) {
  console.error(`App directory not found or not a directory: ${appDir}`);
  process.exit(2);
}

walk(appDir);

const sortedRoutes = routes.sort((a, b) =>
  a.path.localeCompare(b.path, "en", { numeric: true })
);

const routeSet = new Set(sortedRoutes.map((route) => route.path));
const missingRequiredRoutes = requiredRoutes.filter((route) => !routeSet.has(route));

if (doCheck && missingRequiredRoutes.length > 0) {
  console.error("Missing required Next routes:");
  for (const route of missingRequiredRoutes) {
    console.error(`- ${route}`);
  }
  process.exit(2);
}

if (wantMarkdown) {
  const now = new Date().toISOString();
  const lines = [];
  lines.push("# Next.js Route Inventory");
  lines.push(`- app directory: ${path.relative(process.cwd(), appDir)}`);
  lines.push(`- generated at: ${now}`);
  lines.push(`- route count: ${sortedRoutes.length}`);
  lines.push("");
  lines.push("## Required Routes");
  for (const route of requiredRoutes) {
    const marker = routeSet.has(route) ? "✅" : "❌";
    lines.push(`- ${marker} ${route}`);
  }
  lines.push("");
  lines.push("## Collected Routes");
  for (const route of sortedRoutes) {
    lines.push(`- ${route.path} (${route.route_type}) -> ${route.source}`);
  }
  process.stdout.write(lines.join("\n"));
  process.stdout.write("\n");
} else {
  const result = {
    generated_at_utc: new Date().toISOString(),
    app_dir: path.relative(process.cwd(), appDir),
    route_count: sortedRoutes.length,
    required_routes: requiredRoutes,
    missing_required_routes: missingRequiredRoutes,
    routes: sortedRoutes,
  };
  process.stdout.write(JSON.stringify(result, null, 2));
  process.stdout.write("\n");
}
