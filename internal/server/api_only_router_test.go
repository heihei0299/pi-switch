package server

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMgmtAPIRouterRetiresRootAndKeepsDesktopWebUI(t *testing.T) {
	isolateConfig(t)

	api := NewMgmtAPIRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})
	root := getAPI(api, "/", nil)
	if root.Code != http.StatusGone || !strings.Contains(root.Body.String(), "WebUI has been retired") || !strings.Contains(root.Body.String(), "/api/") {
		t.Fatalf("API-only root = %d %q, want retirement notice and API path", root.Code, root.Body.String())
	}
	if assets := getAPI(api, "/assets/index.js", nil); assets.Code != http.StatusNotFound {
		t.Fatalf("API-only asset route = %d, want 404", assets.Code)
	}

	legacy := NewMgmtRouter()
	root = getAPI(legacy, "/", nil)
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `id="root"`) {
		t.Fatalf("legacy Desktop root = %d %q, want embedded WebUI", root.Code, root.Body.String())
	}
}

func TestMgmtAPIRouterPreservesRoutesAndAuth(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	opts := MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword}
	api := NewMgmtAPIRouterWithAuth(opts)
	legacy := NewMgmtRouterWithAuth(opts)

	routes := func(engine *gin.Engine) []string {
		var got []string
		for _, route := range engine.Routes() {
			if route.Path == "/" || route.Path == "/assets/*filepath" {
				continue
			}
			got = append(got, route.Method+" "+route.Path)
		}
		sort.Strings(got)
		return got
	}
	if apiRoutes, legacyRoutes := routes(api), routes(legacy); !reflect.DeepEqual(apiRoutes, legacyRoutes) {
		t.Fatalf("API/health routes differ from legacy router:\nAPI: %v\nlegacy: %v", apiRoutes, legacyRoutes)
	}

	for _, tc := range []struct {
		name   string
		router *gin.Engine
	}{{"API-only", api}, {"Desktop fallback", legacy}} {
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
