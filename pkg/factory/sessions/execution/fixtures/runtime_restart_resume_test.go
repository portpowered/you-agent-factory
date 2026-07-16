// backendsizecheck:ignore-file restart-resume coverage keeps persistence, replay, provider, and event-lineage assertions in one integration fixture.
// pkgmaintcheck:ignore-file-lines restart-resume coverage keeps persistence, replay, provider, and event-lineage assertions in one integration fixture.
package fixtures_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	factorycontext "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	factoryruntime "github.com/portpowered/infinite-you/pkg/factory/runtime"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestPetriRuntime_MutationsPersistAndReloadThroughFactorySessionOwner(t *testing.T) {
	const sessionID = "~default"
	store, err := runtimepersist.NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	owner := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(), Persistence: store,
	})

	runtime, err := factoryruntime.New(
		factory.WithNet(petriRecordingNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &acceptedPetriExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithWorkflowContext(&factorycontext.FactoryContext{SessionID: sessionID}),
		factory.WithPetriMutationRecorder(owner.RecordPetriTokenMutations),
	)
	if err != nil {
		t.Fatalf("New Petri runtime: %v", err)
	}
	request := requests.WorkRequestFromSubmitRequests([]work.SubmitRequest{{
		WorkTypeID: "task", WorkID: "work-petri-recording", TraceID: "trace-petri-recording",
	}})
	if _, err := runtime.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run Petri runtime: %v", err)
	}
	primaryResult := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText, Text: "persisted Petri completion",
	}}
	if err := owner.RecordPetriSessionCompletion(sessionID, fse.PetriSessionCompletion{
		Status: fse.LifecycleStatusSucceeded, PrimaryResult: primaryResult,
	}); err != nil {
		t.Fatalf("RecordPetriSessionCompletion: %v", err)
	}

	reloaded := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(), Persistence: store,
	})
	session, err := reloaded.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after owner restart: %v", err)
	}
	if session.Status != fse.LifecycleStatusSucceeded || session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("reloaded session = %#v, want SUCCEEDED with FINAL result", session)
	}
	result, err := reloaded.GetResult(context.Background(), sessionID, fse.ResultRequest{Mode: fse.ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult after owner restart: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal || !strings.Contains(string(result.PrimaryResult), "persisted Petri completion") {
		t.Fatalf("reloaded result = %#v, want persisted FINAL primary result", result)
	}
	assertPersistedPetriMutationAndCanonicalProjection(t, store, sessionID)
}

func TestPersistedRuntimeSessionState_MixedTypedHistoryRoundTripsAndReplays(t *testing.T) {
	const sessionID = "dur-sess-1234567890abcdef1234567890abcdef"
	canonical := json.RawMessage(`{
		"schemaVersion":"agent-factory.event.v1",
		"id":"session-started/dur-sess-1234567890abcdef1234567890abcdef",
		"type":"SESSION_STARTED",
		"context":{"sequence":1,"tick":0,"eventTime":"2026-07-12T07:30:00Z","sessionId":"dur-sess-1234567890abcdef1234567890abcdef","orchestratorKind":"petri"},
		"payload":{"startedAt":"2026-07-12T07:30:00Z"}
	}`)
	checkpoint := workflowruntime.RuntimeRecord{
		Sequence: 2,
		Kind:     workflowruntime.RecordKindCheckpoint,
		Checkpoint: &workflowruntime.CheckpointRecord{
			ID: "checkpoint-1", Label: "approval",
			State: map[string]any{"position": "review"},
		},
	}
	mutation := interfaces.TokenMutationRecord{
		DispatchID: "dispatch-1", TransitionID: "review", Type: interfaces.MutationMove,
		TokenID: "token-1", FromPlace: "review:pending", ToPlace: "review:approved",
		Reason: "transition fired",
	}
	snapshot := fse.PersistedRuntimeSessionState{Records: []fse.DurableSessionRecord{
		{Kind: fse.DurableRecordKindCanonicalFactoryEvent, CanonicalEvent: canonical},
		{Kind: fse.DurableRecordKindJavaScriptRuntime, JavaScriptRecord: &checkpoint},
		{Kind: fse.DurableRecordKindPetriTokenMutation, PetriMutation: &mutation},
	}}

	replayed := persistAndReloadRuntimeSnapshot(t, sessionID, snapshot)
	if len(replayed.Records) != 3 {
		t.Fatalf("record count = %d, want 3", len(replayed.Records))
	}
	session, _, err := fse.ReplaySessionProjection([]json.RawMessage{replayed.Records[0].CanonicalEvent})
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.SessionID != sessionID || session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("replayed session = %#v, want running %s", session, sessionID)
	}
	if got := replayed.Records[1].JavaScriptRecord; got == nil || got.Checkpoint == nil || got.Checkpoint.State["position"] != "review" {
		t.Fatalf("replayed checkpoint = %#v", got)
	}
	if got := replayed.Records[2].PetriMutation; got == nil || got.Type != interfaces.MutationMove || got.TransitionID != "review" || got.ToPlace != "review:approved" {
		t.Fatalf("replayed Petri mutation = %#v", got)
	}
}

