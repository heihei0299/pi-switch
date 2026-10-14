package sqlpackages

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	packageusecase "github.com/heihei0299/pi-switch/internal/usecase/packages"
	_ "modernc.org/sqlite"
)

const createPackagesTableSQL = `CREATE TABLE IF NOT EXISTS packages (
	id TEXT PRIMARY KEY,
	spec TEXT NOT NULL,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	version TEXT,
	description TEXT,
	homepage TEXT,
	has_extensions INTEGER NOT NULL DEFAULT 0,
	has_skills INTEGER NOT NULL DEFAULT 0,
	has_prompts INTEGER NOT NULL DEFAULT 0,
	has_themes INTEGER NOT NULL DEFAULT 0,
	installed INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	installed_at INTEGER,
	updated_at INTEGER,
	package_json TEXT,
	origin TEXT NOT NULL DEFAULT 'manual',
	source_path TEXT
)`

const packageColumns = "id, spec, type, name, version, description, homepage, origin, has_extensions, has_skills, has_prompts, has_themes, installed, enabled, installed_at"

type Repository struct {
	path string
}

func New(path string) *Repository {
	if path == "" {
		path = PiSwitchDBPath()
	}
	return &Repository{path: path}
}

func PiSwitchDBPath() string {
	if p := os.Getenv("PI_SWITCH_DB"); p != "" {
		return filepath.Join(filepath.Dir(p), "pi-switch.db")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch.db"
	}
	return filepath.Join(home, ".pi-switch", "pi-switch.db")
}

func (r *Repository) open() (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", r.path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(createPackagesTableSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	for column, typ := range map[string]string{"origin": "TEXT NOT NULL DEFAULT 'manual'", "source_path": "TEXT"} {
		if err := ensurePackageColumn(db, column, typ); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func ensurePackageColumn(db *sql.DB, name, typ string) error {
	rows, err := db.Query(`PRAGMA table_info(packages)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var column, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &column, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if column == name {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE packages ADD COLUMN ` + name + ` ` + typ)
	return err
}

func (r *Repository) ListInstalled() ([]packageusecase.Record, error) {
	if _, err := os.Stat(r.path); errors.Is(err, os.ErrNotExist) {
		return []packageusecase.Record{}, nil
	} else if err != nil {
		return nil, err
	}
	db, err := r.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT ` + packageColumns + ` FROM packages WHERE installed=1 ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []packageusecase.Record{}
	for rows.Next() {
		record, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetInstalled(id string) (packageusecase.Record, error) {
	db, err := r.open()
	if err != nil {
		return packageusecase.Record{}, err
	}
	defer db.Close()
	row := db.QueryRow(`SELECT `+packageColumns+` FROM packages WHERE id=?`, id)
	record, err := scanPackage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return packageusecase.Record{}, packageusecase.ErrPackageNotFound
	}
	return record, err
}

func (r *Repository) Add(record packageusecase.Record) error {
	db, err := r.open()
	if err != nil {
		return err
	}
	defer db.Close()
	enabled := boolInt(record.Enabled)
	now := time.Now().UnixMilli()
	_, err = db.Exec(`INSERT INTO packages(id, spec, type, name, installed, enabled, installed_at, updated_at, origin) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET spec=excluded.spec, type=excluded.type, name=excluded.name, installed=1, enabled=excluded.enabled, updated_at=excluded.updated_at, origin='manual'`, record.ID, record.Spec, record.Type, record.Name, 1, enabled, now, now, "manual")
	return err
}

func (r *Repository) DeleteInstalled(id string) error {
	db, err := r.open()
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.Exec(`UPDATE packages SET installed=0, updated_at=? WHERE id=? AND installed=1`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return packageusecase.ErrPackageNotFound
	}
	return nil
}

func (r *Repository) ToggleInstalled(id string) (bool, error) {
	db, err := r.open()
	if err != nil {
		return false, err
	}
	defer db.Close()
	var enabled int
	if err := db.QueryRow(`SELECT enabled FROM packages WHERE id=? AND installed=1`, id).Scan(&enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, packageusecase.ErrPackageNotFound
		}
		return false, err
	}
	newEnabled := 0
	if enabled == 0 {
		newEnabled = 1
	}
	result, err := db.Exec(`UPDATE packages SET enabled=?, updated_at=? WHERE id=? AND installed=1`, newEnabled, time.Now().UnixMilli(), id)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		return false, packageusecase.ErrPackageNotFound
	}
	return newEnabled == 1, nil
}

func (r *Repository) SyncImported(packages []packageusecase.ImportedPackage) error {
	db, err := r.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}

	now := time.Now().UnixMilli()
	seen := map[string]bool{}
	for _, pkg := range packages {
		seen[pkg.ID] = true
		if _, err := tx.Exec(`INSERT INTO packages(id, spec, type, name, version, description, homepage, has_extensions, has_skills, has_prompts, has_themes, installed, enabled, installed_at, updated_at, package_json, origin, source_path)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET spec=excluded.spec, type=excluded.type, name=excluded.name, version=excluded.version, description=excluded.description, homepage=excluded.homepage,
				has_extensions=excluded.has_extensions, has_skills=excluded.has_skills, has_prompts=excluded.has_prompts, has_themes=excluded.has_themes,
				installed=1, enabled=excluded.enabled, installed_at=excluded.installed_at, updated_at=excluded.updated_at, package_json=excluded.package_json, origin='pi', source_path=excluded.source_path`,
			pkg.ID, pkg.Spec, pkg.Type, pkg.Name, pkg.Version, pkg.Description, pkg.Homepage,
			boolInt(pkg.HasExtensions), boolInt(pkg.HasSkills), boolInt(pkg.HasPrompts), boolInt(pkg.HasThemes),
			1, boolInt(pkg.Enabled), now, now, pkg.Manifest, "pi", pkg.SourcePath); err != nil {
			return rollback(err)
		}
	}
	rows, err := tx.Query(`SELECT id FROM packages WHERE origin='pi'`)
	if err != nil {
		return rollback(err)
	}
	var stale []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		if !seen[id] {
			stale = append(stale, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	_ = rows.Close()
	for _, id := range stale {
		if _, err := tx.Exec(`UPDATE packages SET installed=0, updated_at=? WHERE id=?`, now, id); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func scanPackage(scanner interface{ Scan(...any) error }) (packageusecase.Record, error) {
	var id, spec, typ, name, version, description, homepage, origin sql.NullString
	var hasExt, hasSkills, hasPrompts, hasThemes, installed, enabled sql.NullInt64
	var installedAt sql.NullInt64
	if err := scanner.Scan(&id, &spec, &typ, &name, &version, &description, &homepage, &origin, &hasExt, &hasSkills, &hasPrompts, &hasThemes, &installed, &enabled, &installedAt); err != nil {
		return packageusecase.Record{}, err
	}
	var at *int64
	if installedAt.Valid {
		value := installedAt.Int64
		at = &value
	}
	return packageusecase.Record{
		ID: id.String, Spec: spec.String, Type: typ.String, Name: name.String, Version: version.String,
		Description: description.String, Homepage: homepage.String, Origin: origin.String,
		HasExtensions: hasExt.Int64 == 1, HasSkills: hasSkills.Int64 == 1, HasPrompts: hasPrompts.Int64 == 1,
		HasThemes: hasThemes.Int64 == 1, Installed: installed.Int64 == 1, Enabled: enabled.Int64 == 1, InstalledAt: at,
	}, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
