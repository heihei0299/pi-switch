package stats

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/heihei0299/pi-switch/internal/conversation"
)

// Window is an inclusive/exclusive epoch-millisecond range.
type Window struct {
	From int64
	To   int64
}

type Service struct {
	DB         *sql.DB
	Source     conversation.Source
	Candidates []conversation.Candidate
}

// RequestFact keeps database nullability until the DTO boundary. In
// particular, missing token facts are not silently converted to zero.
type RequestFact struct {
	ID               int64
	TS               *string
	Provider         *string
	Model            *string
	Success          *bool
	PromptTokens     *int64
	CompletionTokens *int64
	CachedTokens     *int64
	ReasoningTokens  *int64
	Cost             *float64
	ConversationID   *string
	ConversationName *string
	LatencyMs        *int64
}

type ProviderStat struct {
	Total        int      `json:"total"`
	OK           int      `json:"ok"`
	Failed       int      `json:"failed"`
	PromptTokens int64    `json:"promptTokens"`
	OutputTokens int64    `json:"outputTokens"`
	CachedTokens int64    `json:"cachedTokens"`
	Reasoning    int64    `json:"reasoningTokens"`
	Cost         *float64 `json:"cost"`
	CacheRate    string   `json:"cacheRate"`
}

type ModelStat struct {
	Total        int      `json:"total"`
	OK           int      `json:"ok"`
	PromptTokens int64    `json:"promptTokens"`
	OutputTokens int64    `json:"outputTokens"`
	CachedTokens int64    `json:"cachedTokens"`
	Reasoning    int64    `json:"reasoningTokens"`
	Cost         *float64 `json:"cost"`
	CacheRate    string   `json:"cacheRate"`
}

type ConversationSummary struct {
	ConversationID string   `json:"conversationId"`
	Name           *string  `json:"name"`
	Requests       int      `json:"requests"`
	InputTokens    int64    `json:"inputTokens"`
	OutputTokens   int64    `json:"outputTokens"`
	CachedTokens   int64    `json:"cachedTokens"`
	Reasoning      int64    `json:"reasoningTokens"`
	CacheRate      string   `json:"cacheRate"`
	LastActive     *string  `json:"lastActive"`
	Cost           *float64 `json:"cost"`
}

type RequestDTO struct {
	TS               *string  `json:"ts"`
	Provider         *string  `json:"provider"`
	Model            *string  `json:"model"`
	OK               *bool    `json:"ok"`
	Status           *int     `json:"status"`
	Error            any      `json:"error"`
	PromptTokens     *int64   `json:"promptTokens"`
	CompletionTokens *int64   `json:"completionTokens"`
	CachedTokens     *int64   `json:"cachedTokens"`
	ReasoningTokens  *int64   `json:"reasoningTokens"`
	TotalTokens      *int64   `json:"totalTokens"`
	CacheRate        string   `json:"cacheRate"`
	Cost             *float64 `json:"cost"`
	CostTotal        *float64 `json:"costTotal,omitempty"`
	ConversationID   *string  `json:"conversationId"`
	ConversationName *string  `json:"conversationName"`

	PromptTokensSnake     *int64  `json:"prompt_tokens"`
	CompletionTokensSnake *int64  `json:"completion_tokens"`
	CachedTokensSnake     *int64  `json:"cached_tokens"`
	ReasoningTokensSnake  *int64  `json:"reasoning_tokens"`
	ConversationIDS       *string `json:"conversation_id"`
	ConversationNameS     *string `json:"conversation_name"`
	Success               *bool   `json:"success"`
}

type ConversationRequestDTO struct {
	TS               *string  `json:"ts"`
	Provider         *string  `json:"provider"`
	Model            *string  `json:"model"`
	OK               *bool    `json:"ok"`
	Status           *int     `json:"status"`
	Error            any      `json:"error"`
	PromptTokens     *int64   `json:"promptTokens"`
	CompletionTokens *int64   `json:"completionTokens"`
	CachedTokens     *int64   `json:"cachedTokens"`
	ReasoningTokens  *int64   `json:"reasoningTokens"`
	TotalTokens      *int64   `json:"totalTokens"`
	CacheRate        string   `json:"cacheRate"`
	Cost             *float64 `json:"cost"`
	ConversationID   *string  `json:"conversationId"`
	ConversationName *string  `json:"conversationName"`
}

