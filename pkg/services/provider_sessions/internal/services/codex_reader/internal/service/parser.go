package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// ParsedDetails contains the provider-independent summary and transcript
// extracted from a Codex JSONL stream.
type ParsedDetails struct {
	Summary    providersessions.ParseSummary
	Transcript []providersessions.TranscriptEntry
}

// ParseDetails parses a Codex JSONL stream into summary and transcript data.
func ParseDetails(reader io.Reader) (ParsedDetails, error) {
	parsed, err := parseCodexSessionDetails(context.Background(), reader)
	return detachParsedDetails(parsed), err
}

// Codex JSONL reconstruction preserves source line order for transcript,
// parse-summary, and turn facts. turn_context opens a new turn; later events
// without an explicit turn inherit the current turn until the next turn_context.
// When timestamps are absent, ordering follows JSONL line order only.
// Mirrored user/assistant messages emitted as both event_msg and response_item
// message records are deduplicated. Function outputs attach to the earliest
// matching call_id within the reconstructed stream.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func parseCodexSessionDetails(ctx context.Context, reader io.Reader) (ParsedDetails, error) {
	return parseCodexSessionDetailsForSession(ctx, reader, "")
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func parseCodexSessionDetailsForSession(ctx context.Context, reader io.Reader, sessionID string) (ParsedDetails, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		return ParsedDetails{}, fmt.Errorf("%w: rollout reader is nil", providersessions.ErrSessionStorageUnavailable)
	}
	parser := codexSessionParser{
		sessionID: sessionID,
		summary: providersessions.ParseSummary{
			Turns:                 []providersessions.TurnSummary{},
			FunctionCalls:         []providersessions.FunctionCallSummary{},
			Reasoning:             []providersessions.ReasoningSummary{},
			ParseErrors:           []providersessions.LineError{},
			CumulativeInputTokens: []int{},
			UnknownEvents:         []providersessions.UnknownEvent{},
		},
		transcript: []providersessions.TranscriptEntry{},
	}
	budgetReader := &codexInspectionReader{reader: reader, budget: &parser.budget}
	bufferedReader := bufio.NewReaderSize(budgetReader, maxCodexJSONLReadBufferSize)
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			return parser.details(), err
		}
		lineBytes, atEOF, observedLineBytes, readErr := readCodexJSONLLine(bufferedReader)
		if readErr != nil {
			switch {
			case errors.Is(readErr, errCodexInspectionByteLimit):
				limitErr := parser.stopAtLimit(
					codexInspectionLimitBytes,
					maxCodexJSONLBytesPerInspection,
					parser.budget.bytesRead,
					lineNumber+1,
					diagnosticInspectionByteLimit,
				)
				return parser.details(), limitErr
			case errors.Is(readErr, errCodexInspectionRecordLimit):
				limitErr := parser.stopAtLimit(
					codexInspectionLimitRecord,
					maxCodexJSONLLineBytes,
					observedLineBytes,
					lineNumber+1,
					diagnosticInspectionRecordLimit,
				)
				return parser.details(), limitErr
			default:
				return parser.details(), fmt.Errorf("%w: rollout read failed", providersessions.ErrSessionStorageUnavailable)
			}
		}
		if atEOF && len(lineBytes) == 0 {
			break
		}

		lineNumber++
		if !parser.budget.beginLine() {
			limitErr := parser.stopAtLimit(
				codexInspectionLimitLines,
				int64(maxCodexJSONLLinesPerInspection),
				int64(lineNumber),
				lineNumber,
				diagnosticInspectionLineLimit,
			)
			return parser.details(), limitErr
		}
		line := bytes.TrimSpace(lineBytes)
		if len(line) == 0 {
			if atEOF {
				break
			}
			continue
		}
		parser.summary.LineCount++
		var event map[string]any
		if jsonErr := json.Unmarshal(line, &event); jsonErr != nil {
			message := diagnosticInvalidJSONEvent
			if atEOF {
				message = diagnosticTruncatedJSONEvent
			}
			parser.recordMalformedLine(lineNumber, message)
			if atEOF {
				break
			}
			continue
		}
		parser.summary.EventCount++
		parser.currentLine = lineNumber
		parser.recordEvent(lineNumber, event)
		if parser.budget.stopParsing {
			return parser.details(), parser.limitError()
		}
		if atEOF {
			break
		}
	}
	return parser.details(), nil
}

