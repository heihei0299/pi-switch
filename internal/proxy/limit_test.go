package proxy

import "testing"

func TestClampMaxTokens_RequestedExceeds(t *testing.T) {
	// contextWindow 128000, maxTokens 16384, rawLen small -> available huge but capped by maxTokens
	// est = ceil(400/4)=100, available=128000-100-4096=123804, clamped = min(123804,16384)=16384
	req := 999999
	clamped, detail := ClampMaxTokens(128000, 16384, 400, &req)
	if clamped != 16384 {
		t.Fatalf("clamped = %d, want 16384, detail=%s", clamped, detail)
	}
	if detail == "" {
		t.Fatalf("detail empty")
	}
}

func TestClampMaxTokens_RequestedWithin(t *testing.T) {
	req := 100
	clamped, _ := ClampMaxTokens(128000, 16384, 400, &req)
	if clamped != 100 {
		t.Fatalf("clamped = %d, want 100 (within)", clamped)
	}
}

func TestClampMaxTokens_NoRequested(t *testing.T) {
	clamped, _ := ClampMaxTokens(128000, 16384, 400, nil)
	if clamped != 16384 {
		t.Fatalf("clamped = %d, want 16384", clamped)
	}
}

func TestClampMaxTokens_SmallWindow(t *testing.T) {
	// window just enough to force 16 floor
	// contextWindow 5000, est 1000, available=5000-1000-4096=-96 -> 16, clamped 16 (maxTokens bigger)
	req := 100
	clamped, _ := ClampMaxTokens(5000, 16384, 4000, &req)
	if clamped != 16 {
		t.Fatalf("clamped = %d, want 16", clamped)
	}
	// requested within 16 should keep 10? But requested 10 <16, so kept
	req2 := 10
	clamped2, _ := ClampMaxTokens(5000, 16384, 4000, &req2)
	if clamped2 != 10 {
		t.Fatalf("clamped2 = %d, want 10 (requested within 16)", clamped2)
	}
}

func TestClampMaxTokens_EstRounding(t *testing.T) {
	// len 5 -> ceil(5/4)=2, available = 128000-2-4096=123902, clamped 16384
	req := 20000
	clamped, _ := ClampMaxTokens(128000, 16384, 5, &req)
	if clamped != 16384 {
		t.Fatalf("clamped = %d, want 16384", clamped)
	}
}
