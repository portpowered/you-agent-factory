package providerparity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestFixtureByID_Unknown(t *testing.T) {
	t.Parallel()

	if _, ok := FixtureByID("missing-fixture"); ok {
		t.Fatal("FixtureByID() = true, want false for unknown id")
	}
}

func TestValidateSanitized_RejectsForbiddenContent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		contents string
	}{
		{name: "forbidden token", contents: "contains sk-parity-secret-token in body"},
		{name: "secret pattern", contents: `api_key: "leaked"`},
		{name: "host path", contents: "/Users/alice/project/file.txt"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSanitized([]byte(tc.contents)); err == nil {
				t.Fatalf("ValidateSanitized() = nil, want error for %q", tc.name)
			}
		})
	}
}

func TestDecodeTransportCLINDJSON_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lines []string
	}{
		{name: "empty", lines: nil},
		{name: "bad json", lines: []string{`{not-json`}},
		{name: "wrong record type", lines: []string{
			`{"recordType":"invocation_result","invocation":{}}`,
			`{"recordType":"invocation_result","invocation":{}}`,
		}},
		{name: "retired progress", lines: []string{
			`{"recordType":"progress","sequence":1,"payload":"planning"}`,
			`{"recordType":"invocation_result","invocation":{"requestId":"req-1","status":"COMPLETED"}}`,
		}},
		{name: "retired compaction", lines: []string{
			`{"recordType":"compaction","reason":"truncated"}`,
			`{"recordType":"invocation_result","invocation":{"requestId":"req-1","status":"COMPLETED"}}`,
		}},
		{name: "retired primary_result final", lines: []string{
			`{"recordType":"primary_result","invocation":{"requestId":"req-1","status":"COMPLETED"}}`,
		}},
		{name: "retired stream_gap", lines: []string{
			`{"recordType":"stream_gap","reason":"progress_backlog"}`,
			`{"recordType":"invocation_result","invocation":{"requestId":"req-1","status":"COMPLETED"}}`,
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := DecodeTransportCLINDJSON(tc.lines); err == nil {
				t.Fatalf("DecodeTransportCLINDJSON() = nil, want error for %q", tc.name)
			}
		})
	}
}

func TestDecodeTransportAPIRecords_RejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	records := []apisurface.FactoryResponseEventRecord{{
		Sequence: 1,
		Kind:     string(factorysessions.ResponseEventKindMessage),
		Data:     []byte(`{`),
	}}
	if _, err := DecodeTransportAPIRecords(records); err == nil {
		t.Fatal("DecodeTransportAPIRecords() = nil, want error for invalid payload")
	}
}

func TestDecodeTransportSSEFrame_RejectsInvalidFrame(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		frame string
	}{
		{name: "missing id", frame: "data: {}\n\n"},
		{name: "bad json", frame: "id: 1\ndata: {bad\n\n"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeTransportSSEFrame(tc.frame); err == nil {
				t.Fatalf("DecodeTransportSSEFrame() = nil, want error for %q", tc.name)
			}
		})
	}
}

func TestAssertObservableToolLifecycle_RejectsIncompleteLifecycle(t *testing.T) {
	t.Parallel()

	if err := AssertObservableToolLifecycle(nil); err == nil {
		t.Fatal("AssertObservableToolLifecycle() = nil, want error for missing lifecycle")
	}
}

func TestAssertObservableToolLifecycle_RejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	started := toolEvent(factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{
		ToolCallID: "tool-1",
		ToolName:   "lookup",
	})
	completedBadJSON := factorysessions.FactoryResponseEvent{
		Kind:    factorysessions.ResponseEventKindTool,
		Phase:   factorysessions.ResponseEventPhaseCompleted,
		Payload: json.RawMessage(`{`),
	}
	if err := AssertObservableToolLifecycle([]factorysessions.FactoryResponseEvent{started, completedBadJSON}); err == nil {
		t.Fatal("AssertObservableToolLifecycle() = nil, want decode error")
	}
}

func TestAssertObservableToolLifecycle_RejectsIdentityDrift(t *testing.T) {
	t.Parallel()

	events := []factorysessions.FactoryResponseEvent{
		toolEvent(factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{
			ToolCallID: "tool-1",
			ToolName:   "lookup",
		}),
		toolEvent(factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{
			ToolCallID: "tool-2",
			ToolName:   "lookup",
		}),
	}
	if err := AssertObservableToolLifecycle(events); err == nil {
		t.Fatal("AssertObservableToolLifecycle() = nil, want identity drift error")
	}
}

