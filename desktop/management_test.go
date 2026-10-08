package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/server"
)

func TestManagementUIOriginBoundaryAndSharedRouter(t *testing.T) {
	const appHost = "a1b2c3"
	handler := managementUIHandler(appHost, server.NewMgmtRouter())

	request := httptest.NewRequest(http.MethodGet, "http://evil.example/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong host: status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "http://"+appHost+"/healthz", nil)
	request.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong origin: status=%d", response.Code)
	}

	for _, origin := range []string{"", "null", managementUIScheme + "://" + appHost} {
		request = httptest.NewRequest(http.MethodGet, "http://"+appHost+"/healthz", nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("origin %q: status=%d body=%s", origin, response.Code, response.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodGet, "http://"+appHost+"/", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="root"`) {
		t.Fatalf("embedded WebUI entrypoint: status=%d body=%s", response.Code, response.Body.String())
	}
}