type StatsResponse struct {
	TotalRequests       int                     `json:"totalRequests"`
	OKRequests          int                     `json:"okRequests"`
	FailedRequests      int                     `json:"failedRequests"`
	SuccessRate         string                  `json:"successRate"`
	AvgLatencyMs        int64                   `json:"avgLatencyMs"`
	ByProvider          map[string]ProviderStat `json:"byProvider"`
	ByModel             map[string]ModelStat    `json:"byModel"`
	TotalTokens         map[string]int64        `json:"totalTokens"`
	CacheHitRate        string                  `json:"cacheHitRate"`
	TotalCost           *float64                `json:"totalCost"`
	CostUnknown         int64                   `json:"costUnknown"`
	ByConversation      []ConversationSummary   `json:"byConversation"`
	RecentRequests      []RequestDTO            `json:"recentRequests"`
	RecentRequestTotal  int                     `json:"recentRequestTotal"`
	Rows                []RequestDTO            `json:"rows"`
	RecentRequestTotalS int                     `json:"recent_request_total"`
}

func (s Service) Stats(window *Window, page, limit int) (StatsResponse, error) {
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	var response StatsResponse
	response.ByProvider = map[string]ProviderStat{}
	response.ByModel = map[string]ModelStat{}
	response.ByConversation = []ConversationSummary{}
	response.RecentRequests = []RequestDTO{}
	response.Rows = []RequestDTO{}
	var totalLatency, latencyCount int64
	var totalInput, totalOutput, totalCached, totalReasoning int64
	var totalCost *float64
	var conversations map[string]*conversationAggregate
	if s.Source != conversation.SourceOff {
		conversations = map[string]*conversationAggregate{}
	}
	factIndex := 0

	err := s.forEachFact(window, "DESC", func(fact RequestFact) error {
		response.TotalRequests++
		if fact.Success != nil && *fact.Success {
			response.OKRequests++
		}
		if fact.LatencyMs != nil {
			totalLatency += *fact.LatencyMs
			latencyCount++
		}
		countable := fact.countable()
		if countable {
			totalInput += *fact.PromptTokens
			totalOutput += *fact.CompletionTokens
			if fact.CachedTokens != nil {
				totalCached += *fact.CachedTokens
			}
			if fact.ReasoningTokens != nil {
				totalReasoning += *fact.ReasoningTokens
			}
			if fact.Cost != nil {
				totalCost = addCost(totalCost, *fact.Cost)
			} else {
				response.CostUnknown++
			}
		}

		provider := fact.valueOr(fact.Provider, "unknown")
		providerStat := response.ByProvider[provider]
		providerStat.Total++
		if fact.Success != nil && *fact.Success {
			providerStat.OK++
		} else {
			providerStat.Failed++
		}
		if countable {
			providerStat.PromptTokens += *fact.PromptTokens
			providerStat.OutputTokens += *fact.CompletionTokens
			if fact.CachedTokens != nil {
				providerStat.CachedTokens += *fact.CachedTokens
			}
			if fact.ReasoningTokens != nil {
				providerStat.Reasoning += *fact.ReasoningTokens
			}
			if fact.Cost != nil {
				providerStat.Cost = addCost(providerStat.Cost, *fact.Cost)
			}
		}
		response.ByProvider[provider] = providerStat

		model := fact.valueOr(fact.Model, "unknown")
		modelStat := response.ByModel[model]
		modelStat.Total++
		if fact.Success != nil && *fact.Success {
			modelStat.OK++
		}
		if countable {
			modelStat.PromptTokens += *fact.PromptTokens
			modelStat.OutputTokens += *fact.CompletionTokens
			if fact.CachedTokens != nil {
				modelStat.CachedTokens += *fact.CachedTokens
			}
			if fact.ReasoningTokens != nil {
				modelStat.Reasoning += *fact.ReasoningTokens
			}
			if fact.Cost != nil {
				modelStat.Cost = addCost(modelStat.Cost, *fact.Cost)
			}
		}
		response.ByModel[model] = modelStat

		var result conversation.MatchResult
		if s.Source != conversation.SourceOff {
			result = s.match(fact)
			aggregateConversation(conversations, fact, result, countable)
		}
		factIndex++
		return nil
	})
	if err != nil {
		return StatsResponse{}, err
	}
	recentFacts, err := s.pageFacts(window, page, limit)
	if err != nil {
		return StatsResponse{}, err
	}
	for _, fact := range recentFacts {
		var result conversation.MatchResult
		if s.Source != conversation.SourceOff {
			result = s.match(fact)
		}
		response.RecentRequests = append(response.RecentRequests, requestDTO(fact, result, s.Source))
	}

	for provider, value := range response.ByProvider {
		value.CacheRate = cacheRate(value.PromptTokens, value.CachedTokens)
		response.ByProvider[provider] = value
	}
	for model, value := range response.ByModel {
		value.CacheRate = cacheRate(value.PromptTokens, value.CachedTokens)
		response.ByModel[model] = value
	}
	if conversations != nil {
		response.ByConversation = conversationSummaries(conversations)
	}
	response.RecentRequestTotal = factIndex
	response.RecentRequestTotalS = factIndex
	response.Rows = response.RecentRequests
	if latencyCount > 0 {
		response.AvgLatencyMs = totalLatency / latencyCount
	}
	response.FailedRequests = response.TotalRequests - response.OKRequests
	response.SuccessRate = "0%"
	if response.TotalRequests > 0 {
		response.SuccessRate = fmt.Sprintf("%.1f%%", float64(response.OKRequests)/float64(response.TotalRequests)*100)
	}
	response.TotalTokens = map[string]int64{
		"input": totalInput, "output": totalOutput, "total": totalInput + totalOutput,
		"cached": totalCached, "reasoning": totalReasoning,
	}
	response.CacheHitRate = cacheRate(totalInput, totalCached)
	response.TotalCost = totalCost
	return response, nil
}