func persistAndReloadRuntimeSnapshot(t *testing.T, sessionID string, snapshot fse.PersistedRuntimeSessionState) fse.PersistedRuntimeSessionState {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal mixed snapshot: %v", err)
	}
	store, err := runtimepersist.NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("Save: %v", err)
	}
	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var replayed fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(persisted, &replayed); err != nil {
		t.Fatalf("decode mixed persisted history: %v", err)
	}
	return replayed
}

func TestPersistedRuntimeSessionState_MixedTypedHistoryRejectsUnknownAndMalformedRecords(t *testing.T) {
	tests := map[string]string{
		"unknown kind":        `{"Records":[{"kind":"future_orchestrator","canonicalEvent":{"type":"SESSION_STARTED"}}]}`,
		"missing payload":     `{"Records":[{"kind":"petri_token_mutation"}]}`,
		"cross-kind payload":  `{"Records":[{"kind":"petri_token_mutation","javascriptRecord":{"sequence":1,"kind":"phase","phase":{"name":"run"}},"petriMutation":{"type":"move"}}]}`,
		"malformed canonical": `{"Records":[{"kind":"canonical_factory_event","canonicalEvent":{"payload":{}}}]}`,
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			var snapshot fse.PersistedRuntimeSessionState
			if err := json.Unmarshal([]byte(encoded), &snapshot); err == nil {
				t.Fatal("json.Unmarshal succeeded, want explicit compatibility error")
			}
		})
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-interrupted-001")
	assertInterruptedResumePreconditions(t, harness)

	firstDispatchBeforeResume := getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-interrupted-resume-001")
	waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)

	assertProviderCallCount(t, harness.provider, 3)
	assertResumedDispatchParity(t, resumedService, harness.sessionID, firstDispatchBeforeResume)
	assertFinalResult(t, resumedService, harness.sessionID)
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_RehydratesCheckpointStateForControlFlow(t *testing.T) {
	harness := startInterruptedCheckpointStateBranchSession(t, "req-runtime-resume-checkpoint-state-001")
	assertInterruptedResumePreconditions(t, harness)

	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-checkpoint-state-resume-001")
	waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)

	assertProviderCallCount(t, harness.provider, 3)
	assertResumedCheckpointStateBranchResult(t, resumedService, harness.sessionID)
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_PreservesLiveChildOutput(t *testing.T) {
	harness := startInterruptedResumableSessionForWorkflow(
		t,
		"req-runtime-resume-live-child-output-001",
		"resumable-live-child-output.workflow.js",
		"resumable-live-child-output",
	)
	assertInterruptedResumePreconditions(t, harness)

	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-live-child-output-resume-001")
	waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)

	assertProviderCallCount(t, harness.provider, 3)
	assertResumedLiveChildOutputResult(t, resumedService, harness.sessionID, harness.provider.workflowName)
}

