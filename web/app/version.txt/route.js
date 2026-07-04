import { comparableBuildVersion, getAppVersionInfo } from "../../lib/appVersion.js";

export function GET() {
  return new Response(`${comparableBuildVersion(getAppVersionInfo())}\n`, {
    headers: {
      "Cache-Control": "no-store",
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
}
