/*
 * Shared app frame for the phase-one Next.js shell.
 * It keeps navigation stable between / and /admin while the real pages are
 * covered by backend release readiness checks.
 */
import { buildVersionLabel, getAppVersionInfo } from "../lib/appVersion.js";

const navItems = [
  { key: "cs", label: "消息端", href: "/" },
  { key: "admin", label: "管理", href: "/admin" },
];

export function AppShell({ active, children }) {
  const version = getAppVersionInfo();

  return (
    <main className="min-h-screen bg-[#f6f7f9]">
      <header className="border-b border-[#dfe3ec] bg-[#172033] text-white">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-3">
          <div className="text-sm font-semibold tracking-normal">IM Console</div>
          <div className="flex min-w-0 items-center gap-3">
            <span
              className="hidden max-w-[11rem] truncate text-xs text-[#9fb0c9] sm:inline"
              title={`${version.version} ${version.commit} ${version.buildTime}`}
            >
              {buildVersionLabel(version)}
            </span>
            <nav className="flex shrink-0 items-center gap-1" aria-label="主导航">
              {navItems.map((item) => {
                const selected = item.key === active;
                return (
                  <a
                    key={item.key}
                    href={item.href}
                    className={
                      selected
                        ? "rounded bg-white px-3 py-1.5 text-sm font-medium text-[#172033]"
                        : "rounded px-3 py-1.5 text-sm font-medium text-[#d7deea] hover:bg-[#26324a]"
                    }
                  >
                    {item.label}
                  </a>
                );
              })}
            </nav>
          </div>
        </div>
      </header>
      {children}
    </main>
  );
}
