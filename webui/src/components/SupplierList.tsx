import { useEffect, useState } from "react";
import type { AppState } from "../types";
import { hasUpstreams, resolvedUpstreams } from "../types";
import { effectiveResponsesMode } from "../lib/responsesMode";
import { api } from "../api";
import { useI18n } from "../i18n";
import { Badge, Button, Card, useAction } from "./ui";
import { SupplierCreditsPanel } from "./SupplierCreditsPanel";
import { useProtocolCapabilities } from "../lib/protocolContext";

export function SupplierList({
  state,
  refresh,
  onEdit,
  onModels,
}: {
  state: AppState;
  refresh: () => Promise<void>;
  onEdit: (name: string | null) => void;
  onModels: (name: string) => void;
}) {
  const run = useAction();
  const { t } = useI18n() as any;
  const caps = useProtocolCapabilities();
  const entries = Object.entries(state.profiles).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div className="space-y-2">
      {entries.length === 0 && (
        <Card>
          <div className="text-sm text-zinc-500">
            {t("No profiles yet.")} {t('Add one with the "+ Add profile" button or import from cc-switch.')}
          </div>
        </Card>
      )}
      {entries.map(([name, p]) => {
        const isCurrent = state.current === name;
        const exposed = (p.upstreams ?? []).reduce((n, u) => n + (u.exposedModels?.length ?? 0), 0);
        const modelCount = (p.upstreams ?? []).reduce((n, u) => n + (u.models?.length ?? 0), 0);
        const upstreamsList = hasUpstreams(p) ? resolvedUpstreams(p) : [];
        const mainUrl = (upstreamsList[0]?.baseUrl || p.baseUrl) || t("no base url");
        return (
          <Card
            key={name}
            variant={isCurrent ? "active" : "default"}
            className="flex flex-col gap-3 transition-all"
          >
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="truncate font-mono text-[14px] font-semibold text-zinc-100">{name}</span>
                  {isCurrent && (
                    <Badge tone="amber" dot mono>
                      {t("current")}
                    </Badge>
                  )}
                  {p.proxy && (
                    <Badge tone="indigo" mono>
                      {t("proxy")}
                    </Badge>
                  )}
                  <Badge mono>{p.api || "?"}</Badge>
                  <Badge tone="amber" mono>
                    {t("Responses")}: {effectiveResponsesMode(p, caps)}
                  </Badge>
                  {exposed > 0 && (
                    <Badge tone="green" dot mono>
                      {exposed} {t("exposed")}
                    </Badge>
                  )}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-2 font-mono text-xs text-zinc-400">
                  <span className="truncate max-w-md text-zinc-300" title={mainUrl}>
                    {mainUrl}
                  </span>
                  <span className="text-zinc-600">·</span>
                  <span>{modelCount} {t("models")}</span>
                  {hasUpstreams(p) && (
                    <>
                      <span className="text-zinc-600">·</span>
                      <span className="text-indigo-400">{upstreamsList.length} upstream(s)</span>
                    </>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-1.5 lg:shrink-0 lg:justify-end">
                <Button onClick={() => onModels(name)}>{t("Models")}</Button>
                <Button onClick={() => onEdit(name)}>{t("Edit")}</Button>
                <ProfileCardMenu
                  name={name}
                  onTest={() =>
                    run(
                      async () => {
                        const r = await api.testProfile(name);
                        if (!r.success) throw new Error(r.message);
                        return r;
                      },
                      t("Test OK"),
                    )
                  }
                  onCopy={() => {
                    const to = prompt(
                      `${t("Duplicate profile '{{name}}' as:").replace("{{name}}", name)}`,
                      `${name}-copy`,
                    );
                    if (to) run(() => api.duplicateProfile(name, to), t("Duplicated"), refresh);
                  }}
                  onDelete={() => {
                    if (confirm(t("Delete profile '{{name}}'?").replace("{{name}}", name)))
                      run(() => api.deleteProfile(name), t("Deleted"), refresh);
                  }}
                />
              </div>
            </div>
            <SupplierCreditsPanel name={name} profile={p} />
          </Card>
        );
      })}
    </div>
  );
}

function ProfileCardMenu({
  name,
  onTest,
  onCopy,
  onDelete,
}: {
  name: string;
  onTest: () => void;
  onCopy: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const { t } = useI18n() as any;
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);
  return (
    <div className="relative">
      <Button
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t("Actions for {{name}}").replace("{{name}}", name)}
        onClick={() => setOpen((v) => !v)}
        className="px-2"
      >
        …
      </Button>
      {open && (
        <>
          <button className="fixed inset-0 z-[55]" aria-label={t("Close menu")} onClick={() => setOpen(false)} />
          <div role="menu" className="absolute right-0 z-[55] mt-1 w-36 rounded-lg border border-line bg-zinc-900 p-1 shadow-xl">
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-white/5" onClick={() => { setOpen(false); onTest(); }}>{t("Test")}</button>
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-white/5" onClick={() => { setOpen(false); onCopy(); }}>{t("Copy")}</button>
            <div className="my-1 h-px bg-white/10" />
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-red-300 hover:bg-red-500/10" onClick={() => { setOpen(false); onDelete(); }}>{t("Delete")}</button>
          </div>
        </>
      )}
    </div>
  );
}
