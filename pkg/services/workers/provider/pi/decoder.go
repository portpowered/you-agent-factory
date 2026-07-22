package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const (
	maximumBufferedRecordBytes = 256 * 1024
	maximumSummaryBytes        = 256
	maximumToolInputBytes      = 64 * 1024
)

type decoder struct {
	context adapter.DecoderContext
	pending []byte

	sessionRef    string
	turnSequence  int
	turnID        string
	messageID     string
	messageText   strings.Builder
	completedText map[string]string
	toolNames     map[string]string
	toolSummaries map[string]string
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

func newDecoder(input adapter.DecoderContext) *decoder {
	return &decoder{
		context:       input,
		completedText: make(map[string]string),
		toolNames:     make(map[string]string),
		toolSummaries: make(map[string]string),
	}
}

func (d *decoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: "pi_stderr_ignored", Message: "Pi stderr output was omitted"}}}, nil
	}
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}
	if len(d.pending)+len(observation.Chunk) > maximumBufferedRecordBytes {
		d.pending = nil
		return safeDiagnostic("pi_record_too_large", "Pi emitted an oversized structured record"), nil
	}
	d.pending = append(d.pending, observation.Chunk...)
	return d.drain(false)
}

func (d *decoder) Flush(_ context.Context, _ adapter.FlushContext) (adapter.DecodeResult, error) {
	return d.drain(true)
}

func (d *decoder) drain(flush bool) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	for {
		newline := bytes.IndexByte(d.pending, '\n')
		if newline < 0 {
			if !flush || len(bytes.TrimSpace(d.pending)) == 0 {
				return result, nil
			}
			record := append([]byte(nil), d.pending...)
			d.pending = nil
			decoded, err := d.decodeRecord(record)
			return appendResult(result, decoded), err
		}
		record := append([]byte(nil), d.pending[:newline]...)
		d.pending = d.pending[newline+1:]
		if len(bytes.TrimSpace(record)) == 0 {
			continue
		}
		decoded, err := d.decodeRecord(record)
		result = appendResult(result, decoded)
		if err != nil {
			return result, err
		}
	}
}

func (d *decoder) decodeRecord(raw []byte) (adapter.DecodeResult, error) {
	var envelope nativeEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return safeDiagnostic("pi_malformed_record", "Pi emitted a malformed structured record"), nil
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
		return safeDiagnostic("pi_unknown_record", "Pi emitted an unsupported additive record"), nil
	}
}

func (d *decoder) decodeLifecycleRecord(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	switch envelope.Type {
	case "session":
		d.sessionRef = strings.TrimSpace(envelope.ID)
		payload, err := marshalPayload(responseevents.SessionPayload{Status: "started"})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindSession, Phase: responseevents.PhaseStarted,
			ProviderSessionRef: d.sessionRef, Provenance: provenance("session", responseevents.RepresentationNotification), Payload: payload,
		})
	case "agent_start":
		payload, err := marshalPayload(responseevents.RunPayload{Status: "started"})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindRun, Phase: responseevents.PhaseStarted,
			ProviderSessionRef: d.sessionRef, Provenance: provenance("agent_start", responseevents.RepresentationNotification), Payload: payload,
		})
	case "agent_end":
		payload, err := marshalPayload(responseevents.RunPayload{Status: "completed"})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindRun, Phase: responseevents.PhaseCompleted,
			ProviderSessionRef: d.sessionRef, Provenance: provenance("agent_end", responseevents.RepresentationNotification), Payload: payload,
		})
	case "turn_start":
		d.turnSequence++
		d.turnID = fmt.Sprintf("pi-turn-%d", d.turnSequence)
		payload, err := marshalPayload(responseevents.TurnPayload{TurnIndex: d.turnSequence, Status: "started"})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTurn, Phase: responseevents.PhaseStarted,
			ProviderSessionRef: d.sessionRef, Provenance: provenance("turn_start", responseevents.RepresentationNotification), Payload: payload,
		})
	case "turn_end":
		payload, err := marshalPayload(responseevents.TurnPayload{TurnIndex: d.turnSequence, Status: "completed"})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTurn, Phase: responseevents.PhaseCompleted,
			ProviderSessionRef: d.sessionRef, Provenance: provenance("turn_end", responseevents.RepresentationNotification), Payload: payload,
		})
	case "message_start":
		d.messageText.Reset()
		if envelope.Message != nil {
			d.messageID = stableMessageID(envelope.Message.ID, d.context)
		} else {
			d.messageID = stableMessageID("", d.context)
		}
		payload, err := marshalPayload(responseevents.MessagePayload{
			Role:          "assistant",
			ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: ""}},
		})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindMessage, Phase: responseevents.PhaseStarted,
			ItemID: d.messageID, ProviderSessionRef: d.sessionRef, Provenance: provenance("message_start", responseevents.RepresentationNotification), Payload: payload,
		})
	default:
		return safeDiagnostic("pi_unknown_record", "Pi emitted an unsupported additive record"), nil
	}
}

