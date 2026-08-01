package cursor

import (
	"errors"
	"fmt"
	"sort"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
)

// SessionData is parsed cursor-agent CLI storage for one session store.db file.
type SessionData struct {
	SessionID   string
	StoreDBPath string
	Bubbles     map[string]*RawBubble
	Composers   []*RawComposer
	Contexts    map[string][]*MessageContext
	ParseStats  SessionParseStats
	TokenUsage  SessionTokenUsage
}

// SessionParseStats summarizes readable vs unavailable blob records while parsing.
type SessionParseStats struct {
	BlobCount            int
	ReadableBlobCount    int
	UnavailableBlobCount int
	MalformedBlobCount   int
	MetaCount            int
	MalformedMetaCount   int
}

// LoadSessionData opens a resolved store.db and parses bubbles, composers, and contexts in-process.
func LoadSessionData(ins *inspection, files providersessionsinternal.FileSystem, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, resolved ResolvedStoreDB) (*SessionData, error) {
	if resolved.AbsolutePath == "" {
		return nil, fmt.Errorf("cursor session path is empty")
	}
	info, err := files.Stat(resolved.AbsolutePath)
	if err != nil {
		return nil, fmt.Errorf("stat cursor session store: %w", err)
	}
	_ = info

	bubbles, composers, contexts, stats, tokenUsage, err := loadSessionFromStoreDBWithStats(ins, files, openSQLDatabase, resolved.AbsolutePath)
	if err != nil {
		if errors.Is(err, providersessions.ErrResourceLimitExceeded) && len(bubbles) > 0 {
			return &SessionData{
				SessionID:   resolved.SessionID,
				StoreDBPath: resolved.AbsolutePath,
				Bubbles:     bubbles,
				Composers:   composers,
				Contexts:    contexts,
				ParseStats:  stats,
				TokenUsage:  tokenUsage,
			}, err
		}
		return nil, err
	}
	return &SessionData{
		SessionID:   resolved.SessionID,
		StoreDBPath: resolved.AbsolutePath,
		Bubbles:     bubbles,
		Composers:   composers,
		Contexts:    contexts,
		ParseStats:  stats,
		TokenUsage:  tokenUsage,
	}, nil
}

// OrderedBubbles preserves composer header order when available, then appends
// unreferenced bubbles by timestamp and bubble ID as the deterministic fallback.
func (s *SessionData) OrderedBubbles() []*RawBubble {
	if s == nil || len(s.Bubbles) == 0 {
		return nil
	}
	fallback := make([]*RawBubble, 0, len(s.Bubbles))
	for _, bubble := range s.Bubbles {
		fallback = append(fallback, bubble)
	}
	sort.Slice(fallback, func(i, j int) bool {
		left := fallback[i].GetTimestamp()
		right := fallback[j].GetTimestamp()
		if !left.Equal(right) {
			return left.Before(right)
		}
		return fallback[i].BubbleID < fallback[j].BubbleID
	})

	composers := append([]*RawComposer(nil), s.Composers...)
	sort.Slice(composers, func(i, j int) bool {
		if composers[i].CreatedAt != composers[j].CreatedAt {
			return composers[i].CreatedAt < composers[j].CreatedAt
		}
		return composers[i].ComposerID < composers[j].ComposerID
	})
	ordered := make([]*RawBubble, 0, len(s.Bubbles))
	seen := make(map[string]struct{}, len(s.Bubbles))
	for _, composer := range composers {
		for _, header := range composer.FullConversationHeadersOnly {
			if bubble := s.Bubbles[header.BubbleID]; bubble != nil {
				if _, exists := seen[bubble.BubbleID]; !exists {
					ordered = append(ordered, bubble)
					seen[bubble.BubbleID] = struct{}{}
				}
			}
		}
	}
	for _, bubble := range fallback {
		if _, exists := seen[bubble.BubbleID]; !exists {
			ordered = append(ordered, bubble)
		}
	}
	return ordered
}

