package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestStatsPageLimitExtremeValues(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	maxText := strconv.Itoa(maxInt)
	for _, tc := range []struct {
		name                string
		query               string
		max                 int
		wantPage, wantLimit int
	}{
		{name: "page zero and limit fifty", query: "?page=0&limit=50", max: 500, wantPage: 0, wantLimit: 50},
		{name: "maximum page", query: "?page=" + maxText + "&limit=50", max: 500, wantPage: maxInt, wantLimit: 50},
		{name: "maximum limit clamped", query: "?page=0&limit=" + maxText, max: 500, wantPage: 0, wantLimit: 500},
		{name: "maximum page and limit", query: "?page=" + maxText + "&limit=" + maxText, max: 0, wantPage: maxInt, wantLimit: maxInt},
		{name: "negative page", query: "?page=-1&limit=50", max: 500, wantPage: 0, wantLimit: 50},
		{name: "zero and negative limit", query: "?page=0&limit=0", max: 0, wantPage: 0, wantLimit: 50},
		{name: "negative limit", query: "?page=0&limit=-1", max: 0, wantPage: 0, wantLimit: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/stats"+tc.query, nil)
			page, limit := statsPageLimit(c, tc.max)
			if page != tc.wantPage || limit != tc.wantLimit {
				t.Fatalf("page=%d limit=%d, want page=%d limit=%d", page, limit, tc.wantPage, tc.wantLimit)
			}
		})
	}
}
