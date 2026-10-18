package config

import "testing"

func TestValidatePort(t *testing.T) {
	for _, port := range []int{-1, 0, 1, 43112, 65535, 65536, 70000} {
		wantValid := port >= 1 && port <= 65535
		if err := ValidatePort(port); (err == nil) != wantValid {
			t.Errorf("ValidatePort(%d) = %v", port, err)
		}
	}
}
