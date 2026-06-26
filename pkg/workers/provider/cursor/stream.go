package cursor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	CursorOutputFormatJSON       = "json"
	CursorOutputFormatStreamJSON = "stream-json"
)

type StreamFragmentKind string

const (
	StreamFragmentKindProgress StreamFragmentKind = "PROGRESS_FRAGMENT"
	StreamFragmentKindResponse StreamFragmentKind = "RESPONSE_FRAGMENT"
)

type StreamFragment struct {
	Kind            StreamFragmentKind
	Payload         string
	ProviderSession *interfaces.ProviderSessionMetadata
}

type StreamParser struct {
	provider string
	observer func(StreamFragment)

	pending []byte
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

func ParseInferenceStreamResult(provider string, stdout []byte) (*InferenceResult, *ParseFailure) {
	parser := NewStreamParser(provider, nil)
	parser.Consume(stdout)
	parser.Flush()

	var result *InferenceResult
	lines := splitNonEmptyLines(stdout)
	for _, line := range lines {
		parsed, ok, failure := parseStreamResultLine(provider, line)
		if failure != nil {
			return nil, failure
		}
		if ok {
			result = parsed
		}
	}
	if result == nil {
		return nil, resultParseFailure(provider, "cursor stream-json output missing terminal result event", nil)
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
		return
	}

	switch stringField(event, "type") {
	case "system":
		if stringField(event, "subtype") == "init" {
			p.emit(StreamFragmentKindProgress, "Cursor session initialized", streamProviderSession(p.provider, event))
		}
	case "assistant":
		if !shouldEmitAssistantDelta(event) {
			return
		}
		if text := streamMessageText(event); text != "" {
			p.emit(StreamFragmentKindResponse, text, streamProviderSession(p.provider, event))
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
	}
}

func (p *StreamParser) emit(kind StreamFragmentKind, payload string, session *interfaces.ProviderSessionMetadata) {
	if p == nil || p.observer == nil || strings.TrimSpace(payload) == "" {
		return
	}
	p.observer(StreamFragment{
		Kind:            kind,
		Payload:         payload,
		ProviderSession: interfaces.CloneProviderSessionMetadata(session),
	})
}

func parseStreamResultLine(provider string, line string) (*InferenceResult, bool, *ParseFailure) {
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
		return nil, false, resultErrorSubtype(provider, payload)
	}
	if payload.IsError {
		return nil, false, &ParseFailure{
			Type:    interfaces.WorkFailureTypeInternalServerError,
			Message: "cursor stream-json result reported is_error=true",
		}
	}

	session := canonicalProviderSession(provider, payload.SessionID)
	if session == nil {
		return nil, false, resultParseFailure(provider, "cursor stream-json success result is missing or invalid session_id", nil)
	}

	return &InferenceResult{
		Content:          payload.Result,
		ProviderSession:  session,
		ResponseMetadata: responseMetadataFromPayload(payload),
	}, true, nil
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

func streamProviderSession(provider string, event map[string]any) *interfaces.ProviderSessionMetadata {
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
	return strings.Join(parts, "")
}

func streamToolCallName(event map[string]any) string {
	toolCall, _ := event["tool_call"].(map[string]any)
	if toolCall == nil {
		return ""
	}
	if function, _ := toolCall["function"].(map[string]any); function != nil {
		if name := stringField(function, "name"); name != "" {
			return name
		}
	}
	for key := range toolCall {
		if strings.TrimSpace(key) != "" {
			return key
		}
	}
	return ""
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
