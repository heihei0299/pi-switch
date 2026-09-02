// TestRustArtifactsRemoved is a migration guard for ADR 0007 (Go-only). It
// asserts that all Rust toolchain artifacts are absent per spec F1/F2/F4.
// Keep until v202610 and remove after the Go-only release is proven stable.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustArtifactsRemoved(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// F1: Rust 源码与构建配置必须不存在
	for _, p := range []string{"src-rust", "Cargo.toml", "Cargo.lock", "build.rs"} {
		full := filepath.Join(repoRoot, p)
		if _, err := os.Stat(full); err == nil {
			t.Errorf("F1: Rust 产物应已移除但仍存在: %s", p)
		}
	}

	// F2: napi 产物必须不存在
	for _, p := range []string{"pi-switch-native.cjs", "index.js", "index.d.ts", "pi-switch-native.linux-x64-gnu.node"} {
		full := filepath.Join(repoRoot, p)
		if _, err := os.Stat(full); err == nil {
			t.Errorf("F2: napi 产物应已移除但仍存在: %s", p)
		}
	}
	// 额外：根目录不应存在任何 .node
	if matches, _ := filepath.Glob(filepath.Join(repoRoot, "*.node")); len(matches) > 0 {
		t.Errorf("F2: 根目录不应存在 .node 文件: %v", matches)
	}
	if matches, _ := filepath.Glob(filepath.Join(repoRoot, "pi-switch-native.*.node")); len(matches) > 0 {
		t.Errorf("F2: 不应存在 pi-switch-native.*.node: %v", matches)
	}

	// package.json 不应引用 Rust/napi
	pkgPath := filepath.Join(repoRoot, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		content := string(data)
		// files 不应包含 index.js / index.d.ts / .node
		if strings.Contains(content, `"index.js"`) {
			t.Errorf("F2: package.json files 不应包含 index.js")
		}
		if strings.Contains(content, `"index.d.ts"`) {
			t.Errorf("F2: package.json files 不应包含 index.d.ts")
		}
		if strings.Contains(content, ".node") {
			t.Errorf("F2: package.json 不应包含 .node")
		}
		if strings.Contains(content, "napi") {
			t.Errorf("F2: package.json 不应包含 napi")
		}
		if strings.Contains(content, `"main"`) && strings.Contains(content, "index.js") {
			t.Errorf("F2: package.json main 不应指向 index.js")
		}
	}

	// F4: CI 不应包含 Rust
	ciPath := filepath.Join(repoRoot, ".github/workflows/ci.yml")
	if data, err := os.ReadFile(ciPath); err == nil {
		content := string(data)
		for _, kw := range []string{"cargo", "rust-toolchain", "Install Rust", "rustfmt", "clippy"} {
			if strings.Contains(strings.ToLower(content), strings.ToLower(kw)) {
				t.Errorf("F4: CI 不应包含 Rust 关键字 %q", kw)
			}
		}
		if strings.Contains(content, ".node") {
			t.Errorf("F4: CI 不应包含 .node 产物")
		}
	}

	// BUILD_MUSL.md 若存在则不应包含 Rust/napi
	muslPath := filepath.Join(repoRoot, "BUILD_MUSL.md")
	if data, err := os.ReadFile(muslPath); err == nil {
		content := string(data)
		if strings.Contains(strings.ToLower(content), "rust") || strings.Contains(strings.ToLower(content), "napi") || strings.Contains(content, ".node") {
			t.Errorf("F4: BUILD_MUSL.md 不应包含 Rust/napi/.node，若无 Go 意义应删除")
		}
	}

	// README/WEBUI_GUIDE 不应包含 src-rust 引用
	for _, p := range []string{"README.md", "WEBUI_GUIDE.md"} {
		full := filepath.Join(repoRoot, p)
		if data, err := os.ReadFile(full); err == nil {
			content := string(data)
			if strings.Contains(content, "src-rust") {
				t.Errorf("F4: %s 不应包含 src-rust", p)
			}
			// WEBUI_GUIDE 中的 Rust core 描述应已改为 Go
			if p == "WEBUI_GUIDE.md" && strings.Contains(content, "shared Rust core") {
				t.Errorf("F4: WEBUI_GUIDE.md 不应包含 shared Rust core")
			}
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("cannot find repo root (go.mod)")
		}
		dir = parent
	}
}
