package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maximumBufferedRecordBytes = 256 * 1024
	maximumSummaryBytes        = 256
	maximumToolInputBytes      = 64 * 1024
	maxDetailBytes             = 1024
)

type decoder struct {
	attemptID string

	pending []byte
	flushed bool

	sessionID     string
	turnSequence  int
	turnID        string
	messageID     string
	messageText   strings.Builder
	completedText map[string]string
	toolNames     map[string]string
	toolSummaries map[string]string

	progress        []providers.ExecuteProgress
	declaredFailure *providers.ExecuteFailure
	decodeErr       error
}

type nativeMessage struct {
	ID           string        `json:"id"`
	Role         string        `json:"role"`
	Content      []nativeBlock `json:"content"`
	StopReason   string        `json:"stopReason"`
	ErrorMessage string        `json:"errorMessage"`
}

type nativeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type assistantMessageEvent struct {
	Type         string `json:"type"`
	Delta        string `json:"delta"`
	ContentIndex int    `json:"contentIndex"`
	PartialJSON  string `json:"partialJson"`
	ToolCallID   string `json:"toolCallId"`
	ToolName     string `json:"toolName"`
}

type nativeEnvelope struct {
	Type                  string                 `json:"type"`
	ID                    string                 `json:"id"`
	Message               *nativeMessage         `json:"message"`
	AssistantMessageEvent *assistantMessageEvent `json:"assistantMessageEvent"`
	ToolCallID            string                 `json:"toolCallId"`
	ToolName              string                 `json:"toolName"`
	Args                  json.RawMessage        `json:"args"`
	PartialResult         json.RawMessage        `json:"partialResult"`
	Result                json.RawMessage        `json:"result"`
	IsError               bool                   `json:"isError"`
	Attempt               *int                   `json:"attempt"`
	RetryDelayMS          *int64                 `json:"retryDelayMs"`
	ErrorStatus           json.RawMessage        `json:"errorStatus"`
}

func newDecoder(attemptID string) *decoder {
	return &decoder{
		attemptID:     attemptID,
		completedText: make(map[string]string),
		toolNames:     make(map[string]string),
		toolSummaries: make(map[string]string),
	}
}

func (d *decoder) observe(chunk []byte) error {
	if d.flushed {
		return errors.New("pi stream received output after finalization")
	}
	if len(chunk) == 0 {
		return nil
	}
	if len(d.pending)+len(chunk) > maximumBufferedRecordBytes {
		d.pending = nil
		d.markDecodeFailure("oversized_record")
		return nil
	}
	d.pending = append(d.pending, chunk...)
	return d.drain(false)
}

func (d *decoder) flush() error {
	if d.flushed {
		return errors.New("pi stream finalized more than once")
	}
	d.flushed = true
	return d.drain(true)
}

func (d *decoder) drain(flush bool) error {
	for {
		newline := bytes.IndexByte(d.pending, '\n')
		if newline < 0 {
			if !flush || len(bytes.TrimSpace(d.pending)) == 0 {
				return nil
			}
			record := append([]byte(nil), d.pending...)
			d.pending = nil
			return d.decodeRecord(record)
		}
		record := append([]byte(nil), d.pending[:newline]...)
		d.pending = d.pending[newline+1:]
		if len(bytes.TrimSpace(record)) == 0 {
			continue
		}
		if err := d.decodeRecord(record); err != nil {
			return err
		}
	}
}

func (d *decoder) progressFacts() []providers.ExecuteProgress {
	progress := make([]providers.ExecuteProgress, len(d.progress))
	for index := range d.progress {
		progress[index] = d.progress[index].Clone()
	}
	return progress
}

func (d *decoder) sessionRef() *providers.SessionRef {
	sessionID := strings.TrimSpace(d.sessionID)
	if sessionID == "" {
		return nil
	}
	return &providers.SessionRef{
		Provider: providers.IDPi,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}
}

