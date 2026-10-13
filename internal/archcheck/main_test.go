package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckArchitectureDependencies(t *testing.T) {
	const module = "github.com/heihei0299/pi-switch"
	stdlib := map[string]bool{"context": true, "fmt": true, "net/http": true, "os": true}

	tests := []struct {
		name          string
		files         map[string]string
		wantViolation string
	}{
		{
			name: "allowed dependency directions",
			files: map[string]string{
				"internal/domain/provider/provider.go":            "package provider\nimport \"fmt\"\n",
				"internal/usecase/providers/service.go":           "package providers\nimport (\"context\"; \"github.com/heihei0299/pi-switch/internal/domain/provider\")\n",
				"internal/adapters/inbound/httpmgmt/handler.go":   "package httpmgmt\nimport (\"net/http\"; \"github.com/gin-gonic/gin\"; \"github.com/heihei0299/pi-switch/internal/usecase/providers\")\n",
				"internal/adapters/outbound/fileconfig/config.go": "package fileconfig\nimport (\"os\"; \"github.com/heihei0299/pi-switch/internal/usecase/providers\")\n",
				"internal/bootstrap/wire.go":                      "package bootstrap\nimport \"github.com/heihei0299/pi-switch/internal/adapters/inbound/httpmgmt\"\n",
			},
		},
		{
			name: "domain rejects third-party imports",
			files: map[string]string{
				"internal/domain/provider/provider.go": "package provider\nimport \"github.com/gin-gonic/gin\"\n",
			},
			wantViolation: "github.com/gin-gonic/gin",
		},
		{
			name: "usecase rejects third-party imports",
			files: map[string]string{
				"internal/usecase/providers/service.go": "package providers\nimport \"github.com/gin-gonic/gin\"\n",
			},
			wantViolation: "github.com/gin-gonic/gin",
		},
		{
			name: "usecase rejects adapters",
			files: map[string]string{
				"internal/usecase/providers/service.go": "package providers\nimport \"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig\"\n",
			},
			wantViolation: module + "/internal/adapters/outbound/fileconfig",
		},
		{
			name: "inbound rejects outbound",
			files: map[string]string{
				"internal/adapters/inbound/httpmgmt/handler.go": "package httpmgmt\nimport \"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig\"\n",
			},
			wantViolation: module + "/internal/adapters/outbound/fileconfig",
		},
		{
			name: "outbound rejects inbound",
			files: map[string]string{
				"internal/adapters/outbound/fileconfig/config.go": "package fileconfig\nimport \"github.com/heihei0299/pi-switch/internal/adapters/inbound/httpmgmt\"\n",
			},
			wantViolation: module + "/internal/adapters/inbound/httpmgmt",
		},
		{
			name: "adapters stay under inbound or outbound",
			files: map[string]string{
				"internal/adapters/adapter.go": "package adapters\n",
			},
			wantViolation: "<package>",
		},
		{
			name: "root bootstrap rejects desktop module",
			files: map[string]string{
				"internal/bootstrap/wire.go": "package bootstrap\nimport \"github.com/heihei0299/pi-switch/desktop\"\n",
			},
			wantViolation: module + "/desktop",
		},
		{
			name: "root command rejects desktop module",
			files: map[string]string{
				"cmd/pi-switch/main.go": "package main\nimport \"github.com/heihei0299/pi-switch/desktop\"\n",
			},
			wantViolation: module + "/desktop",
		},
		{
			name: "root web package rejects desktop module",
			files: map[string]string{
				"webui/embed.go": "package webui\nimport \"github.com/heihei0299/pi-switch/desktop\"\n",
			},
			wantViolation: module + "/desktop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, source := range tt.files {
				fullPath := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}

			violations, err := check(root, module, stdlib)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantViolation == "" {
				if len(violations) != 0 {
					t.Fatalf("clean dependency graph reported violations: %+v", violations)
				}
				return
			}
			for _, violation := range violations {
				if violation.ImportPath == tt.wantViolation {
					return
				}
			}
			t.Fatalf("missing violation for %s; got %+v", tt.wantViolation, violations)
		})
	}
}

func TestCheckArchitectureDependenciesAcceptsLegacyTree(t *testing.T) {
	violations, err := check("../..", "github.com/heihei0299/pi-switch", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("legacy tree reported violations: %+v", violations)
	}
}
