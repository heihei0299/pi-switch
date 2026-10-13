package limit

import "testing"

func TestClampMaxTokens_RequestedExceeds(t *testing.T) {
	req := 999999
	if got := ClampMaxTokens(128000, 16384, 400, &req); got != 16384 {
		t.Fatalf("clamped = %d, want 16384", got)
	}
}

func TestClampMaxTokens_RequestedWithin(t *testing.T) {
	req := 100
	if got := ClampMaxTokens(128000, 16384, 400, &req); got != 100 {
		t.Fatalf("clamped = %d, want 100", got)
	}
}

func TestClampMaxTokens_NoRequested(t *testing.T) {
	if got := ClampMaxTokens(128000, 16384, 400, nil); got != 16384 {
		t.Fatalf("clamped = %d, want 16384", got)
	}
}

func TestClampMaxTokens_SmallWindow(t *testing.T) {
	req := 100
	if got := ClampMaxTokens(5000, 16384, 4000, &req); got != 16 {
		t.Fatalf("clamped = %d, want 16", got)
	}

	req = 10
	if got := ClampMaxTokens(5000, 16384, 4000, &req); got != 10 {
		t.Fatalf("clamped = %d, want requested 10", got)
	}
}

func TestClampMaxTokens_EstRounding(t *testing.T) {
	req := 20000
	if got := ClampMaxTokens(128000, 16384, 5, &req); got != 16384 {
		t.Fatalf("clamped = %d, want 16384", got)
	}
}
