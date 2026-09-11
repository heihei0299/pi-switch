package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func assertInferenceEnvelope(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("status = %d (%s), want %d", w.Code, w.Body.String(), code)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode inference envelope: %v (%s)", err, w.Body.String())
	}
	if body.Error.Message == "" || body.Error.Type == "" {
		t.Fatalf("inference envelope = %s, want {\"error\":{\"message\":...,\"type\":...}}", w.Body.String())
	}
}

func assertManagementEnvelope(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("status = %d (%s), want %d", w.Code, w.Body.String(), code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Fatalf("management envelope = %s, want {\"error\":\"...\"}", w.Body.String())
	}
}

// system-contract 2.8: the inference surface keeps the OpenAI error object for
// framework-generated responses too — unknown routes and recovered panics.
func TestInferenceSurface_FrameworkErrorsUseOpenAIEnvelope(t *testing.T) {
	isolateConfig(t)
	r := NewProxyRouter()
	r.GET("/v1/panic", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/does-not-exist", nil))
	assertInferenceEnvelope(t, w, http.StatusNotFound)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/v1/panic", nil))
	assertInferenceEnvelope(t, w2, http.StatusInternalServerError)
}

// The management surface answers the same recovered panic with its string envelope.
func TestManagementSurface_PanicUsesStringEnvelope(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()
	r.GET("/api/panic", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/panic", nil))
	assertManagementEnvelope(t, w, http.StatusInternalServerError)
}
