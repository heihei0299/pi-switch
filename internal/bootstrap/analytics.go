package bootstrap

import (
	"database/sql"
	"time"

	"github.com/heihei0299/pi-switch/internal/adapters/outbound/sqlanalytics"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/domain/conversation"
	"github.com/heihei0299/pi-switch/internal/scan"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
)

func NewManagementAnalyticsService(db *sql.DB) (analytics.Service, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return analytics.Service{}, err
	}
	source := conversation.Source(cfg.Settings.ConversationSource)
	return analytics.NewService(sqlanalytics.New(db), source, managementConversationCandidates(source)), nil
}

func OpenManagementAnalyticsService() (analytics.Service, error) {
	db, err := store.GetDB()
	if err != nil {
		return analytics.Service{}, err
	}
	return NewManagementAnalyticsService(db)
}

func NewSummaryAnalyticsService(db *sql.DB) analytics.Service {
	return analytics.NewService(sqlanalytics.New(db), conversation.SourceOff, nil)
}

func OpenSummaryAnalyticsService() (analytics.Service, error) {
	db, err := store.GetDB()
	if err != nil {
		return analytics.Service{}, err
	}
	return NewSummaryAnalyticsService(db), nil
}

func managementConversationCandidates(source conversation.Source) []conversation.Candidate {
	if source != conversation.SourceSessionScan {
		return nil
	}
	sessions := scan.ScanCached()
	candidates := make([]conversation.Candidate, 0, len(sessions))
	for id, session := range sessions {
		candidateID := session.ID
		if candidateID == "" {
			candidateID = id
		}
		if session.LastActiveAt == nil {
			continue
		}
		lastActive, err := time.Parse(time.RFC3339Nano, *session.LastActiveAt)
		if err != nil {
			continue
		}
		model := ""
		if session.Model != nil {
			model = *session.Model
		}
		candidates = append(candidates, conversation.Candidate{
			ID: candidateID, Name: session.Title, Model: model,
			LastActiveAt: lastActive, PromptTokensHint: session.PromptTokensHint,
		})
	}
	return candidates
}
