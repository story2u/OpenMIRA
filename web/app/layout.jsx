/*
 * Next.js root layout for the phase-one frontend rewrite.
 * The shell is intentionally static until API contracts and realtime events
 * are migrated behind compatibility tests.
 */
import "./globals.css";
import { ClientTelemetry } from "../components/ClientTelemetry.jsx";
import { getAppVersionInfo } from "../lib/appVersion.js";

export const metadata = {
  title: "WeWork Console",
  description: "Go and Next.js migration shell",
};

export default function RootLayout({ children }) {
  const version = getAppVersionInfo();

  return (
    <html lang="zh-CN">
      <body
        data-build-version={version.version}
        data-build-commit={version.commit}
        data-build-time={version.buildTime}
      >
        <ClientTelemetry />
        {children}
      </body>
    </html>
  );
}
