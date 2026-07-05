/*
 * Next.js root layout for the standalone IM frontend.
 * The shell is intentionally static until API contracts and realtime events
 * are covered by release readiness tests.
 */
import "./globals.css";
import { ClientTelemetry } from "../components/ClientTelemetry.jsx";
import { getAppVersionInfo } from "../lib/appVersion.js";
import { Providers } from "./providers";

export const metadata = {
  title: "IM Console",
  description: "Go and Next.js IM console",
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
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