func (d *decoder) decodeRecord(raw []byte) error {
	var envelope nativeEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		d.addDiagnostic("pi_malformed_record")
		return nil
	}
	switch envelope.Type {
	case "session", "agent_start", "agent_end", "turn_start", "turn_end", "message_start":
		return d.decodeLifecycleRecord(envelope)
	case "message_update":
		return d.decodeMessageUpdate(envelope)
	case "message_end":
		return d.decodeMessageEnd(envelope)
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		return d.decodeToolRecord(envelope)
	case "queue_update", "compaction_start", "compaction_end", "auto_retry_start", "auto_retry_end":
		return d.decodeControlRecord(envelope)
	default:
		d.addDiagnostic("pi_unknown_record")
		return nil
	}
}

func (d *decoder) decodeLifecycleRecord(envelope nativeEnvelope) error {
	switch envelope.Type {
	case "session":
		d.sessionID = strings.TrimSpace(envelope.ID)
		d.addProgress("session.started", "started", nil)
	case "agent_start":
		d.addProgress("run.started", "started", map[string]string{"native_event": "agent_start"})
	case "agent_end":
		d.addProgress("run.completed", "completed", map[string]string{"native_event": "agent_end"})
	case "turn_start":
		d.turnSequence++
		d.turnID = fmt.Sprintf("pi-turn-%d", d.turnSequence)
		d.addProgress("turn.started", "started", map[string]string{"turn_id": d.turnID})
	case "turn_end":
		d.addProgress("turn.completed", "completed", map[string]string{"turn_id": d.turnID})
	case "message_start":
		d.messageText.Reset()
		if envelope.Message != nil {
			d.messageID = stableMessageID(envelope.Message.ID, d.attemptID)
		} else {
			d.messageID = stableMessageID("", d.attemptID)
		}
		d.addProgress("message.started", "", messageMetadata(d.messageID))
	default:
		d.addDiagnostic("pi_unknown_record")
	}
	return nil
}

func (d *decoder) decodeToolRecord(envelope nativeEnvelope) error {
	switch envelope.Type {
	case "tool_execution_start":
		return d.decodeToolStart(envelope)
	case "tool_execution_update":
		return d.decodeToolUpdate(envelope)
	case "tool_execution_end":
		return d.decodeToolEnd(envelope)
	default:
		d.addDiagnostic("pi_unknown_record")
		return nil
	}
}

func (d *decoder) decodeControlRecord(envelope nativeEnvelope) error {
	switch envelope.Type {
	case "queue_update", "auto_retry_end":
		return nil
	case "compaction_start":
		d.addProgress("progress.updated", "Pi started context compaction.", map[string]string{
			"label": "context_compaction",
		})
	case "compaction_end":
		d.addProgress("progress.updated", "Pi completed context compaction.", map[string]string{
			"label": "context_compaction",
		})
	case "auto_retry_start":
		d.addProgress("error.updated", "Pi reported a provider API retry.", map[string]string{
			"code":      "pi_api_retry",
			"retryable": "true",
		})
	default:
		d.addDiagnostic("pi_unknown_record")
	}
	return nil
}

func (d *decoder) decodeMessageUpdate(envelope nativeEnvelope) error {
	if envelope.Message != nil {
		if id := stableMessageID(envelope.Message.ID, d.attemptID); id != "" {
			d.messageID = id
		}
	}
	if envelope.AssistantMessageEvent == nil {
		return nil
	}
	event := envelope.AssistantMessageEvent
	switch event.Type {
	case "text_delta":
		if event.Delta == "" {
			return nil
		}
		d.messageText.WriteString(event.Delta)
		d.addProgress("message.delta", boundedDetail(event.Delta), messageMetadata(d.stableMessageID()))
	case "thinking_delta":
		if event.Delta == "" {
			return nil
		}
		d.addProgress("reasoning.delta", boundedDetail(event.Delta), messageMetadata(d.stableMessageID()))
	case "tool_call_delta", "input_json_delta":
		toolCallID := strings.TrimSpace(firstNonEmpty(event.ToolCallID, envelope.ToolCallID))
		if toolCallID == "" {
			d.addDiagnostic("pi_missing_tool_call_id")
			return nil
		}
		summary := boundedToolSummary(firstNonEmpty(event.PartialJSON, event.Delta))
		if summary == d.toolSummaries[toolCallID] {
			return nil
		}
		d.toolSummaries[toolCallID] = summary
		d.addProgress("tool.delta", summary, toolMetadata(toolCallID, d.stableMessageID(), event.Type))
	default:
		d.addDiagnostic("pi_unknown_message_update")
	}
	return nil
}

