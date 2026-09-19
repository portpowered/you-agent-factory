package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	defaultResponseStreamProgressQueueCapacity = 64
	responseStreamProgressDrainTimeout         = 250 * time.Millisecond
)

func TestWriteInvocationError_WritesOneStandardErrorResponseForTerminalFailure(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	handled := WriteInvocationError(&stderr, invocationCLIError{
		Code:      "INVOCATION_RUNTIME_FAILURE",
		Message:   "goal execution failed",
		SessionID: "session-failed",
		WorkID:    "work-failed",
	}, false)
	if !handled {
		t.Fatal("terminal invocation failure was not handled")
	}

	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr.String()), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	if response.Message != "goal execution failed [session=session-failed workId=work-failed]" {
		t.Fatalf("message = %q", response.Message)
	}
}

func TestEmitHistoricalReplayInspectionStableFactoryFactsAndSafeQuoting(t *testing.T) {
	t.Parallel()

	workA := work.FactoryWorkItem{ID: "work-a", WorkTypeID: "task", State: "done", TraceID: "trace-a"}
	workZ := work.FactoryWorkItem{
		ID: "work-z", WorkTypeID: "review", State: "failed", TraceID: "trace-z", ParentID: "work-a",
		CurrentChainingTraceID: "chain-z", PreviousChainingTraceIDs: []string{"trace-z", "trace-a", "trace-z"},
	}
	state := recordings.FactoryWorldState{
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{
			"request-z": {
				RequestID: "request-z", Type: work.WorkRequestTypeFactoryRequestBatch,
				WorkItems: []work.FactoryWorkItem{workZ, workA},
			},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"dispatch-only": {ID: "dispatch-only", WorkTypeID: "internal", State: "running"},
			workZ.ID:        workZ,
			workA.ID:        workA,
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			"dispatch-only": {{Type: "INTERNAL", TargetWorkID: "work-a"}},
			workZ.ID:        {{Type: "DEPENDS_ON", SourceWorkID: workZ.ID, TargetWorkID: workA.ID, RequiredState: "done", RequestID: "request-z", TraceID: "trace-z"}},
		},
		FailureDetailsByWorkID: map[string]recordings.FactoryWorldFailureDetail{
			workZ.ID: {
				DispatchID: "dispatch-z", TransitionID: "transition-z", WorkItem: workZ,
				FailureDetail: &workerexecution.FailureDetail{
					Reason: workerexecution.WorkFailureTypeUnknown, Message: "recorded failure\nwith safe quoting",
				},
			},
		},
		SessionBracket: &recordings.FactoryWorldSessionBracketState{
			LifecycleControlStatus: "RUNNING", Terminal: true, FinalStatus: "SUCCEEDED",
		},
	}
	inspection := factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{SessionID: "legacy-session", Status: factorysessions.LifecycleStatusSucceeded},
		FactoryProjection: factorysessions.HistoricalReplayFactoryProjection{
			Availability: factorysessions.HistoricalReplayFactoryProjectionAvailable,
			State:        &state,
		},
	}

	var first string
	for attempt := 0; attempt < 20; attempt++ {
		var output bytes.Buffer
		if err := emitHistoricalReplayInspection(&output, inspection); err != nil {
			t.Fatalf("emitHistoricalReplayInspection() attempt %d: %v", attempt, err)
		}
		if attempt == 0 {
			first = output.String()
		} else if output.String() != first {
			t.Fatalf("historical replay output changed on attempt %d:\nfirst=%s\ncurrent=%s", attempt, first, output.String())
		}
	}

	for _, want := range []string{
		`Factory projection: AVAILABLE (reason=)`,
		`Session lifecycle: control="RUNNING" terminal=true final="SUCCEEDED"`,
		`Work: id="work-a" type="task" state="done" trace="trace-a" parent=""`,
		`Work: id="work-z" type="review" state="failed" trace="trace-z" parent="work-a"`,
		`Lineage: work="work-z" current="chain-z" previous="trace-a,trace-z"`,
		`Relation: source="work-z" type="DEPENDS_ON" target="work-a" required="done" request="request-z" trace="trace-z"`,
		`Failure: work="work-z" dispatch="dispatch-z" reason="unknown" message="recorded failure\nwith safe quoting"`,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("historical replay output = %q, want %q", first, want)
		}
	}
	if strings.Contains(first, "dispatch-only") {
		t.Fatalf("historical replay output fabricated dispatch-only Work facts: %q", first)
	}
	if strings.Index(first, `Work: id="work-a"`) > strings.Index(first, `Work: id="work-z"`) {
		t.Fatalf("Work rows are not sorted: %q", first)
	}
}

