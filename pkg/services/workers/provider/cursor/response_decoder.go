package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const (
	cursorDiagnosticMalformedRecord = "cursor_malformed_record"
	cursorDiagnosticUnknownRecord   = "cursor_unknown_record"
	cursorDiagnosticInvalidSession  = "cursor_invalid_session"
	cursorDiagnosticInvalidToolCall = "cursor_invalid_tool_call"
	cursorDiagnosticStderrIgnored   = "cursor_stderr_ignored"
)

// ResponseEventDecoder translates one Cursor stream-json invocation into
// provider-neutral response-event drafts. It owns only per-invocation decode
// state; publication metadata remains session-owned.
type ResponseEventDecoder struct {
	context            adapter.DecoderContext
	pendingStdout      []byte
	providerSessionRef string
	tools              map[string]cursorToolState
}

// NewResponseEventDecoder creates a stateful Cursor decoder for one invocation.
func NewResponseEventDecoder(input adapter.DecoderContext) *ResponseEventDecoder {
	return &ResponseEventDecoder{context: input, tools: make(map[string]cursorToolState)}
}

// Observe accepts ordered subprocess chunks without assuming record-aligned IO.
func (d *ResponseEventDecoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		if len(bytes.TrimSpace(observation.Chunk)) == 0 {
			return adapter.DecodeResult{}, nil
		}
		return cursorDiagnostic(cursorDiagnosticStderrIgnored, "Cursor stream stderr was ignored"), nil
	}
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}

	d.pendingStdout = append(d.pendingStdout, observation.Chunk...)
	return d.consumeCompleteLines(false)
}

// Flush processes a final unterminated NDJSON record exactly once.
func (d *ResponseEventDecoder) Flush(_ context.Context, input adapter.FlushContext) (adapter.DecodeResult, error) {
	result, err := d.consumeCompleteLines(true)
	if err != nil {
		return result, err
	}
	closed, closeErr := d.closeUnresolvedTools(cursorToolFlushOutcome(input.Reason))
	return appendCursorDecodeResult(result, closed), closeErr
}

func (d *ResponseEventDecoder) consumeCompleteLines(flushRemainder bool) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	for {
		newline := bytes.IndexByte(d.pendingStdout, '\n')
		if newline < 0 {
			if !flushRemainder {
				return result, nil
			}
			remainder := bytes.TrimSpace(d.pendingStdout)
			d.pendingStdout = nil
			if len(remainder) == 0 {
				return result, nil
			}
			decoded, err := d.decodeRecord(remainder)
			return appendCursorDecodeResult(result, decoded), err
		}

		record := bytes.TrimSpace(d.pendingStdout[:newline])
		d.pendingStdout = append([]byte(nil), d.pendingStdout[newline+1:]...)
		if len(record) == 0 {
			continue
		}
		decoded, err := d.decodeRecord(record)
		result = appendCursorDecodeResult(result, decoded)
		if err != nil {
			return result, err
		}
	}
}

type cursorStreamRecord struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype"`
	SessionID   string          `json:"session_id"`
	TimestampMS *int64          `json:"timestamp_ms"`
	ModelCallID string          `json:"model_call_id"`
	Message     json.RawMessage `json:"message"`
	CallID      string          `json:"call_id"`
	ToolCall    json.RawMessage `json:"tool_call"`
	IsError     bool            `json:"is_error"`
	Result      string          `json:"result"`
}

type cursorAssistantMessage struct {
	Role    string                   `json:"role"`
	Content []cursorAssistantContent `json:"content"`
}

type cursorAssistantContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (d *ResponseEventDecoder) decodeRecord(raw []byte) (adapter.DecodeResult, error) {
	var record cursorStreamRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return cursorDiagnostic(cursorDiagnosticMalformedRecord, "Cursor stream ignored a malformed JSON record"), nil
	}

	switch strings.TrimSpace(record.Type) {
	case "system":
		if strings.TrimSpace(record.Subtype) != "init" {
			return cursorDiagnostic(cursorDiagnosticUnknownRecord, "Cursor stream ignored an unsupported system record"), nil
		}
		return d.decodeInitialization(record)
	case "assistant":
		return d.decodeAssistant(record)
	case "tool_call":
		return d.decodeToolCall(record)
	case ResultTypeResult:
		return d.decodeResult(record)
	default:
		return cursorDiagnostic(cursorDiagnosticUnknownRecord, "Cursor stream ignored an unknown record type"), nil
	}
}

func (d *ResponseEventDecoder) decodeInitialization(record cursorStreamRecord) (adapter.DecodeResult, error) {
	session := canonicalProviderSession(string(modelprovider.ProviderCursor), record.SessionID)
	if session == nil {
		return cursorDiagnostic(cursorDiagnosticInvalidSession, "Cursor initialization omitted a valid session identifier"), nil
	}
	d.providerSessionRef = session.ID
	d.markToolsInterrupted(cursorToolGapReconnect)
	payload, err := json.Marshal(responseevents.SessionPayload{Status: "started"})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor session payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindSession, Phase: responseevents.PhaseStarted,
		ItemID: session.ID, ProviderSessionRef: session.ID,
		Provenance: cursorResponseProvenance("system", "init", responseevents.RepresentationNotification, responseevents.FidelityNormalized),
		Payload:    payload,
	}}}, nil
}

func (d *ResponseEventDecoder) decodeAssistant(record cursorStreamRecord) (adapter.DecodeResult, error) {
	if record.TimestampMS == nil || strings.TrimSpace(record.ModelCallID) != "" {
		return adapter.DecodeResult{}, nil
	}
	var message cursorAssistantMessage
	if err := json.Unmarshal(record.Message, &message); err != nil {
		return cursorDiagnostic(cursorDiagnosticMalformedRecord, "Cursor stream ignored a malformed assistant record"), nil
	}
	text := cursorAssistantText(message.Content)
	if text == "" {
		return adapter.DecodeResult{}, nil
	}
	providerRef := d.providerRef(record.SessionID)
	originalLength := len(text)
	text = boundedText(text, PublishedTextLimit)
	fidelity := responseevents.FidelityLossless
	if len(text) != originalLength {
		fidelity = responseevents.FidelityLossy
	}
	payload, err := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         text,
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor assistant payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
		ItemID: d.messageItemID(), ProviderSessionRef: providerRef,
		Provenance: cursorResponseProvenance("assistant", "", responseevents.RepresentationDelta, fidelity),
		Payload:    payload,
	}}}, nil
}

func (d *ResponseEventDecoder) providerRef(sessionID string) string {
	if session := canonicalProviderSession(string(modelprovider.ProviderCursor), sessionID); session != nil {
		d.providerSessionRef = session.ID
	}
	return d.providerSessionRef
}

func (d *ResponseEventDecoder) messageItemID() string {
	correlation := strings.TrimSpace(d.context.RunID)
	if correlation == "" {
		correlation = strings.TrimSpace(d.context.DispatchID)
	}
	if correlation == "" {
		return "cursor-message"
	}
	return "cursor-message/" + correlation
}

func cursorAssistantText(content []cursorAssistantContent) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		if strings.TrimSpace(block.Type) == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

func cursorResponseProvenance(nativeType, nativeSubtype string, representation responseevents.Representation, fidelity responseevents.Fidelity) responseevents.Provenance {
	return responseevents.Provenance{
		Provider: workerexecution.CanonicalProviderSessionProvider(string(modelprovider.ProviderCursor)), NativeEventType: nativeType,
		NativeEventSubtype: nativeSubtype, Delivery: responseevents.DeliveryNativeStream,
		Representation: representation, Fidelity: fidelity,
	}
}

func cursorDiagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func appendCursorDecodeResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

var _ adapter.Decoder = (*ResponseEventDecoder)(nil)
