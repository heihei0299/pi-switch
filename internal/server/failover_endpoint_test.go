package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFailoverEndpoint_Gone(t *testing.T) {
	router := NewMgmtRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/proxy/failover", strings.NewReader(`{"failover":["a"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 410 {
		t.Fatalf("PUT /api/proxy/failover: got %d body %s, want 410", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "gone") {
		t.Fatalf("body should contain gone, got %s", w.Body.String())
	}
}