func (d *decoder) decodeToolRecord(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	switch envelope.Type {
	case "tool_execution_start":
		return d.decodeToolStart(envelope)
	case "tool_execution_update":
		return d.decodeToolUpdate(envelope)
	case "tool_execution_end":
		return d.decodeToolEnd(envelope)
	default:
		return safeDiagnostic("pi_unknown_record", "Pi emitted an unsupported additive record"), nil
	}
}

func (d *decoder) decodeControlRecord(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	switch envelope.Type {
	case "queue_update", "auto_retry_end":
		return adapter.DecodeResult{}, nil
	case "compaction_start":
		return d.compactionObservation("compaction_start", "Pi started context compaction.")
	case "compaction_end":
		return d.compactionObservation("compaction_end", "Pi completed context compaction.")
	case "auto_retry_start":
		return d.retryObservation(envelope)
	default:
		return safeDiagnostic("pi_unknown_record", "Pi emitted an unsupported additive record"), nil
	}
}

func (d *decoder) decodeMessageUpdate(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	if envelope.Message != nil {
		if id := stableMessageID(envelope.Message.ID, d.context); id != "" {
			d.messageID = id
		}
	}
	if envelope.AssistantMessageEvent == nil {
		return adapter.DecodeResult{}, nil
	}
	event := envelope.AssistantMessageEvent
	switch event.Type {
	case "text_delta":
		if event.Delta == "" {
			return adapter.DecodeResult{}, nil
		}
		d.messageText.WriteString(event.Delta)
		payload, err := marshalPayload(responseevents.MessageDeltaPayload{
			ContentBlockIndex: event.ContentIndex, ContentBlockKind: responseevents.ContentBlockText, TextDelta: event.Delta,
		})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
			ItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
			Provenance: provenance("text_delta", responseevents.RepresentationDelta), Payload: payload,
		})
	case "thinking_delta":
		if event.Delta == "" {
			return adapter.DecodeResult{}, nil
		}
		payload, err := marshalPayload(responseevents.ReasoningPayload{SummaryDelta: event.Delta})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindReasoning, Phase: responseevents.PhaseDelta,
			ItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
			Provenance: provenance("thinking_delta", responseevents.RepresentationDelta), Payload: payload,
		})
	case "tool_call_delta", "input_json_delta":
		toolCallID := strings.TrimSpace(firstNonEmpty(event.ToolCallID, envelope.ToolCallID))
		if toolCallID == "" {
			return safeDiagnostic("pi_missing_tool_call_id", "Pi tool delta omitted stable call identity"), nil
		}
		summary := boundedToolSummary(firstNonEmpty(event.PartialJSON, event.Delta))
		if summary == d.toolSummaries[toolCallID] {
			return adapter.DecodeResult{}, nil
		}
		d.toolSummaries[toolCallID] = summary
		payload, err := marshalPayload(responseevents.ToolDeltaPayload{ToolCallID: toolCallID, OutputDelta: summary})
		if err != nil {
			return adapter.DecodeResult{}, err
		}
		return oneDraft(responseevents.Draft{
			RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTool, Phase: responseevents.PhaseDelta,
			ItemID: toolCallID, ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
			Provenance: provenance(event.Type, responseevents.RepresentationDelta), Payload: payload,
		})
	default:
		return safeDiagnostic("pi_unknown_message_update", "Pi emitted an unsupported assistant message update"), nil
	}
}

