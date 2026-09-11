package server

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
)

type gatewayPreviewRequest struct {
	Selected *[]gateway.GatewaySelection `json:"selected"`
	Draft    map[string]interface{}      `json:"draft"`
}

func handleGetGateway(c *gin.Context) {
	path := gateway.ModelsPath()
	b, err := os.ReadFile(path)
	if err != nil {
		c.JSON(200, gin.H{"gateway": nil})
		return
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	c.JSON(200, gin.H{"gateway": m})
}

func handleGatewayPreview(c *gin.Context) {
	serveGatewayPreview(c, nil, true)
}

func handleGatewayPreviewPost(c *gin.Context) {
	var request gatewayPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid gateway preview request"})
		return
	}
	generated := true
	var edited map[string]interface{}
	if request.Selected != nil {
		cfg, configErr := loadConfig()
		if configErr != nil {
			c.JSON(500, gin.H{"error": configErr.Error()})
			return
		}
		var err error
		if request.Draft != nil {
			edited, err = gateway.ApplyGatewaySelectionToDraft(cfg, request.Draft, *request.Selected)
		} else {
			edited, err = gateway.BuildSelectedGatewayEntry(cfg, *request.Selected)
		}
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		generated = false
	} else if request.Draft != nil {
		edited = request.Draft
		generated = false
	}
	serveGatewayPreview(c, edited, generated)
}

// serveGatewayPreview renders one preview. generated must be stated by the
// caller: nil edited only happens for a generated preview, but the plan kind is
// never inferred from nil.
func serveGatewayPreview(c *gin.Context, edited map[string]interface{}, generated bool) {
	cfg, configErr := loadConfig()
	if configErr != nil {
		c.JSON(500, gin.H{"error": configErr.Error()})
		return
	}
	current, err := gateway.ReadCurrent()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	var plan gateway.CanonicalGatewayPlan
	var summary catalog.EnrichSummary
	if generated {
		plan, summary = buildGeneratedGatewayPlan(cfg, current)
	} else {
		plan, summary = buildDraftGatewayPlan(cfg, current, edited)
	}
	currentForResp := interface{}(map[string]interface{}{})
	if provs, ok := plan.Current["providers"].(map[string]interface{}); ok {
		currentForResp = provs
	}
	var proposedForResp interface{}
	if provs, ok := plan.Proposed["providers"].(map[string]interface{}); ok {
		proposedForResp = provs
	}
	c.JSON(200, gin.H{
		"current":         currentForResp,
		"proposed":        proposedForResp,
		"conflicts":       plan.Conflicts,
		"diagnostics":     plan.Diagnostics,
		"pending_count":   plan.PendingCount,
		"groups":          plan.Groups,
		"removed":         plan.PreviewRemoved,
		"diff":            gin.H{"added": plan.Added, "removed": plan.Removed, "changed": plan.Changed},
		"enrich":          gin.H{"enriched": summary.Enriched, "skipped": summary.Skipped, "stale": summary.Stale, "warning": summary.Warning},
		"fixed_providers": gateway.FixedGatewayProviders(),
	})
}

// buildGeneratedGatewayPlan is the shared generated flow: the gateway package
// builds, enriches and plans from one proposal.
func buildGeneratedGatewayPlan(cfg config.PiSwitchConfig, current map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	return gateway.BuildEnrichedGeneratedPlan(cfg, current)
}

// buildDraftGatewayPlan builds the canonical plan for a user draft, enriching the
// draft from the catalog first so the plan's derived views match what is published.
func buildDraftGatewayPlan(cfg config.PiSwitchConfig, current, draft map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	summary := gateway.EnrichProposedModels(cfg, draft)
	return gateway.BuildDraftPlan(cfg, current, draft), summary
}

// PublishedAuthCaveat reports the authentication caveat for the providers a plan
// publishes, or "" when there is none.
//
// The caveat exists because the published entries carry `"apiKey":
// "pi-switch-proxy"`, which a client sends as a Bearer token, while the proxy's /v1
// surface requires HTTP Basic whenever it is bound beyond loopback — and a bind
// beyond loopback always has a password, because ValidateBindAuth refuses to start
// without one.
//
// It deliberately does NOT judge the bind address of the listener answering the
// request: where the management API listens says nothing about whether the proxy
// surface authenticates (a loopback webui with an exposed proxy is a supported
// pairing, and warning there would be noise). And it cannot judge the published
// baseUrl alone either: BuildProposedGatewayEntry rewrites a wildcard or empty host
// to 127.0.0.1, so the plan reads as local exactly when the proxy is exposed. The
// configured proxy host is therefore the primary source, with the plan's own
// baseUrls as a second, so a hand-edited plan is judged too.
//
// Residual limit: a proxy started with a --host that differs from the configured one
// is invisible here; the running daemon's host would be the exact source.
func PublishedAuthCaveat(cfg config.PiSwitchConfig, published map[string]interface{}) string {
	if !IsLoopback(cfg.Settings.Proxy.Host) || planReachesLan(published) {
		return publishedAuthCaveatText
	}
	return ""
}

