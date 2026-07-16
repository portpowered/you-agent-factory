package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const maxStructuredRecordBytes = 1024 * 1024

type structuredDecoder struct {
	context adapter.DecoderContext
	pending []byte

	discardingOversized bool
	runStarted          bool
	runTerminal         bool
	explicitFailure     bool
	sessionID           string
}

func newStructuredDecoder(input adapter.DecoderContext) *structuredDecoder {
	return &structuredDecoder{context: input}
}

func (d *structuredDecoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		if len(observation.Chunk) == 0 {
			return adapter.DecodeResult{}, nil
		}
		return safeDiagnostic("stderr_ignored", "OpenCode structured stderr was ignored"), nil
	}
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}
	return d.consume(observation.Chunk), nil
}

func (d *structuredDecoder) Flush(_ context.Context, input adapter.FlushContext) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	if !d.discardingOversized && len(bytes.TrimSpace(d.pending)) > 0 {
		result = appendDecodeResult(result, d.decodeRecord(d.pending))
	}
	d.pending = nil
	d.discardingOversized = false
	if !d.runStarted || d.runTerminal {
		return result, nil
	}

	phase, status := responseevents.PhaseCompleted, "completed"
	switch {
	case input.Reason == adapter.FlushReasonCanceled:
		phase, status = responseevents.PhaseCanceled, "canceled"
	case input.Reason == adapter.FlushReasonTerminated || d.explicitFailure:
		phase, status = responseevents.PhaseFailed, "failed"
	}
	result.Drafts = append(result.Drafts, d.terminalRunDraft(phase, status))
	d.runTerminal = true
	return result, nil
}

func (d *structuredDecoder) consume(chunk []byte) adapter.DecodeResult {
	var result adapter.DecodeResult
	for len(chunk) > 0 {
		if d.discardingOversized {
			newline := bytes.IndexByte(chunk, '\n')
			if newline < 0 {
				return result
			}
			d.discardingOversized = false
			chunk = chunk[newline+1:]
			continue
		}

		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			d.pending = append(d.pending, chunk...)
			if len(d.pending) > maxStructuredRecordBytes {
				d.pending = nil
				d.discardingOversized = true
				result = appendDecodeResult(result, safeDiagnostic("oversized_record", "OpenCode emitted an oversized structured record"))
			}
			return result
		}
		d.pending = append(d.pending, chunk[:newline]...)
		if len(d.pending) > maxStructuredRecordBytes {
			result = appendDecodeResult(result, safeDiagnostic("oversized_record", "OpenCode emitted an oversized structured record"))
		} else if len(bytes.TrimSpace(d.pending)) > 0 {
			result = appendDecodeResult(result, d.decodeRecord(d.pending))
		}
		d.pending = nil
		chunk = chunk[newline+1:]
	}
	return result
}

func (d *structuredDecoder) decodeRecord(raw []byte) adapter.DecodeResult {
	record, err := decodeStructuredRecord(raw)
	if err != nil || record.Type == "" {
		return safeDiagnostic("malformed_record", "OpenCode emitted a malformed structured record")
	}
	if sessionID := recordSessionID(record); sessionID != "" {
		d.sessionID = sessionID
	}

	switch record.Type {
	case "step_start":
		return d.ensureRunStarted("step_start")
	case "text":
		return d.decodeText(record)
	case "reasoning":
		return d.decodeReasoning(record)
	case "tool_use":
		return d.decodeTool(record)
	case "step_finish":
		return d.decodeUsage(record)
	case "error":
		return d.decodeError(record)
	default:
		return safeDiagnostic("unknown_record", "OpenCode emitted an unsupported additive structured record")
	}
}

func (d *structuredDecoder) decodeText(record structuredRecord) adapter.DecodeResult {
	if !validCorrelation(record.Part.ID) || strings.TrimSpace(record.Part.Text) == "" {
		return safeDiagnostic("invalid_text_record", "OpenCode emitted an invalid text record")
	}
	phase := responseevents.PhaseDelta
	representation := responseevents.RepresentationDelta
	var payload any = responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0, ContentBlockKind: responseevents.ContentBlockText,
		TextDelta: boundedText(record.Part.Text),
	}
	if record.Part.Time.End != nil {
		phase = responseevents.PhaseCompleted
		representation = responseevents.RepresentationSnapshot
		payload = responseevents.MessagePayload{Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{
			Kind: responseevents.ContentBlockText, Text: boundedText(record.Part.Text),
		}}}
	}
	draft := d.draft(record.Type, responseevents.KindMessage, phase, representation, payload)
	draft.ItemID = record.Part.ID
	if validCorrelation(record.Part.MessageID) {
		draft.ParentItemID = record.Part.MessageID
	}
	return d.withRunStart(draft, record.Type)
}