func assertResumedCheckpointStateBranchResult(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	if projected["path"] != "from-checkpoint" {
		t.Fatalf("projected path = %#v, want from-checkpoint", projected["path"])
	}
	if projected["step"] != float64(1) {
		t.Fatalf("projected step = %#v, want 1", projected["step"])
	}
	if projected["firstLabel"] != "step-one" {
		t.Fatalf("projected firstLabel = %#v, want step-one", projected["firstLabel"])
	}
	second, ok := projected["second"].(map[string]any)
	if !ok || second["label"] != "step-two" {
		t.Fatalf("projected second = %#v, want step-two label", projected["second"])
	}
}

func assertResumedLiveChildOutputResult(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID, workflowName string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	firstOutputText, ok := projected["firstOutputText"].(string)
	if !ok || firstOutputText == "" {
		t.Fatalf("projected firstOutputText = %#v, want non-empty string", projected["firstOutputText"])
	}
	wantLiveOutput := fmt.Sprintf("live:%s:step-one:step-one:workflows", workflowName)
	if !strings.Contains(firstOutputText, wantLiveOutput) {
		t.Fatalf("projected firstOutputText = %q, want restored live provider output containing %q", firstOutputText, wantLiveOutput)
	}
	if strings.HasPrefix(firstOutputText, "fake:") {
		t.Fatalf("projected firstOutputText = %q, want restored live provider output not synthetic fake replay", firstOutputText)
	}
	if projected["secondLabel"] != "step-two" {
		t.Fatalf("projected secondLabel = %#v, want step-two", projected["secondLabel"])
	}
}

func assertInterruptedResumePreconditions(t *testing.T, harness interruptedResumableHarness) {
	t.Helper()
	if harness.provider.CallCount() < 2 {
		t.Fatalf("provider infer calls = %d, want at least 2 before interrupt", harness.provider.CallCount())
	}
	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(harness.projectRoot), harness.sessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("interrupted snapshot must be durable before cross-instance resume: %v", err)
	}
	getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
}

func getCompletedDispatch(t *testing.T, service fse.Service, sessionID, dispatchID string) fse.DispatchDetail {
	t.Helper()
	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch %s: %v", dispatchID, err)
	}
	if dispatch.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch %s = %#v, want COMPLETED", dispatchID, dispatch)
	}
	return dispatch
}

func assertProviderCallCount(t *testing.T, provider *sequentialBlockingProvider, want int) {
	t.Helper()
	if provider.CallCount() != want {
		t.Fatalf("provider infer calls = %d, want %d", provider.CallCount(), want)
	}
}

func assertResumedDispatchParity(
	t *testing.T,
	service *fse.JavaScriptRuntimeService,
	sessionID string,
	firstDispatchBeforeResume fse.DispatchDetail,
) {
	t.Helper()
	firstDispatchAfterResume := getCompletedDispatch(t, service, sessionID, "dispatch-1")
	if firstDispatchAfterResume.ID != firstDispatchBeforeResume.ID {
		t.Fatalf("dispatch-1 id changed across resume: %q -> %q", firstDispatchBeforeResume.ID, firstDispatchAfterResume.ID)
	}
	getCompletedDispatch(t, service, sessionID, "dispatch-2")
}

func assertFinalResult(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
}
func TestJavaScriptRuntimeService_ResumeInterruptedSession_ExposesReadSurfacesAndEventLineage(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-reads-001")
	assertInterruptedLifecycleHasTimestamp(t, harness.interrupted.Lifecycle)

	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-reads-resume-001")
	success := waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	assertResumedSessionReadSurfaces(t, success)
	assertResumedResultAndDispatches(t, resumedService, harness.sessionID)

	events, err := resumedService.ReadEvents(context.Background(), harness.sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertResumeEventLineage(t, events.Events, harness.sessionID)
	assertResumedReplayProjection(t, events.Events)
	assertResumedReconnectEvents(t, resumedService, harness.sessionID, events.Events)
}

func reconnectAfterFirstEvent(t *testing.T, events []json.RawMessage) fse.EventReconnectRequest {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("expected at least one event for reconnect cursor")
	}
	var envelope struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(events[0], &envelope); err != nil {
		t.Fatalf("parse first event: %v", err)
	}
	return fse.EventReconnectRequest{AfterEventID: envelope.ID}
}