func (d *decoder) decodeMessageEnd(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	if envelope.Message == nil {
		return safeDiagnostic("pi_malformed_message_end", "Pi message_end omitted message payload"), nil
	}
	messageID := stableMessageID(envelope.Message.ID, d.context)
	text := assistantText(*envelope.Message)
	if text == "" {
		text = d.messageText.String()
	}
	fingerprint := messageID + "\x00" + text
	if d.completedText[messageID] == fingerprint {
		return adapter.DecodeResult{}, nil
	}
	d.completedText[messageID] = fingerprint
	payload, err := marshalPayload(responseevents.MessagePayload{
		Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: text}},
	})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
		ItemID: messageID, ProviderSessionRef: d.sessionRef,
		Provenance: provenance("message_end", responseevents.RepresentationSnapshot), Payload: payload,
	})
}

func (d *decoder) decodeToolStart(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		return safeDiagnostic("pi_missing_tool_call_id", "Pi tool start omitted stable call identity"), nil
	}
	d.toolNames[toolCallID] = strings.TrimSpace(envelope.ToolName)
	body := responseevents.ToolPayload{ToolCallID: toolCallID, ToolName: d.toolNames[toolCallID], Status: "running"}
	if summary, ok := safeJSONSummary(envelope.Args); ok {
		body.ArgumentsSummary = json.RawMessage(summary)
	}
	payload, err := marshalPayload(body)
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTool, Phase: responseevents.PhaseStarted,
		ItemID: toolCallID, ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
		Provenance: provenance("tool_execution_start", responseevents.RepresentationSnapshot), Payload: payload,
	})
}

func (d *decoder) decodeToolUpdate(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		return safeDiagnostic("pi_missing_tool_call_id", "Pi tool update omitted stable call identity"), nil
	}
	if name := strings.TrimSpace(envelope.ToolName); name != "" {
		d.toolNames[toolCallID] = name
	}
	summary := boundedToolSummary(string(envelope.PartialResult))
	if summary == d.toolSummaries[toolCallID] {
		return adapter.DecodeResult{}, nil
	}
	d.toolSummaries[toolCallID] = summary
	payload, err := marshalPayload(responseevents.ToolDeltaPayload{ToolCallID: toolCallID, OutputDelta: summary})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTool, Phase: responseevents.PhaseDelta,
		ItemID: toolCallID, ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
		Provenance: provenance("tool_execution_update", responseevents.RepresentationDelta), Payload: payload,
	})
}

func (d *decoder) decodeToolEnd(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	toolCallID := strings.TrimSpace(envelope.ToolCallID)
	if toolCallID == "" {
		return safeDiagnostic("pi_missing_tool_call_id", "Pi tool end omitted stable call identity"), nil
	}
	status := "completed"
	phase := responseevents.PhaseCompleted
	if envelope.IsError {
		status = "failed"
		phase = responseevents.PhaseFailed
	}
	body := responseevents.ToolPayload{
		ToolCallID: toolCallID, ToolName: firstNonEmpty(envelope.ToolName, d.toolNames[toolCallID]), Status: status,
	}
	if summary, ok := safeJSONSummary(envelope.Result); ok {
		body.ResultSummary = json.RawMessage(summary)
	}
	payload, err := marshalPayload(body)
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID, Kind: responseevents.KindTool, Phase: phase,
		ItemID: toolCallID, ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
		Provenance: provenance("tool_execution_end", responseevents.RepresentationSnapshot), Payload: payload,
	})
}

func (d *decoder) stableMessageID() string {
	if strings.TrimSpace(d.messageID) != "" {
		return d.messageID
	}
	return stableMessageID("", d.context)
}

func stableMessageID(nativeID string, context adapter.DecoderContext) string {
	if id := strings.TrimSpace(nativeID); id != "" {
		return id
	}
	if context.DispatchID != "" {
		return "pi-message-" + context.DispatchID
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

func provenance(nativeType string, representation responseevents.Representation) responseevents.Provenance {
	return responseevents.Provenance{
		Provider: "pi", NativeEventType: nativeType, Delivery: responseevents.DeliveryNativeStream,
		Representation: representation, Fidelity: responseevents.FidelityNormalized,
	}
}

func marshalPayload(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal Pi canonical payload: %w", err)
	}
	return payload, nil
}

func safeDiagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func oneDraft(draft responseevents.Draft) (adapter.DecodeResult, error) {
	if err := responseevents.ValidateDraft(draft); err != nil {
		return safeDiagnostic("pi_invalid_draft", "Pi emitted an invalid canonical draft"), nil
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{draft}}, nil
}

func appendResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
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

var _ adapter.Decoder = (*decoder)(nil)
