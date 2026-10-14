package sqlpackages

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestListInstalledMigratesLegacyPackageRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pi-switch.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE packages (
		id TEXT PRIMARY KEY, spec TEXT NOT NULL, type TEXT NOT NULL, name TEXT NOT NULL,
		version TEXT, description TEXT, homepage TEXT,
		has_extensions INTEGER NOT NULL DEFAULT 0, has_skills INTEGER NOT NULL DEFAULT 0,
		has_prompts INTEGER NOT NULL DEFAULT 0, has_themes INTEGER NOT NULL DEFAULT 0,
		installed INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 1,
		installed_at INTEGER, updated_at INTEGER, package_json TEXT
	)`)
	if err == nil {
		_, err = db.Exec(`INSERT INTO packages(id,spec,type,name,installed,enabled,installed_at) VALUES('npm:old','npm:old','npm','old',1,1,1700000000123)`)
	}
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	packages, err := New(path).ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].ID != "npm:old" || packages[0].Origin != "manual" || packages[0].InstalledAt == nil || *packages[0].InstalledAt != 1700000000123 {
		t.Fatalf("legacy package row after migration = %+v", packages)
	}
}
