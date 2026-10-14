package requestlog

import (
	"context"
	"errors"

	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

// Record is the completed request fact written to both compatibility stores.
type Record struct {
	TS               string
	Provider         string
	Model            string
	Success          bool
	Usage            *usage.UsageSummary
	Cost             *float64
	ConversationID   string
	ConversationName string
	LatencyMs        int64
	Status           int
	Error            string
	UpstreamURL      string
}

type Database interface {
	Insert(context.Context, Record) error
}

type Legacy interface {
	Append(context.Context, Record) error
}

type Service struct {
	database Database
	legacy   Legacy
}

func New(database Database, legacy Legacy) Service {
	return Service{database: database, legacy: legacy}
}

// Result keeps the two compatibility writes independent: one failed target
// must not prevent the other target from receiving the request fact.
type Result struct {
	DatabaseError error
	LegacyError   error
}

func (s Service) Record(ctx context.Context, record Record) Result {
	result := Result{}
	if s.database == nil {
		result.DatabaseError = errors.New("request log database unavailable")
	} else {
		result.DatabaseError = s.database.Insert(ctx, record)
	}
	if s.legacy == nil {
		result.LegacyError = errors.New("request log legacy sink unavailable")
	} else {
		result.LegacyError = s.legacy.Append(ctx, record)
	}
	return result
}
