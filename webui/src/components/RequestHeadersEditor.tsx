import { useState } from "react";
import { Button, Input } from "./ui";
import { useI18n } from "../i18n";

export function RequestHeadersEditor({
  headers,
  onHeadersChange,
}: {
  headers: Record<string, string>;
  onHeadersChange: (next: Record<string, string>) => void;
}) {
  const { t } = useI18n() as any;
  const entries = Object.entries(headers);

  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  function set(key: string, value: string) {
    const next = { ...headers };
    next[key] = value;
    onHeadersChange(next);
  }
  function remove(key: string) {
    const next = { ...headers };
    delete next[key];
    onHeadersChange(next);
  }
  function add() {
    const k = newKey.trim();
    if (!k) return;
    if (headers[k] !== undefined) return;
    onHeadersChange({ ...headers, [k]: newValue });
    setNewKey("");
    setNewValue("");
  }

  return (
    <div className="border-l border-white/10 pl-3">
      <div className="flex items-center justify-between gap-2">
        <div>
          <div className="text-sm font-medium text-zinc-200">{t("Request headers") || "请求头"}</div>
          <div className="text-xs text-zinc-500">
            {t("Optional HTTP headers sent with provider requests, e.g. HTTP-Referer or X-Title.") ||
              "随供应商请求发送的可选 HTTP 请求头，如 HTTP-Referer 或 X-Title。"}
          </div>
        </div>
        {/* header add is via inline form */}
        <div className="hidden sm:block text-xs text-zinc-500" />
      </div>

      {entries.length === 0 && (
        <div className="mt-2 text-sm text-zinc-500">{t("No custom headers") || "暂无自定义请求头"}</div>
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
                value={v}
                onChange={(e) => set(k, e.target.value)}
                placeholder="value"
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
          placeholder={t("Header name") || "Header 名称"}
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
          placeholder="Value"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
        />
        <Button type="button" onClick={add} className="whitespace-nowrap">
          + {t("Add header") || "添加请求头"}
        </Button>
      </div>
    </div>
  );
}