func TestAssertObservableToolLifecycle_RejectsMissingIdentity(t *testing.T) {
	t.Parallel()

	started := toolEvent(factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{ToolName: "lookup"})
	if err := AssertObservableToolLifecycle([]factorysessions.FactoryResponseEvent{started}); err == nil {
		t.Fatal("AssertObservableToolLifecycle() = nil, want missing identity error")
	}
}

func TestAssertObservableToolLifecycle_RejectsCompletedMismatch(t *testing.T) {
	t.Parallel()

	events := []factorysessions.FactoryResponseEvent{
		toolEvent(factorysessions.ResponseEventPhaseStarted, factorysessions.ResponseEventTool{
			ToolCallID: "tool-1",
			ToolName:   "lookup",
		}),
		toolEvent(factorysessions.ResponseEventPhaseCompleted, factorysessions.ResponseEventTool{
			ToolCallID: "tool-2",
			ToolName:   "lookup",
		}),
	}
	if err := AssertObservableToolLifecycle(events); err == nil {
		t.Fatal("AssertObservableToolLifecycle() = nil, want completed identity mismatch error")
	}
}

func TestDecodeTransportSSEFrame_RejectsSequenceMismatch(t *testing.T) {
	t.Parallel()

	fixture, ok := FixtureByID(FixtureFullStreamClaude)
	if !ok {
		t.Fatal("missing full-stream fixture")
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity() error = %v", err)
	}
	frame, err := EncodeTransportSSEFrame(outcome.Events[0])
	if err != nil {
		t.Fatalf("EncodeTransportSSEFrame() error = %v", err)
	}
	frame = strings.Replace(frame, "id: ", "id: 999", 1)
	if _, err := DecodeTransportSSEFrame(frame); err == nil {
		t.Fatal("DecodeTransportSSEFrame() = nil, want sequence mismatch error")
	}
}

func TestAssertPrimaryStreamModeParity_RejectsEmptyStreamEvents(t *testing.T) {
	t.Parallel()

	err := AssertPrimaryStreamModeParity(TransportParityOutcome{})
	if err == nil || !strings.Contains(err.Error(), "no response events") {
		t.Fatalf("AssertPrimaryStreamModeParity() = %v, want empty stream events error", err)
	}
}

func TestAssertCLIAPITransportParity_RejectsEmptyEvents(t *testing.T) {
	t.Parallel()

	err := AssertCLIAPITransportParity(TransportParityOutcome{})
	if err == nil || !strings.Contains(err.Error(), "no publishable response events") {
		t.Fatalf("AssertCLIAPITransportParity() = %v, want empty-events error", err)
	}
}

func TestProjectResponseStreamInvocation_RejectsEncodeFailure(t *testing.T) {
	t.Parallel()

	_, _, err := ProjectResponseStreamInvocation(TransportParityOutcome{
		Events: []factorysessions.FactoryResponseEvent{{
			Kind:  factorysessions.ResponseEventKindMessage,
			Phase: factorysessions.ResponseEventPhaseCompleted,
		}},
	})
	if err == nil {
		t.Fatal("ProjectResponseStreamInvocation() = nil, want error for invalid event envelope")
	}
}

func TestAssertCrossProviderParityCatalog(t *testing.T) {
	t.Parallel()

	if err := AssertCrossProviderParityCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeTransportCLINDJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	fixture, ok := FixtureByID(FixtureFullStreamClaude)
	if !ok {
		t.Fatal("missing full-stream fixture")
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity() error = %v", err)
	}
	lines, err := EncodeTransportCLINDJSON(outcome.Events, outcome.InvocationResult)
	if err != nil {
		t.Fatalf("EncodeTransportCLINDJSON() error = %v", err)
	}
	decodedEvents, decodedInvocation, err := DecodeTransportCLINDJSON(lines)
	if err != nil {
		t.Fatalf("DecodeTransportCLINDJSON() error = %v", err)
	}
	if len(decodedEvents) != len(outcome.Events) {
		t.Fatalf("decoded event count = %d, want %d", len(decodedEvents), len(outcome.Events))
	}
	if decodedInvocation.Status == "" {
		t.Fatal("decoded invocation missing status")
	}
}

