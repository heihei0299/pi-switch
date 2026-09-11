package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/scan"
	webuiFS "github.com/heihei0299/pi-switch/webui"
)

var webUIFS = webuiFS.FS

var (
	Version     = "dev"
	BuildTime   = "unknown"
	BuildCommit = "unknown"
	BuildTarget = "unknown"
	BuildDirty  = "unknown"
)

var (
	webuiEmbedded   bool
	webuiIndexHash  string
	webuiScript     string
	webuiAssetCount int
	webuiOnce       sync.Once
)

func init() {
	computeBuildInfo()
}

func computeBuildInfo() {
	webuiOnce.Do(func() {
		// index hash
		if data, err := webUIFS.ReadFile("dist/index.html"); err == nil && len(data) > 0 {
			webuiEmbedded = true
			h := sha256.Sum256(data)
			webuiIndexHash = hex.EncodeToString(h[:])
		} else if data, err := fs.ReadFile(webUIFS, "dist/index.html"); err == nil && len(data) > 0 {
			webuiEmbedded = true
			h := sha256.Sum256(data)
			webuiIndexHash = hex.EncodeToString(h[:])
		} else {
			// placeholder hash (so indexHash non-empty even without embed)
			placeholder := []byte(`<!doctype html><html><head><meta charset="utf-8"><title>pi-switch</title></head><body><div id="root">pi-switch WebUI placeholder</div><p>embed.FS placeholder for webui/dist</p></body></html>`)
			h := sha256.Sum256(placeholder)
			webuiIndexHash = hex.EncodeToString(h[:])
			webuiEmbedded = false
		}
		// assets
		var assetsFS fs.FS = webUIFS
		if sub, err := fs.Sub(webUIFS, "dist/assets"); err == nil {
			assetsFS = sub
			if entries, err := fs.ReadDir(assetsFS, "."); err == nil {
				webuiAssetCount = len(entries)
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					name := e.Name()
					if webuiScript == "" && len(name) > 3 && name[len(name)-3:] == ".js" && len(name) > 6 && name[:6] == "index-" {
						webuiScript = "/assets/" + name
					}
				}
				// if no index-*.js but has any js, fallback
				if webuiScript == "" {
					for _, e := range entries {
						if !e.IsDir() && len(e.Name()) > 3 && e.Name()[len(e.Name())-3:] == ".js" {
							webuiScript = "/assets/" + e.Name()
							break
						}
					}
				}
			} else {
				webuiAssetCount = 0
			}
		} else {
			// try listing dist/assets via ReadDir on webUIFS
			if entries, err := fs.ReadDir(webUIFS, "dist/assets"); err == nil {
				webuiAssetCount = len(entries)
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					name := e.Name()
					if webuiScript == "" && len(name) > 3 && name[len(name)-3:] == ".js" && len(name) > 6 && name[:6] == "index-" {
						webuiScript = "/assets/" + name
					}
				}
			} else {
				webuiAssetCount = 0
				// also try to find script via glob of dist/assets in webUIFS directly
			}
		}
		// if still no script but embedded, try to parse index.html for script src
		if webuiScript == "" && webuiEmbedded {
			if data, err := webUIFS.ReadFile("dist/index.html"); err == nil {
				s := string(data)
				// naive search for /assets/index-*.js
				idx := 0
				for {
					pos := indexOf(s[idx:], "/assets/")
					if pos < 0 {
						break
					}
					pos += idx
					end := pos
					for end < len(s) && s[end] != 34 && s[end] != 39 && s[end] != 32 && s[end] != 62 {
						end++
					}
					candidate := s[pos:end]
					if len(candidate) > 4 && candidate[len(candidate)-3:] == ".js" {
						webuiScript = candidate
						break
					}
					idx = end
					if idx >= len(s) {
						break
					}
				}
			}
		}
	})
}

