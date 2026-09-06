package catalog

import (
	"testing"
)

const ambiguousFixture = `{
  "deepseek": {"models": {
    "deepseek/deepseek-v4-flash": {"name": "Flash D", "reasoning": true, "modalities": {"input": ["text"]}, "limit": {"context": 1048576, "output": 384000}, "cost": {"input": 0.14, "output": 0.28, "cache_read": 0.0028}}
  }},
  "tokengo": {"models": {
    "deepseek/deepseek-v4-flash": {"name": "Flash T", "reasoning": true, "modalities": {"input": ["text"]}, "limit": {"context": 200, "output": 20}, "cost": {"input": 0.098, "output": 0.196, "cache_read": 0.028}}
  }}
}`

func TestLookupWithProvider_DeepseekAmbiguous(t *testing.T) {
	snap, err := ParseSnapshot([]byte(ambiguousFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := snap.Lookup("deepseek-v4-flash"); ok {
		t.Fatalf("ambiguous bare should miss without provider")
	}
	meta, ok := snap.LookupWithProvider("deepseek-v4-flash", "deepseek")
	if !ok {
		t.Fatalf("LookupWithProvider deepseek miss")
	}
	if meta.ContextWindow != 1048576 {
		t.Fatalf("deepseek ContextWindow = %d, want 1048576", meta.ContextWindow)
	}
	meta2, ok := snap.LookupWithProvider("deepseek-v4-flash", "tokengo")
	if !ok {
		t.Fatalf("LookupWithProvider tokengo miss")
	}
	if meta2.ContextWindow != 200 {
		t.Fatalf("tokengo ContextWindow = %d, want 200", meta2.ContextWindow)
	}
	// provider qualified id also works
	meta3, ok := snap.LookupWithProvider("tokengo/deepseek-v4-flash", "tokengo")
	if !ok || meta3.ContextWindow != 200 {
		t.Fatalf("qualified tokengo lookup failed: %v %v", ok, meta3)
	}
}

func TestLookup_UniqueStillWorks(t *testing.T) {
	snap, _ := ParseSnapshot([]byte(`{"openai":{"models":{"openai/gpt-4o-mini":{"name":"Mini","limit":{"context":128000,"output":16384},"cost":{"input":0.15,"output":0.6,"cache_read":0.075}}}}}`))
	if _, ok := snap.Lookup("gpt-4o-mini"); !ok {
		t.Fatalf("unique lookup should hit")
	}
	if meta, ok := snap.LookupWithProvider("gpt-4o-mini", "openai"); !ok || meta.ContextWindow != 128000 {
		t.Fatalf("provider lookup for unique failed: %v %v", ok, meta)
	}
}
