package piagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writePiManifest(t *testing.T, root, manifest string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPackagesImportsConfiguredPackageFromManagedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"packages":["npm:demo-pi"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	pkgRoot := filepath.Join(root, "npm", "node_modules", "demo-pi")
	writePiManifest(t, pkgRoot, `{"name":"demo-pi","version":"1.2.3","description":"demo","pi":{"extensions":["./extensions/index.ts"],"skills":["./skills"]}}`)

	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusImported || result.Count != 1 || result.Packages[0].ID != "npm:demo-pi" {
		t.Fatalf("unexpected result: %+v", result)
	}
	pkg := result.Packages[0]
	if !pkg.HasExtensions || !pkg.HasSkills || pkg.HasPrompts || pkg.HasThemes || pkg.Version != "1.2.3" {
		t.Fatalf("package metadata = %+v", pkg)
	}
}

func TestDiscoverPackagesRespectsAutoloadAndDeduplicatesIdentity(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	settings := filepath.Join(root, "settings.json")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"packages":["npm:demo-pi",{"source":"npm:demo-pi","autoload":false}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	pkgRoot := filepath.Join(root, "npm", "node_modules", "demo-pi")
	writePiManifest(t, pkgRoot, `{"name":"demo-pi","version":"1.2.3","pi":{"prompts":["./prompts"]}}`)

	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 || result.Packages[0].ID != "npm:demo-pi" || result.Packages[0].Enabled {
		t.Fatalf("dedupe/autoload result = %+v", result)
	}
}

func TestDiscoverPackagesUsesSettingsAsPackageRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"packages":["npm:configured"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	writePiManifest(t, filepath.Join(root, "npm", "node_modules", "configured"), `{"name":"configured","version":"1.0.0","pi":{"extensions":["./index.ts"]}}`)
	writePiManifest(t, filepath.Join(root, "npm", "node_modules", "transitive-dependency"), `{"name":"transitive-dependency","version":"9.9.9","pi":{"extensions":["./index.ts"]}}`)

	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 || result.Packages[0].ID != "npm:configured" {
		t.Fatalf("settings registry was not authoritative: %+v", result)
	}
}

func TestDiscoverPackagesDistinguishesMissingManifestAndMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	missing := filepath.Join(root, "npm", "node_modules", "missing-manifest")
	if err := os.MkdirAll(missing, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"packages":["npm:missing-manifest"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusEmpty || result.Count != 0 || result.Skipped == 0 {
		t.Fatalf("missing manifest result = %+v", result)
	}

	t.Setenv("PI_AGENT_ROOT", filepath.Join(t.TempDir(), "does-not-exist"))
	result, err = DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusNotFound || result.OK {
		t.Fatalf("missing root result = %+v", result)
	}
}

func TestDiscoverPackagesStoresNormalizedManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"packages":["git:github.com/user/demo"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	pkgRoot := filepath.Join(root, "git", "github.com", "user", "demo")
	writePiManifest(t, pkgRoot, `{"name":"demo","version":"0.1.0","pi":{"themes":["./themes"]}}`)

	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Packages) != 1 {
		t.Fatalf("packages = %+v", result.Packages)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal([]byte(result.Packages[0].Manifest), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["name"] != "demo" {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestDiscoverPackagesDoesNotGuessNodeModulesWithoutSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pi", "agent")
	t.Setenv("PI_AGENT_ROOT", root)
	for _, name := range []string{"active-looking", "transitive-a", "transitive-b"} {
		writePiManifest(t, filepath.Join(root, "npm", "node_modules", name), `{"name":"`+name+`","version":"1.0.0","pi":{"extensions":["./index.ts"]}}`)
	}

	result, err := DiscoverPackages()
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusNotFound || result.OK || result.Count != 0 {
		t.Fatalf("missing settings must not import guessed packages: %+v", result)
	}
}