func (d *structuredDecoder) decodeReasoning(record structuredRecord) adapter.DecodeResult {
	if !validCorrelation(record.Part.ID) || strings.TrimSpace(record.Part.Text) == "" {
		return safeDiagnostic("invalid_reasoning_record", "OpenCode emitted an invalid reasoning record")
	}
	phase := responseevents.PhaseDelta
	representation := responseevents.RepresentationDelta
	payload := responseevents.ReasoningPayload{SummaryDelta: boundedText(record.Part.Text)}
	if record.Part.Time.End != nil {
		phase = responseevents.PhaseCompleted
		representation = responseevents.RepresentationSnapshot
		payload = responseevents.ReasoningPayload{Summary: boundedText(record.Part.Text)}
	}
	draft := d.draft(record.Type, responseevents.KindReasoning, phase, representation, payload)
	draft.ItemID = record.Part.ID
	return d.withRunStart(draft, record.Type)
}

func (d *structuredDecoder) decodeTool(record structuredRecord) adapter.DecodeResult {
	if !validCorrelation(record.Part.ID) || !validCorrelation(record.Part.CallID) || strings.TrimSpace(record.Part.Tool) == "" {
		return safeDiagnostic("invalid_tool_record", "OpenCode emitted an invalid tool lifecycle record")
	}
	phase := responseevents.PhaseStarted
	status := strings.ToLower(strings.TrimSpace(record.Part.State.Status))
	switch status {
	case "completed":
		phase = responseevents.PhaseCompleted
	case "error", "failed":
		phase = responseevents.PhaseFailed
	case "pending", "running", "":
		status = "running"
	default:
		return safeDiagnostic("invalid_tool_status", "OpenCode emitted an unsupported tool lifecycle status")
	}
	draft := d.draft(record.Type, responseevents.KindTool, phase, responseevents.RepresentationSnapshot, responseevents.ToolPayload{
		ToolCallID: record.Part.CallID, ToolName: boundedName(record.Part.Tool), Status: status,
	})
	draft.ItemID = record.Part.ID
	if validCorrelation(record.Part.MessageID) {
		draft.ParentItemID = record.Part.MessageID
	}
	return d.withRunStart(draft, record.Type)
}

func (d *structuredDecoder) decodeUsage(record structuredRecord) adapter.DecodeResult {
	usage := record.Part.Tokens
	if usage.Input < 0 || usage.Output < 0 || usage.Reasoning < 0 {
		return safeDiagnostic("invalid_usage_record", "OpenCode emitted an invalid usage record")
	}
	draft := d.draft(record.Type, responseevents.KindUsage, responseevents.PhaseUpdated, responseevents.RepresentationNotification, responseevents.UsagePayload{
		InputTokens: usage.Input, OutputTokens: usage.Output, TotalTokens: usage.Input + usage.Output + usage.Reasoning,
	})
	if validCorrelation(record.Part.ID) {
		draft.ItemID = record.Part.ID
	}
	return d.withRunStart(draft, record.Type)
}

func (d *structuredDecoder) decodeError(record structuredRecord) adapter.DecodeResult {
	failure := classifyStructuredError(record.Error.Name, record.Error.Data)
	d.explicitFailure = true
	draft := d.draft(record.Type, responseevents.KindError, responseevents.PhaseFailed, responseevents.RepresentationNotification, responseevents.ErrorPayload{
		Code: string(failure.failureType), Message: failure.message, Retryable: failure.retryable,
	})
	return d.withRunStart(draft, record.Type)
}

func (d *structuredDecoder) ensureRunStarted(nativeType string) adapter.DecodeResult {
	if d.runStarted {
		return adapter.DecodeResult{}
	}
	d.runStarted = true
	return adapter.DecodeResult{Drafts: []responseevents.Draft{d.runDraft(responseevents.PhaseStarted, "started", nativeType)}}
}

func (d *structuredDecoder) withRunStart(draft responseevents.Draft, nativeType string) adapter.DecodeResult {
	started := d.ensureRunStarted(nativeType)
	started.Drafts = append(started.Drafts, draft)
	return started
}

func (d *structuredDecoder) runDraft(phase responseevents.Phase, status, nativeType string) responseevents.Draft {
	return d.draft(nativeType, responseevents.KindRun, phase, responseevents.RepresentationNotification, responseevents.RunPayload{Status: status})
}

func (d *structuredDecoder) terminalRunDraft(phase responseevents.Phase, status string) responseevents.Draft {
	draft := d.runDraft(phase, status, "process_outcome")
	draft.Provenance.Delivery = responseevents.DeliverySynthesized
	draft.Provenance.Fidelity = responseevents.FidelityLifecycleOnly
	return draft
}

func (d *structuredDecoder) draft(nativeType string, kind responseevents.Kind, phase responseevents.Phase, representation responseevents.Representation, payload any) responseevents.Draft {
	return responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: kind, Phase: phase, ProviderSessionRef: d.sessionID,
		Provenance: responseevents.Provenance{
			Provider: string(adapter.Identity("opencode")), NativeEventType: nativeType,
			Delivery: responseevents.DeliveryNativeStream, Representation: representation,
			Fidelity: responseevents.FidelityNormalized,
		},
		Payload: marshalCanonicalPayload(payload),
	}
}

func boundedName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxCorrelationLength {
		return value[:maxCorrelationLength]
	}
	return value
}

func marshalCanonicalPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func safeDiagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func appendDecodeResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

var _ adapter.Decoder = (*structuredDecoder)(nil)
