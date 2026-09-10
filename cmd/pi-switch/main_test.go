package main

import "testing"

// The bind address reaches the auth guard through flag parsing, so the flag
// path is where a fail-open would hide: `--host ""` must arrive as an empty
// host (which the guard refuses), not be silently replaced by a local default.

func TestParseHostPort_FlagPath(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantHost string
		wantPort int
		wantDae  bool
	}{
		{"defaults when no flags", nil, "127.0.0.1", 43110, false},
		{"explicit wildcard", []string{"--host", "0.0.0.0"}, "0.0.0.0", 43110, false},
		{"explicit loopback", []string{"--host", "127.0.0.1"}, "127.0.0.1", 43110, false},
		{"empty host is passed through, not defaulted", []string{"--host", ""}, "", 43110, false},
		{"port and daemon", []string{"--host", "0.0.0.0", "--port", "43999", "--daemon"}, "0.0.0.0", 43999, true},
		{"invalid port keeps default", []string{"--port", "not-a-number"}, "127.0.0.1", 43110, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, daemon := parseHostPort(tc.args, "127.0.0.1", 43110)
			if host != tc.wantHost || port != tc.wantPort || daemon != tc.wantDae {
				t.Fatalf("parseHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
					tc.args, host, port, daemon, tc.wantHost, tc.wantPort, tc.wantDae)
			}
		})
	}
}

func TestHasGeneratePasswordFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"webui", "start", "--host", "0.0.0.0"}, false},
		{[]string{"webui", "start", "--host", "0.0.0.0", "--generate-password"}, true},
		{[]string{"--generate-password"}, true},
	}
	for _, tc := range cases {
		if got := hasGeneratePasswordFlag(tc.args); got != tc.want {
			t.Fatalf("hasGeneratePasswordFlag(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
