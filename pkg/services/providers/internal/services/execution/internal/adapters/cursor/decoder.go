package cursor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxRecordBytes        = 256 * 1024
	maxDetailBytes        = 1024
	maxPublishedTextBytes = 2048
	maxDiagnosticBytes    = 96

	resultTypeResult     = "result"
	resultSubtypeSuccess = "success"
)

var safeCursorSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type decoder struct {
	pending         []byte
	discardLine     bool
	flushed         bool
	sessionID       string
	emittedResponse string
	finalContent    string
	finalSessionID  string
	hasResult       bool
	unsafeSuccessSession bool

	progress        []providers.ExecuteProgress
	declaredFailure *providers.ExecuteFailure
	decodeErr       error
}

type nativeEnvelope struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype"`
	SessionID   string          `json:"session_id"`
	IsError     bool            `json:"is_error"`
	Result      string          `json:"result"`
	CallID      string          `json:"call_id"`
	ModelCallID string          `json:"model_call_id"`
	TimestampMS *int64          `json:"timestamp_ms"`
	Message     json.RawMessage `json:"message"`
	ToolCall    json.RawMessage `json:"tool_call"`
}

type nativeMessage struct {
	Role    string         `json:"role"`
	Content []nativeBlock  `json:"content"`
}

type nativeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func newDecoder() *decoder {
	return &decoder{}
}

func (decoder *decoder) observe(chunk []byte) error {
	if decoder.flushed {
		return errors.New("cursor stream received output after finalization")
	}
	for len(chunk) > 0 {
		if decoder.discardLine {
			newline := bytes.IndexByte(chunk, '\n')
			if newline < 0 {
				return nil
			}
			decoder.discardLine = false
			chunk = chunk[newline+1:]
			continue
		}
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if len(decoder.pending)+len(chunk) > maxRecordBytes {
				decoder.pending = nil
				decoder.discardLine = true
				decoder.markDecodeFailure("oversized_record")
				return nil
			}
			decoder.pending = append(decoder.pending, chunk...)
			return nil
		}
		if len(decoder.pending)+newline > maxRecordBytes {
			decoder.markDecodeFailure("oversized_record")
		} else {
			decoder.pending = append(decoder.pending, chunk[:newline]...)
			decoder.decodeRecord(decoder.pending)
		}
		decoder.pending = decoder.pending[:0]
		chunk = chunk[newline+1:]
	}
	return nil
}

func (decoder *decoder) flush() error {
	if decoder.flushed {
		return errors.New("cursor stream finalized more than once")
	}
	decoder.flushed = true
	if decoder.discardLine {
		decoder.pending = nil
		decoder.discardLine = false
		return nil
	}
	if len(bytes.TrimSpace(decoder.pending)) > 0 {
		decoder.decodeRecord(decoder.pending)
	}
	decoder.pending = nil
	return nil
}

func (decoder *decoder) final() (string, *providers.SessionRef, error) {
	if !decoder.flushed {
		return "", nil, errors.New("cursor stream was not finalized")
	}
	if !decoder.hasResult || strings.TrimSpace(decoder.finalContent) == "" {
		return "", nil, errors.New("cursor stream did not contain a terminal result")
	}
	if decoder.unsafeSuccessSession {
		return "", nil, errors.New("cursor stream success result is missing or invalid session_id")
	}
	sessionID := strings.TrimSpace(decoder.finalSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(decoder.sessionID)
	}
	var session *providers.SessionRef
	if safeSessionID(sessionID) {
		session = &providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return decoder.finalContent, session, nil
}

func (decoder *decoder) progressFacts() []providers.ExecuteProgress {
	progress := make([]providers.ExecuteProgress, len(decoder.progress))
	for index := range decoder.progress {
		progress[index] = decoder.progress[index].Clone()
	}
	return progress
}

func (decoder *decoder) decodeRecord(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var envelope nativeEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		decoder.markDecodeFailure("malformed_json")
		return
	}
	decoder.observeSession(envelope.SessionID)
	switch envelope.Type {
	case "system":
		if strings.TrimSpace(envelope.Subtype) == "init" {
			decoder.addProgress("session.started", "started", nil)
		}
	case "assistant":
		decoder.decodeAssistant(envelope)
	case "tool_call":
		decoder.decodeToolCall(envelope)
	case resultTypeResult:
		decoder.decodeResult(envelope)
	default:
		if strings.TrimSpace(envelope.Type) != "" {
			decoder.addDiagnostic("unknown_event")
		}
	}
}

func (decoder *decoder) observeSession(sessionID string) {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" || !safeSessionID(normalized) {
		return
	}
	if decoder.sessionID == "" {
		decoder.sessionID = normalized
	}
}

func (decoder *decoder) decodeAssistant(envelope nativeEnvelope) {
	if envelope.TimestampMS == nil || strings.TrimSpace(envelope.ModelCallID) != "" {
		return
	}
	text := assistantText(envelope.Message)
	if text == "" {
		return
	}
	decoder.emittedResponse += text
	decoder.addProgress(
		"message.delta",
		boundedDetail(text),
		messageMetadata(decoder.stableMessageID()),
	)
}

