package gateway_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domaincatalog "github.com/heihei0299/pi-switch/internal/domain/catalog"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

type fakeStore struct {
	current   map[string]interface{}
	exists    bool
	published []domaingateway.CanonicalGatewayPlan
}

func (s *fakeStore) Read(context.Context) (map[string]interface{}, error) { return s.current, nil }
func (s *fakeStore) Exists(context.Context) bool                          { return s.exists }
func (s *fakeStore) Publish(_ context.Context, plan domaingateway.CanonicalGatewayPlan) error {
	s.published = append(s.published, plan)
	return nil
}

type fakeCatalog struct{ snapshot domaincatalog.Snapshot }

func (c fakeCatalog) Snapshot(context.Context) (domaincatalog.Snapshot, bool, string, error) {
	return c.snapshot, false, "", nil
}

func TestCurrentExistsUsesStoreAndHandlesUnavailableStore(t *testing.T) {
	if gatewayusecase.New(nil, nil).CurrentExists(context.Background()) {
		t.Fatal("unavailable store reported models.json as existing")
	}
	if !gatewayusecase.New(&fakeStore{exists: true}, nil).CurrentExists(context.Background()) {
		t.Fatal("existing models.json was not reported")
	}
}

func TestPublishNoOpDoesNotCallStore(t *testing.T) {
	store := &fakeStore{}
	service := gatewayusecase.New(store, fakeCatalog{})
	if err := service.Publish(context.Background(), domaingateway.CanonicalGatewayPlan{Proposed: map[string]interface{}{"providers": map[string]interface{}{}}, PendingCount: 0}); err != nil {
		t.Fatal(err)
	}
	if len(store.published) != 0 {
		t.Fatalf("publish calls = %d, want 0", len(store.published))
	}
}

func TestPublishUsesThePreviewedPlanAndHonorsCancellation(t *testing.T) {
	store := &fakeStore{}
	service := gatewayusecase.New(store, fakeCatalog{})
	plan := domaingateway.CanonicalGatewayPlan{
		Current:      map[string]interface{}{"providers": map[string]interface{}{}},
		Proposed:     map[string]interface{}{"providers": map[string]interface{}{"pi-switch-chat": map[string]interface{}{"models": []interface{}{}}}},
		PendingCount: 1,
	}
	if err := service.Publish(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(store.published) != 1 || !reflect.DeepEqual(store.published[0], plan) {
		t.Fatalf("published plans = %#v, want the exact previewed plan %#v", store.published, plan)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Publish(ctx, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled publish error = %v, want context.Canceled", err)
	}
	if len(store.published) != 1 {
		t.Fatalf("canceled publish reached storage; calls = %d", len(store.published))
	}
}

func TestGeneratedAndDraftPlanUseDifferentCatalogFillPolicies(t *testing.T) {
	const catalogJSON = `{"acme":{"models":{"m1":{"name":"Catalog name","reasoning":true,"modalities":{"input":["text","image"]},"limit":{"context":200,"output":20},"cost":{"input":1,"output":2,"cache_read":3}}}}}`
	snapshot, err := domaincatalog.ParseSnapshot([]byte(catalogJSON))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{current: map[string]interface{}{"providers": map[string]interface{}{}}}
	service := gatewayusecase.New(store, fakeCatalog{snapshot: snapshot})
	cfg := domainconfig.PiSwitchConfig{Profiles: map[string]domainconfig.ProviderProfile{
		"acme": {ModelsDevProvider: stringPtr("acme"), Upstreams: []domainconfig.Upstream{{
			Name: stringPtr("main"), API: domaingateway.ChatAPI,
			Models:        []domainconfig.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10, Name: stringPtr("Explicit name")}},
			ExposedModels: []string{"m1"},
		}}},
	}}
	generated, _, err := service.BuildGeneratedPlan(context.Background(), cfg, store.current)
	if err != nil {
		t.Fatal(err)
	}
	draft := map[string]interface{}{"providers": map[string]interface{}{domaingateway.ChatProvider: map[string]interface{}{
		"api": domaingateway.ChatAPI, "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy",
		"models": []interface{}{map[string]interface{}{"id": "m1", "contextWindow": float64(100), "maxTokens": float64(10), "name": "Explicit name"}},
	}}}
	draftPlan, _, err := service.BuildDraftPlan(context.Background(), cfg, store.current, draft)
	if err != nil {
		t.Fatal(err)
	}
	generatedModel := generated.Proposed["providers"].(map[string]interface{})[domaingateway.ChatProvider].(map[string]interface{})["models"].([]interface{})[0].(map[string]interface{})
	draftModel := draftPlan.Proposed["providers"].(map[string]interface{})[domaingateway.ChatProvider].(map[string]interface{})["models"].([]interface{})[0].(map[string]interface{})
	if generatedModel["contextWindow"] != float64(200) || draftModel["contextWindow"] != float64(100) {
		t.Fatalf("generated/draft context windows = %v / %v", generatedModel["contextWindow"], draftModel["contextWindow"])
	}
}

func stringPtr(value string) *string { return &value }
