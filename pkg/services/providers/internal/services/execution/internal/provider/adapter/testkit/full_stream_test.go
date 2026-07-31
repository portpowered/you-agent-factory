package testkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter/testkit"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	fixtureProviderRef = "session-42"
	fixtureMessageID   = "message-7"
	fixtureToolItemID  = "tool-item-9"
	fixtureToolCallID  = "call-9"
	privatePrompt      = "private prompt must not escape"
	secretToken        = "sk-fixture-secret"
)

var errFixtureRetry = errors.New("fixture provider requested retry")

func TestFullStreamAdapterConformance(t *testing.T) {
	t.Parallel()

	session := workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: fixtureProviderRef}
	testkit.RunFullStream(t, testkit.FullStreamFixture{
		NewAdapter: func() adapter.Adapter { return fullStreamAdapter{} },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-conformance"},
			Model:    "fixture-model", UserMessage: privatePrompt,
		},
		ContentAndTools: observations(
			line(`{"type":"orbit","session":"session-42","additive":"accepted"}`),
			line(`{"type":"glyph","item":"message-7","text":"Hello "}`),
			line(`{"type":"glyph","item":"message-7","text":"world"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world"}`),
			line(`{"type":"lever","item":"tool-item-9","call":"call-9","name":"weather","arguments":{"city":"Oslo"}}`),
			line(`{"type":"spark","item":"tool-item-9","call":"call-9","text":"12 C"}`),
			line(`{"type":"latch","item":"tool-item-9","call":"call-9","name":"weather","result":{"temperature":12}}`),
		),
		RetryableFailure: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":"pause","seconds":2}`),
		),
		UnsafeAndRecovering: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":`+privatePrompt+`,"token":"`+secretToken+`"}`),
			line(`{"type":"future_shape","prompt":"`+privatePrompt+`","token":"`+secretToken+`"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world","future":true}`),
		),
		UnterminatedFinal: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			[]byte(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		FinalResult: workers.CommandResult{Stdout: []byte(`{"content":"Hello world","session":"session-42"}`)},
		Expected: testkit.FullStreamExpected{
			ProviderSession: session,
			ProviderRef:     fixtureProviderRef, MessageItemID: fixtureMessageID,
			ToolItemID: fixtureToolItemID, ToolCallID: fixtureToolCallID,
			MessageDeltas: []string{"Hello ", "world"}, FinalContent: "Hello world",
			ToolOutputDelta: "12 C", RetryAfter: 2,
		},
		ForbiddenDiagnostic: []string{privatePrompt, secretToken},
	})

	testkit.RunDecoderConformance(t, testkit.DecoderConformanceFixture{
		NewDecoder: func(input adapter.DecoderContext) adapter.Decoder {
			return &fullStreamDecoder{context: input}
		},
		Lifecycle: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":"glyph","item":"message-7","text":"Hello world"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world"}`),
			line(`{"type":"lever","item":"tool-item-9","call":"call-9","name":"weather","arguments":{"city":"Oslo"}}`),
			line(`{"type":"latch","item":"tool-item-9","call":"call-9","name":"weather","result":{"temperature":12}}`),
		),
		UnsafeAndRecovering: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":"future_shape","prompt":"`+privatePrompt+`","token":"`+secretToken+`"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		UnterminatedFinal: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			[]byte(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		Expected: testkit.DecoderConformanceExpected{
			ProviderRef: fixtureProviderRef, MessageItemID: fixtureMessageID,
			ToolItemID: fixtureToolItemID, ToolCallID: fixtureToolCallID, FinalContent: "Hello world",
		},
		ForbiddenDiagnostic: []string{privatePrompt, secretToken},
	})

	t.Run("explicit failure decoder conformance", func(t *testing.T) {
		testkit.RunDecoderConformance(t, testkit.DecoderConformanceFixture{
			NewDecoder: func(input adapter.DecoderContext) adapter.Decoder {
				return &fullStreamDecoder{context: input}
			},
			Lifecycle: observations(
				line(`{"type":"orbit","session":"session-42"}`),
				line(`{"type":"lever","item":"tool-item-9","call":"call-9","name":"weather","arguments":{"city":"Oslo"}}`),
				line(`{"type":"latch","item":"tool-item-9","call":"call-9","name":"weather","result":{"temperature":12}}`),
				line(`{"type":"break"}`),
			),
			UnsafeAndRecovering: observations(
				line(`{"type":"future_shape","prompt":"`+privatePrompt+`"}`),
				line(`{"type":"break"}`),
			),
			UnterminatedFinal: observations([]byte(`{"type":"break"}`)),
			Expected: testkit.DecoderConformanceExpected{
				ProviderRef: fixtureProviderRef, ToolItemID: fixtureToolItemID, ToolCallID: fixtureToolCallID,
			},
			ForbiddenDiagnostic: []string{privatePrompt},
		})
	})
}

func TestFullStreamAdapterConformanceSupportsSnapshotOnlyNativeStreams(t *testing.T) {
	t.Parallel()

	session := workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: fixtureProviderRef}
	testkit.RunFullStream(t, testkit.FullStreamFixture{
		NewAdapter: func() adapter.Adapter { return snapshotStreamAdapter{} },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-snapshot-conformance"},
			Model:    "fixture-model", UserMessage: privatePrompt,
		},
		ContentAndTools: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		RetryableFailure: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":"pause","seconds":2}`),
		),
		UnsafeAndRecovering: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			line(`{"type":`+privatePrompt+`,"token":"`+secretToken+`"}`),
			line(`{"type":"future_shape","prompt":"`+privatePrompt+`","token":"`+secretToken+`"}`),
			line(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		UnterminatedFinal: observations(
			line(`{"type":"orbit","session":"session-42"}`),
			[]byte(`{"type":"seal","item":"message-7","text":"Hello world"}`),
		),
		FinalResult: workers.CommandResult{Stdout: []byte(`{"content":"Hello world","session":"session-42"}`)},
		Expected: testkit.FullStreamExpected{
			Capabilities:    adapter.Capabilities{NativeStreaming: true, MessageSnapshots: true, StableItemIDs: true},
			ProviderSession: session,
			ProviderRef:     fixtureProviderRef, MessageItemID: fixtureMessageID,
			FinalContent: "Hello world", RetryAfter: 2,
		},
		ForbiddenDiagnostic: []string{privatePrompt, secretToken},
	})
}