func TestClassifyRunInputFailurePreservesTypedFirstCorruptReplayDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic := recordings.ReplayArtifactDiagnostic{
		Code:    recordings.ReplayArtifactDiagnosticMalformed,
		Area:    "events",
		Path:    "events[3]",
		Message: `event "event-3" is malformed`,
		Action:  recordings.ReplayArtifactStructuralRepairAction,
	}
	artifactErr := &recordings.ReplayArtifactError{
		Kind:       recordings.ReplayArtifactErrorCorruptInput,
		Diagnostic: diagnostic,
		Cause:      recordings.ErrCorruptReplayInput,
	}
	inputErr := &recordings.ReplayInputError{
		Family:     recordings.ReplayInputFamilyLegacy,
		Diagnostic: diagnostic,
		Cause:      artifactErr,
	}

	got := classifyRunInputFailure(RunConfig{ReplayPath: `C:\private\recording.jsonl`}, inputErr)
	want := `Error: MALFORMED_REPLAY_ARTIFACT area=events path=events[3] event="event-3" action="REPLACE_OR_REGENERATE_RECORDING": event "event-3" is malformed`
	if got.Error() != want {
		t.Fatalf("classified replay error = %q, want %q", got.Error(), want)
	}
	if !clidiag.HasCodedDiagnostic(got) {
		t.Fatal("classified replay error has no CLI diagnostic")
	}
	if !errors.Is(got, recordings.ErrCorruptReplayInput) {
		t.Fatal("classified replay error did not preserve corruption cause")
	}
}

func TestClassifyRunInputFailurePreservesForeignReplayDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostic := recordings.ReplayArtifactDiagnostic{
		Code:    recordings.ReplayArtifactDiagnosticForeignReference,
		Area:    "events",
		Path:    "events[4]",
		Message: `event "dispatch-response" has a foreign reference`,
		Action:  recordings.ReplayArtifactStructuralRepairAction,
	}
	err := &recordings.ReplayInputError{
		Family:     recordings.ReplayInputFamilyLegacy,
		Diagnostic: diagnostic,
		Cause: &recordings.ReplayArtifactError{
			Kind:       recordings.ReplayArtifactErrorForeign,
			Diagnostic: diagnostic,
			Cause:      recordings.ErrForeignPortableArtifact,
		},
	}

	got := classifyRunInputFailure(RunConfig{ReplayPath: "recording.jsonl"}, err)
	want := `Error: FOREIGN_REPLAY_ARTIFACT_REFERENCE area=events path=events[4] event="dispatch-response" action="REPLACE_OR_REGENERATE_RECORDING": event "dispatch-response" has a foreign reference`
	if got.Error() != want {
		t.Fatalf("classified foreign replay error = %q, want %q", got.Error(), want)
	}
}

func TestClassifyRunInputFailurePreservesTypedResumeDiagnosticWithoutSourceDetails(t *testing.T) {
	t.Parallel()

	cause := errors.New(`open C:\private\broken.recording.json secret=TOPSECRET`)
	inputErr := &recordings.ReplayInputError{
		Family: recordings.ReplayInputFamilyPortable,
		Diagnostic: recordings.ReplayArtifactDiagnostic{
			Code: recordings.ReplayArtifactDiagnosticMalformed,
			Area: "recording", Path: "recording", Message: "untrusted source contents",
		},
		Cause: cause,
	}

	got := classifyRunInputFailure(RunConfig{ResumePath: `C:\private\broken.recording.json`}, inputErr)
	want := "MALFORMED_REPLAY_ARTIFACT: preserve the recording and replace it from a trusted backup before retrying"
	if got.Error() != want {
		t.Fatalf("classified resume error = %q, want %q", got.Error(), want)
	}
	if !clidiag.HasCodedDiagnostic(got) {
		t.Fatal("classified resume error has no CLI diagnostic")
	}
	if !errors.Is(got, cause) {
		t.Fatal("classified resume error did not preserve its cause")
	}
	if strings.Contains(got.Error(), "TOPSECRET") || strings.Contains(got.Error(), `C:\private`) {
		t.Fatalf("classified resume error leaked source details: %q", got)
	}
}

