package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"go.uber.org/zap"
)

const (
	loadableProviderSessionProvider = "codex"
	loadableProviderSessionKind     = "session_id"
)

var safeProviderSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var codexTimestampPrefixedSessionPattern = regexp.MustCompile(`^rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-([A-Za-z0-9_-]+)\.jsonl$`)

var (
	errInvalidProviderSessionIdentifier = errors.New("invalid provider session identifier")
	errProviderSessionNotFound          = errors.New("provider session not found")
	errAmbiguousProviderSessionFile     = errors.New("ambiguous provider session file")
)

func (s *Server) GetProviderSessionDetails(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	if params.Provider != factoryapi.Codex || params.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		s.writeError(w, http.StatusBadRequest, "invalid request parameter", "BAD_REQUEST")
		return
	}

	details, err := loadProviderSessionDetails(
		s.codexSessionsRoot,
		string(params.Id),
	)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidProviderSessionIdentifier):
			s.writeError(w, http.StatusBadRequest, "provider session must be a codex session_id identifier without path separators", "BAD_REQUEST")
			return
		case errors.Is(err, errProviderSessionNotFound):
			s.writeError(w, http.StatusNotFound, "provider session not found", "NOT_FOUND")
			return
		case errors.Is(err, errAmbiguousProviderSessionFile):
			s.writeError(w, http.StatusInternalServerError, "multiple provider session files match session identifier", "INTERNAL_ERROR")
			return
		default:
			s.logger.Error("load provider session details failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load provider session details", "INTERNAL_ERROR")
			return
		}
	}

	s.writeJSON(w, http.StatusOK, details)
}

func loadProviderSessionDetails(root, id string) (factoryapi.ProviderSessionDetailResponse, error) {
	normalizedID := strings.TrimSpace(id)
	if !safeProviderSessionIDPattern.MatchString(normalizedID) {
		return factoryapi.ProviderSessionDetailResponse{}, errInvalidProviderSessionIdentifier
	}

	resolved, err := resolveCodexSessionFile(root, normalizedID)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}

	file, err := os.Open(resolved.absolutePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return factoryapi.ProviderSessionDetailResponse{}, errProviderSessionNotFound
		}
		return factoryapi.ProviderSessionDetailResponse{}, fmt.Errorf("open provider session file: %w", err)
	}
	defer file.Close()

	parsed, err := parseCodexSessionDetails(file)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}

	return factoryapi.ProviderSessionDetailResponse{
		ProviderSession: factoryapi.LoadableProviderSessionRef{
			Provider: factoryapi.LoadableProviderSessionProvider(loadableProviderSessionProvider),
			Kind:     factoryapi.LoadableProviderSessionKind(loadableProviderSessionKind),
			Id:       normalizedID,
		},
		Source: factoryapi.ProviderSessionSourceMetadata{
			RelativePath: resolved.relativePath,
			SizeBytes:    resolved.sizeBytes,
			ModifiedAt:   resolved.modifiedAt,
		},
		Parse:      parsed.Summary,
		Transcript: parsed.Transcript,
	}, nil
}

type resolvedCodexSessionFile struct {
	absolutePath string
	relativePath string
	sizeBytes    int64
	modifiedAt   *time.Time
	layout       codexSessionFileLayout
}

type codexSessionFileLayout int

const (
	codexSessionFileLayoutExact codexSessionFileLayout = iota + 1
	codexSessionFileLayoutTimestampPrefixed
)

func resolveCodexSessionFile(root, id string) (resolvedCodexSessionFile, error) {
	cleanRoot, resolvedRoot, err := resolveCodexSessionsRoot(root)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}

	targetName := "rollout-" + id + ".jsonl"
	matches, err := collectCodexSessionMatches(cleanRoot, id, targetName)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}
	if len(matches) == 0 {
		return resolvedCodexSessionFile{}, errProviderSessionNotFound
	}
	sort.Strings(matches)
	return buildResolvedCodexSessionCandidates(cleanRoot, resolvedRoot, matches, targetName)
}