var (
	errCodexInspectionByteLimit   = errors.New("codex rollout byte inspection limit reached")
	errCodexInspectionRecordLimit = errors.New("codex rollout record inspection limit reached")
)

const (
	codexInspectionLimitBytes       = "byte"
	codexInspectionLimitLines       = "line"
	codexInspectionLimitRecord      = "record"
	codexInspectionLimitDiagnostics = "diagnostic"
)

type codexInspectionReader struct {
	reader io.Reader
	budget *parseBudget
}

func (r *codexInspectionReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	remaining := maxCodexJSONLBytesPerInspection - r.budget.bytesRead
	if remaining > 0 {
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
		n, err := r.reader.Read(p)
		if n > 0 {
			r.budget.bytesRead += int64(n)
		}
		return n, err
	}

	// The configured cap is inclusive. Probe one byte from the source so an
	// exact-cap rollout can report EOF successfully, while cap+1 remains a
	// resource-limit failure. A non-progressing reader cannot establish EOF at
	// the boundary, so fail closed rather than retrying without a bound.
	n, err := r.reader.Read(p[:1])
	if n > 0 {
		r.budget.bytesRead += int64(n)
		return n, errCodexInspectionByteLimit
	}
	if err == nil {
		return 0, errCodexInspectionByteLimit
	}
	return n, err
}

func readCodexJSONLLine(reader *bufio.Reader) ([]byte, bool, int64, error) {
	var line []byte
	var lineBytes int64
	for {
		fragment, err := reader.ReadSlice('\n')
		lineBytes += int64(len(fragment))
		if lineBytes > maxCodexJSONLLineBytes {
			return nil, false, lineBytes, errCodexInspectionRecordLimit
		}
		line = append(line, fragment...)
		switch {
		case err == nil:
			return line, false, lineBytes, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, true, lineBytes, nil
			}
			return line, true, lineBytes, nil
		case errors.Is(err, errCodexInspectionByteLimit):
			return nil, false, lineBytes, err
		default:
			return nil, false, lineBytes, err
		}
	}
}

type codexInspectionLimitError struct {
	sessionID  string
	category   string
	configured int64
	observed   int64
	line       int
}

func (e *codexInspectionLimitError) Error() string {
	session := strings.TrimSpace(e.sessionID)
	if session == "" {
		session = "unspecified"
	}
	return fmt.Sprintf(
		"codex provider session %q rollout inspection %s limit reached (configured %d, observed %d, line %d)",
		session,
		e.category,
		e.configured,
		e.observed,
		e.line,
	)
}

func (e *codexInspectionLimitError) Unwrap() error {
	return providersessions.ErrResourceLimitExceeded
}

type codexSessionParser struct {
	summary          providersessions.ParseSummary
	transcript       []providersessions.TranscriptEntry
	currentTurnIndex int
	currentLine      int
	sessionID        string
	budget           parseBudget
}

func (p *codexSessionParser) details() ParsedDetails {
	return ParsedDetails{
		Summary:    p.summary,
		Transcript: p.transcript,
	}
}

func (p *codexSessionParser) stopAtLimit(category string, configured, observed int64, line int, diagnostic string) error {
	p.budget.setLimit(category, configured, observed, line)
	p.recordMalformedLine(line, diagnostic)
	return p.limitError()
}

func (p *codexSessionParser) limitError() error {
	category := p.budget.limitCategory
	if category == "" {
		category = codexInspectionLimitDiagnostics
	}
	configured := p.budget.limitConfigured
	if configured == 0 {
		configured = int64(maxCodexDiagnosticRecords)
	}
	observed := p.budget.limitObserved
	if observed == 0 {
		observed = int64(p.budget.diagnosticRecords + 1)
	}
	return &codexInspectionLimitError{
		sessionID:  p.sessionID,
		category:   category,
		configured: configured,
		observed:   observed,
		line:       p.budget.limitLine,
	}
}