type fullStreamAdapter struct{}

type snapshotStreamAdapter struct{ fullStreamAdapter }

func (snapshotStreamAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageSnapshots: true, StableItemIDs: true,
	}}, nil
}

func (fullStreamAdapter) Identity() adapter.Identity { return "fixture-full-stream" }

func (fullStreamAdapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	return adapter.CommandBuildResult{Request: workers.CommandRequest{
		Command: "fixture-provider", Args: []string{"run", input.Request.Model},
		Stdin: []byte(input.Request.UserMessage), DispatchID: input.Request.Dispatch.DispatchID,
	}}, nil
}

func (fullStreamAdapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	return &fullStreamDecoder{context: input}, nil
}

func (fullStreamAdapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if bytes.Contains(input.CommandResult.Stdout, []byte(`"type":"pause"`)) {
		return adapter.FinalParseResult{}, errFixtureRetry
	}
	var result struct {
		Content string `json:"content"`
		Session string `json:"session"`
	}
	if err := json.Unmarshal(input.CommandResult.Stdout, &result); err != nil {
		return adapter.FinalParseResult{}, errors.New("fixture final response was malformed")
	}
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
		Content:         result.Content,
		ProviderSession: &workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: result.Session},
	}}, nil
}

func (fullStreamAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (fullStreamAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if !errors.Is(input.ParseError, errFixtureRetry) {
		return adapter.FailureResult{}
	}
	retryAfter := 2 * time.Second
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: workerexecution.WorkFailureFamilyThrottle, Type: workerexecution.WorkFailureTypeThrottled,
		Message:         "fixture provider is temporarily busy",
		Retry:           adapter.RetryGuidance{Retryable: true, RetryAfter: &retryAfter},
		ProviderSession: &workerexecution.ProviderSessionMetadata{Provider: "fixture", Kind: "session_id", ID: fixtureProviderRef},
	}}
}