func (d *decoder) decodeMessageEnd(envelope nativeEnvelope) error {
	if envelope.Message == nil {
		d.addDiagnostic("pi_malformed_message_end")
		return nil
	}
	messageID := stableMessageID(envelope.Message.ID, d.attemptID)
	text := assistantText(*envelope.Message)
	if text == "" {
		text = d.messageText.String()
	}
	fingerprint := messageID + "\x00" + text
	if d.completedText[messageID] == fingerprint {
		return nil
	}
	d.completedText[messageID] = fingerprint
	d.addProgress("message.completed", boundedDetail(text), messageMetadata(messageID))
	return nil
}

func (d *decoder) decodeToolStart(envelope nativeEnvelope) error {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		d.addDiagnostic("pi_missing_tool_call_id")
		return nil
	}
	d.toolNames[toolCallID] = strings.TrimSpace(envelope.ToolName)
	detail := d.toolNames[toolCallID]
	if summary, ok := safeJSONSummary(envelope.Args); ok {
		detail = summary
	}
	d.addProgress("tool.started", boundedDetail(detail), toolMetadata(toolCallID, d.stableMessageID(), envelope.ToolName))
	return nil
}

func (d *decoder) decodeToolUpdate(envelope nativeEnvelope) error {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		d.addDiagnostic("pi_missing_tool_call_id")
		return nil
	}
	if name := strings.TrimSpace(envelope.ToolName); name != "" {
		d.toolNames[toolCallID] = name
	}
	summary := boundedToolSummary(string(envelope.PartialResult))
	if summary == d.toolSummaries[toolCallID] {
		return nil
	}
	d.toolSummaries[toolCallID] = summary
	d.addProgress("tool.delta", summary, toolMetadata(toolCallID, d.stableMessageID(), "tool_execution_update"))
	return nil
}

func (d *decoder) decodeToolEnd(envelope nativeEnvelope) error {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		d.addDiagnostic("pi_missing_tool_call_id")
		return nil
	}
	phase := "tool.completed"
	if envelope.IsError {
		phase = "tool.failed"
	}
	detail := firstNonEmpty(envelope.ToolName, d.toolNames[toolCallID])
	if summary, ok := safeJSONSummary(envelope.Result); ok {
		detail = summary
	}
	d.addProgress(phase, boundedDetail(detail), toolMetadata(toolCallID, d.stableMessageID(), envelope.ToolName))
	return nil
}

func (d *decoder) stableMessageID() string {
	if strings.TrimSpace(d.messageID) != "" {
		return d.messageID
	}
	return stableMessageID("", d.attemptID)
}

func stableMessageID(nativeID, attemptID string) string {
	if id := strings.TrimSpace(nativeID); id != "" {
		return id
	}
	if attemptID != "" {
		return "pi-message-" + attemptID
	}
	return "pi-message"
}

