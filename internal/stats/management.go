package stats

import (
	"database/sql"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/scan"
	"github.com/heihei0299/pi-switch/internal/store"
)

// NewManagementService binds Stats to the configured conversation source. Both
// the management API and Native UI use this constructor.
func NewManagementService(db *sql.DB) (Service, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return Service{}, err
	}
	source := conversation.Source(cfg.Settings.ConversationSource)
	return Service{
		DB:         db,
		Source:     source,
		Candidates: managementConversationCandidates(source),
	}, nil
}

// OpenManagementService opens the configured request database and constructs
// the management Stats service without going through an HTTP adapter.
func OpenManagementService() (Service, error) {
	db, err := store.GetDB()
	if err != nil {
		return Service{}, err
	}
	return NewManagementService(db)
}

func managementConversationCandidates(source conversation.Source) []conversation.Candidate {
	if source != conversation.SourceSessionScan {
		return nil
	}
	sessions := scan.ScanCached()
	candidates := make([]conversation.Candidate, 0, len(sessions))
	for id, sess := range sessions {
		candidateID := sess.ID
		if candidateID == "" {
			candidateID = id
		}
		if sess.LastActiveAt == nil {
			continue
		}
		lastActive, err := time.Parse(time.RFC3339Nano, *sess.LastActiveAt)
		if err != nil {
			continue
		}
		model := ""
		if sess.Model != nil {
			model = *sess.Model
		}
		candidates = append(candidates, conversation.Candidate{
			ID:               candidateID,
			Name:             sess.Title,
			Model:            model,
			LastActiveAt:     lastActive,
			PromptTokensHint: sess.PromptTokensHint,
		})
	}
	return candidates
}