func (p *codexSessionParser) recordMalformedLine(lineNumber int, message string) {
	p.summary.MalformedLineCount++
	p.appendDiagnostic(lineNumber, message)
}

func (p *codexSessionParser) recordTranscriptLimit(lineNumber int) {
	if p.budget.transcriptLimitReported {
		return
	}
	if lineNumber <= 0 {
		lineNumber = max(1, p.summary.LineCount)
	}
	p.budget.transcriptLimitReported = true
	p.recordMalformedLine(lineNumber, diagnosticInspectionTranscriptLimit)
}

func (p *codexSessionParser) recordRetainedTextLimit() {
	if p.budget.retainedTextLimitReported {
		return
	}
	p.budget.retainedTextLimitReported = true
	lineNumber := p.currentLine
	if lineNumber <= 0 {
		lineNumber = 1
	}
	p.recordMalformedLine(lineNumber, diagnosticInspectionRetainedTextLimit)
}

func (p *codexSessionParser) appendDiagnostic(lineNumber int, message string) {
	if !p.budget.canRecordDiagnostic() {
		if !p.budget.stopParsing {
			p.budget.setLimit(
				codexInspectionLimitDiagnostics,
				int64(maxCodexDiagnosticRecords),
				int64(p.budget.diagnosticRecords+1),
				lineNumber,
			)
		}
		return
	}
	p.summary.ParseErrors = append(p.summary.ParseErrors, providersessions.LineError{
		LineNumber: lineNumber,
		Message:    sanitizeDiagnosticMessage(message),
	})
	p.budget.recordedDiagnostic()
}

func (p *codexSessionParser) boundedString(value string) string {
	return truncateCodexText(strings.TrimSpace(value), maxCodexRetainedFieldBytes)
}

func (p *codexSessionParser) boundedStringPtr(value string) *string {
	return stringPtrIfNotEmpty(p.boundedString(value))
}

func (p *codexSessionParser) retainedText(value string) string {
	retained, truncated := p.budget.retainText(value)
	if truncated {
		p.recordRetainedTextLimit()
	}
	return retained
}