func resolveCodexSessionsRoot(root string) (string, string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", fmt.Errorf("resolve codex sessions root: %w", err)
	}
	rootInfo, err := os.Stat(cleanRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", errProviderSessionNotFound
		}
		return "", "", fmt.Errorf("stat codex sessions root: %w", err)
	}
	if !rootInfo.IsDir() {
		return "", "", fmt.Errorf("codex sessions root is not a directory: %s", cleanRoot)
	}
	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve codex sessions root symlinks: %w", err)
	}
	return cleanRoot, resolvedRoot, nil
}

func collectCodexSessionMatches(cleanRoot, id, targetName string) ([]string, error) {
	matches := make([]string, 0, 1)
	err := filepath.WalkDir(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeType != 0 && entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if matchesCodexSessionBaseName(filepath.Base(path), id, targetName) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk codex sessions root: %w", err)
	}
	return matches, nil
}

func buildResolvedCodexSessionCandidates(cleanRoot, resolvedRoot string, matches []string, targetName string) (resolvedCodexSessionFile, error) {
	candidates := make([]resolvedCodexSessionFile, 0, len(matches))
	for _, match := range matches {
		candidate, err := resolvedCodexSessionCandidate(cleanRoot, resolvedRoot, match, targetName)
		if err != nil {
			return resolvedCodexSessionFile{}, err
		}
		candidates = append(candidates, candidate)
	}
	return selectResolvedCodexSessionFile(candidates)
}

func resolvedCodexSessionCandidate(cleanRoot, resolvedRoot, match, targetName string) (resolvedCodexSessionFile, error) {
	resolvedMatch, err := filepath.EvalSymlinks(match)
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("resolve provider session symlink: %w", err)
	}
	if !pathInsideRoot(resolvedRoot, resolvedMatch) {
		return resolvedCodexSessionFile{}, errInvalidProviderSessionIdentifier
	}
	info, err := os.Stat(resolvedMatch)
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("stat provider session file: %w", err)
	}
	rel, err := filepath.Rel(cleanRoot, match)
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("rel provider session file: %w", err)
	}
	modifiedAt := info.ModTime().UTC()
	return resolvedCodexSessionFile{
		absolutePath: resolvedMatch,
		relativePath: filepath.ToSlash(rel),
		sizeBytes:    info.Size(),
		modifiedAt:   &modifiedAt,
		layout:       classifyCodexSessionFileLayout(filepath.Base(match), targetName),
	}, nil
}

func matchesCodexSessionBaseName(baseName, id, exactName string) bool {
	if baseName == exactName {
		return true
	}
	matches := codexTimestampPrefixedSessionPattern.FindStringSubmatch(baseName)
	if matches == nil {
		return false
	}
	return matches[2] == id
}

func classifyCodexSessionFileLayout(baseName, exactName string) codexSessionFileLayout {
	if baseName == exactName {
		return codexSessionFileLayoutExact
	}
	return codexSessionFileLayoutTimestampPrefixed
}

func selectResolvedCodexSessionFile(candidates []resolvedCodexSessionFile) (resolvedCodexSessionFile, error) {
	exactMatches := make([]resolvedCodexSessionFile, 0, 1)
	timestampMatches := make([]resolvedCodexSessionFile, 0, 1)
	for _, candidate := range candidates {
		switch candidate.layout {
		case codexSessionFileLayoutExact:
			exactMatches = append(exactMatches, candidate)
		case codexSessionFileLayoutTimestampPrefixed:
			timestampMatches = append(timestampMatches, candidate)
		}
	}

	switch {
	case len(exactMatches) == 1:
		return exactMatches[0], nil
	case len(exactMatches) > 1:
		return resolvedCodexSessionFile{}, errAmbiguousProviderSessionFile
	case len(timestampMatches) == 1:
		return timestampMatches[0], nil
	case len(timestampMatches) > 1:
		return resolvedCodexSessionFile{}, errAmbiguousProviderSessionFile
	default:
		return resolvedCodexSessionFile{}, errProviderSessionNotFound
	}
}

func pathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type parsedCodexSessionDetails struct {
	Summary    factoryapi.ProviderSessionParseSummary
	Transcript []factoryapi.ProviderSessionTranscriptEntry
}

func parseCodexSessionSummary(reader io.Reader) (factoryapi.ProviderSessionParseSummary, error) {
	parsed, err := parseCodexSessionDetails(reader)
	if err != nil {
		return factoryapi.ProviderSessionParseSummary{}, err
	}
	return parsed.Summary, nil
}

func parseCodexSessionDetails(reader io.Reader) (parsedCodexSessionDetails, error) {
	parser := codexSessionParser{
		summary: factoryapi.ProviderSessionParseSummary{
			Turns:         []factoryapi.ProviderSessionTurnSummary{},
			FunctionCalls: []factoryapi.ProviderSessionFunctionCallSummary{},
			Reasoning:     []factoryapi.ProviderSessionReasoningSummary{},
			ParseErrors:   []factoryapi.ProviderSessionLineError{},
			UnknownEvents: []factoryapi.ProviderSessionUnknownEvent{},
		},
		transcript: []factoryapi.ProviderSessionTranscriptEntry{},
	}
	bufferedReader := bufio.NewReader(reader)
	lineNumber := 0
	for {
		lineBytes, err := bufferedReader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(lineBytes) == 0 {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return parsedCodexSessionDetails{}, fmt.Errorf("read provider session stream: %w", err)
		}

		lineNumber++
		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		parser.summary.LineCount++
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			parser.summary.MalformedLineCount++
			parser.summary.ParseErrors = append(parser.summary.ParseErrors, factoryapi.ProviderSessionLineError{
				LineNumber: lineNumber,
				Message:    "invalid JSON event record",
			})
			continue
		}
		parser.summary.EventCount++
		parser.recordEvent(lineNumber, event)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return parsedCodexSessionDetails{
		Summary:    parser.summary,
		Transcript: parser.transcript,
	}, nil
}

type codexSessionParser struct {
	summary          factoryapi.ProviderSessionParseSummary
	transcript       []factoryapi.ProviderSessionTranscriptEntry
	currentTurnIndex int
}

func (p *codexSessionParser) recordEvent(lineNumber int, event map[string]any) {
	eventType := stringField(event, "type")
	timestamp := timeField(event, "timestamp")
	switch eventType {
	case "session_meta":
		return
	case "turn_context":
		p.startTurn(timestamp).EventCount++
	case "event_msg":
		p.recordEventMessage(lineNumber, event, timestamp)
	case "response_item":
		p.recordResponseItem(lineNumber, event, timestamp)
	default:
		p.recordUnknownEvent(lineNumber, eventType, nestedPayloadType(event))
	}
}

func (p *codexSessionParser) recordEventMessage(lineNumber int, event map[string]any, timestamp *time.Time) {
	payload, _ := event["payload"].(map[string]any)
	payloadType := stringField(payload, "type")
	switch payloadType {
	case "token_count":
		p.recordTokenUsage(payload)
	case "agent_message", "user_message", "task_started", "task_complete", "patch_apply_end":
		turn := p.ensureTurn(timestamp)
		turn.EventCount++
		p.appendEventMessageTranscript(lineNumber, payloadType, payload, timestamp, turn)
	case "agent_reasoning":
		turn := p.ensureTurn(timestamp)
		turn.EventCount++
		p.appendReasoning("agent_reasoning", payload, timestamp, lineNumber, turn)
	default:
		p.recordUnknownEvent(lineNumber, "event_msg", payloadType)
	}
}

