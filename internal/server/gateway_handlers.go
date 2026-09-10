package server

import (
	"encoding/json"
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
	serveGatewayPreview(c, nil)
}

func handleGatewayPreviewPost(c *gin.Context) {
	var request gatewayPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid gateway preview request"})
		return
	}
	var edited map[string]interface{}
	if request.Selected != nil {
		cfg, _, configErr := config.LoadConfigAtPath(configPath())
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
	} else if request.Draft != nil {
		edited = request.Draft
	}
	serveGatewayPreview(c, edited)
}

func serveGatewayPreview(c *gin.Context, edited map[string]interface{}) {
	cfg, _, configErr := config.LoadConfigAtPath(configPath())
	if configErr != nil {
		c.JSON(500, gin.H{"error": configErr.Error()})
		return
	}
	current, err := gateway.ReadCurrent()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	plan, summary := buildGatewayPlan(cfg, current, edited)
	currentForResp := interface{}(map[string]interface{}{})
	if provs, ok := plan.Current["providers"].(map[string]interface{}); ok {
		currentForResp = provs
	}
	var proposedForResp interface{}
	if provs, ok := plan.Proposed["providers"].(map[string]interface{}); ok {
		proposedForResp = provs
	}
	c.JSON(200, gin.H{
		"current":       currentForResp,
		"proposed":      proposedForResp,
		"conflicts":     plan.Conflicts,
		"diagnostics":   plan.Diagnostics,
		"pending_count": plan.PendingCount,
		"groups":        plan.Groups,
		"removed":       plan.PreviewRemoved,
		"diff":          gin.H{"added": plan.Added, "removed": plan.Removed, "changed": plan.Changed},
		"enrich":        gin.H{"enriched": summary.Enriched, "skipped": summary.Skipped, "stale": summary.Stale, "warning": summary.Warning},
	})
}
func buildGatewayPlan(cfg config.PiSwitchConfig, current, draft map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	proposal := draft
	if proposal == nil {
		proposal = gateway.BuildProposedGatewayEntry(cfg)
	}
	summary := enrichProposedModels(proposal)
	return gateway.BuildCanonicalGatewayPlanWithPublishedMetadata(cfg, current, proposal, draft == nil), summary
}

// enrichProposedModels fills gateway proposed models from the models.dev snapshot.
// It prefers provider/bare lookup using the supplier's resolved modelsDevProvider
// (or supplier name as hint) to disambiguate duplicate bare ids, and overwrites
// stale defaults (e.g. 128000→1048576) when the catalog provides non-zero values.
func enrichProposedModels(proposed map[string]interface{}) catalog.EnrichSummary {
	snap, stale, warning := catalog.Ensure()
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	enriched, skipped := 0, 0
	// Fixed gateway providers use bare ids; resolve catalog metadata through the originating supplier.
	modelSuppliers := map[string]string{}
	ambiguousModelSuppliers := map[string]bool{}
	for supplier, prof := range cfg.Profiles {
		for _, upstream := range prof.Upstreams {
			for _, id := range upstream.ExposedModels {
				if previous, ok := modelSuppliers[id]; ok && previous != supplier {
					delete(modelSuppliers, id)
					ambiguousModelSuppliers[id] = true
					continue
				}
				if !ambiguousModelSuppliers[id] {
					modelSuppliers[id] = supplier
				}
			}
		}
	}
	if provs, ok := proposed["providers"].(map[string]interface{}); ok {
		for providerKey, pv := range provs {
			entry, ok := pv.(map[string]interface{})
			if !ok {
				continue
			}
			models, _ := entry["models"].([]interface{})
			// Non-fixed provider entries use the provider key as their supplier hint.
			supplierHint := providerKey
			if idx := strings.Index(providerKey, "/"); idx > 0 {
				supplierHint = providerKey[:idx]
			}
			for _, m := range models {
				mm, ok := m.(map[string]interface{})
				if !ok {
					skipped++
					continue
				}
				id, _ := mm["id"].(string)
				supplier := supplierHint
				if providerKey == "pi-switch-res" || providerKey == "pi-switch-chat" {
					supplier = modelSuppliers[id]
				}
				if id == "" {
					skipped++
					continue
				}
				var meta catalog.Meta
				var found bool
				if supplier != "" {
					if prof, ok := cfg.Profiles[supplier]; ok {
						pk := resolveModelsDevProvider(prof)
						if pk != "" {
							meta, found = snap.LookupWithProvider(id, pk)
						}
					}
					if !found {
						meta, found = snap.LookupWithProvider(id, supplier)
					}
				}
				if !found {
					meta, found = snap.Lookup(id)
				}
				if !found {
					skipped++
					continue
				}
				if catalog.FillOverwrite(mm, meta) {
					enriched++
				} else {
					skipped++
				}
			}
		}
		return catalog.EnrichSummary{Enriched: enriched, Skipped: skipped, Stale: stale, Warning: warning}
	}
	return catalog.EnrichSummary{Enriched: enriched, Skipped: skipped, Stale: stale, Warning: warning}
}