type fullStreamDecoder struct {
	context    adapter.DecoderContext
	stdout     []byte
	runStarted bool
	sessionRef string
}

func (d *fullStreamDecoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: "stderr_ignored", Message: "fixture provider stderr was ignored"}}}, nil
	}
	d.stdout = append(d.stdout, observation.Chunk...)
	var result adapter.DecodeResult
	for {
		newline := bytes.IndexByte(d.stdout, '\n')
		if newline < 0 {
			break
		}
		result = appendDecodeResult(result, d.decodeRecord(d.stdout[:newline]))
		d.stdout = d.stdout[newline+1:]
	}
	return result, nil
}

func (d *fullStreamDecoder) Flush(_ context.Context, _ adapter.FlushContext) (adapter.DecodeResult, error) {
	if len(bytes.TrimSpace(d.stdout)) == 0 {
		d.stdout = nil
		return adapter.DecodeResult{}, nil
	}
	result := d.decodeRecord(d.stdout)
	d.stdout = nil
	return result, nil
}

type nativeRecord struct {
	Type      string          `json:"type"`
	Session   string          `json:"session"`
	Item      string          `json:"item"`
	Call      string          `json:"call"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Seconds   int64           `json:"seconds"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
}

func (d *fullStreamDecoder) decodeRecord(raw []byte) adapter.DecodeResult {
	var record nativeRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return safeDiagnostic("malformed_record", "fixture provider emitted a malformed record")
	}
	switch record.Type {
	case "orbit":
		d.sessionRef = record.Session
		return adapter.DecodeResult{}
	case "glyph":
		return d.withRunStart(messageDelta(record, d.sessionRef))
	case "seal":
		return d.withRunStart(messageSnapshot(record, d.sessionRef))
	case "lever":
		return d.withRunStart(toolSnapshot(record, d.sessionRef, factorysessions.ResponseEventPhaseStarted))
	case "spark":
		return d.withRunStart(toolDelta(record, d.sessionRef))
	case "latch":
		return d.withRunStart(toolSnapshot(record, d.sessionRef, factorysessions.ResponseEventPhaseCompleted))
	case "pause":
		return d.withRunStart(retryNotification(record, d.sessionRef))
	case "break":
		return d.withRunStart(failureNotification(d.sessionRef))
	default:
		return safeDiagnostic("unknown_record", "fixture provider emitted an unsupported additive record")
	}
}

func (d *fullStreamDecoder) withRunStart(draft factorysessions.ResponseEventDraft) adapter.DecodeResult {
	if d.runStarted {
		return adapter.DecodeResult{Drafts: []factorysessions.ResponseEventDraft{draft}}
	}
	d.runStarted = true
	started := factorysessions.ResponseEventDraft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: factorysessions.ResponseEventKindRun, Phase: factorysessions.ResponseEventPhaseStarted,
		Provenance: fixtureProvenance("synthesized", factorysessions.ResponseEventDeliverySynthesized, factorysessions.ResponseEventRepresentationNotification),
		Payload:    marshalPayload(factorysessions.ResponseEventRun{Status: "started"}),
	}
	return adapter.DecodeResult{Drafts: []factorysessions.ResponseEventDraft{started, draft}}
}

