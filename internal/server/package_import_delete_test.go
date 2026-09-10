package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageDeleteSupportsScopedImportedID(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"profiles":{},"settings":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", configPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))

	root := filepath.Join(dir, "pi", "agent")
	pkgRoot := filepath.Join(root, "npm", "node_modules", "@scope", "demo-pi")
	if err := os.MkdirAll(pkgRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"packages":["npm:@scope/demo-pi"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(`{"name":"@scope/demo-pi","version":"1.0.0","pi":{"extensions":["./index.ts"]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_AGENT_ROOT", root)

	r := NewMgmtRouter()
	importResponse := httptest.NewRecorder()
	importRequest := httptest.NewRequest(http.MethodPost, "/api/packages/import", strings.NewReader(`{}`))
	importRequest.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(importResponse, importRequest)
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", importResponse.Code, importResponse.Body.String())
	}

	deleteResponse := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/packages/npm%3A%40scope%2Fdemo-pi", nil)
	r.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("scoped delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}

	listResponse := httptest.NewRecorder()
	r.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/packages", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Packages []map[string]interface{} `json:"packages"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range listed.Packages {
		if pkg["id"] == "npm:@scope/demo-pi" {
			t.Fatalf("scoped package remained installed after delete: %s", listResponse.Body.String())
		}
	}
}