func loadSessionFromStoreDBWithStats(ins *inspection, files providersessionsinternal.FileSystem, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionParseStats, SessionTokenUsage, error) {
	db, err := OpenDatabase(files, openSQLDatabase, dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(ins, db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	meta, err := QueryMetaTable(ins, db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}

	bubbles, composers, contexts, tokenUsage, err := parseSessionRecords(ins, blobs, meta, dbPath)
	if err != nil {
		return bubbles, composers, contexts, SessionParseStats{}, tokenUsage, err
	}

	stats := SessionParseStats{
		BlobCount:            len(blobs),
		ReadableBlobCount:    len(bubbles),
		UnavailableBlobCount: max(0, len(blobs)-len(bubbles)),
		MetaCount:            len(meta),
	}
	return bubbles, composers, contexts, stats, tokenUsage, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type normalizedSessionFacts struct {
	transcript    []providersessions.TranscriptEntry
	functionCalls []providersessions.FunctionCallSummary
	reasoning     []providersessions.ReasoningSummary
	turns         []providersessions.TurnSummary
	unknownCount  int
	seen          map[string]struct{}
	callByID      map[string]int
}

func reconstructSessionFacts(ins *inspection, session *SessionData) normalizedSessionFacts {
	facts := normalizedSessionFacts{
		transcript:    []providersessions.TranscriptEntry{},
		functionCalls: []providersessions.FunctionCallSummary{},
		reasoning:     []providersessions.ReasoningSummary{},
		turns:         []providersessions.TurnSummary{},
		seen:          make(map[string]struct{}),
		callByID:      make(map[string]int),
	}
	if session == nil {
		return facts
	}
	turnIndex := -1
	for _, bubble := range session.OrderedBubbles() {
		if ins != nil && ins.stopReconstruct {
			break
		}
		if ins != nil {
			if err := ins.checkCanceled(); err != nil {
				break
			}
		}
		if bubble.Type == 1 {
			turnIndex++
		}
		if turnIndex < 0 {
			turnIndex = 0
		}
		if len(bubble.content) == 0 {
			facts.addBubble(ins, bubble, turnIndex)
			continue
		}
		for _, item := range bubble.content {
			facts.addContentItem(ins, bubble, item, turnIndex)
		}
	}
	return facts
}

func (f *normalizedSessionFacts) addBubble(ins *inspection, bubble *RawBubble, turnIndex int) {
	text := truncateSessionText(bubble.DisplayText())
	if text == "" {
		return
	}
	entry := providersessions.TranscriptEntry{
		SourceType: stringPtrIfNotEmpty("cursor_bubble"),
		Text:       stringPtrIfNotEmpty(text),
		Timestamp:  bubbleTimestamp(bubble),
		TurnIndex:  intPtr(turnIndex),
		Type:       providersessions.TranscriptEntryType(bubble.TranscriptEntryType()),
	}
	if f.appendEntry(ins, &entry) {
		f.recordTurn(entry, false, false)
	}
}

func (f *normalizedSessionFacts) addContentItem(ins *inspection, bubble *RawBubble, item rawContentItem, turnIndex int) {
	switch {
	case isTextKind(item.kind):
		f.addTextItem(ins, bubble, item, turnIndex)
	case isReasoningKind(item.kind):
		f.addReasoningItem(ins, bubble, item, turnIndex)
	case item.kind == "tool_call" || item.kind == "function_call":
		f.addToolCall(ins, bubble, item, turnIndex)
	case item.kind == "tool" || item.kind == "tool_result" || item.kind == "function_call_output":
		f.addToolOutput(ins, bubble, item, turnIndex)
	default:
		f.unknownCount++
		if ins != nil {
			ins.recordUnknownRecord(0)
		}
	}
}

func (f *normalizedSessionFacts) addTextItem(ins *inspection, bubble *RawBubble, item rawContentItem, turnIndex int) {
	text := truncateSessionText(item.text)
	if text == "" {
		return
	}
	entry := providersessions.TranscriptEntry{
		SourceType: stringPtrIfNotEmpty("cursor_message"),
		Text:       stringPtrIfNotEmpty(text),
		Timestamp:  bubbleTimestamp(bubble),
		TurnIndex:  intPtr(turnIndex),
		Type:       providersessions.TranscriptEntryType(bubble.TranscriptEntryType()),
	}
	if f.appendEntry(ins, &entry) {
		f.recordTurn(entry, false, false)
	}
}

func (f *normalizedSessionFacts) addReasoningItem(ins *inspection, bubble *RawBubble, item rawContentItem, turnIndex int) {
	entry := providersessions.TranscriptEntry{
		Encrypted:  boolPtrIfTrue(item.encrypted),
		SourceType: stringPtrIfNotEmpty("cursor_reasoning"),
		Summary:    stringPtrIfNotEmpty(truncateSessionText(item.summary)),
		Text:       stringPtrIfNotEmpty(truncateSessionText(item.text)),
		Timestamp:  bubbleTimestamp(bubble),
		TurnIndex:  intPtr(turnIndex),
		Type:       providersessions.TranscriptReasoning,
	}
	if !f.appendEntry(ins, &entry) {
		return
	}
	f.reasoning = append(f.reasoning, providersessions.ReasoningSummary{
		Encrypted:  boolPtrIfTrue(item.encrypted),
		Order:      entry.Order,
		SourceType: "cursor_reasoning",
		Summary:    cloneString(entry.Summary),
		Text:       cloneString(entry.Text),
		TurnIndex:  intPtr(turnIndex),
	})
	f.recordTurn(entry, false, true)
}

func (f *normalizedSessionFacts) addToolCall(ins *inspection, bubble *RawBubble, item rawContentItem, turnIndex int) {
	entry := providersessions.TranscriptEntry{
		Arguments:  stringPtrIfNotEmpty(truncateSessionText(item.arguments)),
		CallID:     stringPtrIfNotEmpty(item.callID),
		Name:       stringPtrIfNotEmpty(item.name),
		SourceType: stringPtrIfNotEmpty("cursor_tool_call"),
		Status:     stringPtrIfNotEmpty(item.status),
		Timestamp:  bubbleTimestamp(bubble),
		TurnIndex:  intPtr(turnIndex),
		Type:       providersessions.TranscriptToolCall,
	}
	if !f.appendEntry(ins, &entry) {
		return
	}
	call := providersessions.FunctionCallSummary{
		Arguments: cloneString(entry.Arguments),
		CallID:    cloneString(entry.CallID),
		Name:      cloneString(entry.Name),
		Order:     entry.Order,
		Status:    cloneString(entry.Status),
		TurnIndex: intPtr(turnIndex),
		Type:      item.kind,
	}
	f.functionCalls = append(f.functionCalls, call)
	if item.callID != "" {
		f.callByID[item.callID] = len(f.functionCalls) - 1
	}
	f.recordTurn(entry, true, false)
}

func (f *normalizedSessionFacts) addToolOutput(ins *inspection, bubble *RawBubble, item rawContentItem, turnIndex int) {
	entry := providersessions.TranscriptEntry{
		CallID:     stringPtrIfNotEmpty(item.callID),
		Name:       stringPtrIfNotEmpty(item.name),
		Output:     stringPtrIfNotEmpty(truncateSessionText(item.output)),
		SourceType: stringPtrIfNotEmpty("cursor_tool_output"),
		Status:     stringPtrIfNotEmpty(item.status),
		Timestamp:  bubbleTimestamp(bubble),
		TurnIndex:  intPtr(turnIndex),
		Type:       providersessions.TranscriptToolOutput,
	}
	if !f.appendEntry(ins, &entry) {
		return
	}
	if index, ok := f.callByID[item.callID]; ok {
		f.functionCalls[index].Output = cloneString(entry.Output)
		if entry.Status != nil {
			f.functionCalls[index].Status = cloneString(entry.Status)
		}
	}
	f.recordTurn(entry, false, false)
}

func (f *normalizedSessionFacts) appendEntry(ins *inspection, entry *providersessions.TranscriptEntry) bool {
	key := transcriptEntryKey(*entry)
	if _, exists := f.seen[key]; exists {
		return false
	}
	if ins != nil {
		if err := ins.recordTranscriptFact(); err != nil {
			return false
		}
	}
	f.seen[key] = struct{}{}
	entry.Order = len(f.transcript) + 1
	f.transcript = append(f.transcript, *entry)
	return true
}

func transcriptEntryKey(entry providersessions.TranscriptEntry) string {
	timestamp := ""
	if entry.Timestamp != nil {
		timestamp = entry.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	turn := ""
	if entry.TurnIndex != nil {
		turn = fmt.Sprintf("%d", *entry.TurnIndex)
	}
	encrypted := entry.Encrypted != nil && *entry.Encrypted
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t",
		entry.Type, stringValue(entry.CallID), stringValue(entry.Name),
		stringValue(entry.Text), stringValue(entry.Arguments),
		stringValue(entry.Output), stringValue(entry.Summary), timestamp, turn, encrypted)
}

func (f *normalizedSessionFacts) recordTurn(entry providersessions.TranscriptEntry, functionCall, reasoning bool) {
	if entry.TurnIndex == nil {
		return
	}
	for len(f.turns) <= *entry.TurnIndex {
		f.turns = append(f.turns, providersessions.TurnSummary{Index: len(f.turns)})
	}
	turn := &f.turns[*entry.TurnIndex]
	turn.EventCount++
	turn.ResponseItemCount++
	if functionCall {
		turn.FunctionCallCount++
	}
	if reasoning {
		turn.ReasoningCount++
	}
	if turn.StartedAt == nil && entry.Timestamp != nil {
		turn.StartedAt = cloneTime(entry.Timestamp)
	}
}

func bubbleTimestamp(bubble *RawBubble) *time.Time {
	if bubble == nil || bubble.Timestamp <= 0 {
		return nil
	}
	utc := bubble.GetTimestamp().UTC()
	return &utc
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intPtr(value int) *int {
	return &value
}

func boolPtrIfTrue(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

// SessionTokenUsage aggregates Cursor CLI usage counters discovered while parsing store.db.
type SessionTokenUsage struct {
	InputTokens      *int
	OutputTokens     *int
	CacheReadTokens  *int
	CacheWriteTokens *int
}

func mergeSessionTokenUsage(into *SessionTokenUsage, next SessionTokenUsage) {
	if into == nil {
		return
	}
	if next.InputTokens != nil {
		into.InputTokens = next.InputTokens
	}
	if next.OutputTokens != nil {
		into.OutputTokens = next.OutputTokens
	}
	if next.CacheReadTokens != nil {
		into.CacheReadTokens = next.CacheReadTokens
	}
	if next.CacheWriteTokens != nil {
		into.CacheWriteTokens = next.CacheWriteTokens
	}
}

func tokenUsageFromData(data map[string]interface{}) SessionTokenUsage {
	if usageMap, ok := nestedMapField(data, "usage"); ok {
		if usage := tokenUsageFromUsageMap(usageMap); hasAnyTokenUsage(usage) {
			return usage
		}
	}
	return tokenUsageFromUsageMap(data)
}

func tokenUsageFromUsageMap(data map[string]interface{}) SessionTokenUsage {
	var usage SessionTokenUsage
	if value, ok := intField(data, "inputTokens", "input_tokens"); ok {
		usage.InputTokens = &value
	}
	if value, ok := intField(data, "outputTokens", "output_tokens"); ok {
		usage.OutputTokens = &value
	}
	if value, ok := intField(data, "cacheReadTokens", "cache_read_tokens", "cachedInputTokens", "cached_input_tokens"); ok {
		usage.CacheReadTokens = &value
	}
	if value, ok := intField(data, "cacheWriteTokens", "cache_write_tokens"); ok {
		usage.CacheWriteTokens = &value
	}
	return usage
}

func hasAnyTokenUsage(usage SessionTokenUsage) bool {
	return usage.InputTokens != nil ||
		usage.OutputTokens != nil ||
		usage.CacheReadTokens != nil ||
		usage.CacheWriteTokens != nil
}

func nestedMapField(data map[string]interface{}, key string) (map[string]interface{}, bool) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return nil, false
	}
	mapped, ok := raw.(map[string]interface{})
	return mapped, ok
}

func intField(data map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case int64:
			return int(typed), true
		}
	}
	return 0, false
}

func computeTotalTokens(usage SessionTokenUsage) *int {
	if usage.InputTokens == nil && usage.OutputTokens == nil &&
		usage.CacheReadTokens == nil && usage.CacheWriteTokens == nil {
		return nil
	}
	total := 0
	if usage.InputTokens != nil {
		total += *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		total += *usage.OutputTokens
	}
	if usage.CacheReadTokens != nil {
		total += *usage.CacheReadTokens
	}
	if usage.CacheWriteTokens != nil {
		total += *usage.CacheWriteTokens
	}
	return &total
}