func TestOperationRunEmitsReplayDriftAfterHistoricalInspection(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := &Operation{
		cfg:    RunConfig{Output: &output, SuppressDashboardRendering: true},
		runner: stubFactoryService{run: func(context.Context) error { return nil }},
		historicalReplay: &factorysessions.HistoricalReplayInspection{
			Session: factorysessions.SessionReadResult{SessionID: "replay-session"},
		},
		replayMetadataWarnings: []recordings.MetadataMismatchWarning{{Key: "workers_hash"}},
	}

	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful historical replay", err)
	}
	inspectionIndex := strings.Index(output.String(), "Replayed Factory Session: replay-session\n")
	warning := "Replay warning: current Factory Definition differs from the recording; affected components: workers. Replay continues with recorded inputs.\n"
	warningIndex := strings.Index(output.String(), warning)
	if inspectionIndex < 0 || warningIndex < 0 || warningIndex < inspectionIndex {
		t.Fatalf("historical replay output ordering or warning is wrong:\n%s", output.String())
	}
}

func TestWriteIncompleteDrainError_WritesExactHumanDiagnostic(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	err := fmt.Errorf("runtime stopped: %w", &factoryruntime.IncompleteDrainError{NonTerminalWorkCount: 3})
	if !WriteIncompleteDrainError(&stderr, err) {
		t.Fatal("incomplete drain error was not handled")
	}
	if got, want := stderr.String(), "Error: factory session drained with 3 non-terminal work items; run is incomplete\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestWriteIncompleteDrainError_IgnoresOtherFailures(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	if WriteIncompleteDrainError(&stderr, errors.New("ordinary failure")) {
		t.Fatal("ordinary failure was incorrectly classified as incomplete drain")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestMapCurrentFactoryFailureWritesDeclaredStandardErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantCode   factoryapi.ErrorResponseCode
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name:       "missing",
			err:        fmt.Errorf("load Current Factory: %w", fs.ErrNotExist),
			wantCode:   CurrentFactoryNotFoundCode,
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "invalid",
			err:        errors.New("parse Current Factory: malformed JSON"),
			wantCode:   CurrentFactoryInvalidCode,
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			mapped := MapCurrentFactoryFailure(test.err)
			if !WriteInvocationError(&stderr, mapped, false) {
				t.Fatal("WriteInvocationError did not recognize Current Factory failure")
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &response); err != nil {
				t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
			}
			if response.Code != test.wantCode || response.Family != test.wantFamily {
				t.Fatalf("ErrorResponse = %#v", response)
			}
		})
	}
}

func TestMapServerFailureWritesDeclaredStandardError(t *testing.T) {
	t.Parallel()

	cause := &platformhttpserver.BindError{
		Host: "127.0.0.1", PreferredPort: 65534, Cause: errors.New("address in use"),
	}
	mapped := MapServerFailure(fmt.Errorf("host runtime: %w", cause))
	var stderr bytes.Buffer
	if !WriteInvocationError(&stderr, mapped, false) {
		t.Fatal("WriteInvocationError did not recognize server bind failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(ServerBindFailedCode) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse = %#v", response)
	}
}

func TestMapServerFailureDiagnosesPreReadinessFailureWithoutRawCause(t *testing.T) {
	t.Parallel()

	secretCause := errors.New("provider-token=SECRET")
	mapped := MapServerFailure(&initializer.RuntimeHostStartupError{Cause: secretCause})
	var invocationErr *InvocationError
	if !errors.As(mapped, &invocationErr) {
		t.Fatalf("mapped error = %T, want InvocationError", mapped)
	}
	if invocationErr.Code != ServerStartFailedCode {
		t.Fatalf("mapped code = %q, want %q", invocationErr.Code, ServerStartFailedCode)
	}
	if !strings.Contains(invocationErr.Message, "requested server did not start") ||
		!strings.Contains(invocationErr.Message, "failure_class=runtime_startup_failed") {
		t.Fatalf("mapped message = %q, want safe startup classification", invocationErr.Message)
	}
	if strings.Contains(invocationErr.Message, "SECRET") {
		t.Fatalf("mapped message = %q, must not expose raw startup cause", invocationErr.Message)
	}
}