func (p *codexSessionParser) recordResponseItem(lineNumber int, event map[string]any, timestamp *time.Time) {
	payload, _ := event["payload"].(map[string]any)
	if payload == nil {
		if item, ok := event["item"].(map[string]any); ok {
			payload = item
		} else {
			payload = event
		}
	}
	itemType := stringField(payload, "type")
	if itemType == "" {
		itemType = stringField(payload, "item.type")
	}

	turn := p.ensureTurn(timestamp)
	turn.EventCount++
	turn.ResponseItemCount++

	switch itemType {
	case "message":
		p.appendResponseMessage(payload, timestamp, lineNumber, turn)
	case "reasoning":
		p.appendReasoning(itemType, payload, timestamp, lineNumber, turn)
	case "function_call", "custom_tool_call":
		p.appendFunctionCall(itemType, payload, timestamp, lineNumber, turn)
	case "function_call_output", "custom_tool_call_output":
		p.attachFunctionOutput(itemType, payload, timestamp, lineNumber, turn)
	default:
		p.recordUnknownEvent(lineNumber, "response_item", itemType)
	}
}

func (p *codexSessionParser) startTurn(startedAt *time.Time) *factoryapi.ProviderSessionTurnSummary {
	p.summary.Turns = append(p.summary.Turns, factoryapi.ProviderSessionTurnSummary{
		Index:             len(p.summary.Turns) + 1,
		StartedAt:         startedAt,
		EventCount:        0,
		ResponseItemCount: 0,
		FunctionCallCount: 0,
		ReasoningCount:    0,
	})
	p.currentTurnIndex = len(p.summary.Turns)
	return &p.summary.Turns[p.currentTurnIndex-1]
}

func (p *codexSessionParser) ensureTurn(startedAt *time.Time) *factoryapi.ProviderSessionTurnSummary {
	if p.currentTurnIndex == 0 {
		return p.startTurn(startedAt)
	}
	turn := &p.summary.Turns[p.currentTurnIndex-1]
	if turn.StartedAt == nil && startedAt != nil {
		turn.StartedAt = startedAt
	}
	return turn
}

func (p *codexSessionParser) appendFunctionCall(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *factoryapi.ProviderSessionTurnSummary) {
	turn.FunctionCallCount++
	order := len(p.summary.FunctionCalls) + 1
	call := factoryapi.ProviderSessionFunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallId:    stringPtrIfNotEmpty(firstStringField(payload, "call_id", "callId", "id")),
		Type:      itemType,
		Name:      stringPtrIfNotEmpty(firstStringField(payload, "name", "tool_name", "toolName")),
		Arguments: stringPtrIfNotEmpty(firstCompactField(payload, "arguments", "arguments_json", "input")),
		Status:    stringPtrIfNotEmpty(firstStringField(payload, "status")),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.transcript = append(p.transcript, factoryapi.ProviderSessionTranscriptEntry{
		Arguments:  call.Arguments,
		CallId:     call.CallId,
		LineNumber: intPtr(lineNumber),
		Name:       call.Name,
		Order:      len(p.transcript) + 1,
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     call.Status,
		Timestamp:  timestamp,
		TurnIndex:  call.TurnIndex,
		Type:       factoryapi.ProviderSessionTranscriptEntryType("tool_call"),
	})
}

func (p *codexSessionParser) attachFunctionOutput(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *factoryapi.ProviderSessionTurnSummary) {
	callID := firstStringField(payload, "call_id", "callId", "id")
	output := firstCompactField(payload, "output", "content", "result")
	status := firstStringField(payload, "status")
	if status == "" && output != "" {
		status = "completed"
	}
	for i := range p.summary.FunctionCalls {
		if stringValue(p.summary.FunctionCalls[i].CallId) == callID && callID != "" {
			p.summary.FunctionCalls[i].Output = stringPtrIfNotEmpty(output)
			p.summary.FunctionCalls[i].Status = stringPtrIfNotEmpty(status)
			p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, p.summary.FunctionCalls[i].Name, p.summary.FunctionCalls[i].TurnIndex)
			return
		}
	}

	order := len(p.summary.FunctionCalls) + 1
	call := factoryapi.ProviderSessionFunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallId:    stringPtrIfNotEmpty(callID),
		Type:      itemType,
		Output:    stringPtrIfNotEmpty(output),
		Status:    stringPtrIfNotEmpty(status),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, call.Name, call.TurnIndex)
}

