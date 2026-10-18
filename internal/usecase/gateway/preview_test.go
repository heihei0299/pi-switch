package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	domaincatalog "github.com/heihei0299/pi-switch/internal/domain/catalog"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

type previewConfig struct {
	cfg   domainconfig.PiSwitchConfig
	err   error
	reads int
}

func (c *previewConfig) Load() (domainconfig.PiSwitchConfig, string, error) {
	c.reads++
	return c.cfg, "memory", c.err
}

type previewStore struct {
	fakeStore
	err   error
	reads int
}

func (s *previewStore) Read(context.Context) (map[string]interface{}, error) {
	s.reads++
	return s.current, s.err
}

type previewCatalog struct {
	err    error
	reads  int
	cancel context.CancelFunc
}

func (c *previewCatalog) Snapshot(context.Context) (domaincatalog.Snapshot, bool, string, error) {
	c.reads++
	if c.cancel != nil {
		c.cancel()
	}
	return domaincatalog.Snapshot{}, true, "cached catalog", c.err
}

func previewFixture() (*previewConfig, *previewStore, *previewCatalog) {
	cfg := &previewConfig{cfg: domainconfig.PiSwitchConfig{Profiles: map[string]domainconfig.ProviderProfile{
		"supplier": {Upstreams: []domainconfig.Upstream{{Name: stringPtr("main"), API: domaingateway.ChatAPI, APIKey: "fake-preview-secret", Models: []domainconfig.ModelEntry{{ID: "m1"}}, ExposedModels: []string{"m1"}}}},
	}}}
	cfg.cfg.Settings.Proxy.Host = "0.0.0.0"
	cfg.cfg.Settings.Proxy.Port = 43112
	store := &previewStore{fakeStore: fakeStore{current: map[string]interface{}{"providers": map[string]interface{}{}}}}
	return cfg, store, &previewCatalog{}
}

func TestPreviewFlowsReadOnceWithoutPublishing(t *testing.T) {
	for _, flow := range []string{"generated", "draft", "selected", "selected-draft"} {
		t.Run(flow, func(t *testing.T) {
			cfg, store, catalog := previewFixture()
			svc := gatewayusecase.New(cfg, store, catalog)
			draft := domaingateway.BuildProposedGatewayEntry(cfg.cfg)
			selection := []domaingateway.GatewaySelection{{Supplier: "supplier", Channel: "main", Model: "m1"}}
			var preview gatewayusecase.Preview
			var err error
			switch flow {
			case "generated":
				preview, err = svc.PreviewGenerated(context.Background())
			case "draft":
				preview, err = svc.PreviewDraft(context.Background(), draft)
			case "selected-draft":
				preview, err = svc.PreviewSelectedDraft(context.Background(), draft, selection)
			case "selected":
				var selected gatewayusecase.SelectedPlan
				selected, preview.Enrich, err = svc.PreviewSelected(context.Background(), selection)
				preview.Plan = selected.Plan
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.reads != 1 || store.reads != 1 || catalog.reads != 1 || len(store.published) != 0 {
				t.Fatalf("reads=%d/%d/%d publishes=%d", cfg.reads, store.reads, catalog.reads, len(store.published))
			}
			if len(preview.Plan.Conflicts) != 0 || !preview.Enrich.Stale || preview.Enrich.Warning != "cached catalog" {
				t.Fatalf("preview = %+v", preview)
			}
			if flow != "selected" && preview.ProxyHost != "0.0.0.0" {
				t.Fatal("lost configured host for publish warning")
			}
			encoded, err := json.Marshal(preview)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "fake-preview-secret") {
				t.Fatal("preview leaked config credentials")
			}
		})
	}
}

func TestPreviewSelectedDistinguishesNilAndEmpty(t *testing.T) {
	cfg, store, catalog := previewFixture()
	store.current = domaingateway.BuildProposedGatewayEntry(cfg.cfg)
	svc := gatewayusecase.New(cfg, store, catalog)
	inherited, _, err := svc.PreviewSelected(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inherited.Selected) != 1 || inherited.Plan.PendingCount != 0 {
		t.Fatalf("nil selection = %+v", inherited)
	}
	empty, _, err := svc.PreviewSelected(context.Background(), []domaingateway.GatewaySelection{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Selected) != 0 || empty.Plan.PendingCount == 0 {
		t.Fatalf("explicit empty selection = %+v", empty)
	}
}

func TestPreviewFailureStopsBeforeLaterDependencies(t *testing.T) {
	for _, stage := range []string{"config", "current", "catalog", "canceled", "catalog-canceled"} {
		t.Run(stage, func(t *testing.T) {
			cfg, store, catalog := previewFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("injected failure")
			wantReads := []int{1, 1, 1}
			switch stage {
			case "config":
				cfg.err = failure
				wantReads = []int{1, 0, 0}
			case "current":
				store.err = failure
				wantReads = []int{1, 1, 0}
			case "catalog":
				catalog.err = failure
			case "canceled":
				cancel()
				failure = context.Canceled
				wantReads = []int{0, 0, 0}
			case "catalog-canceled":
				catalog.cancel = cancel
				failure = context.Canceled
			}
			_, err := gatewayusecase.New(cfg, store, catalog).PreviewGenerated(ctx)
			if !errors.Is(err, failure) {
				t.Fatalf("error=%v want=%v", err, failure)
			}
			if got := []int{cfg.reads, store.reads, catalog.reads}; !reflect.DeepEqual(got, wantReads) || len(store.published) != 0 {
				t.Fatalf("reads=%v want=%v publishes=%d", got, wantReads, len(store.published))
			}
		})
	}
}

func TestPreviewSelectedDraftInvalidSelectionPrecedesCurrentRead(t *testing.T) {
	cfg, store, catalog := previewFixture()
	_, err := gatewayusecase.New(cfg, store, catalog).PreviewSelectedDraft(context.Background(), nil, []domaingateway.GatewaySelection{{Supplier: "missing", Channel: "main", Model: "m1"}})
	var selectionError *gatewayusecase.SelectionError
	if !errors.As(err, &selectionError) || store.reads != 0 || catalog.reads != 0 {
		t.Fatalf("error=%v reads=%d/%d", err, store.reads, catalog.reads)
	}
}
