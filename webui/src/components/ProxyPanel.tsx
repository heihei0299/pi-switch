import { useEffect, useState } from "react";
import type { AppState, DaemonResult } from "../types";
import { api } from "../api";
import { Badge, Button, Card, Field, Input, SectionTitle } from "./ui";
import { useAction } from "./ui";
import { useI18n } from "../i18n";

export function ProxyPanel({
  state,
  refresh: _refresh,
}: {
  state: AppState;
  refresh: () => Promise<void>;
}) {
  const run = useAction();
  const { t } = useI18n();
  const [status, setStatus] = useState<DaemonResult | null>(null);
  const [host, setHost] = useState(state.settings.proxy.host);
  const [port, setPort] = useState(String(state.settings.proxy.port));

  const loadStatus = async () => {
    try {
      setStatus(await api.proxyStatus());
    } catch {
      setStatus(null);
    }
  };
  useEffect(() => {
    void loadStatus();
  }, []);

  return (
    <div>
      <SectionTitle hint={t("routes by profile/model in the request body")}>{t("Proxy")}</SectionTitle>

      <Card className="mb-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-2">
            <Badge tone={status?.running ? "green" : "zinc"}>
              {status?.running ? t("running") : t("stopped")}
            </Badge>
            {status?.running && (
              <span className="break-all text-sm text-zinc-400">
                PID {status.pid} · http://{status.host}:{status.port}
              </span>
            )}
          </div>
          <Button onClick={() => void loadStatus()} className="self-start sm:self-auto">{t("Refresh")}</Button>
        </div>
        {status?.message && <div className="mt-2 text-xs text-zinc-500">{status.message}</div>}

        <div className="mt-4 grid gap-x-4 sm:grid-cols-2">
          <Field label={t("Proxy host")}>
            <Input value={host} onChange={(e) => setHost(e.target.value)} />
          </Field>
          <Field label={t("Proxy port")}>
            <Input value={port} onChange={(e) => setPort(e.target.value)} />
          </Field>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="primary"
            disabled={status?.running}
            onClick={() =>
              run(
                () => api.proxyStart(host.trim(), parseInt(port, 10) || undefined),
                t("Proxy started"),
                loadStatus,
              )
            }
          >
            {t("Start")}
          </Button>
          <Button
            variant="danger"
            disabled={!status?.running}
            onClick={() => run(() => api.proxyStop(), t("Proxy stopped"), loadStatus)}
          >
            {t("Stop")}
          </Button>
        </div>
      </Card>
    </div>
  );
}