// publishedAuthCaveatText is the single copy of the operator-facing wording, shared
// by the HTTP responses and `pi-switch gateway publish`.
const publishedAuthCaveatText = `the proxy is exposed beyond loopback, so its /v1 surface requires HTTP Basic authentication, but the published providers carry "apiKey": "pi-switch-proxy", which clients send as a Bearer token — such clients get 401. Bind the proxy to loopback, or use a client that can send Basic.`

// planReachesLan reports whether any published entry points a client at a host
// beyond loopback. The empty host and an unparsable URL are skipped: an entry
// without a usable baseUrl makes no claim about reachability.
func planReachesLan(published map[string]interface{}) bool {
	providers, _ := published["providers"].(map[string]interface{})
	for _, raw := range providers {
		entry, _ := raw.(map[string]interface{})
		base, _ := entry["baseUrl"].(string)
		u, err := url.Parse(base)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if !IsLoopback(u.Hostname()) {
			return true
		}
	}
	return false
}

// gatewayPublishOK is the success body for a publish, with the caveat attached only
// when there is one (a loopback proxy stays byte-identical to before).
func gatewayPublishOK(cfg config.PiSwitchConfig, published map[string]interface{}) gin.H {
	resp := gin.H{"ok": true}
	if caveat := PublishedAuthCaveat(cfg, published); caveat != "" {
		resp["warnings"] = []string{caveat}
	}
	return resp
}

func handlePutGateway(c *gin.Context) {
	raw, _ := c.GetRawData()
	var gw map[string]interface{}
	if err := json.Unmarshal(raw, &gw); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	if gw == nil {
		c.JSON(400, gin.H{"error": "providers is required"})
		return
	}
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	current, currentErr := gateway.ReadCurrent()
	if currentErr != nil {
		c.JSON(500, gin.H{"error": currentErr.Error()})
		return
	}
	plan, _ := buildDraftGatewayPlan(cfg, current, gw)
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := gateway.PublishPlan(plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(cfg, gw))
}

// gatewayHealthPayload reports the gateway's state. has_models_file and
// upstreams_total are read from reality; the rest are constants, and deliberately
// so: the gateway is a logical concept ("logical-isolation" — publishing
// ~/.pi/agent/models.json), so there is no process to report and no notification
// record to date. has_models_file used to be one of those constants (literally
// `true`), which made "already published" always true for a frontend that decodes
// it as a required boolean.
func gatewayHealthPayload(cfg config.PiSwitchConfig) gin.H {
	_, statErr := os.Stat(gateway.ModelsPath())
	return gin.H{
		"running":         true,
		"mode":            "logical-isolation",
		"gateway_id":      "pi-switch",
		"has_models_file": statErr == nil,
		"last_notify":     nil,
		"upstreams_total": len(cfg.Profiles),
		"message":         "ok",
	}
}

func handleGatewayHealth(c *gin.Context) {
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	c.JSON(200, gatewayHealthPayload(cfg))
}
func handleGatewayStart(c *gin.Context) {
	c.JSON(200, gin.H{"running": true, "mode": "logical-isolation", "gateway_id": "pi-switch"})
}

func handleGatewayPublish(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	var body map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			c.JSON(400, gin.H{"error": "invalid json"})
			return
		}
		if len(raw) > 0 && body == nil {
			c.JSON(400, gin.H{"error": "providers is required"})
			return
		}
	}
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	current, currentErr := gateway.ReadCurrent()
	if currentErr != nil {
		c.JSON(500, gin.H{"error": currentErr.Error()})
		return
	}
	var toPublish map[string]interface{}
	var plan gateway.CanonicalGatewayPlan
	if body != nil && len(body) > 0 {
		if _, ok := body["providers"]; !ok {
			c.JSON(400, gin.H{"error": "providers is required"})
			return
		}
		toPublish = body
		plan, _ = buildDraftGatewayPlan(cfg, current, toPublish)
	} else {
		toPublish = gateway.BuildProposedGatewayEntry(cfg)
		plan, _ = buildGeneratedGatewayPlan(cfg, current)
	}
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := gateway.PublishPlan(plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(cfg, toPublish))
}