func TestMapServerFailurePreservesSafePreReadinessCause(t *testing.T) {
	t.Parallel()

	cause := &InvocationError{Code: InvocationErrorCodeFailed, Message: "required input is missing"}
	mapped := MapServerFailure(&initializer.RuntimeHostStartupError{Cause: cause})
	var invocationErr *InvocationError
	if !errors.As(mapped, &invocationErr) {
		t.Fatalf("mapped error = %T, want InvocationError", mapped)
	}
	if invocationErr.Code != cause.Code ||
		invocationErr.Message != "requested server did not start: RUN_INVOCATION_FAILED: required input is missing" {
		t.Fatalf("mapped error = %#v, want preserved safe cause", invocationErr)
	}
}

func TestMapServerFailurePreservesTypedResumeDiagnosticWithoutSourceDetails(t *testing.T) {
	t.Parallel()

	cause := errors.New(`open C:\private\broken.recording.json secret=TOPSECRET`)
	inputErr := &recordings.ReplayInputError{
		Family: recordings.ReplayInputFamilyPortable,
		Diagnostic: recordings.ReplayArtifactDiagnostic{
			Code: recordings.ReplayArtifactDiagnosticMalformed,
			Area: "recording", Path: "recording", Message: "untrusted source contents",
		},
		Cause: cause,
	}
	mapped := MapServerFailure(&initializer.RuntimeHostStartupError{
		Cause: fmt.Errorf("open resumed runtime: %w", inputErr),
	})
	var invocationErr *InvocationError
	if !errors.As(mapped, &invocationErr) {
		t.Fatalf("mapped error = %T, want InvocationError", mapped)
	}
	wantMessage := "requested server did not start: MALFORMED_REPLAY_ARTIFACT: preserve the recording and replace it from a trusted backup before retrying"
	if invocationErr.Code != string(recordings.ReplayArtifactDiagnosticMalformed) || invocationErr.Message != wantMessage {
		t.Fatalf("mapped error = %#v, want code and safe message %q", invocationErr, wantMessage)
	}
	if strings.Contains(invocationErr.Message, "TOPSECRET") || strings.Contains(invocationErr.Message, `C:\private`) ||
		!errors.Is(mapped, cause) {
		t.Fatalf("mapped error leaked source details or lost its cause: %q", invocationErr.Message)
	}
}

func TestMapInvocationFailure_PreservesCancellationCode(t *testing.T) {
	t.Parallel()

	err := MapInvocationFailure(context.Canceled)
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %T, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("InvocationError = %#v", invocationErr)
	}
}

func TestHumanFactoryEventRenderer_CustomerLifecycleGolden(t *testing.T) {
	t.Parallel()

	phaseName := "synthesize"
	label := "release review"
	providerResponse := "SECRET_PROVIDER_RESPONSE"
	providerOutput := "SECRET_PROVIDER_OUTPUT"
	events := []factorydefinitions.FactoryEvent{
		canonicalFactoryEventWithPayload(1, factorydefinitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Works: []work.WorkRequestEventWork{{Name: "Review release"}},
		}),
		canonicalFactoryEventWithPayload(2, factorydefinitions.FactoryEventTypeSessionStarted, factorydefinitions.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(3, factorydefinitions.FactoryEventTypeDispatchQueued, factorydefinitions.DispatchQueuedEventPayload{Label: &label}),
		canonicalFactoryEventWithPayload(4, factorydefinitions.FactoryEventTypeDispatchRequest, factorydefinitions.DispatchRequestEventPayload{TransitionID: "release review"}),
		canonicalFactoryEventWithPayload(5, factorydefinitions.FactoryEventTypeInferenceRequest, workerexecution.InferenceRequestEventPayload{Attempt: 1}),
		canonicalFactoryEventWithPayload(6, factorydefinitions.FactoryEventTypeInferenceResponse, workerexecution.InferenceResponseEventPayload{
			Attempt: 1, Outcome: workerexecution.InferenceOutcomeSucceeded, Response: &providerResponse,
		}),
		canonicalFactoryEventWithPayload(7, factorydefinitions.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			TransitionID: "release review", Outcome: workerexecution.OutcomeAccepted, Output: &providerOutput,
		}),
		canonicalFactoryEventWithPayload(8, factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, factorydefinitions.OrchestratorPhaseChangedEventPayload{
			PhaseStatus: factorydefinitions.OrchestratorPhaseStatusActive,
		}),
		canonicalFactoryEventWithPayload(9, factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten, factorydefinitions.OrchestratorCheckpointWrittenEventPayload{
			Label: "draft-ready", ResumabilityStatus: factorydefinitions.CheckpointResumabilityStatusResumable,
		}),
		canonicalFactoryEventWithPayload(10, factorydefinitions.FactoryEventTypeSessionResultUpdated, factorydefinitions.FactorySessionResultUpdatedEventPayload{
			ResultStatus: factorydefinitions.FactorySessionResultStatusFinal,
		}),
		canonicalFactoryEventWithPayload(11, factorydefinitions.FactoryEventTypeSessionCompleted, factorydefinitions.FactorySessionCompletedEventPayload{
			FinalStatus: factorydefinitions.FactorySessionLifecycleStatusSucceeded,
		}),
	}
	events[7].Context.PhaseName = &phaseName

	var output strings.Builder
	renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        factorydefinitions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "approved"}},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	want := "[1] work accepted: Review release\n" +
		"[2] factory started\n" +
		"[3] workstation queued: release review\n" +
		"[4] workstation started: release review\n" +
		"[5] inference started (attempt 1)\n" +
		"[6] inference completed (attempt 1)\n" +
		"[7] workstation completed: release review\n" +
		"[8] workflow phase synthesize: ACTIVE\n" +
		"[9] workflow checkpoint written: draft-ready (RESUMABLE)\n" +
		"[10] final output updated: FINAL\n" +
		"[11] factory completed: SUCCEEDED\n\n" +
		responseStreamPrimaryResultHeader + "\napproved"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	for _, forbidden := range []string{providerResponse, providerOutput, "INFERENCE_RESPONSE"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("human lifecycle exposed private provider value %q: %s", forbidden, output.String())
		}
	}
}