func assertResumeEventLineage(t *testing.T, events []json.RawMessage, sessionID string) {
	t.Helper()
	flags := resumeEventLineageFlags{}
	for _, raw := range events {
		applyResumeEventLineage(t, raw, sessionID, &flags)
	}
	if !flags.checkpoint {
		t.Fatal("ORCHESTRATOR_CHECKPOINT_WRITTEN event missing from resumed session history")
	}
	if !flags.resumed {
		t.Fatal("SESSION_RESUMED event missing from resumed session history")
	}
	if !flags.completed {
		t.Fatal("SESSION_COMPLETED event missing from resumed session history")
	}
}

type resumeEventLineageFlags struct {
	checkpoint bool
	resumed    bool
	completed  bool
}

func applyResumeEventLineage(t *testing.T, raw json.RawMessage, sessionID string, flags *resumeEventLineageFlags) {
	t.Helper()
	var envelope struct {
		Type    string `json:"type"`
		Context struct {
			SessionID    *string `json:"sessionId"`
			CheckpointID *string `json:"checkpointId"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if envelope.Context.SessionID == nil || *envelope.Context.SessionID != sessionID {
		return
	}
	switch envelope.Type {
	case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
		flags.checkpoint = true
		if envelope.Context.CheckpointID == nil || strings.TrimSpace(*envelope.Context.CheckpointID) == "" {
			t.Fatalf("checkpoint event missing checkpointId: %s", string(raw))
		}
	case "SESSION_RESUMED":
		flags.resumed = true
		assertSessionResumedEventPayload(t, envelope.Payload)
	case "SESSION_COMPLETED":
		flags.completed = true
		assertSessionCompletedEventPayload(t, envelope.Payload)
	}
}

func assertSessionResumedEventPayload(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var decoded struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal SESSION_RESUMED payload: %v", err)
	}
	if decoded.Status != string(fse.LifecycleStatusResuming) {
		t.Fatalf("SESSION_RESUMED status = %q, want RESUMING", decoded.Status)
	}
}

func assertSessionCompletedEventPayload(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var decoded struct {
		FinalStatus string `json:"finalStatus"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal SESSION_COMPLETED payload: %v", err)
	}
	if decoded.FinalStatus != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("SESSION_COMPLETED finalStatus = %q, want SUCCEEDED", decoded.FinalStatus)
	}
}
func TestJavaScriptRuntimeService_ResumeInterruptedSession_MissingCheckpointReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	rewritePersistedSnapshot(t, projectRoot, sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		snapshot.CheckpointSummary = nil
	})

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})

	before, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession before resume: %v", err)
	}
	beforeEvents, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before resume: %v", err)
	}

	_, err = service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-missing-checkpoint-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeMissingCheckpoint, "checkpointSummary")

	after, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after resume: %v", err)
	}
	if after.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("status after failed resume = %q, want INTERRUPTED", after.Status)
	}
	if after.SessionID != before.SessionID || after.Phase != before.Phase {
		t.Fatalf("session read changed after failed resume: before=%#v after=%#v", before, after)
	}

	afterEvents, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after resume: %v", err)
	}
	if len(afterEvents.Events) != len(beforeEvents.Events) {
		t.Fatalf("event count changed after failed resume: before=%d after=%d", len(beforeEvents.Events), len(afterEvents.Events))
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_CorruptedPersistenceReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupted snapshot: %v", err)
	}

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-corrupted-persistence-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeCorruptedPersistence, "")
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_InvalidCheckpointSummaryReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	rewritePersistedSnapshot(t, projectRoot, sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		if snapshot.CheckpointSummary != nil {
			snapshot.CheckpointSummary.Kind = "invalid-checkpoint-kind"
		}
	})

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-invalid-checkpoint-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "checkpointSummary.kind")
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_NonApprovedCheckpointReturnsTypedFailure(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-non-approved-001")
	beforeCalls := harness.provider.CallCount()
	rewritePersistedSnapshot(t, harness.projectRoot, harness.sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		snapshot.CheckpointSummary.ResumeStrategy = ""
	})

	service := newResumedRuntimeService(harness)
	_, err := service.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-non-approved-resume-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "checkpointSummary.resumeStrategy")
	if calls := harness.provider.CallCount(); calls != beforeCalls {
		t.Fatalf("provider calls = %d, want unchanged %d after rejected resume", calls, beforeCalls)
	}
	if strings.Contains(err.Error(), "checkpointState") || strings.Contains(err.Error(), "firstLabel") {
		t.Fatalf("resume diagnostic leaked checkpoint payload detail: %v", err)
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsCheckpointDispatchNotDurablyCompleted(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-dispatch-drift-001")
	beforeCalls := harness.provider.CallCount()
	rewritePersistedSnapshot(t, harness.projectRoot, harness.sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		snapshot.CheckpointSummary.CompletedDispatchIDs = append(snapshot.CheckpointSummary.CompletedDispatchIDs, "dispatch-missing")
	})

	service := newResumedRuntimeService(harness)
	_, err := service.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-dispatch-drift-resume-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "checkpointSummary.completedDispatchIds")
	if calls := harness.provider.CallCount(); calls != beforeCalls {
		t.Fatalf("provider calls = %d, want unchanged %d after rejected recovery", calls, beforeCalls)
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_RejectsRegressedEventCursor(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-cursor-drift-001")
	beforeCalls := harness.provider.CallCount()
	rewritePersistedSnapshot(t, harness.projectRoot, harness.sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		var event map[string]any
		if err := json.Unmarshal(snapshot.Events[1], &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		event["context"].(map[string]any)["sequence"] = float64(1)
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		snapshot.Events[1] = encoded
		canonicalIndex := 0
		for index := range snapshot.Records {
			if snapshot.Records[index].Kind != fse.DurableRecordKindCanonicalFactoryEvent {
				continue
			}
			snapshot.Records[index].CanonicalEvent = snapshot.Events[canonicalIndex]
			canonicalIndex++
		}
	})

	service := newResumedRuntimeService(harness)
	_, err := service.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-cursor-drift-resume-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "events.sequence")
	if calls := harness.provider.CallCount(); calls != beforeCalls {
		t.Fatalf("provider calls = %d, want unchanged %d after rejected recovery", calls, beforeCalls)
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_NonInterruptedSessionReturnsTypedFailure(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(
		t,
		"simple-final.workflow.js",
		"simple-final",
	)
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})
	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-non-interrupted-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	_, err = service.ResumeInterruptedSession(context.Background(), started.SessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-non-interrupted-resume-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "")
}