func indexOf(s, substr string) int {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// configPath and saveConfig are kernel helpers: every domain file reaches the
// config file through them. Both delegate to internal/config so that path
// resolution and the save-time migration have exactly one implementation.
func configPath() string {
	return config.ResolvePath()
}

// loadConfig is the server's single config read path. It returns the error
// instead of a fallback so no handler continues with a silently empty config.
func loadConfig() (config.PiSwitchConfig, error) {
	cfg, _, err := config.LoadConfigAtPath(configPath())
	return cfg, err
}

// loadConfigOrWrite is loadConfig for management (/api) handlers: on failure it
// writes the management error envelope ({"error": "<message>"}) and reports
// ok=false so the handler must stop.
func loadConfigOrWrite(c *gin.Context) (config.PiSwitchConfig, bool) {
	cfg, err := loadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return config.PiSwitchConfig{}, false
	}
	return cfg, true
}

// loadConfigOrChatError is loadConfig for inference (/v1) handlers: the response
// keeps the OpenAI {"error":{message,type}} envelope even when the failure is a
// shared pre-step such as reading the config. See system-contract 2.8.
func loadConfigOrChatError(c *gin.Context) (config.PiSwitchConfig, bool) {
	cfg, err := loadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "config unavailable: " + err.Error(),
			"type":    "internal_error",
		}})
		return config.PiSwitchConfig{}, false
	}
	return cfg, true
}

// notImplemented answers with 501 for capabilities that have no implementation.
// It lives in the kernel because both the settings and profile domains use it
// (domain files must not call each other's helpers). These endpoints used to
// return 200 with a plausible payload — a path that was never written, a restore
// that never happened — so the WebUI reported success for work that did not
// occur. It returns the management envelope: the 501 status already carries the
// "not implemented" kind, and the WebUI only renders a string message.
func notImplemented(what string) gin.H {
	return gin.H{"error": what + " is not implemented"}
}

func NewProxyRouter() *gin.Engine {
	return NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})
}

// NewProxyRouterWithAuth builds the proxy router for a specific bind address.
// A non-loopback bind installs the same Basic guard the management API uses,
// scoped to the inference routes; health probes stay open.
func NewProxyRouterWithAuth(opts MgmtAuthOptions) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	if !IsLoopback(opts.BindHost) {
		r.Use(basicAuthMiddleware(opts.Password, "/v1"))
	}
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.POST("/v1/chat/completions", handleChatCompletions)
	r.POST("/v1/completions", handleChatCompletions)
	r.POST("/v1/responses", handleChatCompletions)
	r.POST("/v1/messages", handleChatCompletions)
	r.GET("/v1/models", handleModels)
	return r
}

// NewMgmtRouter builds the management router for loopback use, with no
// authentication. Callers that bind elsewhere must use NewMgmtRouterWithAuth:
// an empty bind host means "every interface", not "local".
func NewMgmtRouter() *gin.Engine {
	return NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})
}

