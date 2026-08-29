import { useEffect, useMemo, useState } from "react";
import { Button, Modal } from "./ui";
import { diffGateway, validateGatewayJson } from "../lib/gatewayDiff";
import { useI18n } from "../i18n";
import { ModelCard } from "./ModelCard";
import { draftFromEntry, modelPreview, newModelDraft, type ModelDraft } from "../lib/piModel";
import type { ModelEntry } from "../types";

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

export function GatewayPreviewModal({
  current,
  proposed,
  conflicts,
  onConfirm,
  onClose,
}: {
  current: Record<string, unknown> | null;
  proposed: Record<string, unknown>;
  conflicts: string[];
  onConfirm: (edited: Record<string, unknown>) => void;
  onClose: () => void;
}) {
  const { t } = useI18n() as any;
  const initial = useMemo(() => JSON.stringify(proposed, null, 2), [proposed]);
  const [text, setText] = useState(initial);
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const diff = useMemo(() => diffGateway(current as any, proposed as any), [current, proposed]);
  const validation = useMemo(() => validateGatewayJson(text), [text]);
  const conflictSet = new Set(conflicts);

  // Structured drafts derived from text
  const [drafts, setDrafts] = useState<ModelDraft[]>(() => {
    const rec = asRecord(proposed);
    const ms = Array.isArray(rec.models) ? (rec.models as unknown[]) : [];
    return ms.map((m) => draftFromEntry(m as ModelEntry));
  });
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());

  // When text changes (raw edit), try to sync drafts
  useEffect(() => {
    if (mode !== "raw") return;
    const parsed = (() => {
      try {
        return JSON.parse(text);
      } catch {
        return null;
      }
    })();
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return;
    const rec = asRecord(parsed);
    if (!Array.isArray(rec.models)) return;
    // Rebuild drafts with stable keys where possible
    setDrafts((prev) => {
      const prevById = new Map(prev.map((d) => [d.id, d.key]));
      return (rec.models as unknown[]).map((m) => {
        const id = (asRecord(m).id as string) || "";
        const key = prevById.get(id);
        return draftFromEntry(m as ModelEntry, key);
      });
    });
  }, [text, mode]);

  // When drafts change via structured editing, sync text
  function syncDraftsToText(nextDrafts: ModelDraft[]) {
    setDrafts(nextDrafts);
    try {
      const parsed = JSON.parse(text);
      const rec = asRecord(parsed);
      const next = { ...rec, models: nextDrafts.map((d) => modelPreview(d)) };
      setText(JSON.stringify(next, null, 2));
    } catch {
      // if text is invalid JSON, rebuild from proposed base
      const next = { ...asRecord(proposed), models: nextDrafts.map((d) => modelPreview(d)) };
      setText(JSON.stringify(next, null, 2));
    }
  }

  function addModel() {
    const d = newModelDraft();
    syncDraftsToText([...drafts, d]);
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.add(d.key);
      return ns;
    });
  }

  return (
    <Modal title={t("Gateway preview — confirm to apply")} onClose={onClose} wide>
      <div className="mb-2 text-xs text-zinc-500">
        {t("Left shows diff, right is editable JSON. Conflicts highlighted.")}
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        {/* Left diff */}
        <div className="rounded-lg border border-white/10 bg-zinc-950/60 p-3">
          <div className="mb-2 text-sm font-semibold text-zinc-200">Current vs Proposed</div>
          <div className="space-y-2 text-xs">
            {diff.added.length > 0 && (
              <div>
                <div className="font-medium text-emerald-400">+ added</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.added.map((k) => (
                    <span
                      key={k}
                      className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-emerald-500/10 text-emerald-300"}`}
                    >
                      {k}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {diff.removed.length > 0 && (
              <div>
                <div className="font-medium text-red-400">- removed</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.removed.map((k) => (
                    <span
                      key={k}
                      className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-red-500/10 text-red-300"}`}
                    >
                      {k}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {diff.changed.length > 0 && (
              <div>
                <div className="font-medium text-zinc-300">~ changed</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.changed.map((k) => (
                    <span
                      key={k}
                      className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-zinc-800 text-zinc-300"}`}
                    >
                      {k}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {diff.added.length === 0 && diff.removed.length === 0 && diff.changed.length === 0 && (
              <div className="text-zinc-500">{t("No changes")}</div>
            )}
            {conflicts.length > 0 && (
              <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/10 p-2 text-amber-200">
                {t("Conflicts: hand-written fields will be overwritten")}: {conflicts.join(", ")}
              </div>
            )}
            <div className="mt-3">
              <div className="font-medium text-zinc-400">current</div>
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-black/30 p-2 font-mono text-[11px] text-zinc-400">
                {current ? JSON.stringify(current, null, 2) : "null (new gateway)"}
              </pre>
            </div>
            <div className="mt-2">
              <div className="font-medium text-zinc-400">proposed</div>
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-black/30 p-2 font-mono text-[11px] text-zinc-400">
                {JSON.stringify(proposed, null, 2)}
              </pre>
            </div>
          </div>
        </div>

        {/* Right editable — structured + raw */}
        <div className="rounded-lg border border-white/10 bg-zinc-950/60 p-3">
          <div className="mb-2 flex items-center justify-between">
            <div className="text-xs font-medium text-zinc-400">proposed json</div>
            <div className="flex gap-1 rounded-md border border-white/10 bg-zinc-900 p-0.5">
              <button
                type="button"
                onClick={() => setMode("structured")}
                className={`rounded px-2 py-1 text-xs ${mode === "structured" ? "bg-white/10 text-zinc-100" : "text-zinc-400"}`}
              >
                结构化
              </button>
              <button
                type="button"
                onClick={() => setMode("raw")}
                className={`rounded px-2 py-1 text-xs ${mode === "raw" ? "bg-white/10 text-zinc-100" : "text-zinc-400"}`}
              >
                JSON
              </button>
            </div>
          </div>

          {mode === "structured" ? (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs text-zinc-500">模型配置（可展开编辑）</span>
                <Button type="button" onClick={addModel} className="h-7 text-xs">
                  + 添加模型
                </Button>
              </div>
              <div className="max-h-64 space-y-2 overflow-y-auto rounded border border-white/5 bg-zinc-900/40 p-2">
                {drafts.length === 0 && <div className="p-2 text-center text-xs text-zinc-500">暂无模型</div>}
                {drafts.map((d) => (
                  <ModelCard
                    key={d.key}
                    draft={d}
                    exposed={true}
                    onToggleExposed={() => {}}
                    onChange={(next) => syncDraftsToText(drafts.map((x) => (x.key === d.key ? next : x)))}
                    onRemove={() => syncDraftsToText(drafts.filter((x) => x.key !== d.key))}
                    expanded={expandedKeys.has(d.key)}
                    onToggleExpanded={() =>
                      setExpandedKeys((s) => {
                        const ns = new Set(s);
                        ns.has(d.key) ? ns.delete(d.key) : ns.add(d.key);
                        return ns;
                      })
                    }
                  />
                ))}
              </div>
              <div className="rounded border border-white/10 bg-zinc-900 p-2">
                <pre className="max-h-28 overflow-auto font-mono text-[11px] text-zinc-400">{text}</pre>
              </div>
              {!validation.ok && (
                <div className="rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">
                  Invalid JSON: {validation.error}
                </div>
              )}
              {validation.ok && <div className="text-xs text-emerald-400">✓ JSON valid</div>}
            </div>
          ) : (
            <>
              <textarea
                id="gateway-json"
                aria-label="proposed json"
                value={text}
                onChange={(e) => setText(e.target.value)}
                className="h-64 w-full rounded-md border border-white/10 bg-zinc-900 p-2 font-mono text-xs text-zinc-100 outline-none focus:border-indigo-500/70 sm:h-80"
              />
              {!validation.ok && (
                <div className="mt-2 rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">
                  Invalid JSON: {validation.error}
                </div>
              )}
              {validation.ok && <div className="mt-2 text-xs text-emerald-400">✓ JSON valid</div>}
            </>
          )}
        </div>
      </div>

      <div className="mt-4 flex justify-end gap-2">
        <Button onClick={onClose}>{t("Cancel")}</Button>
        <Button
          variant="primary"
          disabled={!validation.ok}
          onClick={() => {
            if (validation.ok && validation.value) onConfirm(validation.value);
          }}
        >
          {t("Confirm")}
        </Button>
      </div>
    </Modal>
  );
}
