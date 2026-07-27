package cursor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	CursorOutputFormatJSON       = "json"
	CursorOutputFormatStreamJSON = "stream-json"

	streamUnknownEventPreviewLimit = 48
)

type StreamFragmentKind string

const (
	StreamFragmentKindProgress StreamFragmentKind = "PROGRESS_FRAGMENT"
	StreamFragmentKindResponse StreamFragmentKind = "RESPONSE_FRAGMENT"
)

type StreamFragment struct {
	Kind            StreamFragmentKind
	Payload         string
	ProviderSession *workerexecution.ProviderSessionMetadata
}

type StreamParser struct {
	provider string
	observer func(StreamFragment)

	pending         []byte
	emittedResponse string
}

func NewStreamParser(provider string, observer func(StreamFragment)) *StreamParser {
	return &StreamParser{
		provider: strings.TrimSpace(provider),
		observer: observer,
	}
}

func (p *StreamParser) Consume(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	p.pending = append(p.pending, chunk...)
	p.consumeCompleteLines(false)
}

func (p *StreamParser) Flush() {
	p.consumeCompleteLines(true)
}

func parseInferenceStreamResult(
	provider string,
	stdout []byte,
	requestedSession *workerexecution.ProviderSessionMetadata,
) (*InferenceResult, *ParseFailure) {
	parser := NewStreamParser(provider, nil)
	parser.Consume(stdout)
	parser.Flush()

	var result *InferenceResult
	var terminalFailure *ParseFailure
	session := cloneCursorProviderSession(requestedSession)
	lines := splitNonEmptyLines(stdout)
	for _, line := range lines {
		if observed := cursorProviderSessionFromStructuredLine(provider, line); observed != nil {
			session = observed
		}
		parsed, ok, failure := parseStreamResultLine(provider, line, session)
		if failure != nil {
			result = nil
			terminalFailure = failure
			continue
		}
		if ok {
			result = parsed
			terminalFailure = nil
		}
	}
	if terminalFailure != nil {
		return nil, terminalFailure
	}
	if result == nil {
		return nil, resultParseFailureWithSession(
			provider, "cursor stream-json output missing terminal result event", nil, session,
		)
	}
	return result, nil
}

func (p *StreamParser) consumeCompleteLines(flushRemainder bool) {
	for {
		index := bytes.IndexByte(p.pending, '\n')
		if index < 0 {
			if flushRemainder {
				line := strings.TrimSpace(string(p.pending))
				p.pending = nil
				if line != "" {
					p.consumeLine(line)
				}
			}
			return
		}

		line := strings.TrimSpace(string(p.pending[:index]))
		p.pending = append([]byte(nil), p.pending[index+1:]...)
		if line == "" {
			continue
		}
		p.consumeLine(line)
	}
}

func (p *StreamParser) consumeLine(line string) {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		p.emit(StreamFragmentKindProgress, "Cursor stream ignored malformed JSON record", nil)
		return
	}

	switch eventType := stringField(event, "type"); eventType {
	case "system":
		if stringField(event, "subtype") == "init" {
			p.emit(StreamFragmentKindProgress, "Cursor session initialized", streamProviderSession(p.provider, event))
		}
	case "assistant":
		if !shouldEmitAssistantDelta(event) {
			return
		}
		if text := streamMessageText(event); text != "" {
			p.emitResponse(text, streamProviderSession(p.provider, event))
		}
	case "tool_call":
		subtype := stringField(event, "subtype")
		if subtype == "" {
			return
		}
		toolName := streamToolCallName(event)
		if toolName == "" {
			toolName = "tool_call"
		}
		p.emit(StreamFragmentKindProgress, fmt.Sprintf("Cursor %s %s", toolName, subtype), streamProviderSession(p.provider, event))
	case ResultTypeResult:
		p.consumeResultEvent(event)
	default:
		p.emit(StreamFragmentKindProgress, unknownStreamEventMessage(eventType), nil)
	}
}

func (p *StreamParser) consumeResultEvent(event map[string]any) {
	subtype := stringField(event, "subtype")
	if subtype == "" {
		p.emit(StreamFragmentKindProgress, "Cursor result finished", streamProviderSession(p.provider, event))
		return
	}

	session := streamProviderSession(p.provider, event)
	resultText := boundedText(rawStringField(event, "result"), PublishedTextLimit)
	isError, _ := event["is_error"].(bool)
	if subtype == ResultSubtypeSuccess && !isError {
		p.emitResultResponse(resultText, session)
		return
	}

	p.emit(StreamFragmentKindProgress, streamResultDiagnostic(subtype, resultText), session)
}

func (p *StreamParser) emit(kind StreamFragmentKind, payload string, session *workerexecution.ProviderSessionMetadata) {
	if p == nil || p.observer == nil || strings.TrimSpace(payload) == "" {
		return
	}
	p.observer(StreamFragment{
		Kind:            kind,
		Payload:         payload,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(session),
	})
}

func (p *StreamParser) emitResponse(payload string, session *workerexecution.ProviderSessionMetadata) {
	if p == nil || strings.TrimSpace(payload) == "" {
		return
	}
	p.emit(StreamFragmentKindResponse, payload, session)
	p.emittedResponse += payload
}

