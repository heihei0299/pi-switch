import { useState } from "react";
import type { AppState } from "../types";
import { useI18n } from "../i18n";
import { Button, SectionTitle } from "./ui";
import { SupplierList } from "./SupplierList";
import { SupplierEditor } from "./SupplierEditor";
import { ModelPoolEditor } from "./ModelPoolEditor";

export function ProfilesPanel({
  state,
  refresh,
}: {
  state: AppState;
  refresh: () => Promise<void>;
}) {
  const { t } = useI18n() as any;
  const [editing, setEditing] = useState<{ name: string | null } | null>(null);
  const [models, setModels] = useState<string | null>(null);
  const entries = Object.entries(state.profiles);

  return (
    <div>
      <SectionTitle hint={`${entries.length} ${t("profile(s)")}`}>{t("Profiles")}</SectionTitle>

      <div className="mb-3 flex gap-2">
        <Button variant="primary" onClick={() => setEditing({ name: null })}>
          {t("+ Add profile")}
        </Button>
      </div>

      <SupplierList
        state={state}
        refresh={refresh}
        onEdit={(name) => setEditing({ name })}
        onModels={setModels}
      />

      {editing && (
        <SupplierEditor
          state={state}
          original={editing.name}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null);
            await refresh();
          }}
        />
      )}

      {models && (
        <ModelPoolEditor
          name={models}
          profile={state.profiles[models]}
          onClose={() => setModels(null)}
          onSaved={async () => {
            setModels(null);
            await refresh();
          }}
        />
      )}
    </div>
  );
}