func messageDelta(record nativeRecord, providerRef string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindMessage, Phase: factorysessions.ResponseEventPhaseDelta,
		ItemID: record.Item, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance(record.Type, factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationDelta),
		Payload:    marshalPayload(factorysessions.ResponseEventMessageDelta{ContentBlockIndex: 0, ContentBlockKind: factorysessions.ResponseEventContentBlockText, TextDelta: record.Text}),
	}
}

func messageSnapshot(record nativeRecord, providerRef string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindMessage, Phase: factorysessions.ResponseEventPhaseCompleted,
		ItemID: record.Item, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance(record.Type, factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationSnapshot),
		Payload:    marshalPayload(factorysessions.ResponseEventMessage{Role: "assistant", ContentBlocks: []factorysessions.ResponseEventContentBlock{{Kind: factorysessions.ResponseEventContentBlockText, Text: record.Text}}}),
	}
}

func toolSnapshot(record nativeRecord, providerRef string, phase factorysessions.ResponseEventPhase) factorysessions.ResponseEventDraft {
	payload := factorysessions.ResponseEventTool{ToolCallID: record.Call, ToolName: record.Name}
	if phase == factorysessions.ResponseEventPhaseStarted {
		payload.Status, payload.ArgumentsSummary = "running", record.Arguments
	} else {
		payload.Status, payload.ResultSummary = "completed", record.Result
	}
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindTool, Phase: phase, ItemID: record.Item, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance(record.Type, factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationSnapshot),
		Payload:    marshalPayload(payload),
	}
}

func toolDelta(record nativeRecord, providerRef string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindTool, Phase: factorysessions.ResponseEventPhaseDelta,
		ItemID: record.Item, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance(record.Type, factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationDelta),
		Payload:    marshalPayload(factorysessions.ResponseEventToolDelta{ToolCallID: record.Call, OutputDelta: record.Text}),
	}
}

func retryNotification(record nativeRecord, providerRef string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindError, Phase: factorysessions.ResponseEventPhaseUpdated, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance(record.Type, factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationNotification),
		Payload:    marshalPayload(factorysessions.ResponseEventErrorPayload{Code: "provider_busy", Message: "fixture provider is temporarily busy", Retryable: true, RetryAfterSeconds: &record.Seconds}),
	}
}

func failureNotification(providerRef string) factorysessions.ResponseEventDraft {
	return factorysessions.ResponseEventDraft{
		Kind: factorysessions.ResponseEventKindError, Phase: factorysessions.ResponseEventPhaseFailed, ProviderSessionRef: providerRef,
		Provenance: fixtureProvenance("break", factorysessions.ResponseEventDeliveryNativeStream, factorysessions.ResponseEventRepresentationNotification),
		Payload:    marshalPayload(factorysessions.ResponseEventErrorPayload{Code: "provider_failed", Message: "fixture provider failed"}),
	}
}

func fixtureProvenance(nativeType string, delivery factorysessions.ResponseEventDelivery, representation factorysessions.ResponseEventRepresentation) factorysessions.ResponseEventProvenance {
	return factorysessions.ResponseEventProvenance{
		Provider: "fixture", NativeEventType: nativeType, Delivery: delivery,
		Representation: representation, Fidelity: factorysessions.ResponseEventFidelityNormalized,
	}
}

func safeDiagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func appendDecodeResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

func marshalPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal fixture payload: %v", err))
	}
	return payload
}

func observations(chunks ...[]byte) []adapter.Observation {
	result := make([]adapter.Observation, 0, len(chunks))
	for index, chunk := range chunks {
		if index == 1 && len(chunk) > 4 {
			result = append(result,
				adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk[:4]},
				adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk[4:]},
			)
			continue
		}
		result = append(result, adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk})
	}
	return result
}

func line(value string) []byte { return []byte(strings.TrimSpace(value) + "\n") }

var _ adapter.Adapter = fullStreamAdapter{}
var _ adapter.Decoder = (*fullStreamDecoder)(nil)
