"use client";

import { useEffect } from "react";
import { startAppVersionMonitor } from "../lib/appVersionMonitor.js";
import { clientLogger } from "../lib/clientLogger.js";

export function ClientTelemetry() {
  useEffect(() => {
    clientLogger.install();
    clientLogger.info("web", "next_app_mounted", "Next app mounted", {
      path: window.location.pathname,
    });
    startAppVersionMonitor();
  }, []);

  return null;
}