func TestJSONFactoryEventRenderer_EmitsCanonicalFactoryEventAndInvocationResponseNDJSON(t *testing.T) {
	const providerCanary = "PRIVATE_PROVIDER_CHUNK_71f2"
	providerResponse := providerCanary
	wantPrimaryResult := []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "first terminal part"},
		{Type: work.WorkContentPartTypeText, Text: "second terminal part"},
	}
	events := append(canonicalJavaScriptFactoryEvents(), canonicalFactoryEventWithPayload(
		4,
		factorydefinitions.FactoryEventTypeInferenceResponse,
		workerexecution.InferenceResponseEventPayload{
			Diagnostics: json.RawMessage(`{"schemaVersion":"agent-factory.response-event.v1","textDelta":"PRIVATE_PROVIDER_CHUNK_71f2"}`),
			Continuation: func() *providers.ContinuationRef {
				ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: providerCanary}.ContinuationRef()
				return &ref
			}(),
			Response: &providerResponse,
		},
	))

	var output strings.Builder
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "request-ndjson", Status: factorydefinitions.InvocationTerminalStatusCompleted,
		PrimaryResult: wantPrimaryResult,
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != len(events)+1 {
		t.Fatalf("NDJSON records = %d, want %d:\n%s", len(lines), len(events)+1, output.String())
	}
	for index, line := range lines[:len(events)] {
		assertFactoryEventNDJSONRecord(t, line, events[index], index)
	}
	assertInvocationResultNDJSONRecord(t, lines[len(lines)-1], wantPrimaryResult)
	for _, forbidden := range []string{
		providerCanary,
		"textDelta",
		"providerSession",
		"FactoryResponseEvent",
		`"recordType":"response_event"`,
		`"invocation":`,
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("NDJSON exposed provider-only value %q:\n%s", forbidden, output.String())
		}
	}
	if !strings.Contains(string(events[len(events)-1].Payload), providerCanary) {
		t.Fatal("presentation redaction mutated canonical Factory Event history")
	}
}

func assertFactoryEventNDJSONRecord(
	t *testing.T, line string, want factorydefinitions.FactoryEvent, index int,
) {
	t.Helper()
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode event record %d: %v", index, err)
	}
	if len(record) != 2 || string(record["recordType"]) != `"factory_event"` || len(record["event"]) == 0 {
		t.Fatalf("event record %d has invalid discriminator shape: %s", index, line)
	}
	var event factorydefinitions.FactoryEvent
	if err := json.Unmarshal(record["event"], &event); err != nil {
		t.Fatalf("decode event %d: %v", index, err)
	}
	if event.Id != want.Id || event.SchemaVersion != want.SchemaVersion || event.Type != want.Type {
		t.Fatalf("event %d canonical identity changed: %#v, want id=%q schema=%q type=%q", index, event, want.Id, want.SchemaVersion, want.Type)
	}
	if event.Context.Sequence != want.Context.Sequence || event.Context.SessionSequence == nil ||
		*event.Context.SessionSequence != *want.Context.SessionSequence {
		t.Fatalf("event %d sequence context changed: %#v", index, event.Context)
	}
}

