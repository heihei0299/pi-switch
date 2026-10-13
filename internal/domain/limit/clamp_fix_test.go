package limit

import "testing"

func TestClampMaxTokens_EncryptedContentConservative(t *testing.T) {
	// Simulate long session with reasoning.encrypted_content base64 dense.
	// window 1_048_576, maxTokens 943_718, rawLen ~855k (est with /3 => 285k)
	// Expected: available = window - est - safety(8192) ≈ 755k, clamped to ~755k
	// Old logic: est=rawLen/4=213k, available=830k, would clamp to 830k (too high, still over window)
	req := 908720
	clamped := ClampMaxTokens(1048576, 943718, 855000, &req)
	// New logic should clamp to ~755k (window - 285k - 8192 = 755384)
	if clamped != 755384 {
		t.Fatalf("clamped = %d, want 755384 (encrypted_content conservative)", clamped)
	}
}

func TestClampMaxTokens_SafetyDynamic(t *testing.T) {
	// window 1_048_576 => safety = max(8192, window/128=8192) = 8192, not 4096
	// rawLen 400 => est=134 (ceil 400/3), available=1048576-134-8192=1040250, clamped=min(1040250,16384)=16384
	// Old safety 4096 would give 1044346, still capped to 16384, but we verify safety change doesn't break small cases
	req := 20000
	clamped := ClampMaxTokens(1048576, 16384, 400, &req)
	if clamped != 16384 {
		t.Fatalf("clamped = %d, want 16384", clamped)
	}
}

func TestClampMaxTokens_EstPastWindowWarn(t *testing.T) {
	// est > window-16 => available floor 16
	// rawLen huge: 4M chars => est=1_333_334 > window, available=16, clamped=16
	req := 100000
	clamped := ClampMaxTokens(1048576, 943718, 4000000, &req)
	if clamped != 16 {
		t.Fatalf("clamped = %d, want 16 (est past window)", clamped)
	}
}