// NewMgmtRouterWithAuth builds the management router for a specific bind
// address: non-loopback bindings require the password from opts, and the
// decision never consults the config file.
func NewMgmtRouterWithAuth(opts MgmtAuthOptions) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	// Any non-loopback bind installs the guard, including an unspecified bind
	// host. With no password the middleware rejects everything, because an
	// exposed listener that cannot authenticate must not serve openly at all.
	// Startup validation normally refuses this state before binding; installing
	// the guard anyway means reaching it cannot silently fail open.
	if !IsLoopback(opts.BindHost) {
		r.Use(basicAuthMiddleware(opts.Password, "/api"))
	}
	// Record the enforced mode for handlers that report it back to clients.
	r.Use(func(c *gin.Context) {
		withAuthState(opts, c)
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	// API group
	api := r.Group("/api")
	{
		api.GET("/config", handleGetConfig)
		api.PUT("/config", handlePutConfig)
		api.GET("/state", handleGetState)
		api.GET("/profiles", handleListProfiles)
		api.POST("/profiles", handlePostProfile)
		api.GET("/profiles/:name", handleGetProfile)
		api.PUT("/profiles/:name", handlePutProfile)
		api.DELETE("/profiles/:name", handleDeleteProfile)
		api.POST("/profiles/:name/duplicate", handleDuplicateProfile)
		api.POST("/profiles/:name/test", handleTestProfile)
		api.POST("/profiles/:name/fetch-models", handleFetchModels)
		api.PUT("/profiles/:name/models", handlePutModels)
		api.PUT("/profiles/:name/expose", handlePutExpose)
		api.PUT("/profiles/:name/spoof", handlePutSpoof)
		api.GET("/profiles/:name/credits", handleGetCredits)
		api.GET("/presets", handlePresets)
		api.GET("/presets/:id", handlePresetDetail)
		api.GET("/doctor", handleDoctor)
		api.GET("/config/validate", handleValidate)
		api.GET("/validate", handleValidate)
		api.POST("/validate", handleValidate)
		api.GET("/backups", handleBackups)
		api.GET("/stats", handleStats)
		api.GET("/stats/conversations", handleStatsConversations)
		api.GET("/stats/conversations/:id/requests", handleConversationRequests)
		api.GET("/proxy/status", handleProxyStatus)
		api.GET("/webui/info", handleWebUIInfo)
		api.GET("/logs/export", handleLogsExport)
		api.GET("/export", handleLogsExport)
		api.GET("/models/gateway", handleGetGateway)
		api.GET("/models/gateway/preview", handleGatewayPreview)
		api.POST("/models/gateway/preview", handleGatewayPreviewPost)
		api.PUT("/models/gateway", handlePutGateway)
		api.POST("/gateway/publish", handleGatewayPublish)
		api.PUT("/gateway/publish", handleGatewayPublish)
		api.GET("/gateway/health", handleGatewayHealth)
		api.POST("/gateway/start", handleGatewayStart)
		api.GET("/packages", handlePackagesList)
		api.POST("/packages", handlePackageAdd)
		api.POST("/packages/import", handlePackageImport)
		api.GET("/packages/:id", handlePackageGet)
		api.DELETE("/packages/*id", handlePackageDelete)
		api.POST("/packages/:id/toggle", handlePackageToggle)
		api.GET("/ccswitch/providers", handleCcsProviders)
		api.POST("/ccswitch/import", handleCcsImport)
		api.POST("/init", handleInit)
		api.POST("/proxy/start", handleProxyStart)
		api.POST("/proxy/stop", handleProxyStop)
		api.PUT("/proxy/failover", handlePutFailover)
		api.GET("/settings", handleGetSettings)
		api.PUT("/settings", handlePutSettings)
		api.POST("/config/export", handleConfigExportStub)
		api.POST("/config/import", handleConfigImportStub)
		api.POST("/config/restore", handleConfigRestoreStub)
		api.GET("/buildInfo", handleBuildInfo)
		api.GET("/dumps", handleDumps)
	}
	// static webui
	r.GET("/", handleWebUIIndex)
	r.GET("/assets/*filepath", handleAssets)
	r.NoRoute(handleWebUIFallback)
	return r
}

// --- auth ---
// isLoopback reports whether host names only the local machine.
//
// An unspecified (empty) host is NOT loopback: gin's Run and net.Listen read
// ":port" as "every interface", so treating "" as local is a fail-open exactly
// when the operator forgot to name an interface. Callers that mean "default to
// local" must say 127.0.0.1 explicitly. Wildcard addresses (0.0.0.0, ::) are
// likewise not loopback — they mean "listen on every interface", the opposite
// of local-only.
func IsLoopback(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "::1" || h == "[::1]" {
		return true
	}
	// bracketed IPv6 keeps its port after the closing bracket
	if i := strings.LastIndex(h, "]"); i >= 0 {
		h = h[:i+1]
	} else if idx := strings.LastIndex(h, ":"); idx >= 0 {
		// bare "::1:port" is ambiguous, so only strip a numeric port
		if port := h[idx+1:]; port != "" && !strings.Contains(port, ":") {
			if _, err := strconv.Atoi(port); err == nil {
				h = h[:idx]
			}
		}
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || h == "[::1]"
}

func webUIPasswordPath() string {
	if p := os.Getenv("PI_SWITCH_WEBUI_PASSWORD_FILE"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return "/tmp/pi-switch-webui_password"
	}
	return filepath.Join(home, ".pi-switch", "webui_password")
}

// authStateCtxKey stores the auth mode this listener actually enforces, so
// read-only handlers report the real posture instead of re-deriving it from a
// source (the config file) that does not know the bind address. gin's
// Set/Get only accept string keys, so this constant is the key contract.
const authStateCtxKey = "pi-switch.auth"

func withAuthState(opts MgmtAuthOptions, c *gin.Context) {
	mode := opts
	mode.Password = "" // never keep the secret in the request context
	c.Set(authStateCtxKey, mode)
}

// effectiveBindHost resolves the posture for a request. An empty bind host means
// every interface, so the safe reading is the wildcard, not local.
func effectiveBindHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "0.0.0.0"
	}
	return host
}