func seedInterruptedCheckpointedSession(t *testing.T) (string, string) {
	t.Helper()
	provider := newSequentialBlockingProvider()
	projectRoot := setupRuntimeWorkflowFixture(
		t,
		"resumable-two-step-fake-children.workflow.js",
		"resumable-two-step-fake-children",
	)
	initial := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		Persistence:       runtimePersistence(projectRoot),
	})
	started, err := initial.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-failure-seed-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-1", fse.DispatchStatusCompleted, 10*time.Second)
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-2", fse.DispatchStatusRunning, 10*time.Second)
	provider.waitUntilBlockedOnInfer(t, 10*time.Second)
	if _, err := initial.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "resume failure seed"},
		DispatchID:     "dispatch-2",
	}); err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	provider.waitForCanceledInfer(t, 10*time.Second)
	interrupted := waitUntilSessionStatus(t, initial, started.SessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	if interrupted.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("seed status = %q, want INTERRUPTED", interrupted.Status)
	}
	waitForPersistedInterruptedSnapshot(t, projectRoot, started.SessionID)

	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), started.SessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("persisted snapshot missing at %s: %v", snapshotPath, err)
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read persisted snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal persisted snapshot: %v", err)
	}
	if snapshot.CheckpointSummary == nil {
		t.Fatal("seed snapshot missing checkpoint summary")
	}
	return started.SessionID, projectRoot
}

