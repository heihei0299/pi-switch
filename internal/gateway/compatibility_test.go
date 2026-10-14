package gateway_test

import (
	"reflect"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	legacygateway "github.com/heihei0299/pi-switch/internal/gateway"
)

func TestLegacyGatewayFacadeDelegatesCanonicalRules(t *testing.T) {
	cfg := config.DefaultConfig()

	if got, want := legacygateway.BuildProposedGatewayEntry(cfg), domaingateway.BuildProposedGatewayEntry(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy proposal differs from canonical proposal:\nlegacy=%#v\ncanonical=%#v", got, want)
	}
	if got, want := legacygateway.FixedGatewayProviders(), domaingateway.FixedGatewayProviders(); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy fixed providers differ from canonical providers: got=%#v want=%#v", got, want)
	}
}