// requestAuthOptions returns the auth mode in effect for this request. A
// request that never passed the auth-state middleware carries no state, which
// only happens when the router installed no guard (loopback), so a loopback
// posture is the honest answer there.
func requestAuthOptions(c *gin.Context) MgmtAuthOptions {
	if v, ok := c.Get(authStateCtxKey); ok {
		if opts, ok := v.(MgmtAuthOptions); ok {
			return opts
		}
	}
	return MgmtAuthOptions{BindHost: "127.0.0.1"}
}

// MgmtAuthOptions carries the *actual* bind address of the listener together
// with the resolved password. The bind address comes from the CLI flag and is
// never written back to config, so it — not cfg.Settings.Web.Host — is the only
// trustworthy input for deciding whether the API is exposed.
type MgmtAuthOptions struct {
	BindHost string
	Password string
	// GeneratePassword lets an operator explicitly ask for a generated
	// credential when binding beyond loopback.
	GeneratePassword bool
}

// ValidateBindAuth is the startup guard: binding the management API beyond
// loopback without a password must refuse to start rather than serve
// unauthenticated. An empty bind host counts as non-loopback because it binds
// every interface. GeneratePassword is the explicit opt-in that makes the
// operator's choice visible instead of silently inventing credentials.
func ValidateBindAuth(opts MgmtAuthOptions) error {
	if IsLoopback(opts.BindHost) || strings.TrimSpace(opts.Password) != "" || opts.GeneratePassword {
		return nil
	}
	return errors.New("refusing to start on non-loopback host " + opts.BindHost +
		" without a password: set PI_SWITCH_WEBUI_PASSWORD (the shared management/proxy password), create the password file, or pass --generate-password")
}

// GenerateAndStorePassword creates a random password, persists it to the
// password file (0600) and returns it so the caller can print it exactly once.
func GenerateAndStorePassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	pw := hex.EncodeToString(buf)
	path := webUIPasswordPath()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(pw+"\n"), 0600); err != nil {
		return "", err
	}
	return pw, nil
}

// adminUser is the fixed username of the single management credential; the
// password is the only secret. Kept next to the middleware that enforces it.
const adminUser = "admin"