func assistantText(message nativeMessage) string {
	var parts []string
	for _, block := range message.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

func (d *decoder) addProgress(phase, detail string, metadata map[string]string) {
	d.progress = append(d.progress, providers.ExecuteProgress{
		Phase:    phase,
		Detail:   detail,
		Metadata: cloneMetadata(metadata),
	})
}

func (d *decoder) addDiagnostic(code string) {
	d.addProgress("diagnostic", "Pi stream record was omitted", map[string]string{"code": code})
}

func (d *decoder) markDecodeFailure(code string) {
	d.addDiagnostic(code)
	if d.decodeErr == nil {
		d.decodeErr = errors.New("Pi stream could not be decoded safely")
	}
}

func messageMetadata(messageID string) map[string]string {
	if messageID == "" {
		return nil
	}
	return map[string]string{"message_id": messageID}
}

func toolMetadata(toolCallID, messageID, toolName string) map[string]string {
	metadata := map[string]string{
		"correlation_id": toolCallID,
	}
	if messageID != "" {
		metadata["message_id"] = messageID
	}
	if toolName := strings.TrimSpace(toolName); toolName != "" {
		metadata["tool_name"] = boundedDetail(toolName)
	}
	return metadata
}

func boundedDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxDetailBytes {
		return value
	}
	return strings.TrimSpace(value[:maxDetailBytes])
}

func boundedSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximumSummaryBytes {
		return value
	}
	return value[:maximumSummaryBytes] + "…"
}

func safeJSONSummary(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if len(raw) > maximumToolInputBytes {
		return "", false
	}
	if !json.Valid(raw) {
		return "", false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	encoded, err := json.Marshal(sanitizeJSONValue(value, 0))
	if err != nil {
		return "", false
	}
	if len(encoded) > maximumSummaryBytes {
		encoded = encoded[:maximumSummaryBytes]
	}
	return string(encoded), true
}

func boundedToolSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if summary, ok := safeJSONSummary(json.RawMessage(value)); ok {
		return summary
	}
	return boundedSummary(value)
}

func sanitizeJSONValue(value any, depth int) any {
	if depth >= 3 {
		return "<omitted>"
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, nested := range typed {
			if sensitiveSummaryKey(key) {
				result[key] = "<redacted>"
				continue
			}
			result[key] = sanitizeJSONValue(nested, depth+1)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeJSONValue(item, depth+1))
		}
		return result
	case string:
		if sensitiveSummaryValue(typed) {
			return "<redacted>"
		}
		return boundedSummary(typed)
	default:
		return typed
	}
}

func sensitiveSummaryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"token", "secret", "password", "credential", "api_key", "apikey", "authorization", "prompt"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func sensitiveSummaryValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "sk-") || strings.Contains(lower, "bearer ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func piRetryFailure(stdout []byte) *providers.ExecuteFailure {
	var latest *nativeEnvelope
	forEachRecord(stdout, func(raw []byte) {
		var envelope nativeEnvelope
		if json.Unmarshal(raw, &envelope) != nil || envelope.Type != "auto_retry_start" {
			return
		}
		copy := envelope
		latest = &copy
	})
	if latest == nil {
		return nil
	}
	kind := providers.ExecuteFailureKindDependency
	if status, ok := boundedStatus(latest.ErrorStatus); ok && status == 429 {
		kind = providers.ExecuteFailureKindThrottled
	}
	failure := &providers.ExecuteFailure{
		Kind:    kind,
		Message: "Pi reported a retryable provider API failure.",
	}
	sessionID := strings.TrimSpace(latest.ID)
	if sessionID == "" {
		sessionID = sessionIDFromStdout(stdout)
	}
	if sessionID != "" {
		failure.SessionRef = &providers.SessionRef{
			Provider: providers.IDPi,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return failure
}

func boundedStatus(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var number int
	if json.Unmarshal(raw, &number) == nil && number >= 100 && number <= 599 {
		return number, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimSpace(text))
	return number, err == nil && number >= 100 && number <= 599
}

func sessionIDFromStdout(stdout []byte) string {
	var sessionID string
	forEachRecord(stdout, func(raw []byte) {
		if sessionID != "" {
			return
		}
		var record struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Type == "session" {
			sessionID = strings.TrimSpace(record.ID)
		}
	})
	return sessionID
}
