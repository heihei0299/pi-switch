package catalog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// S1: models.dev 快照拉取/缓存/查找/补缺。fixture 内联，不依赖真实网络。

const fixtureAPI = `{
  "openai": {"models": {
    "openai/gpt-4o-mini": {"name": "GPT-4o mini", "reasoning": false,
      "modalities": {"input": ["text", "image"]},
      "limit": {"context": 128000, "output": 16384},
      "cost": {"input": 0.15, "output": 0.6, "cache_read": 0.075}},
    "openai/reasoned": {"name": "Reasoned", "reasoning": true,
      "limit": {"context": 1000, "output": 100},
      "cost": {"input": 1, "output": 1, "cache_read": 0.1}},
    "openai/dup": {"name": "Dup A", "reasoning": false,
      "limit": {"context": 100, "output": 10},
      "cost": {"input": 1, "output": 2, "cache_read": 0.1}}
  }},
  "other": {"models": {
    "other/dup": {"name": "Dup B", "reasoning": true,
      "limit": {"context": 200, "output": 20},
      "cost": {"input": 3, "output": 4, "cache_read": 0.3}}
  }}
}`

func fixtureServer(t *testing.T, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureAPI))
	}))
}

func tempCache(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "models-dev.json")
	t.Setenv("PI_SWITCH_CATALOG", p)
	return p
}

