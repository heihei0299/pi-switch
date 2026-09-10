import { Card, SectionTitle } from "./ui";
import { useI18n } from "../i18n";

// Config backup / encrypted export / import / restore have no implementation, so
// the server answers 501 and this panel offers no control that could report
// success for work that never happens. The feature needs its own spec.
export function BackupsPanel() {
  const { t } = useI18n();

  return (
    <div>
      <SectionTitle hint={t("config backups & encrypted sync")}>{t("Backups")}</SectionTitle>

      <Card>
        <div className="text-sm font-semibold text-zinc-200">
          {t("Config backups are not implemented yet")}
        </div>
        <div className="mt-2 text-sm text-zinc-400">
          {t(
            "This build has no backup store and no encrypted export/import/restore. The related API endpoints answer 501 instead of pretending to succeed, so nothing here reports a result it did not produce.",
          )}
        </div>
        <div className="mt-2 text-sm text-zinc-500">
          {t("To safeguard your setup, copy {{path}} yourself, or read its path with pi-switch config show.").replace(
            "{{path}}",
            "~/.pi-switch/config.json",
          )}
        </div>
      </Card>
    </div>
  );
}
