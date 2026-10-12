package server

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMgmtRouterRetiresRootAndDoesNotServeUI(t *testing.T) {
	isolateConfig(t)

	for _, tc := range []struct {
		name   string
		router *gin.Engine
	}{
		{"management", NewMgmtRouter()},
		{"API-only", NewMgmtAPIRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := getAPI(tc.router, "/", nil)
			if root.Code != http.StatusGone || !strings.Contains(root.Body.String(), "WebUI has been retired") || !strings.Contains(root.Body.String(), "/api/") {
				t.Fatalf("root = %d %q, want retirement notice and API path", root.Code, root.Body.String())
			}
			for _, path := range []string{"/assets/index.js", "/some-page"} {
				if response := getAPI(tc.router, path, nil); response.Code != http.StatusNotFound {
					t.Errorf("%s = %d, want 404", path, response.Code)
				}
			}
		})
	}
}

func TestMgmtAPIRouterPreservesRoutesAndAuth(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	opts := MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword}
	api := NewMgmtAPIRouterWithAuth(opts)
	management := NewMgmtRouterWithAuth(opts)

	routes := func(engine *gin.Engine) []string {
		var got []string
		for _, route := range engine.Routes() {
			got = append(got, route.Method+" "+route.Path)
		}
		sort.Strings(got)
		return got
	}
	if apiRoutes, managementRoutes := routes(api), routes(management); !reflect.DeepEqual(apiRoutes, managementRoutes) {
		t.Fatalf("API/health routes differ between constructors:\nAPI: %v\nmanagement: %v", apiRoutes, managementRoutes)
	}

	for _, tc := range []struct {
		name   string
		router *gin.Engine
	}{{"API-only", api}, {"management", management}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, credentials := range []struct {
				name   string
				header map[string]string
				want   int
			}{
				{"missing", nil, http.StatusUnauthorized},
				{"wrong", map[string]string{"Authorization": basicAuthHeader("admin", "wrong")}, http.StatusUnauthorized},
				{"correct", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)}, http.StatusOK},
			} {
				t.Run(credentials.name, func(t *testing.T) {
					response := getAPI(tc.router, "/api/config", credentials.header)
					if response.Code != credentials.want {
						t.Fatalf("API request = %d, want %d", response.Code, credentials.want)
					}
				})
			}
			if health := getAPI(tc.router, "/healthz", nil); health.Code != http.StatusOK {
				t.Fatalf("unauthenticated health probe = %d, want 200", health.Code)
			}
		})
	}

	t.Run("loopback remains open", func(t *testing.T) {
		loopback := NewMgmtAPIRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})
		if response := getAPI(loopback, "/api/config", nil); response.Code != http.StatusOK {
			t.Fatalf("unauthenticated loopback API = %d, want 200", response.Code)
		}
	})
	t.Run("exposed bind fails closed", func(t *testing.T) {
		exposed := NewMgmtAPIRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0"})
		if response := getAPI(exposed, "/api/config", nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated exposed API = %d, want 401", response.Code)
		}
	})
}
