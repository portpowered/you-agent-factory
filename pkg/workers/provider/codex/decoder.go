package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const diagnosticMessage = "codex JSONL record could not be decoded"

const maxJSONLRecordBytes = 1024 * 1024

type Decoder struct {
	context                    adapter.DecoderContext
	stdout                     []byte
	threadID                   string
	turnID                     string
	turnSequence               int
	discardLine                bool
	selectedTerminal           *responseevents.Draft
	selectedTerminalRecognized bool
	terminalFlushed            bool
}

func NewDecoder(input adapter.DecoderContext) *Decoder { return &Decoder{context: input} }

func (d *Decoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}
	var result adapter.DecodeResult
	chunk := observation.Chunk
	for len(chunk) > 0 {
		if d.discardLine {
			newline := bytes.IndexByte(chunk, '\n')
			if newline < 0 {
				return result, nil
			}
			d.discardLine = false
			chunk = chunk[newline+1:]
			continue
		}

		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if len(d.stdout)+len(chunk) > maxJSONLRecordBytes {
				d.stdout = nil
				d.discardLine = true
				result = appendResult(result, oversizedRecordDiagnostic())
				return result, nil
			}
			d.stdout = append(d.stdout, chunk...)
			return result, nil
		}

		if len(d.stdout)+newline > maxJSONLRecordBytes {
			result = appendResult(result, oversizedRecordDiagnostic())
		} else {
			d.stdout = append(d.stdout, chunk[:newline]...)
			result = appendResult(result, d.decodeRecord(d.stdout))
		}
		d.stdout = d.stdout[:0]
		chunk = chunk[newline+1:]
	}
	return result, nil
}

func (d *Decoder) Flush(_ context.Context, input adapter.FlushContext) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	if d.discardLine {
		d.stdout = nil
		d.discardLine = false
	} else if len(bytes.TrimSpace(d.stdout)) != 0 {
		result = d.decodeRecord(d.stdout)
		d.stdout = nil
	}
	if !d.terminalFlushed && input.Reason != adapter.FlushReasonCanceled {
		if terminal, ok := d.terminalDraft(); ok {
			result = appendResult(result, oneDraft(terminal))
		}
	}
	d.terminalFlushed = true
	return result, nil
}

func oversizedRecordDiagnostic() adapter.DecodeResult {
	return diagnostic("codex_oversized_record", "codex JSONL record exceeded the safe size limit")
}

type recordEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
	Usage    *usageRecord    `json:"usage"`
	Error    *threadError    `json:"error"`
	Message  string          `json:"message"`
}

type usageRecord struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

type threadError struct {
	Message string `json:"message"`
}

func (d *Decoder) decodeRecord(raw []byte) adapter.DecodeResult {
	if len(bytes.TrimSpace(raw)) == 0 {
		return adapter.DecodeResult{}
	}
	var record recordEnvelope
	if err := json.Unmarshal(raw, &record); err != nil {
		return diagnostic("codex_malformed_json", diagnosticMessage)
	}
	switch record.Type {
	case "thread.started":
		d.threadID = strings.TrimSpace(record.ThreadID)
		if d.threadID == "" {
			return diagnostic("codex_malformed_thread_started", diagnosticMessage)
		}
		return oneDraft(d.lifecycleDraft(responseevents.KindSession, responseevents.PhaseStarted, "started", "thread.started"))
	case "turn.started":
		d.turnSequence++
		d.turnID = fmt.Sprintf("codex-turn-%d", d.turnSequence)
		return oneDraft(d.lifecycleDraft(responseevents.KindTurn, responseevents.PhaseStarted, "started", "turn.started"))
	case "turn.completed":
		if record.Usage == nil {
			return diagnostic("codex_malformed_turn_completed", diagnosticMessage)
		}
		return appendResult(oneDraft(d.usageDraft(*record.Usage)), oneDraft(d.lifecycleDraft(responseevents.KindTurn, responseevents.PhaseCompleted, "completed", "turn.completed")))
	case "turn.failed":
		if record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
			return diagnostic("codex_malformed_turn_failed", diagnosticMessage)
		}
		d.errorDraft("turn.failed", record.Error.Message)
		return adapter.DecodeResult{}
	case "error":
		if strings.TrimSpace(record.Message) == "" {
			return diagnostic("codex_malformed_error", diagnosticMessage)
		}
		d.errorDraft("error", record.Message)
		return adapter.DecodeResult{}
	case "item.started", "item.updated", "item.completed":
		return d.decodeItem(record.Type, record.Item)
	default:
		return diagnostic("codex_unknown_event", "codex JSONL event type is not supported: unknown")
	}
}

func (d *Decoder) usageDraft(usage usageRecord) responseevents.Draft {
	payload := mustJSON(responseevents.UsagePayload{
		InputTokens: usage.InputTokens, CachedInputTokens: usage.CachedInputTokens,
		OutputTokens: usage.OutputTokens, ReasoningOutputTokens: usage.ReasoningOutputTokens,
	})
	return responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ProviderSessionRef: d.threadID, Kind: responseevents.KindUsage, Phase: responseevents.PhaseUpdated,
		Provenance: provenance("turn.completed", responseevents.RepresentationSnapshot), Payload: payload,
	}
}

func (d *Decoder) errorDraft(nativeType, message string) responseevents.Draft {
	failure, recognized := classifyTerminalMessage(nativeType, message, d.threadID)
	payload := mustJSON(responseevents.ErrorPayload{
		Code: "codex_" + strings.ReplaceAll(nativeType, ".", "_"), Message: failure.Message,
		Retryable: failure.Retryable,
	})
	draft := responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ProviderSessionRef: d.threadID, Kind: responseevents.KindError, Phase: responseevents.PhaseFailed,
		Provenance: provenance(nativeType, responseevents.RepresentationNotification), Payload: payload,
	}
	if shouldSelectTerminalFailure(d.selectedTerminalRecognized, recognized) {
		d.selectedTerminal = &draft
	}
	d.selectedTerminalRecognized = d.selectedTerminalRecognized || recognized
	return draft
}

func (d *Decoder) terminalDraft() (responseevents.Draft, bool) {
	if d.selectedTerminal == nil {
		return responseevents.Draft{}, false
	}
	return *d.selectedTerminal, true
}

func (d *Decoder) lifecycleDraft(kind responseevents.Kind, phase responseevents.Phase, status, nativeType string) responseevents.Draft {
	var payload []byte
	if kind == responseevents.KindSession {
		payload, _ = json.Marshal(responseevents.SessionPayload{Status: status})
	} else {
		payload, _ = json.Marshal(responseevents.TurnPayload{TurnIndex: d.turnSequence, Status: status})
	}
	return responseevents.Draft{RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ProviderSessionRef: d.threadID, Kind: kind, Phase: phase,
		Provenance: provenance(nativeType, responseevents.RepresentationNotification), Payload: payload}
}

func provenance(nativeType string, representation responseevents.Representation) responseevents.Provenance {
	return responseevents.Provenance{Provider: "codex", NativeEventType: nativeType,
		Delivery: responseevents.DeliveryNativeStream, Representation: representation, Fidelity: responseevents.FidelityNormalized}
}

func safeDiscriminator(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func diagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func oneDraft(draft responseevents.Draft) adapter.DecodeResult {
	if err := responseevents.ValidateDraft(draft); err != nil {
		return diagnostic("codex_invalid_draft", diagnosticMessage)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{draft}}
}

func appendResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

var _ adapter.Decoder = (*Decoder)(nil)