func TestCatalog_FetchCachesAndLooksUp(t *testing.T) {
	p := tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)

	snap, stale, warn := Ensure()
	if warn != "" {
		t.Fatalf("warn = %q, want empty", warn)
	}
	if stale {
		t.Fatalf("stale = true on fresh fetch")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	meta, ok := snap.Lookup("gpt-4o-mini")
	if !ok {
		t.Fatalf("lookup gpt-4o-mini missed")
	}
	if meta.Name != "GPT-4o mini" || meta.ContextWindow != 128000 || meta.MaxTokens != 16384 {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.CostInput != 0.15 || meta.CostOutput != 0.6 || meta.CacheRead != 0.075 {
		t.Fatalf("cost = %+v", meta)
	}
	if len(meta.Input) != 2 || meta.Input[0] != "text" {
		t.Fatalf("input = %v", meta.Input)
	}
	// 第二次走缓存，不再请求
	_, _, _ = Ensure()
	if hits != 1 {
		t.Fatalf("hits = %d after cached load, want 1", hits)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}

func TestCatalog_AmbiguousAndMissingSkipped(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)

	snap, _, _ := Ensure()
	if _, ok := snap.Lookup("dup"); ok {
		t.Fatalf("ambiguous bare id must not match")
	}
	if _, ok := snap.Lookup("nope"); ok {
		t.Fatalf("missing bare id must not match")
	}
}

func TestCatalog_FillMissingOnly(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	meta, ok := snap.Lookup("gpt-4o-mini")
	if !ok {
		t.Fatal("lookup missed")
	}
	entry := map[string]interface{}{
		"id":            "sup/gpt-4o-mini",
		"name":          "Custom Name",
		"contextWindow": float64(64000),
		"cost":          map[string]interface{}{"input": 9.0, "output": 9.0, "cacheRead": 0.0, "cacheWrite": 0.0},
	}
	if !FillMissing(entry, meta) {
		t.Fatalf("FillMissing reported no change, want changes (maxTokens/input/reasoning)")
	}
	if entry["name"] != "Custom Name" {
		t.Fatalf("name overwritten: %v", entry["name"])
	}
	if entry["contextWindow"] != float64(64000) {
		t.Fatalf("contextWindow overwritten: %v", entry["contextWindow"])
	}
	if entry["maxTokens"] != float64(16384) {
		t.Fatalf("maxTokens = %v (%T), want float64(16384)", entry["maxTokens"], entry["maxTokens"])
	}
	if input, _ := entry["input"].([]interface{}); len(input) != 2 {
		t.Fatalf("input = %v, want [text image]", entry["input"])
	}
	if entry["reasoning"] != false {
		t.Fatalf("reasoning = %v, want false (spec literal: nil补)", entry["reasoning"])
	}
	cost := entry["cost"].(map[string]interface{})
	if cost["input"] != 9.0 || cost["output"] != 9.0 {
		t.Fatalf("cost overwritten: %v", cost)
	}
	if cost["cacheRead"] != 0.075 {
		t.Fatalf("cacheRead = %v, want 0.075 (filled)", cost["cacheRead"])
	}
}

func TestCatalog_FillReasoningTrue(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	meta, ok := snap.Lookup("reasoned")
	if !ok {
		t.Fatal("lookup missed")
	}
	entry := map[string]interface{}{"id": "sup/reasoned"}
	if !FillMissing(entry, meta) {
		t.Fatal("no change")
	}
	if entry["reasoning"] != true {
		t.Fatalf("reasoning = %v, want true", entry["reasoning"])
	}
}

func TestCatalog_FillWholeCostIncludesCacheWrite(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	meta, _ := snap.Lookup("gpt-4o-mini")
	entry := map[string]interface{}{"id": "sup/gpt-4o-mini"}
	FillMissing(entry, meta)
	cost, ok := entry["cost"].(map[string]interface{})
	if !ok {
		t.Fatalf("cost missing: %v", entry)
	}
	if _, ok := cost["cacheWrite"]; !ok {
		t.Fatalf("cost lacks explicit cacheWrite: %v (pending-never-zero guard)", cost)
	}
}

func TestCatalog_FillModelsCounts(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	models := []interface{}{
		map[string]interface{}{"id": "sup/gpt-4o-mini"},
		"not-a-map",
		map[string]interface{}{"id": ""},
		map[string]interface{}{"id": "sup/nope"},
	}
	enriched, skipped := FillModels(models, snap)
	if enriched != 1 || skipped != 3 {
		t.Fatalf("enriched=%d skipped=%d, want 1/3", enriched, skipped)
	}
}

func TestCatalog_StaleFallsBackWithWarning(t *testing.T) {
	p := tempCache(t)
	if err := os.WriteFile(p, []byte(`{"openai":{"models":{"openai/old":{"name":"Old","limit":{"context":10,"output":10},"cost":{"input":1,"output":1,"cache_read":0.1}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatal(err)
	}
	// 失败的上游
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)

	snap, stale, warn := Ensure()
	if !stale {
		t.Fatalf("stale = false, want true on refresh failure")
	}
	if warn == "" {
		t.Fatalf("warn empty, want failure note")
	}
	if _, ok := snap.Lookup("old"); !ok {
		t.Fatalf("stale snapshot unusable")
	}
}

func TestCatalog_NoCacheAndFailureWarns(t *testing.T) {
	tempCache(t) // 文件不存在
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)

	snap, _, warn := Ensure()
	if warn == "" {
		t.Fatalf("warn empty, want failure note")
	}
	if _, ok := snap.Lookup("gpt-4o-mini"); ok {
		t.Fatalf("empty snapshot must not match")
	}
}

func TestCatalog_FillMissingKeepsTypedNumbers(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	meta, ok := snap.Lookup("gpt-4o-mini")
	if !ok {
		t.Fatal("lookup missed")
	}
	// Go 侧提议值为 uint32/int 等非 float64 类型：非零值不得被覆盖。
	entry := map[string]interface{}{
		"id":            "sup/gpt-4o-mini",
		"contextWindow": uint32(64000),
		"maxTokens":     0,
		"cost":          map[string]interface{}{"input": 9, "output": 9.0, "cacheRead": 0.0},
	}
	if !FillMissing(entry, meta) {
		t.Fatal("want changes (maxTokens/cacheRead)")
	}
	if entry["contextWindow"] != uint32(64000) {
		t.Fatalf("contextWindow overwritten: %v (%T)", entry["contextWindow"], entry["contextWindow"])
	}
	cost := entry["cost"].(map[string]interface{})
	if cost["input"] != 9 {
		t.Fatalf("cost.input overwritten: %v", cost["input"])
	}
	if cost["cacheRead"] != 0.075 {
		t.Fatalf("cacheRead = %v, want 0.075", cost["cacheRead"])
	}
}

func TestCatalog_CacheWriteNormalizationNotCounted(t *testing.T) {
	tempCache(t)
	var hits int
	srv := fixtureServer(t, &hits)
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	snap, _, _ := Ensure()
	meta, _ := snap.Lookup("gpt-4o-mini")
	// 存量 cost 齐全仅缺 cacheWrite：补零归一（保 pending 收敛）但不计 enriched。
	entry := map[string]interface{}{
		"id":            "sup/gpt-4o-mini",
		"name":          "GPT-4o mini",
		"contextWindow": float64(128000),
		"maxTokens":     float64(16384),
		"input":         []interface{}{"text", "image"},
		"reasoning":     false,
		"cost":          map[string]interface{}{"input": 0.15, "output": 0.6, "cacheRead": 0.075},
	}
	if FillMissing(entry, meta) {
		t.Fatalf("pure cacheWrite normalization must not count as filled")
	}
	cost := entry["cost"].(map[string]interface{})
	if _, ok := cost["cacheWrite"]; !ok {
		t.Fatalf("cacheWrite normalization missing: %v", cost)
	}
}

func TestCatalog_WarningCarriesStatus(t *testing.T) {
	tempCache(t) // 文件不存在
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", srv.URL)
	_, _, warn := Ensure()
	if warn == "" || !strings.Contains(warn, "503") {
		t.Fatalf("warn = %q, want status 503 mentioned", warn)
	}
}

func TestCatalog_FillOverwritePreservesExplicitName(t *testing.T) {
	entry := map[string]interface{}{"id": "sup/model", "name": "Browser Edited"}
	changed := FillOverwrite(entry, Meta{Name: "Catalog Default", ContextWindow: 1000})
	if !changed {
		t.Fatal("expected generated metadata to be filled")
	}
	if entry["name"] != "Browser Edited" {
		t.Fatalf("explicit name was overwritten: %v", entry["name"])
	}
	if entry["contextWindow"] != float64(1000) {
		t.Fatalf("contextWindow = %v, want 1000", entry["contextWindow"])
	}
}