func (p *StreamParser) emitResultResponse(resultText string, session *workerexecution.ProviderSessionMetadata) {
	if strings.TrimSpace(resultText) == "" {
		p.emit(StreamFragmentKindProgress, "Cursor result completed", session)
		return
	}
	if p.emittedResponse == "" {
		p.emitResponse(resultText, session)
		return
	}
	if resultText == p.emittedResponse {
		return
	}
	if strings.HasPrefix(resultText, p.emittedResponse) {
		p.emitResponse(resultText[len(p.emittedResponse):], session)
		return
	}
	p.emit(StreamFragmentKindProgress, "Cursor result completed", session)
}

func parseStreamResultLine(
	provider string,
	line string,
	requestedSession *workerexecution.ProviderSessionMetadata,
) (*InferenceResult, bool, *ParseFailure) {
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil, false, nil
	}
	if stringField(event, "type") != ResultTypeResult {
		return nil, false, nil
	}

	payloadBytes, err := json.Marshal(event)
	if err != nil {
		return nil, false, resultParseFailure(provider, "cursor stream-json result event could not be re-encoded", err)
	}

	var payload resultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, false, resultParseFailure(provider, fmt.Sprintf("cursor stream-json result event was invalid JSON: %v", err), err)
	}
	if payload.Subtype != ResultSubtypeSuccess {
		return nil, false, resultErrorSubtypeWithSession(provider, payload, requestedSession)
	}
	if payload.IsError {
		return nil, false, resultErrorSubtypeWithSession(provider, payload, requestedSession)
	}

	session := canonicalProviderSession(provider, payload.SessionID)
	if session == nil {
		session = cloneCursorProviderSession(requestedSession)
	}
	if session == nil {
		return nil, false, resultParseFailure(
			provider, "cursor stream-json success result is missing or invalid session_id", nil,
		)
	}

	return &InferenceResult{
		Content:          safeCursorPublishedText(payload.Result),
		ProviderSession:  session,
		ResponseMetadata: responseMetadataFromPayload(payload),
	}, true, nil
}

func cursorProviderSessionFromStructuredLine(
	provider string,
	line string,
) *workerexecution.ProviderSessionMetadata {
	var record cursorStreamRecord
	if json.Unmarshal([]byte(line), &record) != nil {
		return nil
	}
	if !cursorStructuredRecordCarriesSession(record) {
		return nil
	}
	return canonicalProviderSession(provider, record.SessionID)
}

func cursorStructuredRecordCarriesSession(record cursorStreamRecord) bool {
	switch strings.TrimSpace(record.Type) {
	case "system":
		return strings.TrimSpace(record.Subtype) == "init"
	case "assistant":
		return acceptedCursorAssistantRecord(record)
	case "tool_call":
		return acceptedCursorToolRecord(record)
	case ResultTypeResult:
		return true
	default:
		return false
	}
}

func acceptedCursorAssistantRecord(record cursorStreamRecord) bool {
	if record.TimestampMS == nil || strings.TrimSpace(record.ModelCallID) != "" {
		return false
	}
	var message cursorAssistantMessage
	return json.Unmarshal(record.Message, &message) == nil &&
		cursorAssistantText(message.Content) != ""
}

func acceptedCursorToolRecord(record cursorStreamRecord) bool {
	if strings.TrimSpace(record.CallID) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(record.Subtype)) {
	case "started", "completed", "failed", "canceled", "cancelled":
	default:
		return false
	}
	_, _, _, malformed := decodeCursorToolDetails(record.ToolCall)
	return !malformed
}

func splitNonEmptyLines(stdout []byte) []string {
	rawLines := strings.Split(strings.ReplaceAll(string(stdout), "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func streamProviderSession(provider string, event map[string]any) *workerexecution.ProviderSessionMetadata {
	return canonicalProviderSession(provider, rawStringField(event, "session_id"))
}

func shouldEmitAssistantDelta(event map[string]any) bool {
	if _, ok := event["timestamp_ms"]; !ok {
		return false
	}
	return stringField(event, "model_call_id") == ""
}

func streamMessageText(event map[string]any) string {
	message, _ := event["message"].(map[string]any)
	if message == nil {
		return ""
	}
	content, _ := message["content"].([]any)
	parts := make([]string, 0, len(content))
	for _, item := range content {
		mapped, ok := item.(map[string]any)
		if !ok || stringField(mapped, "type") != "text" {
			continue
		}
		text := rawStringField(mapped, "text")
		if text != "" {
			parts = append(parts, text)
		}
	}
	return boundedText(strings.Join(parts, ""), PublishedTextLimit)
}

func streamToolCallName(event map[string]any) string {
	toolCall, _ := event["tool_call"].(map[string]any)
	if toolCall == nil {
		return ""
	}
	if function, _ := toolCall["function"].(map[string]any); function != nil {
		if name := stringField(function, "name"); name != "" {
			return boundedTrimmedText(name, PublishedDiagnosticLimit)
		}
	}
	for key := range toolCall {
		if strings.TrimSpace(key) != "" {
			return boundedTrimmedText(key, PublishedDiagnosticLimit)
		}
	}
	return ""
}

func streamResultDiagnostic(subtype, resultText string) string {
	reason := classifyTerminalFailure(subtype, resultText)
	return "Cursor reported a failed result: " + cursorFailureGuidance(reason)
}

func stringField(values map[string]any, key string) string {
	return strings.TrimSpace(rawStringField(values, key))
}

func rawStringField(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return value
}

func unknownStreamEventMessage(eventType string) string {
	normalized := strings.TrimSpace(eventType)
	if normalized == "" {
		return "Cursor stream ignored unknown event"
	}
	normalized = boundedTrimmedText(normalized, streamUnknownEventPreviewLimit)
	return fmt.Sprintf("Cursor stream ignored unknown event type %q", normalized)
}
