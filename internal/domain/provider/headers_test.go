package provider

import "testing"

func TestMergeHeadersCanonicalizesChannelOverrides(t *testing.T) {
	got := MergeHeaders(map[string]string{"x-api-key": "profile", "accept": "application/json"}, map[string]string{"X-API-KEY": "channel", "": "ignored"})
	if len(got) != 2 || got["X-Api-Key"] != "channel" || got["Accept"] != "application/json" {
		t.Fatalf("merged headers = %#v", got)
	}
}
