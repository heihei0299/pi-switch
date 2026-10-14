package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatsHandlersPropagateRequestCancellation(t *testing.T) {
	cleanup := setupWebUISyncDB(t)
	defer cleanup()

	for _, path := range []string{
		"/api/stats",
		"/api/stats/conversations",
		"/api/stats/conversations/test/requests",
		"/api/logs/export?format=json",
		"/api/logs/export?format=csv",
	} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
			response := httptest.NewRecorder()

			NewMgmtRouter().ServeHTTP(response, request)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, body = %s; want %d", response.Code, response.Body.String(), http.StatusInternalServerError)
			}
		})
	}
}
