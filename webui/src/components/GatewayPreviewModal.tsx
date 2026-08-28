import { useMemo, useState } from "react";
import { Button, Modal } from "./ui";
import { diffGateway, validateGatewayJson } from "../lib/gatewayDiff";
import { useI18n } from "../i18n";

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
  const { t } = useI18n();
  const initial = useMemo(() => JSON.stringify(proposed, null, 2), [proposed]);
  const [text, setText] = useState(initial);
  const diff = useMemo(() => diffGateway(current as any, proposed as any), [current, proposed]);
  const validation = useMemo(() => validateGatewayJson(text), [text]);
  const conflictSet = new Set(conflicts);

  return (
    <Modal title={t("Gateway preview — confirm to apply")} onClose={onClose} wide>
      <div className="mb-2 text-xs text-zinc-500">
        {t("Left shows diff, right is editable JSON. Conflicts highlighted.")}
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {/* Left diff */}
        <div className="rounded-lg border border-white/10 bg-zinc-950/60 p-3">
          <div className="mb-2 text-sm font-semibold text-zinc-200">Current vs Proposed</div>
          <div className="space-y-2 text-xs">
            {diff.added.length > 0 && (
              <div>
                <div className="font-medium text-emerald-400">+ added</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.added.map((k) => (
                    <span key={k} className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-emerald-500/10 text-emerald-300"}`}>{k}</span>
                  ))}
                </div>
              </div>
            )}
            {diff.removed.length > 0 && (
              <div>
                <div className="font-medium text-red-400">- removed</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.removed.map((k) => (
                    <span key={k} className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-red-500/10 text-red-300"}`}>{k}</span>
                  ))}
                </div>
              </div>
            )}
            {diff.changed.length > 0 && (
              <div>
                <div className="font-medium text-zinc-300">~ changed</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {diff.changed.map((k) => (
                    <span key={k} className={`rounded px-1.5 py-0.5 ${conflictSet.has(k) ? "bg-amber-500/20 text-amber-300 border border-amber-500/30" : "bg-zinc-800 text-zinc-300"}`}>{k}</span>
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
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-black/30 p-2 font-mono text-[11px] text-zinc-400">{current ? JSON.stringify(current, null, 2) : "null (new gateway)"}</pre>
            </div>
            <div className="mt-2">
              <div className="font-medium text-zinc-400">proposed</div>
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-black/30 p-2 font-mono text-[11px] text-zinc-400">{JSON.stringify(proposed, null, 2)}</pre>
            </div>
          </div>
        </div>

        {/* Right editable */}
        <div className="rounded-lg border border-white/10 bg-zinc-950/60 p-3">
          <label htmlFor="gateway-json" className="mb-1 block text-xs font-medium text-zinc-400">proposed json</label>
          <textarea
            id="gateway-json"
            aria-label="proposed json"
            value={text}
            onChange={(e) => setText(e.target.value)}
            className="h-64 w-full rounded-md border border-white/10 bg-zinc-900 p-2 font-mono text-xs text-zinc-100 outline-none focus:border-indigo-500/70 sm:h-80"
          />
          {!validation.ok && (
            <div className="mt-2 rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">Invalid JSON: {validation.error}</div>
          )}
          {validation.ok && (
            <div className="mt-2 text-xs text-emerald-400">✓ JSON valid</div>
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