func (p *codexSessionParser) appendReasoning(sourceType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *factoryapi.ProviderSessionTurnSummary) {
	turn.ReasoningCount++
	order := len(p.summary.Reasoning) + 1
	encryptedContent := firstCompactField(payload, "encrypted_content", "encryptedContent")
	encrypted := encryptedContent != ""
	reasoning := factoryapi.ProviderSessionReasoningSummary{
		Order:            order,
		TurnIndex:        intPtr(turn.Index),
		SourceType:       sourceType,
		Text:             stringPtrIfNotEmpty(firstReasoningText(payload)),
		Summary:          stringPtrIfNotEmpty(firstCompactField(payload, "summary")),
		Encrypted:        &encrypted,
		EncryptedContent: stringPtrIfNotEmpty(encryptedContent),
	}
	p.summary.Reasoning = append(p.summary.Reasoning, reasoning)
	p.transcript = append(p.transcript, factoryapi.ProviderSessionTranscriptEntry{
		Encrypted:        reasoning.Encrypted,
		EncryptedContent: reasoning.EncryptedContent,
		LineNumber:       intPtr(lineNumber),
		Order:            len(p.transcript) + 1,
		SourceType:       stringPtrIfNotEmpty(sourceType),
		Summary:          reasoning.Summary,
		Text:             reasoning.Text,
		Timestamp:        timestamp,
		TurnIndex:        reasoning.TurnIndex,
		Type:             factoryapi.ProviderSessionTranscriptEntryType("reasoning"),
	})
}

