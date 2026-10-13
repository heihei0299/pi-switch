// Package catalog preserves the legacy catalog entrypoints. Pure lookup and fill
// rules live in domain/catalog; cache and network IO live in outbound/catalog.
package catalog

import (
	"context"

	outboundcatalog "github.com/heihei0299/pi-switch/internal/adapters/outbound/catalog"
	domaincatalog "github.com/heihei0299/pi-switch/internal/domain/catalog"
)

const (
	FetchURLVar     = outboundcatalog.FetchURLVar
	CachePathVar    = outboundcatalog.CachePathVar
	DefaultFetchURL = outboundcatalog.DefaultFetchURL
	TTL             = outboundcatalog.TTL
	FetchTimeout    = outboundcatalog.FetchTimeout
)

type Meta = domaincatalog.Meta
type Snapshot = domaincatalog.Snapshot
type EnrichSummary = domaincatalog.EnrichSummary

func CachePath() string                           { return outboundcatalog.CachePath() }
func ParseSnapshot(data []byte) (Snapshot, error) { return domaincatalog.ParseSnapshot(data) }
func FillMissing(entry map[string]interface{}, meta Meta) bool {
	return domaincatalog.FillMissing(entry, meta)
}
func FillOverwrite(entry map[string]interface{}, meta Meta) bool {
	return domaincatalog.FillOverwrite(entry, meta)
}
func FillModels(models []interface{}, snapshot Snapshot) (int, int) {
	return domaincatalog.FillModels(models, snapshot)
}

func Ensure() (Snapshot, bool, string) {
	snapshot, stale, warning, _ := outboundcatalog.New().Snapshot(context.Background())
	return snapshot, stale, warning
}
