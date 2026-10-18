package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestGatewayPublishMissingProvidersDoesNotFetchCatalog(t *testing.T) {
	path := isolateConfig(t)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(filepath.Dir(path), "models.json"))
	t.Setenv("PI_SWITCH_CATALOG", filepath.Join(filepath.Dir(path), "catalog.json"))
	var calls atomic.Int32
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer catalog.Close()
	t.Setenv("PI_SWITCH_CATALOG_URL", catalog.URL)
	w := callMgmt(NewMgmtRouter(), http.MethodPost, "/api/gateway/publish", `{"invalid":true}`)
	if w.Code != http.StatusBadRequest || calls.Load() != 0 {
		t.Fatalf("status=%d catalog calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
	}
}
