package requestlog

import (
	"context"
	"errors"
	"testing"
)

type recordingDatabase struct {
	calls int
	err   error
}

func (s *recordingDatabase) Insert(context.Context, Record) error {
	s.calls++
	return s.err
}

type recordingLegacy struct {
	calls int
	err   error
}

func (s *recordingLegacy) Append(context.Context, Record) error {
	s.calls++
	return s.err
}

func TestRequestLogSQLiteFailureStillWritesLegacy(t *testing.T) {
	database := &recordingDatabase{err: errors.New("sqlite unavailable")}
	legacy := &recordingLegacy{}
	result := New(database, legacy).Record(context.Background(), Record{Provider: "provider", Model: "model"})
	if result.DatabaseError == nil {
		t.Fatal("database failure was not returned")
	}
	if result.LegacyError != nil {
		t.Fatalf("legacy write failed: %v", result.LegacyError)
	}
	if database.calls != 1 || legacy.calls != 1 {
		t.Fatalf("calls = database %d, legacy %d; want 1, 1", database.calls, legacy.calls)
	}
}

func TestRequestLogLegacyFailureKeepsSQLite(t *testing.T) {
	database := &recordingDatabase{}
	legacy := &recordingLegacy{err: errors.New("legacy unavailable")}
	result := New(database, legacy).Record(context.Background(), Record{Provider: "provider", Model: "model"})
	if result.DatabaseError != nil {
		t.Fatalf("database write failed: %v", result.DatabaseError)
	}
	if result.LegacyError == nil {
		t.Fatal("legacy failure was not returned")
	}
	if database.calls != 1 || legacy.calls != 1 {
		t.Fatalf("calls = database %d, legacy %d; want 1, 1", database.calls, legacy.calls)
	}
}
