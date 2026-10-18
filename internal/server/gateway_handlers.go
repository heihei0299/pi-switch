package server

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

type gatewayPreviewRequest struct {
	Selected *[]domaingateway.GatewaySelection `json:"selected"`
	Draft    map[string]interface{}            `json:"draft"`
}

// handleGetGateway answers the current published Gateway. It reads through the one
// Gateway read boundary, so a missing file is an empty current, while an unreadable
// or corrupt models.json is an error — the server no longer invents a `gateway:
// null` for both.
func handleGetGateway(c *gin.Context) {
	current, err := bootstrap.NewGatewayService().ReadCurrent(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"gateway": current})
}

func handleGatewayPreview(c *gin.Context) {
	preview, err := bootstrap.NewGatewayService().PreviewGenerated(c.Request.Context())
	writeGatewayPreview(c, preview, err)
}

func handleGatewayPreviewPost(c *gin.Context) {
	var request gatewayPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid gateway preview request"})
		return
	}
	service := bootstrap.NewGatewayService()
	var preview gatewayusecase.Preview
	var err error
	switch {
	case request.Selected != nil:
		preview, err = service.PreviewSelectedDraft(c.Request.Context(), request.Draft, *request.Selected)
	case request.Draft != nil:
		preview, err = service.PreviewDraft(c.Request.Context(), request.Draft)
	default:
		preview, err = service.PreviewGenerated(c.Request.Context())
	}
	writeGatewayPreview(c, preview, err)
}

func writeGatewayPreview(c *gin.Context, preview gatewayusecase.Preview, err error) {
	if err != nil {
		status := 500
		var selectionError *gatewayusecase.SelectionError
		if errors.As(err, &selectionError) {
			status = 400
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	plan, summary := preview.Plan, preview.Enrich
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
		"fixed_providers": domaingateway.FixedGatewayProviders(),
	})
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
// surface authenticates (a loopback management API with an exposed proxy is a supported
// pairing, and warning there would be noise). And it cannot judge the published
// baseUrl alone either: BuildProposedGatewayEntry rewrites a wildcard or empty host
// to 127.0.0.1, so the plan reads as local exactly when the proxy is exposed. The
// configured proxy host is therefore the primary source, with the plan's own
// baseUrls as a second, so a hand-edited plan is judged too.
//
// Only pi-switch's own providers are judged: a third-party provider the plan
// carries over from models.json publishes someone else's endpoint, so where it
// points says nothing about this proxy's authentication. That also means the caveat
// needs at least one pi-switch provider with a model to be part of what the plan
// publishes: a plan that publishes none of them (every model unexposed, a fixed
// provider left with an empty model list, or a hand-kept third-party provider as the
// only entry) cannot produce the 401 this warns about, so it stays silent instead of
// complaining about entries no client can call.
//
// The entry's apiKey value is deliberately not part of the judgement. A Pi client
// sends whatever sits in that field as a Bearer token, so a hand-edited fixed
// provider carrying a real key gets the same 401 a placeholder-keyed one gets: the
// hazard is the provider, not the literal placeholder. The reverse case needs no
// handling — ValidateProposedGateway rejects any non-fixed provider whose apiKey is
// the placeholder ("unsupported third pi-switch provider"), so the only placeholder
// entries that can ever be published are the two fixed ones.
//
// Residual limit: a proxy started with a --host that differs from the configured one
// is invisible here; the running daemon's host would be the exact source.
func PublishedAuthCaveat(proxyHost string, published map[string]interface{}) string {
	if !publishesFixedGatewayProvider(published) {
		return ""
	}
	if !IsLoopback(proxyHost) || planReachesLan(published) {
		return publishedAuthCaveatText
	}
	return ""
}

// publishesFixedGatewayProvider reports whether the plan publishes one of pi-switch's
// own providers with at least one model — the only entries a client reads from
// models.json and can actually call against this proxy. A fixed provider carrying an
// empty model list is published but unroutable (ValidateGatewayProvider accepts
// `"models": []`), so it cannot produce the 401 either.
func publishesFixedGatewayProvider(published map[string]interface{}) bool {
	providers, _ := published["providers"].(map[string]interface{})
	for key, raw := range providers {
		if !domaingateway.IsFixedGatewayProvider(key) {
			continue
		}
		entry, _ := raw.(map[string]interface{})
		if models, ok := entry["models"].([]interface{}); ok && len(models) > 0 {
			return true
		}
	}
	return false
}

// publishedAuthCaveatText is the single copy of the operator-facing wording, shared
// by the HTTP responses and `pi-switch gateway publish`. It describes the mechanism
// rather than naming the placeholder: the judgement deliberately still warns for a
// hand-edited fixed provider carrying a real key, where claiming the published
// entries carry `apiKey: pi-switch-proxy` would misdescribe what was published.
const publishedAuthCaveatText = `the proxy is exposed beyond loopback, so its /v1 surface requires HTTP Basic authentication, while clients reach the published pi-switch providers with a Bearer token — such clients get 401. Bind the proxy to loopback, or use a client that can send Basic.`

// planReachesLan reports whether any published pi-switch entry points a client at a
// host beyond loopback. The empty host and an unparsable URL are skipped: an entry
// without a usable baseUrl makes no claim about reachability.
func planReachesLan(published map[string]interface{}) bool {
	providers, _ := published["providers"].(map[string]interface{})
	for key, raw := range providers {
		if !domaingateway.IsFixedGatewayProvider(key) {
			continue
		}
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
func gatewayPublishOK(proxyHost string, published map[string]interface{}) gin.H {
	resp := gin.H{"ok": true}
	if caveat := PublishedAuthCaveat(proxyHost, published); caveat != "" {
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
	service := bootstrap.NewGatewayService()
	preview, err := service.PreviewDraft(c.Request.Context(), gw)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	plan := preview.Plan
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := service.Publish(c.Request.Context(), plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(preview.ProxyHost, plan.Proposed))
}

// gatewayHealthPayload reports the gateway's state. has_models_file and
// upstreams_total are read from reality; the rest are constants, and deliberately
// so: the gateway is a logical concept ("logical-isolation" — publishing
// ~/.pi/agent/models.json), so there is no process to report and no notification
// record to date. has_models_file used to be one of those constants (literally
// `true`), which made "already published" always true for clients that rely on
// it as a required boolean.
func gatewayHealthPayload(cfg domainconfig.PiSwitchConfig, hasModelsFile bool) gin.H {
	return gin.H{
		"running":         true,
		"mode":            "logical-isolation",
		"gateway_id":      "pi-switch",
		"has_models_file": hasModelsFile,
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
	c.JSON(200, gatewayHealthPayload(cfg, bootstrap.NewGatewayService().CurrentExists(c.Request.Context())))
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
	if len(body) > 0 {
		if _, ok := body["providers"]; !ok {
			c.JSON(400, gin.H{"error": "providers is required"})
			return
		}
	}
	service := bootstrap.NewGatewayService()
	var preview gatewayusecase.Preview
	if len(body) > 0 {
		preview, err = service.PreviewDraft(c.Request.Context(), body)
	} else {
		preview, err = service.PreviewGenerated(c.Request.Context())
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	plan := preview.Plan
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := service.Publish(c.Request.Context(), plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(preview.ProxyHost, plan.Proposed))
}