func rewritePersistedSnapshot(
	t *testing.T,
	projectRoot, sessionID string,
	mutate func(*fse.PersistedRuntimeSessionState),
) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	mutate(&snapshot)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

func assertResumeError(t *testing.T, err error, wantOutcome fse.ResumeOutcome, wantField string) {
	t.Helper()
	var resumeErr *fse.ResumeError
	if !errors.As(err, &resumeErr) {
		t.Fatalf("error = %T %v, want *fse.ResumeError", err, err)
	}
	if resumeErr.Outcome != wantOutcome {
		t.Fatalf("outcome = %q, want %q", resumeErr.Outcome, wantOutcome)
	}
	if wantField != "" && resumeErr.Field != wantField {
		t.Fatalf("field = %q, want %q", resumeErr.Field, wantField)
	}
}

func waitForPersistedInterruptedSnapshot(t *testing.T, projectRoot, sessionID string) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var snapshot fse.PersistedRuntimeSessionState
			if json.Unmarshal(raw, &snapshot) == nil && snapshot.CheckpointSummary != nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("persisted interrupted snapshot not ready at %s", path)
}
func TestJavaScriptRuntimeService_NonResumedFakeChild_PreservesShippedTransportSemantics(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-fake-child-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	wantDispatch := assertAgentRunFakeChildSessionRead(t, read)
	assertNonResumedLifecycleLineage(t, read)
	assertAgentRunFakeChildDispatch(t, service, completed.SessionID, wantDispatch)
	assertAgentRunFakeChildArtifact(t, service, completed.SessionID)

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertRuntimeEventSource(t, events.Events)
	assertNonResumeRestartEventLineage(t, events.Events, completed.SessionID)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)

	reconnect, err := service.ReadEvents(context.Background(), completed.SessionID, reconnectAfterFirstEvent(t, events.Events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}

	_, err = service.Pause(context.Background(), completed.SessionID, fse.ControlRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("pause on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}

	assertRuntimeInspectionExcludesForbiddenVocabulary(t, liveSession, liveResult, events.Events)
}

func TestJavaScriptRuntimeService_NonResumedSimpleFinal_PreservesReplayReconnectAndTerminalResult(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-simple-final-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertNonResumedLifecycleLineage(t, liveSession)
	assertNonResumeRestartEventLineage(t, events.Events, completed.SessionID)

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want empty for simple-final", dispatches.Dispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want empty for simple-final", artifacts.Artifacts)
	}

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	if replayedResult.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("replayed resultStatus = %q, want FINAL", replayedResult.ResultStatus)
	}

	reconnect, err := service.ReadEvents(context.Background(), completed.SessionID, reconnectAfterFirstEvent(t, events.Events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}

	assertRuntimeInspectionExcludesForbiddenVocabulary(t, liveSession, liveResult, events.Events)
}

func TestJavaScriptRuntimeService_NonResumedTerminalSnapshot_OmitsCheckpointSummaryAndReloadsAcrossFreshServices(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	initial := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})

	completed, err := initial.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-persisted-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	waitForPersistedTerminalSnapshot(t, projectRoot, completed.SessionID)
	snapshot := readPersistedRuntimeSnapshot(t, projectRoot, completed.SessionID)
	if snapshot.CheckpointSummary != nil {
		t.Fatalf("checkpointSummary = %#v, want nil for non-interrupted terminal session", snapshot.CheckpointSummary)
	}
	if snapshot.Session.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("persisted status = %q, want SUCCEEDED", snapshot.Session.Status)
	}

	reloaded := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: runtimePersistence(projectRoot),
	})
	read, err := reloaded.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession after reload: %v", err)
	}
	if read.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("reloaded status = %q, want SUCCEEDED", read.Status)
	}
	assertNonResumedLifecycleLineage(t, read)

	result, err := reloaded.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult after reload: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("reloaded resultStatus = %q, want FINAL", result.ResultStatus)
	}

	dispatches, err := reloaded.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after reload: %v", err)
	}
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("reloaded dispatches = %#v, want one fake child", dispatches.Dispatches)
	}
}