func (s Service) Conversations(window *Window, page, limit int) ([]ConversationSummary, int, error) {
	if s.Source == conversation.SourceOff {
		return []ConversationSummary{}, 0, nil
	}
	groups := map[string]*conversationAggregate{}
	err := s.forEachFact(window, "DESC", func(fact RequestFact) error {
		aggregateConversation(groups, fact, s.match(fact), fact.countable())
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	list := conversationSummaries(groups)
	return pageItems(list, page, limit), len(list), nil
}

func (s Service) ConversationRequests(id string, page, limit int) ([]ConversationRequestDTO, int, error) {
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	start := page * limit
	end := start + limit
	matched := make([]ConversationRequestDTO, 0, limit)
	total := 0
	err := s.forEachFact(nil, "DESC", func(fact RequestFact) error {
		if s.match(fact).ID != id {
			return nil
		}
		if total >= start && total < end {
			matched = append(matched, detailRequestDTO(fact, id))
		}
		total++
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return matched, total, nil
}

func scanFact(rows *sql.Rows) (RequestFact, error) {
	var id int64
	var ts, provider, model, convID, convName sql.NullString
	var success, prompt, completion, cached, reasoning, latency sql.NullInt64
	var cost sql.NullFloat64
	if err := rows.Scan(&id, &ts, &provider, &model, &success, &prompt, &completion, &cached, &reasoning, &cost, &convID, &convName, &latency); err != nil {
		return RequestFact{}, err
	}
	return RequestFact{
		ID: id, TS: nullableString(ts), Provider: nullableString(provider), Model: nullableString(model),
		Success: nullableBool(success), PromptTokens: nullableInt(prompt), CompletionTokens: nullableInt(completion),
		CachedTokens: nullableInt(cached), ReasoningTokens: nullableInt(reasoning), Cost: nullableFloat(cost),
		ConversationID: nullableString(convID), ConversationName: nullableString(convName), LatencyMs: nullableInt(latency),
	}, nil
}

func (s Service) pageFacts(window *Window, page, limit int) ([]RequestFact, error) {
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id,ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms FROM requests`
	args := []any{}
	if window != nil {
		query += ` WHERE julianday(ts) >= julianday(? / 1000.0, 'unixepoch') AND julianday(ts) < julianday(? / 1000.0, 'unixepoch')`
		args = append(args, float64(window.From), float64(window.To))
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, page*limit)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := make([]RequestFact, 0, limit)
	for rows.Next() {
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

func (s Service) forEachFact(window *Window, order string, visit func(RequestFact) error) error {
	if s.DB == nil {
		return fmt.Errorf("stats database is nil")
	}
	query := `SELECT id,ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms FROM requests`
	args := []any{}
	if window != nil {
		query += ` WHERE julianday(ts) >= julianday(? / 1000.0, 'unixepoch') AND julianday(ts) < julianday(? / 1000.0, 'unixepoch')`
		args = append(args, float64(window.From), float64(window.To))
	}
	if order != "ASC" {
		order = "DESC"
	}
	query += ` ORDER BY id ` + order
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
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

func (f RequestFact) countable() bool {
	return f.Success != nil && *f.Success && f.PromptTokens != nil && f.CompletionTokens != nil
}

func (f RequestFact) valueOr(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}

func (s Service) match(fact RequestFact) conversation.MatchResult {
	var timestamp time.Time
	if fact.TS != nil {
		timestamp, _ = parseTimestamp(*fact.TS)
	}
	var promptTokens *uint64
	if fact.PromptTokens != nil && *fact.PromptTokens >= 0 {
		value := uint64(*fact.PromptTokens)
		promptTokens = &value
	}
	return conversation.Match(s.Source, conversation.MatchInput{
		ExplicitID:   valueString(fact.ConversationID),
		ExplicitName: valueString(fact.ConversationName),
		Provider:     valueString(fact.Provider),
		Model:        valueString(fact.Model),
		Timestamp:    timestamp,
		PromptTokens: promptTokens,
	}, s.Candidates)
}

type conversationAggregate struct {
	ID        string
	Name      *string
	Requests  int
	Input     int64
	Output    int64
	Cached    int64
	Reasoning int64
	Cost      *float64
	Last      *string
}

func aggregateConversation(groups map[string]*conversationAggregate, fact RequestFact, result conversation.MatchResult, countable bool) {
	if groups == nil {
		return
	}
	group := groups[result.ID]
	if group == nil {
		group = &conversationAggregate{ID: result.ID}
		groups[result.ID] = group
	}
	group.Requests++
	if result.Name != "" {
		name := result.Name
		group.Name = &name
	} else if group.Name == nil && fact.ConversationName != nil && *fact.ConversationName != "" {
		name := *fact.ConversationName
		group.Name = &name
	}
	if fact.TS != nil && (group.Last == nil || *fact.TS > *group.Last) {
		last := *fact.TS
		group.Last = &last
	}
	if !countable {
		return
	}
	group.Input += *fact.PromptTokens
	group.Output += *fact.CompletionTokens
	if fact.CachedTokens != nil {
		group.Cached += *fact.CachedTokens
	}
	if fact.ReasoningTokens != nil {
		group.Reasoning += *fact.ReasoningTokens
	}
	if fact.Cost != nil {
		group.Cost = addCost(group.Cost, *fact.Cost)
	}
}

func conversationSummaries(groups map[string]*conversationAggregate) []ConversationSummary {
	list := make([]ConversationSummary, 0, len(groups))
	for _, group := range groups {
		list = append(list, ConversationSummary{
			ConversationID: group.ID, Name: group.Name, Requests: group.Requests,
			InputTokens: group.Input, OutputTokens: group.Output, CachedTokens: group.Cached,
			Reasoning: group.Reasoning, CacheRate: cacheRate(group.Input, group.Cached), LastActive: group.Last, Cost: group.Cost,
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		left, right := stringValue(list[i].LastActive), stringValue(list[j].LastActive)
		if left != right {
			return left > right
		}
		return list[i].ConversationID < list[j].ConversationID
	})
	return list
}

func requestDTO(fact RequestFact, result conversation.MatchResult, source conversation.Source) RequestDTO {
	dto := RequestDTO{
		TS: fact.TS, Provider: fact.Provider, Model: fact.Model, Error: nil, CacheRate: "-",
		Cost: fact.Cost, CostTotal: fact.Cost,
	}
	if fact.Success != nil {
		ok := *fact.Success
		dto.OK = &ok
		dto.Success = &ok
		status := 500
		if ok {
			status = 200
		}
		dto.Status = &status
	}
	if fact.countable() {
		dto.PromptTokens = fact.PromptTokens
		dto.PromptTokensSnake = fact.PromptTokens
		dto.CompletionTokens = fact.CompletionTokens
		dto.CompletionTokensSnake = fact.CompletionTokens
		cached := int64(0)
		if fact.CachedTokens != nil {
			cached = *fact.CachedTokens
		}
		dto.CachedTokens = &cached
		dto.CachedTokensSnake = &cached
		reasoning := int64(0)
		if fact.ReasoningTokens != nil {
			reasoning = *fact.ReasoningTokens
		}
		dto.ReasoningTokens = &reasoning
		dto.ReasoningTokensSnake = &reasoning
		total := *fact.PromptTokens + *fact.CompletionTokens
		dto.TotalTokens = &total
		dto.CacheRate = cacheRate(*fact.PromptTokens, cached)
	}
	if source != conversation.SourceOff {
		id := result.ID
		dto.ConversationID = &id
		dto.ConversationIDS = &id
		if result.Name != "" {
			name := result.Name
			dto.ConversationName = &name
			dto.ConversationNameS = &name
		}
	} else if fact.ConversationID != nil {
		dto.ConversationID = fact.ConversationID
		dto.ConversationIDS = fact.ConversationID
	}
	if fact.ConversationName != nil && *fact.ConversationName != "" && (source == conversation.SourceOff || dto.ConversationName == nil) {
		dto.ConversationName = fact.ConversationName
		dto.ConversationNameS = fact.ConversationName
	}
	return dto
}

func detailRequestDTO(fact RequestFact, id string) ConversationRequestDTO {
	dto := ConversationRequestDTO{
		TS: fact.TS, Provider: fact.Provider, Model: fact.Model, Error: nil, CacheRate: "-",
		ConversationID: stringPtr(id), ConversationName: fact.ConversationName, Cost: fact.Cost,
	}
	if fact.Success != nil {
		ok := *fact.Success
		dto.OK = &ok
		status := 500
		if ok {
			status = 200
		}
		dto.Status = &status
	}
	if fact.countable() {
		dto.PromptTokens = fact.PromptTokens
		dto.CompletionTokens = fact.CompletionTokens
		cached := int64(0)
		if fact.CachedTokens != nil {
			cached = *fact.CachedTokens
		}
		dto.CachedTokens = &cached
		reasoning := int64(0)
		if fact.ReasoningTokens != nil {
			reasoning = *fact.ReasoningTokens
		}
		dto.ReasoningTokens = &reasoning
		total := *fact.PromptTokens + *fact.CompletionTokens
		dto.TotalTokens = &total
		dto.CacheRate = cacheRate(*fact.PromptTokens, cached)
	}
	return dto
}

func pageItems[T any](items []T, page, limit int) []T {
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	start := page * limit
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	if items == nil {
		return []T{}
	}
	return items[start:end]
}

func cacheRate(input, cached int64) string {
	if input == 0 {
		return "-"
	}
	if cached == 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", float64(cached)/float64(input)*100)
}

func addCost(current *float64, value float64) *float64 {
	if current == nil {
		result := value
		return &result
	}
	*current += value
	return current
}

func parseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
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
	result := value.Int64 == 1
	return &result
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