// StoredWebUIPassword reads the persisted credential only, ignoring the
// environment.
func StoredWebUIPassword() string {
	b, err := os.ReadFile(webUIPasswordPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// WebUIPasswordConfigured reports the credential a listener would enforce right
// now: the persisted file first, then the environment. Read-only callers such as
// `pi-switch doctor` use it instead of re-deriving the rule.
func WebUIPasswordConfigured() string {
	if pw := StoredWebUIPassword(); pw != "" {
		return pw
	}
	return strings.TrimSpace(os.Getenv("PI_SWITCH_WEBUI_PASSWORD"))
}

// ResolveAuthOptions turns the actual bind address into the auth options the
// routers enforce. It is the single startup entry: main resolves here, and the
// daemon child resolves the identical inputs in its own process.
//
// Precedence is generate > password file > environment. The file outranks the
// environment so that a generated credential is the one actually enforced (and
// so a parent and the daemon child it spawns agree on the same secret);
// --generate-password outranks both because asking for one is explicit.
func ResolveAuthOptions(bindHost string, generate bool, announce func(string)) (MgmtAuthOptions, error) {
	password := ""
	switch {
	case generate:
		pw, err := GenerateAndStorePassword()
		if err != nil {
			return MgmtAuthOptions{}, err
		}
		password = pw
		if announce != nil {
			announce("generated management/proxy password (also stored in " + webUIPasswordPath() + "): " + pw)
		}
	default:
		password = StoredWebUIPassword()
		if password == "" {
			password = strings.TrimSpace(os.Getenv("PI_SWITCH_WEBUI_PASSWORD"))
		}
	}
	opts := MgmtAuthOptions{BindHost: bindHost, Password: password}
	if err := ValidateBindAuth(opts); err != nil {
		return MgmtAuthOptions{}, err
	}
	return opts, nil
}

// basicAuthMiddleware protects the guarded prefixes with the single
// admin:<password> pair. Health probes are deliberately left reachable so that a
// misconfigured listener stays diagnosable; proxies pass their own prefix set
// instead of getting a second authentication implementation.
func basicAuthMiddleware(password string, guardedPrefixes ...string) gin.HandlerFunc {
	if len(guardedPrefixes) == 0 {
		guardedPrefixes = []string{"/api"}
	}
	expected := adminUser + ":" + password
	reject := func(c *gin.Context) {
		c.Header("WWW-Authenticate", `Basic realm="pi-switch"`)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
	}
	return func(c *gin.Context) {
		guarded := false
		for _, prefix := range guardedPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				guarded = true
				break
			}
		}
		if !guarded {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Basic ") {
			reject(c)
			return
		}
		dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		if err != nil || string(dec) != expected {
			reject(c)
			return
		}
		c.Next()
	}
}

// --- static handlers ---
func handleWebUIIndex(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	// try embed
	if data, err := webUIFS.ReadFile("dist/index.html"); err == nil && len(data) > 0 {
		c.Data(200, "text/html; charset=utf-8", data)
		return
	}
	// also try without prefix (depending on embed root)
	if data, err := fs.ReadFile(webUIFS, "index.html"); err == nil {
		c.Data(200, "text/html; charset=utf-8", data)
		return
	}
	c.Data(200, "text/html; charset=utf-8", []byte(`<!doctype html><html><head><meta charset="utf-8"><title>pi-switch</title></head><body><div id="root">pi-switch WebUI placeholder</div><p>embed.FS placeholder for webui/dist</p></body></html>`))
}

