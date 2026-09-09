import { useEffect, useState } from "react";
import type { BuildInfo } from "../apiSchema";
import type { AppState, Settings } from "../types";
import { api } from "../api";
import { Button, Card, Field, Input, SectionTitle, Select, useAction, useToast } from "./ui";
import { useI18n } from "../i18n";
export function SettingsPanel({
  state,
  refresh,
}: {
  state: AppState;
  refresh: () => Promise<void>;
}) {
  const run = useAction();
  const toast = useToast();
  const { t, lang, setLang } = useI18n();
  // Deep clone so edits don't mutate the shared state until saved. API defaults
  // are applied once by apiSchema.ts before the component receives this value.
  const [s, setS] = useState<Settings>(() => JSON.parse(JSON.stringify(state.settings)));
  const [buildInfo, setBuildInfo] = useState<BuildInfo | null>(null);
  useEffect(() => {
    void api.buildInfo().then(setBuildInfo).catch(() => setBuildInfo(null));
  }, []);

  const set = (patch: Partial<Settings>) => setS((prev) => ({ ...prev, ...patch }));
  const setProxy = (patch: Partial<Settings["proxy"]>) =>
    setS((prev) => ({ ...prev, proxy: { ...prev.proxy, ...patch } }));
  const setCb = (patch: Partial<Settings["proxy"]["circuitBreaker"]>) =>
    setS((prev) => ({
      ...prev,
      proxy: { ...prev.proxy, circuitBreaker: { ...prev.proxy.circuitBreaker, ...patch } },
    }));
  const setWeb = (patch: Partial<Settings["web"]>) =>
    setS((prev) => ({ ...prev, web: { ...prev.web, ...patch } }));

  async function save() {
    await api.updateSettings(s);
    toast("ok", "已保存到本地，需到网关发布");
    await refresh();
  }

  return (
    <div>
      <SectionTitle hint={t("written to ~/.pi-switch/config.json")}>{t("Settings")}</SectionTitle>

      <Card className="mb-4">
        <div className="mb-3 text-sm font-semibold text-zinc-200">{t("General")}</div>
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label={t("Write mode")}>
            <Select value={s.writeMode} onChange={(e) => set({ writeMode: e.target.value })}>
              <option value="merge">merge</option>
              <option value="exclusive">exclusive</option>
            </Select>
          </Field>
          <Field label={t("Language")}>
            <Select
              value={s.language ?? ""}
              onChange={(e) => {
                const v = e.target.value || null;
                set({ language: v });
                // Apply immediately (WebUI now has real i18n).
                if (v === "zh") setLang("zh");
                else if (v === "en") setLang("en");
                else setLang(navigator.language.startsWith("zh") ? "zh" : "en");
              }}
            >
              <option value="">{t("auto")}</option>
              <option value="en">en</option>
              <option value="zh">zh</option>
            </Select>
          </Field>
          <Field label={t("Conversation source")}>
            <Select
              aria-label={t("Conversation source")}
              value={s.conversationSource}
              onChange={(e) => set({ conversationSource: e.target.value as Settings["conversationSource"] })}
            >
              <option value="sessionScan">sessionScan</option>
              <option value="proxy">proxy</option>
              <option value="off">off</option>
            </Select>
          </Field>
          <Field label={t("Current UI language")}>
            <Input value={lang === "zh" ? "中文" : "English"} readOnly />
          </Field>
        </div>

        
      </Card>

      <Card className="mb-4">
        <div className="mb-3 text-sm font-semibold text-zinc-200">{t("Proxy")}</div>
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label={t("Proxy host")}>
            <Input value={s.proxy.host} onChange={(e) => setProxy({ host: e.target.value })} />
          </Field>
          <Field label={t("Proxy port")}>
            <Input
              type="number"
              value={s.proxy.port}
              onChange={(e) => setProxy({ port: parseInt(e.target.value, 10) || 0 })}
            />
          </Field>
          <Field label={t("Global User-Agent disguise")}>
            <Select
              value={s.proxy.userAgent ?? ""}
              onChange={(e) => setProxy({ userAgent: e.target.value || undefined })}
            >
              <option value="">{t("none")}</option>
              <option value="claude-code">claude-code</option>
              <option value="codex">codex</option>
              <option value="gemini">gemini</option>
            </Select>
          </Field>
        </div>

        <div className="mt-2 rounded-lg border border-white/10 p-3">
          <label className="flex items-center gap-2 text-sm text-zinc-300">
            <input
              type="checkbox"
              checked={s.proxy.circuitBreaker.enabled}
              onChange={(e) => setCb({ enabled: e.target.checked })}
            />
            {t("Circuit breaker enabled")}
          </label>
          <div className="mt-3 grid gap-x-4 sm:grid-cols-2">
            <Field label={t("Failure threshold")}>
              <Input
                type="number"
                value={s.proxy.circuitBreaker.failureThreshold}
                onChange={(e) =>
                  setCb({ failureThreshold: parseInt(e.target.value, 10) || 0 })
                }
              />
            </Field>
            <Field label={t("Cooldown (seconds)")}>
              <Input
                type="number"
                value={s.proxy.circuitBreaker.cooldownSeconds}
                onChange={(e) =>
                  setCb({ cooldownSeconds: parseInt(e.target.value, 10) || 0 })
                }
              />
            </Field>
          </div>
        </div>
      </Card>

      <Card className="mb-4">
        <div className="mb-3 text-sm font-semibold text-zinc-200">{t("Web UI")}</div>
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label={t("Proxy host")}>
            <Input value={s.web.host} onChange={(e) => setWeb({ host: e.target.value })} />
          </Field>
          <Field label={t("Proxy port")}>
            <Input
              type="number"
              value={s.web.port}
              onChange={(e) => setWeb({ port: parseInt(e.target.value, 10) || 0 })}
            />
          </Field>
        </div>
        <div className="text-xs text-zinc-500">
          {t("Non-loopback hosts require Basic auth (password in ~/.pi-switch/webui_password). Changes take effect on next webui start.")}
        </div>
      </Card>

      {buildInfo && (
        <Card className="mb-4">
          <div className="mb-2 text-sm font-semibold text-zinc-200">{t("Build identity") || "Build identity"}</div>
          <div className="grid gap-1 text-xs text-zinc-400 sm:grid-cols-2">
            <span>version: <code className="text-zinc-200">{buildInfo.version}</code></span>
            <span>commit: <code className="text-zinc-200">{buildInfo.commit}</code></span>
            <span>target: <code className="text-zinc-200">{buildInfo.target}</code></span>
            <span>built: <code className="text-zinc-200">{buildInfo.buildTime}</code> · dirty: <code className="text-zinc-200">{buildInfo.dirty}</code></span>
          </div>
        </Card>
      )}

      <div className="flex justify-end">
        <Button
          variant="primary"
          onClick={() => run(save, undefined)}
        >
          {t("Save settings")}
        </Button>
      </div>

    </div>
  );
}