func (p *codexSessionParser) retainedTextPtr(value string) *string {
	return stringPtrIfNotEmpty(p.retainedText(value))
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

func (p *codexSessionParser) startTurn(startedAt *time.Time) *providersessions.TurnSummary {
	p.summary.Turns = append(p.summary.Turns, providersessions.TurnSummary{
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

func (p *codexSessionParser) ensureTurn(startedAt *time.Time) *providersessions.TurnSummary {
	if p.currentTurnIndex == 0 {
		return p.startTurn(startedAt)
	}
	turn := &p.summary.Turns[p.currentTurnIndex-1]
	if turn.StartedAt == nil && startedAt != nil {
		turn.StartedAt = startedAt
	}
	return turn
}

func (p *codexSessionParser) appendFunctionCall(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	turn.FunctionCallCount++
	order := len(p.summary.FunctionCalls) + 1
	call := providersessions.FunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallID:    p.boundedStringPtr(firstStringField(payload, "call_id", "callId", "id")),
		Type:      itemType,
		Name:      p.boundedStringPtr(firstStringField(payload, "name", "tool_name", "toolName")),
		Arguments: p.retainedTextPtr(firstCompactField(payload, "arguments", "arguments_json", "input")),
		Status:    p.boundedStringPtr(firstStringField(payload, "status")),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		Arguments:  call.Arguments,
		CallID:     call.CallID,
		LineNumber: intPtr(lineNumber),
		Name:       call.Name,
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     call.Status,
		Timestamp:  timestamp,
		TurnIndex:  call.TurnIndex,
		Type:       providersessions.TranscriptEntryType("tool_call"),
	})
}

func (p *codexSessionParser) attachFunctionOutput(itemType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	callID := firstStringField(payload, "call_id", "callId", "id")
	output := p.retainedTextPtr(firstCompactField(payload, "output", "content", "result"))
	status := firstStringField(payload, "status")
	if status == "" && output != nil {
		status = "completed"
	}
	for i := range p.summary.FunctionCalls {
		if stringValue(p.summary.FunctionCalls[i].CallID) == callID && callID != "" {
			p.summary.FunctionCalls[i].Output = output
			p.summary.FunctionCalls[i].Status = p.boundedStringPtr(status)
			p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, p.summary.FunctionCalls[i].Name, p.summary.FunctionCalls[i].TurnIndex)
			return
		}
	}

	order := len(p.summary.FunctionCalls) + 1
	call := providersessions.FunctionCallSummary{
		Order:     order,
		TurnIndex: intPtr(turn.Index),
		CallID:    p.boundedStringPtr(callID),
		Type:      itemType,
		Output:    output,
		Status:    p.boundedStringPtr(status),
	}
	p.summary.FunctionCalls = append(p.summary.FunctionCalls, call)
	p.appendToolOutputTranscript(itemType, callID, output, status, timestamp, lineNumber, call.Name, call.TurnIndex)
}

func (p *codexSessionParser) appendReasoning(sourceType string, payload map[string]any, timestamp *time.Time, lineNumber int, turn *providersessions.TurnSummary) {
	turn.ReasoningCount++
	order := len(p.summary.Reasoning) + 1
	encryptedContent := p.retainedTextPtr(firstCompactField(payload, "encrypted_content", "encryptedContent"))
	encrypted := encryptedContent != nil
	reasoning := providersessions.ReasoningSummary{
		Order:            order,
		TurnIndex:        intPtr(turn.Index),
		SourceType:       sourceType,
		Text:             p.retainedTextPtr(firstReasoningText(payload)),
		Summary:          p.retainedTextPtr(firstCompactField(payload, "summary")),
		Encrypted:        &encrypted,
		EncryptedContent: encryptedContent,
	}
	p.summary.Reasoning = append(p.summary.Reasoning, reasoning)
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		Encrypted:        reasoning.Encrypted,
		EncryptedContent: reasoning.EncryptedContent,
		LineNumber:       intPtr(lineNumber),
		SourceType:       stringPtrIfNotEmpty(sourceType),
		Summary:          reasoning.Summary,
		Text:             reasoning.Text,
		Timestamp:        timestamp,
		TurnIndex:        reasoning.TurnIndex,
		Type:             providersessions.TranscriptEntryType("reasoning"),
	})
}

func (p *codexSessionParser) appendEventMessageTranscript(
	lineNumber int,
	payloadType string,
	payload map[string]any,
	timestamp *time.Time,
	turn *providersessions.TurnSummary,
) {
	entryType := providersessions.TranscriptEntryType("system_event")
	switch payloadType {
	case "user_message":
		entryType = providersessions.TranscriptEntryType("user_message")
	case "agent_message":
		entryType = providersessions.TranscriptEntryType("assistant_message")
	}

	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		LineNumber: intPtr(lineNumber),
		SourceType: stringPtrIfNotEmpty(payloadType),
		Text:       p.retainedTextPtr(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendResponseMessage(
	payload map[string]any,
	timestamp *time.Time,
	lineNumber int,
	turn *providersessions.TurnSummary,
) {
	role := firstStringField(payload, "role")
	entryType := providersessions.TranscriptEntryType("assistant_message")
	if role == "user" {
		entryType = providersessions.TranscriptEntryType("user_message")
	}

	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		LineNumber: intPtr(lineNumber),
		SourceType: stringPtrIfNotEmpty("message"),
		Text:       p.retainedTextPtr(firstMessageText(payload)),
		Timestamp:  timestamp,
		TurnIndex:  intPtr(turn.Index),
		Type:       entryType,
	})
}

func (p *codexSessionParser) appendToolOutputTranscript(
	itemType string,
	callID string,
	output *string,
	status string,
	timestamp *time.Time,
	lineNumber int,
	name *string,
	turnIndex *int,
) {
	p.appendTranscriptEntry(providersessions.TranscriptEntry{
		CallID:     p.boundedStringPtr(callID),
		LineNumber: intPtr(lineNumber),
		Name:       name,
		Output:     output,
		SourceType: stringPtrIfNotEmpty(itemType),
		Status:     p.boundedStringPtr(status),
		Timestamp:  timestamp,
		TurnIndex:  turnIndex,
		Type:       providersessions.TranscriptEntryType("tool_output"),
	})
}

