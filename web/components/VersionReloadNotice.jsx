"use client";

import { useEffect, useState } from "react";
import {
  clearVersionReloadNotice,
  formatVersionReloadNotice,
  readVersionReloadNotice,
} from "../lib/appVersionMonitor.js";

export function VersionReloadNotice() {
  const [notice, setNotice] = useState(null);

  useEffect(() => {
    setNotice(readVersionReloadNotice());
  }, []);

  if (!notice) return null;

  const handleClose = () => {
    clearVersionReloadNotice();
    setNotice(null);
  };

  return (
    <section className="border-b border-[#b7dccf] bg-[#edf8f4] text-[#173f35]" aria-live="polite">
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-3 px-6 py-2">
        <div className="min-w-0">
          <div className="text-xs font-semibold">版本已更新</div>
          <div className="truncate text-xs text-[#3f6f62]">{formatVersionReloadNotice(notice)}</div>
        </div>
        <button
          className="h-8 shrink-0 border border-[#8fbfaf] bg-white px-3 text-xs font-medium text-[#173f35] hover:bg-[#f7fbfa]"
          type="button"
          onClick={handleClose}
        >
          关闭
        </button>
      </div>
    </section>
  );
}
