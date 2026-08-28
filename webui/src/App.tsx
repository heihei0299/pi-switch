import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import type { AppState } from "./types";
import { Button, ToastProvider, cx } from "./components/ui";
import { LanguageProvider, useI18n } from "./i18n";
import { HomePanel } from "./components/HomePanel";
import { ProfilesPanel } from "./components/ProfilesPanel";
import { ProxyPanel } from "./components/ProxyPanel";
import { PackagesPanel } from "./components/PackagesPanel";
import { StatsPanel } from "./components/StatsPanel";
import { BackupsPanel } from "./components/BackupsPanel";
import { SettingsPanel } from "./components/SettingsPanel";
import { DoctorPanel } from "./components/DoctorPanel";

type NavKey = "home" | "profiles" | "proxy" | "packages" | "stats" | "backups" | "settings" | "doctor";

const NAV: { key: NavKey; label: string; icon: string }[] = [
  { key: "home", label: "Home", icon: "🏠" },
  { key: "profiles", label: "Profiles", icon: "👤" },
  { key: "proxy", label: "Proxy", icon: "🔄" },
  { key: "packages", label: "Packages", icon: "📦" },
  { key: "stats", label: "Stats", icon: "📊" },
  { key: "backups", label: "Backups", icon: "💾" },
  { key: "settings", label: "Settings", icon: "⚙️" },
  { key: "doctor", label: "Doctor", icon: "🩺" },
];

export interface PanelProps {
  state: AppState;
  refresh: () => Promise<void>;
}

export default function App() {
  return (
    <ToastProvider>
      <ShellWithLang />
    </ToastProvider>
  );
}

function ShellWithLang() {
  const [configLang, setConfigLang] = useState<string | null>(null);
  return (
    <LanguageProvider configLang={configLang}>
      <Shell onConfigLang={setConfigLang} />
    </LanguageProvider>
  );
}

function Shell({ onConfigLang }: { onConfigLang: (lang: string | null) => void }) {
  const [nav, setNav] = useState<NavKey>("home");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [state, setState] = useState<AppState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { t } = useI18n();

  const refresh = useCallback(async () => {
    try {
      const next = await api.getState();
      setState(next);
      onConfigLang(next.settings?.language ?? null);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [onConfigLang]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const initConfig = useCallback(async () => {
    await api.init();
    await refresh();
  }, [refresh]);

  const handleNav = useCallback((key: NavKey) => {
    setNav(key);
    setDrawerOpen(false);
  }, []);

  useEffect(() => {
    if (!drawerOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setDrawerOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [drawerOpen]);

  return (
    <div className="flex h-full flex-col md:flex-row">
      {/* Mobile top bar */}
      <header className="flex shrink-0 items-center justify-between border-b border-white/10 bg-zinc-950 px-4 py-3 md:hidden">
        <div className="flex items-center gap-3">
          <button
            type="button"
            aria-label={t("Open navigation")}
            aria-expanded={drawerOpen}
            onClick={() => setDrawerOpen((v) => !v)}
            className="rounded-md border border-white/10 bg-white/5 p-2 text-zinc-200 hover:bg-white/10"
          >
            <span aria-hidden className="block text-base leading-none">☰</span>
          </button>
          <div>
            <div className="text-sm font-bold tracking-tight text-zinc-100">pi-switch</div>
            <div className="text-[11px] text-zinc-500">{t("provider control · web")}</div>
          </div>
        </div>
        <div className="text-xs text-zinc-500">{t(NAV.find((n) => n.key === nav)?.label ?? "")}</div>
      </header>

      {/* Sidebar — drawer below lg, static sidebar at lg+ */}
      <aside
        className={cx(
          "flex w-64 shrink-0 flex-col border-r border-white/10 bg-zinc-950/95 backdrop-blur md:bg-zinc-950/60",
          "fixed inset-y-0 left-0 z-40 max-w-[82vw] md:static md:w-56 md:max-w-none md:translate-x-0",
          "transition-transform duration-200 ease-out",
          drawerOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="hidden px-4 py-4 md:block">
          <div className="text-lg font-bold tracking-tight text-zinc-100">pi-switch</div>
          <div className="text-[11px] text-zinc-500">{t("provider control · web")}</div>
        </div>
        <div className="flex items-center justify-between px-4 py-3 md:hidden">
          <div className="text-sm font-semibold text-zinc-100">pi-switch</div>
          <button
            type="button"
            aria-label={t("Close navigation")}
            onClick={() => setDrawerOpen(false)}
            className="rounded-md px-2 py-1 text-zinc-400 hover:bg-white/5 hover:text-zinc-200"
          >
            ✕
          </button>
        </div>
        <nav className="flex-1 overflow-y-auto px-2 py-2">
          {NAV.map((item) => (
            <button
              key={item.key}
              onClick={() => handleNav(item.key)}
              className={cx(
                "mb-0.5 flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm",
                nav === item.key
                  ? "bg-indigo-600/20 text-indigo-200"
                  : "text-zinc-400 hover:bg-white/5 hover:text-zinc-200",
              )}
            >
              <span className="text-base">{item.icon}</span>
              {t(item.label)}
            </button>
          ))}
        </nav>
        <div className="px-4 py-3 text-[11px] text-zinc-600">{t("CLI · TUI · WebUI — same core")}</div>
      </aside>

      {drawerOpen && (
        <button
          type="button"
          aria-label={t("Close navigation")}
          onClick={() => setDrawerOpen(false)}
          className="fixed inset-0 z-30 bg-black/50 backdrop-blur-[1px] md:hidden"
        />
      )}

      {/* Main */}
      <main className="min-w-0 flex-1 overflow-y-auto overflow-x-hidden">
        <div className="mx-auto max-w-5xl px-4 py-4 sm:px-6 sm:py-6">
          {error && (
            <div className="mb-4 rounded-lg border border-red-500/30 bg-red-950/40 px-4 py-3 text-sm text-red-200">
              <div className="font-medium">{t("Could not load config")}</div>
              <div className="mt-1 break-words text-red-300/80">{error}</div>
              <Button variant="primary" className="mt-3" onClick={() => void initConfig()}>
                {t("Initialize config")}
              </Button>
            </div>
          )}

          {!state && !error && <div className="text-zinc-500">{t("Loading…")}</div>}

          {state && (
            <>
              {nav === "home" && <HomePanel state={state} refresh={refresh} onNavigate={handleNav} />}
              {nav === "profiles" && <ProfilesPanel state={state} refresh={refresh} />}
              {nav === "proxy" && <ProxyPanel state={state} refresh={refresh} />}
              {nav === "packages" && <PackagesPanel refresh={refresh} />}
              {nav === "stats" && <StatsPanel state={state} refresh={refresh} />}
              {nav === "backups" && <BackupsPanel state={state} refresh={refresh} />}
              {nav === "settings" && <SettingsPanel state={state} refresh={refresh} />}
              {nav === "doctor" && <DoctorPanel state={state} refresh={refresh} />}
            </>
          )}
        </div>
      </main>
    </div>
  );
}
