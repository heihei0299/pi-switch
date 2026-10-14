package sqlanalytics

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
)

const timestampEpochMillisSQL = "(CAST(strftime('%s', ts) AS INTEGER) * 1000 + CASE WHEN instr(ts, '.') = 0 THEN 0 WHEN substr(ts, instr(ts, '.') + 3, 1) GLOB '[0-9]' THEN CAST(substr(ts, instr(ts, '.') + 1, 3) AS INTEGER) WHEN substr(ts, instr(ts, '.') + 2, 1) GLOB '[0-9]' THEN CAST(substr(ts, instr(ts, '.') + 1, 2) AS INTEGER) * 10 ELSE CAST(substr(ts, instr(ts, '.') + 1, 1) AS INTEGER) * 100 END)"

const requestFactsColumns = "id,ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms,status,error,upstream_url"

type Repository struct {
	DB *sql.DB
}

func New(db *sql.DB) Repository { return Repository{DB: db} }

func (r Repository) ForEachFact(ctx context.Context, window *analytics.Window, order analytics.Order, visit func(analytics.RequestFact) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.DB == nil {
		return fmt.Errorf("stats database is nil")
	}
	query := "SELECT " + requestFactsColumns + " FROM requests"
	args := []any{}
	if window != nil {
		query += ` WHERE ` + timestampEpochMillisSQL + ` >= ? AND ` + timestampEpochMillisSQL + ` < ?`
		args = append(args, window.From, window.To)
	}
	direction := "DESC"
	if order == analytics.OldestFirst {
		direction = "ASC"
	}
	query += ` ORDER BY id ` + direction
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		fact, err := scanFact(rows)
		if err != nil {
			return err
		}
		if err := visit(fact); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r Repository) PageFacts(ctx context.Context, window *analytics.Window, page, limit int) ([]analytics.RequestFact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.DB == nil {
		return nil, fmt.Errorf("stats database is nil")
	}
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	query := "SELECT " + requestFactsColumns + " FROM requests"
	args := []any{}
	if window != nil {
		query += ` WHERE ` + timestampEpochMillisSQL + ` >= ? AND ` + timestampEpochMillisSQL + ` < ?`
		args = append(args, window.From, window.To)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, page*limit)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := make([]analytics.RequestFact, 0, limit)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fact, err := scanFact(rows)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return facts, nil
}

func scanFact(rows *sql.Rows) (analytics.RequestFact, error) {
	var id int64
	var ts, provider, model, convID, convName, errMsg, upstreamURL sql.NullString
	var success, prompt, completion, cached, reasoning, latency, status sql.NullInt64
	var cost sql.NullFloat64
	if err := rows.Scan(&id, &ts, &provider, &model, &success, &prompt, &completion, &cached, &reasoning, &cost, &convID, &convName, &latency, &status, &errMsg, &upstreamURL); err != nil {
		return analytics.RequestFact{}, err
	}
	return analytics.RequestFact{
		ID: id, TS: nullableString(ts), Provider: nullableString(provider), Model: nullableString(model),
		Success: nullableBool(success), Status: nullableInt(status), Error: nullableString(errMsg), UpstreamURL: nullableString(upstreamURL),
		PromptTokens: nullableInt(prompt), CompletionTokens: nullableInt(completion), CachedTokens: nullableInt(cached),
		ReasoningTokens: nullableInt(reasoning), Cost: nullableFloat(cost), ConversationID: nullableString(convID),
		ConversationName: nullableString(convName), LatencyMs: nullableInt(latency),
	}, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullableInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func nullableBool(value sql.NullInt64) *bool {
	if !value.Valid {
		return nil
	}
	valueBool := value.Int64 == 1
	return &valueBool
}
