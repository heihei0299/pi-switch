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

	denied := getAPI(api, "/api/config", nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API request = %d, want 401", denied.Code)
	}
	served := getAPI(api, "/api/config", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if served.Code != http.StatusOK {
		t.Fatalf("authenticated API request = %d, want 200", served.Code)
	}
}