func (p *codexSessionParser) appendTranscriptEntry(entry providersessions.TranscriptEntry) {
	if !p.budget.canRetainTranscript() {
		p.recordTranscriptLimit(intValue(entry.LineNumber))
		return
	}
	if len(p.transcript) > 0 && isDuplicateTranscriptMessage(p.transcript[len(p.transcript)-1], entry) {
		return
	}
	entry.Order = len(p.transcript) + 1
	p.transcript = append(p.transcript, entry)
	p.budget.retainedTranscript()
}

func isDuplicateTranscriptMessage(previous, next providersessions.TranscriptEntry) bool {
	if previous.Type != next.Type || !isTranscriptMessageType(next.Type) {
		return false
	}
	if stringValue(previous.Text) == "" || stringValue(previous.Text) != stringValue(next.Text) {
		return false
	}
	if intValue(previous.TurnIndex) != intValue(next.TurnIndex) {
		return false
	}
	return isCodexMirrorMessageSource(previous.SourceType, next.SourceType)
}

func isTranscriptMessageType(entryType providersessions.TranscriptEntryType) bool {
	return entryType == providersessions.TranscriptUserMessage ||
		entryType == providersessions.TranscriptAssistantMessage
}

func isCodexMirrorMessageSource(previous, next *string) bool {
	previousSource := stringValue(previous)
	nextSource := stringValue(next)
	return isCodexMessageMirrorSource(previousSource, nextSource) ||
		isCodexMessageMirrorSource(nextSource, previousSource)
}

func isCodexMessageMirrorSource(eventMessageSource, responseItemSource string) bool {
	return (eventMessageSource == "agent_message" || eventMessageSource == "user_message") && responseItemSource == "message"
}

func (p *codexSessionParser) recordTokenUsage(payload map[string]any) {
	usage, ok := nestedMap(payload, "info.total_token_usage")
	if !ok {
		return
	}
	inputTokens, inputTokensPresent := intField(usage, "input_tokens")
	if inputTokensPresent && inputTokens >= 0 {
		p.summary.CumulativeInputTokens = append(p.summary.CumulativeInputTokens, inputTokens)
	}
	p.summary.TokenUsage = &providersessions.TokenUsage{
		InputTokens:           intPtrIfPresent(inputTokens, inputTokensPresent),
		CachedInputTokens:     intPtrIfPresent(intField(usage, "cached_input_tokens")),
		OutputTokens:          intPtrIfPresent(intField(usage, "output_tokens")),
		ReasoningOutputTokens: intPtrIfPresent(intField(usage, "reasoning_output_tokens")),
		TotalTokens:           intPtrIfPresent(intField(usage, "total_tokens")),
	}
}

func (p *codexSessionParser) recordUnknownEvent(lineNumber int, eventType string, payloadType string) {
	p.summary.UnknownEventCount++
	if !p.budget.canRecordDiagnostic() {
		p.appendDiagnostic(lineNumber, diagnosticInspectionDiagnosticLimit)
		return
	}
	sanitizedEventType := sanitizeUnknownEventLabel(eventType)
	sanitizedPayloadType := sanitizeUnknownEventLabel(payloadType)
	p.summary.UnknownEvents = append(p.summary.UnknownEvents, providersessions.UnknownEvent{
		LineNumber:  lineNumber,
		Type:        stringPtrIfNotEmpty(sanitizedEventType),
		PayloadType: stringPtrIfNotEmpty(sanitizedPayloadType),
	})
	p.budget.recordedDiagnostic()
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
			return strings.Join(parts, "\n\n")
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
	return truncateCodexText(strings.TrimSpace(value), maxCodexRetainedFieldBytes)
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
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func intPtrIfPresent(value int, ok bool) *int {
	if !ok {
		return nil
	}
	return &value
}

func intPtr(value int) *int {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
