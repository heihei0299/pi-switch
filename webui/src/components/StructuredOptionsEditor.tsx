import { useState } from "react";
import { Button, Input } from "./ui";
import { useI18n } from "../i18n";

function parseValue(raw: string): unknown {
  const t = raw.trim();
  if (t === "true") return true;
  if (t === "false") return false;
  if (t === "null") return null;
  if (/^-?\d+(\.\d+)?$/.test(t)) {
    const n = Number(t);
    if (Number.isFinite(n)) return n;
  }
  try {
    return JSON.parse(t);
  } catch {
    return raw;
  }
}

function stringifyValue(v: unknown): string {
  if (typeof v === "string") return v;
  return JSON.stringify(v);
}

export function StructuredOptionsEditor({
  title,
  hint,
  emptyLabel,
  addLabel,
  options,
  onOptionsChange,
}: {
  title?: string;
  hint?: string;
  emptyLabel?: string;
  addLabel?: string;
  options: Record<string, unknown>;
  onOptionsChange: (next: Record<string, unknown>) => void;
}) {
  const { t } = useI18n() as any;
  const effectiveTitle = title ?? t("Compatibility") ?? "接口兼容性";
  const effectiveHint =
    hint ?? t("Adjust compatibility for endpoints or local services.") ?? "调整兼容端点或本地服务的请求行为。";
  const effectiveEmpty = emptyLabel ?? t("No compatibility options") ?? "暂无兼容性选项";
  const effectiveAdd = addLabel ?? t("Add") ?? "添加";

  const entries = Object.entries(options);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  function set(key: string, valueRaw: string) {
    const next = { ...options };
    next[key] = parseValue(valueRaw);
    onOptionsChange(next);
  }
  function remove(key: string) {
    const next = { ...options };
    delete next[key];
    onOptionsChange(next);
  }
  function add() {
    const k = newKey.trim();
    if (!k) return;
    if (options[k] !== undefined) return;
    onOptionsChange({ ...options, [k]: parseValue(newValue) });
    setNewKey("");
    setNewValue("");
  }

  return (
    <div className="border-l border-white/10 pl-3">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-sm font-medium text-zinc-200">{effectiveTitle}</div>
          <div className="text-xs text-zinc-500">{effectiveHint}</div>
        </div>
        <Button type="button" onClick={add} className="shrink-0">
          + {effectiveAdd}
        </Button>
      </div>

      {entries.length === 0 && (
        <div className="mt-2 text-sm text-zinc-500">{effectiveEmpty}</div>
      )}

      {entries.length > 0 && (
        <div className="mt-3 space-y-2">
          <div className="hidden sm:grid grid-cols-[1fr_1fr_auto] gap-2 px-1 text-xs text-zinc-500">
            <span>Key</span>
            <span>Value</span>
            <span className="w-8" />
          </div>
          {entries.map(([k, v]) => (
            <div key={k} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
              <Input value={k} disabled className="font-mono text-xs" />
              <Input
                defaultValue={stringifyValue(v)}
                onBlur={(e) => set(k, e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    (e.target as HTMLInputElement).blur();
                  }
                }}
                placeholder="false"
                className="font-mono text-xs"
              />
              <Button type="button" onClick={() => remove(k)} className="px-3" aria-label={`remove ${k}`}>
                ✕
              </Button>
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
        <Input
          value={newKey}
          onChange={(e) => setNewKey(e.target.value)}
          placeholder="supportsDeveloperRole"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <Input
          value={newValue}
          onChange={(e) => setNewValue(e.target.value)}
          placeholder="false"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <span className="hidden sm:block w-8" />
      </div>
    </div>
  );
}
