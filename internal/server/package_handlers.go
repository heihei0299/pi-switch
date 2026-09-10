package server

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/piagent"
)

func piSwitchDBPath() string {
	if p := os.Getenv("PI_SWITCH_DB"); p != "" {
		return filepath.Join(filepath.Dir(p), "pi-switch.db")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch.db"
	}
	return filepath.Join(home, ".pi-switch", "pi-switch.db")
}

func openPiSwitchDB() (*sql.DB, error) {
	path := piSwitchDBPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS packages (
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
	  )`); err != nil {
		_ = db.Close()
		return nil, err
	}
	for column, typ := range map[string]string{"origin": "TEXT NOT NULL DEFAULT 'manual'", "source_path": "TEXT"} {
		if err := ensurePiPackageColumn(db, column, typ); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func ensurePiPackageColumn(db *sql.DB, name, typ string) error {
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

func piAgentSettingsPath() string {
	if p := os.Getenv("PI_AGENT_SETTINGS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-agent-settings.json"
	}
	return filepath.Join(home, ".pi", "agent", "settings.json")
}

func parsePackageSpec(spec string) (typ, name string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "unknown", spec
	}
	if idx := strings.Index(spec, ":"); idx >= 0 {
		typ = spec[:idx]
		name = spec[idx+1:]
		if typ == "" {
			typ = "npm"
		}
		if name == "" {
			name = spec
		}
		return typ, name
	}
	if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		typ = "local"
		name = filepath.Base(spec)
		if name == "." || name == "" {
			name = spec
		}
		return typ, name
	}
	return "npm", spec
}

func ListInstalledPackages() ([]map[string]interface{}, error) {
	db, err := openPiSwitchDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, spec, type, name, version, description, homepage, origin, has_extensions, has_skills, has_prompts, has_themes, installed, enabled, installed_at FROM packages WHERE installed=1 ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, spec, typ, name, description, homepage, origin sql.NullString
		var version sql.NullString
		var hasExt, hasSkills, hasPrompts, hasThemes, installed, enabled sql.NullInt64
		var installedAt sql.NullInt64
		if err := rows.Scan(&id, &spec, &typ, &name, &version, &description, &homepage, &origin, &hasExt, &hasSkills, &hasPrompts, &hasThemes, &installed, &enabled, &installedAt); err != nil {
			return nil, err
		}
		m := map[string]interface{}{
			"id":            id.String,
			"spec":          spec.String,
			"type":          typ.String,
			"name":          name.String,
			"version":       version.String,
			"description":   description.String,
			"homepage":      homepage.String,
			"origin":        origin.String,
			"hasExtensions": hasExt.Int64 == 1,
			"hasSkills":     hasSkills.Int64 == 1,
			"hasPrompts":    hasPrompts.Int64 == 1,
			"hasThemes":     hasThemes.Int64 == 1,
			"installed":     installed.Int64 == 1,
			"enabled":       enabled.Int64 == 1,
		}
		if installedAt.Valid && installedAt.Int64 != 0 {
			var t time.Time
			if installedAt.Int64 > 1e12 {
				sec := installedAt.Int64 / 1000
				nsec := (installedAt.Int64 % 1000) * int64(time.Millisecond)
				t = time.Unix(sec, nsec)
			} else {
				t = time.Unix(installedAt.Int64, 0)
			}
			m["installedAt"] = t.Format(time.RFC3339Nano)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func handlePackagesList(c *gin.Context) {
	out, err := ListInstalledPackages()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"packages": out})
}
func handlePackageAdd(c *gin.Context) {
	var body struct {
		Spec    string `json:"spec"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Spec) == "" {
		c.JSON(400, gin.H{"error": "spec required"})
		return
	}
	spec := strings.TrimSpace(body.Spec)
	typ, name := parsePackageSpec(spec)
	db, err := openPiSwitchDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer db.Close()
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	now := time.Now().UnixMilli()
	_, err = db.Exec(`INSERT INTO packages(id, spec, type, name, installed, enabled, installed_at, updated_at, origin) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET spec=excluded.spec, type=excluded.type, name=excluded.name, installed=1, enabled=excluded.enabled, updated_at=excluded.updated_at, origin='manual'`, spec, spec, typ, name, 1, enabled, now, now, "manual")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "id": spec})
}
func ImportPiPackages() (piagent.ImportResult, error) {
	result, err := piagent.DiscoverPackages()
	if err != nil {
		return piagent.ImportResult{}, err
	}
	if result.Status == piagent.StatusNotFound {
		return result, nil
	}
	db, err := openPiSwitchDB()
	if err != nil {
		return piagent.ImportResult{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return piagent.ImportResult{}, err
	}
	rollback := func(err error) (piagent.ImportResult, error) {
		_ = tx.Rollback()
		return piagent.ImportResult{}, err
	}

	now := time.Now().UnixMilli()
	seen := map[string]bool{}
	for _, pkg := range result.Packages {
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
		return piagent.ImportResult{}, err
	}
	return result, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func handlePackageImport(c *gin.Context) {
	result, err := ImportPiPackages()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if result.Status == piagent.StatusNotFound {
		c.JSON(404, gin.H{"error": result.Message, "ok": result.OK, "count": result.Count, "status": result.Status, "message": result.Message, "warnings": result.Warnings})
		return
	}
	c.JSON(200, result)
}
func handlePackageGet(c *gin.Context) {
	id := c.Param("id")
	db, err := openPiSwitchDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer db.Close()
	var dbId, spec, typ, name, version, description, homepage, origin sql.NullString
	var hasExt, hasSkills, hasPrompts, hasThemes, installed, enabled sql.NullInt64
	var installedAt sql.NullInt64
	err = db.QueryRow(`SELECT id, spec, type, name, version, description, homepage, origin, has_extensions, has_skills, has_prompts, has_themes, installed, enabled, installed_at FROM packages WHERE id=?`, id).Scan(&dbId, &spec, &typ, &name, &version, &description, &homepage, &origin, &hasExt, &hasSkills, &hasPrompts, &hasThemes, &installed, &enabled, &installedAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	m := map[string]interface{}{"id": dbId.String, "spec": spec.String, "type": typ.String, "name": name.String, "version": version.String, "description": description.String, "homepage": homepage.String, "origin": origin.String, "hasExtensions": hasExt.Int64 == 1, "hasSkills": hasSkills.Int64 == 1, "hasPrompts": hasPrompts.Int64 == 1, "hasThemes": hasThemes.Int64 == 1, "installed": installed.Int64 == 1, "enabled": enabled.Int64 == 1}
	if installedAt.Valid && installedAt.Int64 != 0 {
		var t time.Time
		if installedAt.Int64 > 1e12 {
			sec := installedAt.Int64 / 1000
			nsec := (installedAt.Int64 % 1000) * int64(time.Millisecond)
			t = time.Unix(sec, nsec)
		} else {
			t = time.Unix(installedAt.Int64, 0)
		}
		m["installedAt"] = t.Format(time.RFC3339)
	}
	c.JSON(200, m)
}
func handlePackageDelete(c *gin.Context) {
	id := strings.TrimPrefix(c.Param("id"), "/")
	decoded, err := url.PathUnescape(id)
	if err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid package id: %v", err)})
		return
	}
	id = decoded
	if id == "" {
		c.JSON(400, gin.H{"error": "package id is required"})
		return
	}

	db, err := openPiSwitchDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer db.Close()
	result, err := db.Exec(`UPDATE packages SET installed=0, updated_at=? WHERE id=? AND installed=1`, time.Now().UnixMilli(), id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(404, gin.H{"error": "package not found"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func handlePackageToggle(c *gin.Context) {
	id := c.Param("id")
	db, err := openPiSwitchDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer db.Close()
	var enabled int
	err = db.QueryRow(`SELECT enabled FROM packages WHERE id=?`, id).Scan(&enabled)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	newEnabled := 0
	if enabled == 0 {
		newEnabled = 1
	}
	_, _ = db.Exec(`UPDATE packages SET enabled=?, updated_at=? WHERE id=?`, newEnabled, time.Now().UnixMilli(), id)
	c.JSON(200, gin.H{"ok": true, "enabled": newEnabled == 1})
}
