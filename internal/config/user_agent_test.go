package config

import "testing"

func TestResolveUserAgentValues(t *testing.T) {
	profile, global, empty := "profile-UA", "global-UA", ""
	for _, tc := range []struct {
		profile, global *string
		want            string
	}{
		{nil, nil, "curl/8.5.0"}, {&empty, &empty, "curl/8.5.0"}, {nil, &global, global}, {&empty, &global, global}, {&profile, &global, profile},
	} {
		if got := ResolveUserAgentValues(tc.profile, tc.global); got != tc.want {
			t.Fatalf("UA = %q, want %q", got, tc.want)
		}
	}
}
