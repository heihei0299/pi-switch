package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/daemon"
)

// known-gaps/02(a): POST /api/proxy/start with an empty body — no `daemon`, no
// ?daemon, no host/port — answered 200 {"running":true,"message":"proxy started
// (stub)"} while that branch starts no daemon, opens no listener and spawns no
// process. It is the request that asks for nothing, and it was told the proxy is
// running.
//
// The honest answer uses the management error envelope
// (system-contract 2.8): 501 + {"error":"<message>"}.
func TestProxyStart_EmptyRequestDoesNotClaimTheProxyIsRunning(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	w := callMgmt(r, http.MethodPost, "/api/proxy/start", `{}`)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("POST /api/proxy/start with an empty body = %d (%s), want 501", w.Code, w.Body.String())
	}

	var body struct {
		Running *bool  `json:"running"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	if body.Running != nil {
		t.Fatalf("the response still claims a running state: %s", w.Body.String())
	}
	// The operator must be told what the request was missing, not just "no".
	if !strings.Contains(body.Error, "daemon") {
		t.Fatalf("the message does not say how to start the proxy for real: %q", body.Error)
	}
}

// known-gaps/02(b): the handler classified a failed start by matching the words
// "already in use" in the error text. A typed error whose message does NOT contain
// those words must still be recognised, which is only possible by kind.
//
// The mapping is exercised directly because handleProxyStart calls daemon.Start
// itself, so the only way to feed it a specific error is through a daemon that
// really fails — and that route always produces the familiar wording, which cannot
// distinguish the two implementations.
func TestProxyStartError_ClassifiesByKindNotByWording(t *testing.T) {
	// Deliberately free of the phrase the old implementation looked for.
	err := fmt.Errorf("bind failed: %w", daemon.ErrPortInUse)

	body := proxyStartError(err)
	if _, ok := body["message"]; !ok {
		t.Fatalf("a typed port-in-use error lost the message field: %v", body)
	}

	// The negative control: an unrelated failure keeps the plain shape, so the
	// assertion above cannot pass for every error.
	plain := proxyStartError(errors.New("boom"))
	if _, ok := plain["message"]; ok {
		t.Fatalf("an unrelated failure gained the port-in-use shape: %v", plain)
	}
}