func TestReadTranscript_MissingFile(t *testing.T) {
	t.Parallel()

	if _, err := ReadTranscript("missing-transcript.ndjson"); err == nil {
		t.Fatal("ReadTranscript() = nil, want error for missing file")
	}
}

func TestRunTerminal_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := RunTerminal(context.Background(), Fixture{
		ID:             "unsupported-provider",
		TranscriptFile: "testdata/full_stream_claude.jsonl",
		Provider:       "unsupported-provider",
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-unsupported"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported parity provider") {
		t.Fatalf("RunTerminal() = %v, want unsupported provider error", err)
	}
}

func TestAssertTruthfulStreamingFidelity_UnsupportedClass(t *testing.T) {
	t.Parallel()

	err := AssertTruthfulStreamingFidelity(
		Fixture{FidelityClass: "unsupported"},
		TransportParityOutcome{},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported fidelity class") {
		t.Fatalf("AssertTruthfulStreamingFidelity() = %v, want unsupported class error", err)
	}
}

func TestAssertCrossProviderParityForFixture_RejectsTerminalContentMismatch(t *testing.T) {
	t.Parallel()

	fixture, ok := FixtureByID(FixtureFullStreamClaude)
	if !ok {
		t.Fatal("missing full-stream fixture")
	}
	fixture.WantContent = "intentionally-wrong-content"
	if err := AssertCrossProviderParityForFixture(context.Background(), fixture); err == nil {
		t.Fatal("AssertCrossProviderParityForFixture() = nil, want terminal content mismatch error")
	}
}

func TestValidateSanitized_RejectsAbsolutePath(t *testing.T) {
	t.Parallel()

	if err := ValidateSanitized([]byte("C:\\Users\\alice\\secret.txt")); err == nil {
		t.Fatal("ValidateSanitized() = nil, want absolute path error")
	}
}

func TestRunTerminal_MissingTranscript(t *testing.T) {
	t.Parallel()

	_, err := RunTerminal(context.Background(), Fixture{
		ID:             "missing-transcript",
		TranscriptFile: "testdata/does-not-exist.jsonl",
	})
	if err == nil {
		t.Fatal("RunTerminal() = nil, want missing transcript error")
	}
}

func TestAssertTruthfulStreamingFidelity_RejectsInvalidOutcomes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		fixture Fixture
		outcome TransportParityOutcome
		want    string
	}{
		{
			name:    "full-stream missing deltas",
			fixture: Fixture{FidelityClass: FidelityFullStream},
			outcome: TransportParityOutcome{
				Terminal: TerminalResult{
					Capabilities: adapter.Capabilities{NativeStreaming: true, MessageDeltas: true},
				},
			},
			want: "no message delta events",
		},
		{
			name:    "partial-stream missing snapshots",
			fixture: Fixture{FidelityClass: FidelityPartialStream},
			outcome: TransportParityOutcome{
				Terminal: TerminalResult{
					Capabilities: adapter.Capabilities{NativeStreaming: true, MessageSnapshots: true},
				},
			},
			want: "no completed message snapshots",
		},
		{
			name:    "snapshot-only claims deltas",
			fixture: Fixture{FidelityClass: FidelitySnapshotOnly},
			outcome: TransportParityOutcome{
				Terminal: TerminalResult{
					Capabilities: adapter.Capabilities{
						NativeStreaming: true, MessageSnapshots: true, MessageDeltas: true,
					},
				},
			},
			want: "must not claim message deltas",
		},
		{
			name:    "final-only missing completed message",
			fixture: Fixture{FidelityClass: FidelityFinalOnly},
			outcome: TransportParityOutcome{
				Terminal: TerminalResult{
					Capabilities: adapter.Capabilities{MessageSnapshots: true, FinalOnly: true},
				},
			},
			want: "no completed message events",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := AssertTruthfulStreamingFidelity(tc.fixture, tc.outcome)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AssertTruthfulStreamingFidelity() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestDefaultForbiddenTokens_NonEmpty(t *testing.T) {
	t.Parallel()

	if len(DefaultForbiddenTokens()) == 0 {
		t.Fatal("DefaultForbiddenTokens() is empty")
	}
}

func toolEvent(phase factorysessions.ResponseEventPhase, payload factorysessions.ResponseEventTool) factorysessions.FactoryResponseEvent {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return factorysessions.FactoryResponseEvent{
		Kind:    factorysessions.ResponseEventKindTool,
		Phase:   phase,
		Payload: encoded,
	}
}