func TestNewExecutionService_FakeProvider_PublishedScenarios_RemainAdditiveAfterRestartResumeLane(t *testing.T) {
	service := newFakeExecutionServiceFromContractFixtures(t)

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	terminalHash, err := fixtures.SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want published fixture digest", terminalHash)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	session, err := service.GetSession(context.Background(), runningRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}
	if session.Lifecycle != nil && (session.Lifecycle.InterruptedAt != nil || session.Lifecycle.ResumedAt != nil) {
		t.Fatalf("running lifecycle = %#v, want no restart-resume lineage", session.Lifecycle)
	}
}

func assertNonResumedLifecycleLineage(t *testing.T, read fse.SessionReadResult) {
	t.Helper()
	if read.Lifecycle == nil {
		return
	}
	if read.Lifecycle.InterruptedAt != nil {
		t.Fatalf("interruptedAt = %v, want nil for non-resumed session", read.Lifecycle.InterruptedAt)
	}
	if read.Lifecycle.ResumedAt != nil {
		t.Fatalf("resumedAt = %v, want nil for non-resumed session", read.Lifecycle.ResumedAt)
	}
}

func assertNonResumeRestartEventLineage(t *testing.T, events []json.RawMessage, sessionID string) {
	t.Helper()
	for index, raw := range events {
		var envelope struct {
			Type    string `json:"type"`
			Context struct {
				SessionID *string `json:"sessionId"`
			} `json:"context"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event[%d]: %v", index, err)
		}
		if envelope.Context.SessionID == nil || *envelope.Context.SessionID != sessionID {
			continue
		}
		switch envelope.Type {
		case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
			t.Fatalf("event[%d] = ORCHESTRATOR_CHECKPOINT_WRITTEN, want absent for non-resumed session", index)
		case "SESSION_RESUMED":
			var payload struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatalf("unmarshal SESSION_RESUMED payload: %v", err)
			}
			if payload.Status == string(fse.LifecycleStatusResuming) {
				t.Fatalf("event[%d] = SESSION_RESUMED with RESUMING, want absent for non-resumed session", index)
			}
		}
	}
}

func assertRuntimeInspectionExcludesForbiddenVocabulary(
	t *testing.T,
	session fse.SessionReadResult,
	result fse.ResultReadResult,
	events []json.RawMessage,
) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Session fse.SessionReadResult `json:"session"`
		Result  fse.ResultReadResult  `json:"result"`
		Events  []json.RawMessage     `json:"events"`
	}{
		Session: session,
		Result:  result,
		Events:  events,
	})
	if err != nil {
		t.Fatalf("marshal inspection snapshot: %v", err)
	}
	responseText := string(encoded)
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(responseText, term) {
			t.Fatalf("inspection response contained forbidden vocabulary %q:\n%s", term, responseText)
		}
	}
	if strings.Contains(responseText, "DynamicWorkflowRunResume") {
		t.Fatalf("inspection response leaked restart-resume-only resource:\n%s", responseText)
	}
}

func waitForPersistedTerminalSnapshot(t *testing.T, projectRoot, sessionID string) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var snapshot fse.PersistedRuntimeSessionState
			if json.Unmarshal(raw, &snapshot) == nil &&
				fse.IsTerminalLifecycleStatus(snapshot.Session.Status) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("persisted terminal snapshot not ready at %s", path)
}

func readPersistedRuntimeSnapshot(t *testing.T, projectRoot, sessionID string) fse.PersistedRuntimeSessionState {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal persisted snapshot: %v", err)
	}
	return snapshot
}
