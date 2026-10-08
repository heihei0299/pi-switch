package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/server"
)

func TestNativePackagesPageUsesSharedPackageServices(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	agentRoot := filepath.Join(dir, "pi", "agent")
	packageRoot := filepath.Join(agentRoot, "npm", "node_modules", "demo-pi")
	if err := os.MkdirAll(packageRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentRoot, "settings.json"), []byte(`{"packages":["npm:demo-pi"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(`{"name":"demo-pi","version":"1.2.3","description":"test package","pi":{"extensions":["./index.ts"],"skills":["./skills"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_AGENT_ROOT", agentRoot)

	preview := newNativeComplexPreview()
	preview.runPackageTask = func(task func() (string, error)) {
		message, err := task()
		if err != nil {
			preview.packagesError = err.Error()
			preview.packagesLoading = false
			preview.packagesLoaded = true
			return
		}
		packages, err := loadNativePackages()
		if err != nil {
			preview.packagesError = err.Error()
			preview.packagesLoading = false
			preview.packagesLoaded = true
			return
		}
		preview.packages = packages
		preview.packagesError = ""
		preview.packagesMessage = message
		preview.packagesLoading = false
		preview.packagesLoaded = true
	}
	tester := ui.NewTester(preview.view, 900, 1000)
	if _, err := os.Stat(server.PiSwitchDBPath()); !os.IsNotExist(err) {
		t.Fatalf("opening the native prototype created package DB: %v", err)
	}
	if err := tester.Click("Packages"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("No packages installed.") {
		t.Fatalf("initial package state = %q", tester.Texts())
	}
	if _, err := os.Stat(server.PiSwitchDBPath()); !os.IsNotExist(err) {
		t.Fatalf("listing packages created package DB: %v", err)
	}

	if err := tester.Click("Import from Pi Agent"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Imported 1 Pi package(s)") || !tester.HasText("Capabilities: extensions, skills") {
		t.Fatalf("Pi Agent import missing from native package list: %q", tester.Texts())
	}
	if err := tester.Click("Disable demo-pi"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Enable demo-pi") {
		t.Fatalf("package toggle did not update UI: %q", tester.Texts())
	}
	imported, err := server.GetInstalledPackage("npm:demo-pi")
	if err != nil || imported["enabled"] != false {
		t.Fatalf("shared package toggle state = %#v, %v", imported, err)
	}

	if err := tester.Click("Package spec"); err != nil {
		t.Fatal(err)
	}
	tester.Type("npm:manual")
	if err := tester.Click("Install package"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Uninstall manual") {
		t.Fatalf("manual package missing from list: %q", tester.Texts())
	}
	if err := tester.Click("Uninstall manual"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Uninstall package?") {
		t.Fatalf("uninstall confirmation missing: %q", tester.Texts())
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.GetInstalledPackage("npm:manual"); err != nil {
		t.Fatalf("canceling uninstall removed package: %v", err)
	}
	if err := tester.Click("Uninstall manual"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Confirm uninstall"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Uninstalled package manual") {
		t.Fatalf("uninstall result missing: %q", tester.Texts())
	}
	if _, err := server.GetInstalledPackage("npm:manual"); err != nil {
		t.Fatalf("uninstall removed package metadata instead of soft-uninstalling it: %v", err)
	}
	remaining, err := loadNativePackages()
	if err != nil || len(remaining) != 1 || remaining[0].id != "npm:demo-pi" {
		t.Fatalf("remaining installed packages = %+v, %v", remaining, err)
	}
	if strings.Contains(strings.Join(tester.Texts(), " "), "package_json") {
		t.Fatal("native package list exposed raw package manifest")
	}
}

func TestNativePackageDisplayNameOmitsGitURLCredentials(t *testing.T) {
	pkg := nativePackage{
		typeName: "git",
		name:     "https://user:secret@git.example.com/team/repo.git?token=secret",
	}
	if got := nativePackageDisplayName(pkg); got != "repo" || strings.Contains(got, "secret") || strings.Contains(got, "user") {
		t.Fatalf("display name = %q", got)
	}
}