// gatewayPublishAuthWarning reports a limitation an operator cannot see from the
// published file: the providers pi-switch writes carry `"apiKey":
// "pi-switch-proxy"` (internal/gateway/gateway.go), which a client sends as a
// Bearer token, while the proxy guard only accepts HTTP Basic
// (basicAuthMiddleware). Beyond loopback the guard is always installed, because
// ValidateBindAuth refuses to start without a password — so there the published
// providers cannot authenticate at all.
//
// The bind address is the only runtime fact available, and it is exactly why this
// cannot be derived from the config file: an empty host means every interface.
// Judging by it can only over-warn when the webui and the proxy are started with
// different hosts, which is the safe direction.
//
// Nothing here may include a credential: the whole point of warning (rather than
// publishing a working key) is that the shared password must not land in
// ~/.pi/agent/models.json.
func gatewayPublishAuthWarning(c *gin.Context) []string {
	if IsLoopback(effectiveBindHost(requestAuthOptions(c).BindHost)) {
		return nil
	}
	return []string{`this listener is bound beyond loopback, so /v1 requires HTTP Basic authentication, but the published providers carry "apiKey": "pi-switch-proxy", which clients send as a Bearer token — such clients get 401. Point them at a Basic-capable configuration, or bind the proxy to loopback.`}
}

// gatewayPublishOK is the success body for a publish, with the warning attached
// only when there is one (a loopback publish stays byte-identical to before).
func gatewayPublishOK(c *gin.Context) gin.H {
	resp := gin.H{"ok": true}
	if warnings := gatewayPublishAuthWarning(c); len(warnings) > 0 {
		resp["warnings"] = warnings
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
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	current, currentErr := gateway.ReadCurrent()
	if currentErr != nil {
		c.JSON(500, gin.H{"error": currentErr.Error()})
		return
	}
	plan, _ := buildGatewayPlan(cfg, current, gw)
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := gateway.PublishPlan(plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(c))
}

// gatewayHealthPayload reports the gateway's state. has_models_file and
// upstreams_total are read from reality; the rest are constants, and deliberately
// so: the gateway is a logical concept ("logical-isolation" — publishing
// ~/.pi/agent/models.json), so there is no process to report and no notification
// record to date. has_models_file used to be one of those constants (literally
// `true`), which made "already published" always true for a frontend that decodes
// it as a required boolean.
func gatewayHealthPayload(c *gin.Context) gin.H {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
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
	c.JSON(200, gatewayHealthPayload(c))
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
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	current, currentErr := gateway.ReadCurrent()
	if currentErr != nil {
		c.JSON(500, gin.H{"error": currentErr.Error()})
		return
	}
	var toPublish map[string]interface{}
	if body != nil && len(body) > 0 {
		if _, ok := body["providers"]; ok {
			toPublish = body
		} else {
			c.JSON(400, gin.H{"error": "providers is required"})
			return
		}
	} else {
		toPublish = gateway.BuildProposedGatewayEntry(cfg)
	}
	plan, _ := buildGatewayPlan(cfg, current, toPublish)
	if len(plan.Conflicts) > 0 {
		c.JSON(400, gin.H{"error": strings.Join(plan.Conflicts, "; ")})
		return
	}
	if err := gateway.PublishPlan(plan); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gatewayPublishOK(c))
}