func assertInvocationResultNDJSONRecord(t *testing.T, line string, wantPrimaryResult []work.WorkContentPart) {
	t.Helper()
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode terminal record: %v", err)
	}
	if len(record) != 2 || string(record["recordType"]) != `"invocation_result"` || len(record["response"]) == 0 {
		t.Fatalf("terminal record has invalid discriminator shape: %s", line)
	}
	var terminal remoteInvocationNDJSONRecord
	if err := json.Unmarshal([]byte(line), &terminal); err != nil {
		t.Fatalf("decode terminal response: %v", err)
	}
	if terminal.Response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal status = %q, want completed", terminal.Response.Status)
	}
	assertGeneratedWorkContentPartsFromResponse(t, terminal.Response.PrimaryResult, wantPrimaryResult)
}

func TestJSONFactoryEventRenderer_WithoutTerminalWritesOnlyFactoryEvents(t *testing.T) {
	var output strings.Builder
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents())
	renderer.StopProgressRendering()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("NDJSON records = %d, want 3 Factory Events:\n%s", len(lines), output.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, `"recordType":"factory_event"`) || strings.Contains(line, "invocation_result") {
			t.Fatalf("unexpected no-terminal record: %s", line)
		}
	}
}

// testResponsePresentation is the service-root test edge used by transport
// encoding tests. Queue and attachment invariants are owned and tested in the
// Factory Visualization package.
func testResponsePresentation() factoryvisualization.ResponsePresentation {
	return fakeResponsePresentation{}
}

type fakeResponsePresentation struct{}

func (fakeResponsePresentation) OpenBestEffortOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity}
}

func (fakeResponsePresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer}
}

func (fakeResponsePresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &fakeFactoryEventStream{
		output: &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity},
		encode: encode,
	}
}

func (fakeResponsePresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &fakeFactoryEventStream{
		output: &fakePresentationOutput{writer: writer},
		encode: encode,
	}
}

type fakeFactoryEventStream struct {
	mu           sync.Mutex
	output       factoryvisualization.Output
	encode       factoryvisualization.FactoryEventEncoder
	progressSeen bool
	finalized    bool
	finalErr     error
}

func (s *fakeFactoryEventStream) PresentFactoryEvents(events []factorydefinitions.FactoryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return
	}
	for _, event := range events {
		payload, ok := s.encode(event)
		if !ok || len(payload) == 0 {
			continue
		}
		if err := s.output.Enqueue(payload); err == nil {
			s.progressSeen = true
		}
	}
}

func (s *fakeFactoryEventStream) Finalize(write factoryvisualization.FinalResponseWriter) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return false, s.finalErr
	}
	s.finalized = true
	if err := s.output.CloseAndDrain(); err != nil {
		s.finalErr = err
		return true, err
	}
	s.finalErr = s.output.WithWriterExclusive(func(writer io.Writer) error {
		return write(writer, s.progressSeen)
	})
	return true, s.finalErr
}

func (s *fakeFactoryEventStream) CloseAndDrain() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finalized {
		s.finalized = true
		s.finalErr = s.output.CloseAndDrain()
	}
	return s.finalErr
}

type fakePresentationOutput struct {
	mu       sync.Mutex
	writer   io.Writer
	capacity int
	pending  [][]byte
	dropped  int
	closed   bool
}

func (o *fakePresentationOutput) Enqueue(payload []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errors.New("fake response presentation output is closed")
	}
	if o.capacity == 0 {
		_, err := o.writer.Write(append(append([]byte(nil), payload...), '\n'))
		return err
	}
	if o.capacity > 0 && len(o.pending) >= o.capacity {
		o.dropped++
		return errors.New("fake response presentation output backlog is full")
	}
	o.pending = append(o.pending, append([]byte(nil), payload...))
	return nil
}

func (o *fakePresentationOutput) CloseAndDrain() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	pending := append([][]byte(nil), o.pending...)
	o.pending = nil
	o.mu.Unlock()
	for _, payload := range pending {
		if _, err := o.writer.Write(append(payload, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (o *fakePresentationOutput) WithWriterExclusive(write func(io.Writer) error) error {
	return write(o.writer)
}

func (o *fakePresentationOutput) Dropped() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped
}
