import { useEffect, useState } from "react";
import type { AppState, DaemonResult } from "../types";
import { hasUpstreams, resolvedUpstreams } from "../types";
import { api } from "../api";
import { Badge, Button, Card, SectionTitle, cx } from "./ui";
import { useI18n } from "../i18n";

export function HomePanel({
  state,
  refresh,
  onNavigate,
}: {
  state: AppState;
  refresh: () => Promise<void>;
  onNavigate: (k: any) => void;
}) {
  const { t } = useI18n();
  const [proxy, setProxy] = useState<DaemonResult | null>(null);
  const profiles = Object.entries(state.profiles);
  const exposedCount = profiles.filter(
    ([, p]) => (p.exposedModels?.length ?? 0) > 0,
  ).length;

  const currentProfile = state.current ? state.profiles[state.current] : null;
  const currentUpstreams = currentProfile
    ? hasUpstreams(currentProfile)
      ? resolvedUpstreams(currentProfile)
      : [{ baseUrl: currentProfile.baseUrl }]
    : [];

  useEffect(() => {
    api.proxyStatus().then(setProxy).catch(() => setProxy(null));
  }, []);


  return (
    <div className="space-y-5">
      <SectionTitle hint={t("CLI / TUI / WebUI share one Rust core")}>
        {t("Overview")}
      </SectionTitle>

      {/* Primary Metrics Grid */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          label={t("Profiles")}
          value={String(profiles.length)}
          subtext={`${exposedCount} ${t("with exposed models")}`}
          onClick={() => onNavigate("profiles")}
        />
        <StatCard
          label={t("Exposed")}
          value={String(exposedCount)}
          subtext={t("ready for gateway")}
          tone="sky"
          onClick={() => onNavigate("gateway")}
        />
        <StatCard
          label={t("Current")}
          value={state.current || "—"}
          subtext={currentProfile?.api || t("No profile selected")}
          tone="amber"
          monoValue
          onClick={() => onNavigate("profiles")}
        />
        <StatCard
          label={t("Proxy")}
          value={proxy?.running ? t("running") : t("stopped")}
          subtext={proxy?.running ? `:${state.settings?.proxy?.port || 4200}` : t("daemon inactive")}
          tone={proxy?.running ? "green" : "zinc"}
          dot={proxy?.running}
          onClick={() => onNavigate("proxy")}
        />
      </div>

      {/* Active Profile Hero & Fast Switcher */}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card
          variant={state.current ? "active" : "subtle"}
          className="flex flex-col justify-between lg:col-span-2"
        >
          <div>
            <div className="flex items-center justify-between gap-2 border-b border-white/10 pb-3">
              <div className="flex items-center gap-2">
                <span className="text-base font-semibold tracking-tight text-zinc-100">
                  {t("Active Profile")}
                </span>
                {state.current && (
                  <Badge tone="amber" dot mono>
                    {state.current}
                  </Badge>
                )}
              </div>
              <div className="flex items-center gap-1.5">
                {currentProfile?.api && (
                  <Badge tone="indigo" mono>
                    {currentProfile.api}
                  </Badge>
                )}
              </div>
            </div>

            {state.current && currentProfile ? (
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div className="rounded-lg border border-white/5 bg-zinc-950/40 p-3">
                  <div className="text-[11px] font-medium uppercase tracking-wider text-zinc-400">
                    {t("Upstream Endpoint")}
                  </div>
                  <div className="mt-1 truncate font-mono text-xs text-zinc-200" title={currentUpstreams[0]?.baseUrl}>
                    {currentUpstreams[0]?.baseUrl || t("no base url")}
                  </div>
                  {currentUpstreams.length > 1 && (
                    <div className="mt-1 text-[11px] text-zinc-400">
                      +{currentUpstreams.length - 1} {t("additional channel(s)")}
                    </div>
                  )}
                </div>

                <div className="rounded-lg border border-white/5 bg-zinc-950/40 p-3">
                  <div className="text-[11px] font-medium uppercase tracking-wider text-zinc-400">
                    {t("Configured Models")}
                  </div>
                  <div className="mt-1 flex items-baseline gap-2">
                    <span className="font-mono text-lg font-semibold text-zinc-100">
                      {currentProfile.models?.length ?? 0}
                    </span>
                    <span className="text-xs text-zinc-400">
                      ({currentProfile.exposedModels?.length ?? 0} {t("exposed")})
                    </span>
                  </div>
                </div>
              </div>
            ) : (
              <div className="py-6 text-center text-sm text-zinc-400">
                {t("No profile selected yet.")} {t("Create a new profile to get started.")}
              </div>
            )}
          </div>

          <div className="mt-5 flex flex-wrap items-center justify-end gap-3 border-t border-white/10 pt-3">
            <div className="flex gap-2">
              <Button variant="primary" onClick={() => onNavigate("profiles")}>
                {t("Manage profiles")}
              </Button>
            </div>
          </div>
        </Card>

        {/* Proxy Pipeline Topology Visualizer */}
        <Card variant="glass" className="flex flex-col justify-between">
          <div>
            <div className="mb-3 text-sm font-semibold tracking-tight text-zinc-200">
              {t("Routing Pipeline")}
            </div>
            <div className="space-y-2">
              {/* Step 1 */}
              <div className="flex items-center gap-3 rounded-lg border border-white/5 bg-zinc-950/60 p-2.5">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-indigo-500/20 font-mono text-xs text-indigo-300">
                  PI
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-medium text-zinc-200">Client Agent</div>
                  <div className="truncate font-mono text-[11px] text-zinc-400">
                    prefix: {state.settings?.providerPrefix || "pi"}
                  </div>
                </div>
                <Badge tone="green" dot>
                  ready
                </Badge>
              </div>

              <div className="flex justify-center text-zinc-600">↓</div>

              {/* Step 2 */}
              <div className="flex items-center gap-3 rounded-lg border border-white/5 bg-zinc-950/60 p-2.5">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-amber-500/20 font-mono text-xs text-amber-300">
                  GW
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-medium text-zinc-200">Local Proxy Daemon</div>
                  <div className="truncate font-mono text-[11px] text-zinc-400">
                    127.0.0.1:{state.settings?.proxy?.port || 4200}
                  </div>
                </div>
                <Badge tone={proxy?.running ? "green" : "zinc"} dot={proxy?.running}>
                  {proxy?.running ? "online" : "stopped"}
                </Badge>
              </div>

              <div className="flex justify-center text-zinc-600">↓</div>

              {/* Step 3 */}
              <div className="flex items-center gap-3 rounded-lg border border-white/5 bg-zinc-950/60 p-2.5">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-emerald-500/20 font-mono text-xs text-emerald-300">
                  UP
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-medium text-zinc-200">Upstream Provider</div>
                  <div className="truncate font-mono text-[11px] text-zinc-400">
                    {state.current || t("no profile")}
                  </div>
                </div>
                <Badge tone={state.current ? "indigo" : "zinc"}>
                  {currentProfile?.api?.split("-")[0] || "auto"}
                </Badge>
              </div>
            </div>
          </div>

          <div className="mt-4 flex justify-end">
            <Button onClick={() => onNavigate("proxy")}>
              {t("Proxy control")}
            </Button>
          </div>
        </Card>
      </div>

      {/* Fast Workflow Guide */}
      <div className="grid gap-3 sm:grid-cols-2">
        <Card variant="subtle">
          <div className="mb-2 text-sm font-semibold text-zinc-200">{t("Gateway workflow")}</div>
          <ol className="ml-4 list-decimal space-y-1 text-xs text-zinc-400">
            <li>{t("Add profiles & set API keys")}</li>
            <li>{t("Expose models to pi (per profile)")}</li>
            <li>{t("Expose models to pi")}</li>
            <li>
              {t("Start the proxy — pi routes by profile/model")}{" "}
              <code className="rounded bg-white/10 px-1 py-0.5 font-mono text-[11px] text-amber-300">
                profile/model
              </code>
            </li>
          </ol>
        </Card>

        <Card variant="subtle">
          <div className="mb-2 text-sm font-semibold text-zinc-200">{t("System Diagnostics")}</div>
          <div className="text-xs text-zinc-400">
            {t("Validate active configs and verify upstream network connectivity.")}
          </div>
          <div className="mt-3 flex gap-2">
            <Button onClick={() => onNavigate("doctor")}>{t("Run Doctor Diagnostics")}</Button>
            <Button onClick={() => onNavigate("backups")}>{t("Config backups")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  subtext,
  tone = "zinc",
  dot = false,
  monoValue = false,
  onClick,
}: {
  label: string;
  value: string;
  subtext?: string;
  tone?: "zinc" | "green" | "amber" | "sky";
  dot?: boolean;
  monoValue?: boolean;
  onClick?: () => void;
}) {
  const tones: Record<string, string> = {
    zinc: "text-zinc-100",
    green: "text-emerald-400",
    amber: "text-amber-300",
    sky: "text-sky-300",
  };

  return (
    <Card
      className={cx(
        "cursor-pointer py-3 transition-all hover:-translate-y-0.5",
        onClick ? "hover:border-white/20" : "",
      )}
      onClick={onClick}
    >
      <div className="flex items-center justify-between">
        <div className="text-[11px] font-semibold uppercase tracking-wider text-zinc-400">{label}</div>
        {dot && <span className="h-2 w-2 rounded-full bg-emerald-400 animate-dot-pulse" />}
      </div>
      <div
        className={cx(
          "mt-1 truncate text-2xl font-bold tracking-tight",
          monoValue && "font-mono",
          tones[tone] || tones.zinc,
        )}
      >
        {value}
      </div>
      {subtext && <div className="mt-0.5 truncate text-[11px] text-zinc-400">{subtext}</div>}
    </Card>
  );
}