func (p *codexSessionParser) appendEventMessageTranscript(
	lineNumber int,
	payloadType string,
	payload map[string]any,
	timestamp *time.Time,
	turn *factoryapi.ProviderSessionTurnSummary,
) {
	entryType := factoryapi.ProviderSessionTranscriptEntryType("system_event")
	switch payloadType {
	case "user_message":
		entryType = factoryapi.ProviderSessionTranscriptEntryType("user_message")
	case "agent_message":
		entryType = factoryapi.ProviderSessionTranscriptEntryType("assistant_message")
	}

	p.transcript = append(p.transcript, factoryapi.ProviderSessionTranscriptEntry{
		LineNumber: intPtr(lineNumber),
		Order:      len(p.transcript) + 1,
		SourceType: stringPtrIfNotEmpty(payloadType),
		Text:       stringPtrIfNotEmpty(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendResponseMessage(
	payload map[string]any,
	timestamp *time.Time,
	lineNumber int,
	turn *factoryapi.ProviderSessionTurnSummary,
) {
	role := firstStringField(payload, "role")
	entryType := factoryapi.ProviderSessionTranscriptEntryType("assistant_message")
	if role == "user" {
		entryType = factoryapi.ProviderSessionTranscriptEntryType("user_message")
	}

	p.transcript = append(p.transcript, factoryapi.ProviderSessionTranscriptEntry{
		LineNumber: intPtr(lineNumber),
		Order:      len(p.transcript) + 1,
		SourceType: stringPtrIfNotEmpty("message"),
		Text:       stringPtrIfNotEmpty(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendToolOutputTranscript(
	itemType string,
	callID string,
	output string,
	status string,
	timestamp *time.Time,
	lineNumber int,
	name *string,
	turnIndex *int,
) {
	p.transcript = append(p.transcript, factoryapi.ProviderSessionTranscriptEntry{
		CallId:     stringPtrIfNotEmpty(callID),
		LineNumber: intPtr(lineNumber),
		Name:       name,
		Order:      len(p.transcript) + 1,
		Output:     stringPtrIfNotEmpty(output),
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     stringPtrIfNotEmpty(status),
		Timestamp:  timestamp,
		TurnIndex:  turnIndex,
		Type:       factoryapi.ProviderSessionTranscriptEntryType("tool_output"),
	})
}

func (p *codexSessionParser) recordTokenUsage(payload map[string]any) {
	usage, ok := nestedMap(payload, "info.total_token_usage")
	if !ok {
		return
	}
	p.summary.TokenUsage = &factoryapi.ProviderSessionTokenUsage{
		InputTokens:           intPtrIfPresent(intField(usage, "input_tokens")),
		CachedInputTokens:     intPtrIfPresent(intField(usage, "cached_input_tokens")),
		OutputTokens:          intPtrIfPresent(intField(usage, "output_tokens")),
		ReasoningOutputTokens: intPtrIfPresent(intField(usage, "reasoning_output_tokens")),
		TotalTokens:           intPtrIfPresent(intField(usage, "total_tokens")),
	}
}

func (p *codexSessionParser) recordUnknownEvent(lineNumber int, eventType string, payloadType string) {
	p.summary.UnknownEventCount++
	p.summary.UnknownEvents = append(p.summary.UnknownEvents, factoryapi.ProviderSessionUnknownEvent{
		LineNumber:  lineNumber,
		Type:        stringPtrIfNotEmpty(eventType),
		PayloadType: stringPtrIfNotEmpty(payloadType),
	})
}

func nestedPayloadType(event map[string]any) string {
	payload, _ := event["payload"].(map[string]any)
	return stringField(payload, "type")
}

func firstReasoningText(payload map[string]any) string {
	if value := firstCompactField(payload, "text", "content"); value != "" {
		return value
	}
	if summary := firstCompactField(payload, "summary"); summary != "" {
		return summary
	}
	return ""
}

func firstMessageText(payload map[string]any) string {
	if value := firstCompactField(payload, "text", "message", "content_text"); value != "" {
		return value
	}
	if items, ok := payload["content"].([]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := firstCompactField(mapped, "text", "content", "value")
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return truncateSessionText(strings.Join(parts, "\n\n"))
		}
	}
	if value := firstCompactField(payload, "content"); value != "" {
		return value
	}
	return ""
}

func firstStringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(values, key); value != "" {
			return value
		}
	}
	return ""
}

func firstCompactField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := nestedValue(values, key); ok {
			if compact := compactSessionValue(raw); compact != "" {
				return compact
			}
		}
	}
	return ""
}

func stringField(values map[string]any, key string) string {
	raw, ok := nestedValue(values, key)
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func intField(values map[string]any, key string) (int, bool) {
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return int(value), true
	case int:
		return value, true
	case json.Number:
		parsed, err := value.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func timeField(values map[string]any, key string) *time.Time {
	value := stringField(values, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func nestedMap(values map[string]any, key string) (map[string]any, bool) {
	raw, ok := nestedValue(values, key)
	if !ok {
		return nil, false
	}
	mapped, ok := raw.(map[string]any)
	return mapped, ok
}

func nestedValue(values map[string]any, key string) (any, bool) {
	current := any(values)
	for _, segment := range strings.Split(key, ".") {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := mapped[segment]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func compactSessionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return truncateSessionText(strings.TrimSpace(typed))
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return truncateSessionText(string(encoded))
	}
}

func truncateSessionText(value string) string {
	const maxSessionSummaryTextLength = 1000
	if len(value) <= maxSessionSummaryTextLength {
		return value
	}
	return value[:maxSessionSummaryTextLength] + "..."
}

func intPtrIfPresent(value int, ok bool) *int {
	if !ok {
		return nil
	}
	return &value
}

func defaultCodexSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(".codex/sessions")
	}
	return filepath.Join(home, ".codex", "sessions")
}
