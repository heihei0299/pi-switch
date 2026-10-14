package stats

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/store"
)

func BenchmarkStatsHistory(b *testing.B) {
	for _, count := range []int{10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			db := benchmarkStatsDB(b, count)
			defer db.Close()
			base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
			candidates := make([]conversation.Candidate, 128)
			for i := range candidates {
				candidates[i] = conversation.Candidate{
					ID: fmt.Sprintf("session-%02d", i), Model: "model", LastActiveAt: base.Add(time.Duration(i) * time.Minute),
				}
			}
			for _, tc := range []struct {
				name    string
				source  conversation.Source
				matches []conversation.Candidate
			}{
				{name: "source_off", source: conversation.SourceOff},
				{name: "session_scan_128", source: conversation.SourceSessionScan, matches: candidates},
			} {
				b.Run(tc.name, func(b *testing.B) {
					service := Service{DB: db, Source: tc.source, Candidates: tc.matches}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := service.Stats(nil, 0, 50); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func benchmarkStatsDB(b *testing.B, count int) *sql.DB {
	b.Helper()
	db, err := store.Open(filepath.Join(b.TempDir(), "requests.db"))
	if err != nil {
		b.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,latency_ms) VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < count; i++ {
		success := 1
		if i%10 == 0 {
			success = 0
		}
		if _, err := statement.Exec("2026-09-10T12:00:00Z", "supplier", "model", success, 100, 20, 10, 2, 0.001, 10); err != nil {
			b.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	return db
}