func (decoder *decoder) decodeToolCall(envelope nativeEnvelope) {
	subtype := strings.ToLower(strings.TrimSpace(envelope.Subtype))
	switch subtype {
	case "started", "completed", "failed", "canceled", "cancelled":
	default:
		return
	}
	callID := strings.TrimSpace(envelope.CallID)
	if callID == "" {
		return
	}
	toolName := toolCallName(envelope.ToolCall)
	if toolName == "" {
		toolName = "tool_call"
	}
	phase := "tool." + subtype
	if subtype == "cancelled" {
		phase = "tool.canceled"
	}
	decoder.addProgress(
		phase,
		boundedDetail(fmt.Sprintf("%s %s", toolName, subtype)),
		toolMetadata(callID, toolName),
	)
}

func (decoder *decoder) decodeResult(envelope nativeEnvelope) {
	subtype := strings.TrimSpace(envelope.Subtype)
	sessionID := strings.TrimSpace(envelope.SessionID)
	if safeSessionID(sessionID) {
		decoder.finalSessionID = sessionID
	}
	if subtype == "" {
		decoder.addProgress("result.completed", "finished", nil)
		return
	}
	resultText := boundedPublishedText(envelope.Result)
	if subtype == resultSubtypeSuccess && !envelope.IsError {
		decoder.hasResult = true
		if sessionID != "" && !safeSessionID(sessionID) {
			decoder.unsafeSuccessSession = true
		}
		if strings.TrimSpace(resultText) == "" {
			decoder.finalContent = strings.TrimSpace(decoder.emittedResponse)
			return
		}
		if decoder.emittedResponse == "" {
			decoder.finalContent = resultText
			return
		}
		if resultText == decoder.emittedResponse {
			decoder.finalContent = resultText
			return
		}
		if strings.HasPrefix(resultText, decoder.emittedResponse) {
			decoder.finalContent = resultText
			return
		}
		decoder.finalContent = resultText
		return
	}
	decoder.declareFailure(providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: boundedDetail("Cursor reported a failed result"),
	})
}

func (decoder *decoder) stableMessageID() string {
	if decoder.sessionID != "" {
		return decoder.sessionID
	}
	return "cursor-stream"
}

func assistantText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var message nativeMessage
	if json.Unmarshal(raw, &message) != nil {
		return ""
	}
	parts := make([]string, 0, len(message.Content))
	for _, block := range message.Content {
		if strings.TrimSpace(block.Type) != "text" {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

func toolCallName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var toolCall map[string]json.RawMessage
	if json.Unmarshal(raw, &toolCall) != nil {
		return ""
	}
	if functionRaw, ok := toolCall["function"]; ok {
		var function map[string]string
		if json.Unmarshal(functionRaw, &function) == nil {
			if name := strings.TrimSpace(function["name"]); name != "" {
				return boundedDiagnostic(name)
			}
		}
	}
	for key := range toolCall {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			return boundedDiagnostic(trimmed)
		}
	}
	return ""
}

func messageMetadata(messageID string) map[string]string {
	return map[string]string{"message_id": messageID}
}

func toolMetadata(correlationID, toolName string) map[string]string {
	return map[string]string{
		"correlation_id": correlationID,
		"tool_name":      toolName,
	}
}

func (decoder *decoder) addProgress(
	phase string,
	detail string,
	metadata map[string]string,
) {
	decoder.progress = append(decoder.progress, providers.ExecuteProgress{
		Phase:    phase,
		Detail:   detail,
		Metadata: cloneMetadata(metadata),
	})
}

func (decoder *decoder) addDiagnostic(code string) {
	decoder.addProgress("diagnostic", "Cursor stream record was omitted", map[string]string{
		"code": code,
	})
}

func (decoder *decoder) markDecodeFailure(code string) {
	decoder.addDiagnostic(code)
	if decoder.decodeErr == nil {
		decoder.decodeErr = errors.New("cursor stream could not be decoded safely")
	}
}

func (decoder *decoder) declareFailure(failure providers.ExecuteFailure) {
	clone := failure.Clone()
	decoder.declaredFailure = &clone
}

func safeSessionID(sessionID string) bool {
	normalized := strings.TrimSpace(sessionID)
	return normalized != "" && safeCursorSessionIDPattern.MatchString(normalized)
}

func boundedDetail(value string) string {
	return boundedText(strings.TrimSpace(value), maxDetailBytes)
}

func boundedPublishedText(value string) string {
	return boundedText(strings.TrimSpace(value), maxPublishedTextBytes)
}

func boundedDiagnostic(value string) string {
	return boundedText(strings.TrimSpace(value), maxDiagnosticBytes)
}

func boundedText(value string, limit int) string {
	if value == "" || limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