func handleWebUIFallback(c *gin.Context) {
	rawPath := c.Request.URL.Path
	// prevent directory traversal - check raw path contains ..
	if strings.Contains(rawPath, "..") {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	path := strings.TrimPrefix(rawPath, "/")
	if path == "" {
		handleWebUIIndex(c)
		return
	}
	// API already handled via group NoRoute? But gin NoRoute catches all not matched. For /api/* unknown, return 404 json.
	if strings.HasPrefix(rawPath, "/api/") {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// try serve file from embed
	clean := filepath.Clean(path)
	// prevent directory traversal after clean as well
	if strings.Contains(clean, "..") {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// try webUIFS
	tryPaths := []string{
		"dist/" + clean,
		clean,
	}
	for _, tp := range tryPaths {
		if data, err := webUIFS.ReadFile(tp); err == nil {
			// guess MIME
			ct := "application/octet-stream"
			if strings.HasSuffix(tp, ".html") {
				ct = "text/html; charset=utf-8"
			} else if strings.HasSuffix(tp, ".js") {
				ct = "application/javascript"
			} else if strings.HasSuffix(tp, ".css") {
				ct = "text/css"
			} else if strings.HasSuffix(tp, ".json") {
				ct = "application/json"
			} else if strings.HasSuffix(tp, ".svg") {
				ct = "image/svg+xml"
			}
			// cache header
			if strings.HasPrefix(clean, "assets/") || strings.HasPrefix(tp, "dist/assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			} else if strings.HasSuffix(tp, ".html") {
				c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			c.Data(200, ct, data)
			return
		}
	}
	// SPA fallback: serve index.html
	handleWebUIIndex(c)
}

func handleAssets(c *gin.Context) {
	rawPath := c.Request.URL.Path
	if strings.Contains(rawPath, "..") {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	filepathParam := c.Param("filepath")
	// gin wildcard includes leading slash
	clean := strings.TrimPrefix(filepathParam, "/")
	clean = filepath.Clean(clean)
	if clean == "." {
		clean = ""
	}
	if strings.Contains(clean, "..") {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if clean == "" {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// try from dist/assets via sub FS
	if sub, err := fs.Sub(webUIFS, "dist/assets"); err == nil {
		if data, err := fs.ReadFile(sub, clean); err == nil {
			ct := "application/octet-stream"
			if strings.HasSuffix(clean, ".js") {
				ct = "application/javascript"
			} else if strings.HasSuffix(clean, ".css") {
				ct = "text/css"
			} else if strings.HasSuffix(clean, ".json") {
				ct = "application/json"
			} else if strings.HasSuffix(clean, ".svg") {
				ct = "image/svg+xml"
			} else if strings.HasSuffix(clean, ".html") {
				ct = "text/html; charset=utf-8"
			}
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
			c.Data(200, ct, data)
			return
		}
	}
	// fallback try direct read
	tryPaths := []string{
		"dist/assets/" + clean,
		"assets/" + clean,
	}
	for _, tp := range tryPaths {
		if data, err := webUIFS.ReadFile(tp); err == nil {
			ct := "application/octet-stream"
			if strings.HasSuffix(tp, ".js") {
				ct = "application/javascript"
			} else if strings.HasSuffix(tp, ".css") {
				ct = "text/css"
			} else if strings.HasSuffix(tp, ".json") {
				ct = "application/json"
			} else if strings.HasSuffix(tp, ".svg") {
				ct = "image/svg+xml"
			}
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
			c.Data(200, ct, data)
			return
		}
	}
	c.JSON(404, gin.H{"error": "not found"})
}

// --- basic config handlers ---

func saveConfig(cfg config.PiSwitchConfig) error {
	return config.SaveAtPath(cfg, configPath())
}

func sessionScanCandidates(source string) map[string]scan.PiSession {
	if source != "sessionScan" {
		return nil
	}
	return scan.ScanCached()
}

func conversationCandidates(sessions map[string]scan.PiSession) []conversation.Candidate {
	candidates := make([]conversation.Candidate, 0, len(sessions))
	for id, sess := range sessions {
		candidateID := sess.ID
		if candidateID == "" {
			candidateID = id
		}
		if sess.LastActiveAt == nil {
			continue
		}
		lastActive, err := time.Parse(time.RFC3339Nano, *sess.LastActiveAt)
		if err != nil {
			continue
		}
		model := ""
		if sess.Model != nil {
			model = *sess.Model
		}
		candidates = append(candidates, conversation.Candidate{
			ID:               candidateID,
			Name:             sess.Title,
			Model:            model,
			LastActiveAt:     lastActive,
			PromptTokensHint: sess.PromptTokensHint,
		})
	}
	return candidates
}

// effectiveConversationID is kept as a compatibility adapter for older
// package-local callers; all attribution still runs through conversation.Match.
func effectiveConversationID(convID, convName, provider, model, ts sql.NullString, source string, sessions map[string]scan.PiSession) (string, string) {
	var timestamp time.Time
	if ts.Valid {
		timestamp, _ = time.Parse(time.RFC3339Nano, ts.String)
	}
	result := conversation.Match(conversation.Source(source), conversation.MatchInput{
		ExplicitID:   convID.String,
		ExplicitName: convName.String,
		Provider:     provider.String,
		Model:        model.String,
		Timestamp:    timestamp,
	}, conversationCandidates(sessions))
	return result.ID, result.Name
}
