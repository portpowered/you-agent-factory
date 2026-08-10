package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func findDispatchInterruptedEventPayload(t *testing.T, events []json.RawMessage, dispatchID string) dispatchInterruptedEventPayload {
	t.Helper()
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		var payload dispatchInterruptedEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal DISPATCH_INTERRUPTED payload: %v", err)
		}
		return payload
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found in %#v", dispatchID, events)
	return dispatchInterruptedEventPayload{}
}

func containsEventType(events []json.RawMessage, eventType string) bool {
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == eventType {
			return true
		}
	}
	return false
}

func findDispatchByID(dispatches []DispatchSummary, dispatchID string) *DispatchSummary {
	for index := range dispatches {
		if dispatches[index].ID == dispatchID {
			return &dispatches[index]
		}
	}
	return nil
}

func TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-checkpoint-events-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			Phase:            "execute",
			SourceHash:       "sha256:fixture",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-1",
			CreatedAt:    startedAt.Add(time.Minute),
		},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{
			Kind: factory.JavaScriptRecordKindCheckpoint,
			Checkpoint: &factory.JavaScriptCheckpointRecord{
				ID:      "checkpoint-1",
				Label:   "after-first-child",
				Summary: "checkpoint after first child",
			},
		}},
	}
	checkpoints := checkpointEventsFromRuntimeState(state)
	if len(checkpoints) != 1 || checkpoints[0].CheckpointID != "checkpoint-1" {
		t.Fatalf("checkpoint events = %#v", checkpoints)
	}
	if checkpoints[0].ResumabilityStatus != "RESUMABLE" {
		t.Fatalf("resumability = %q, want RESUMABLE", checkpoints[0].ResumabilityStatus)
	}

	events := BuildCanonicalRuntimeSessionEvents(state.session, state.result, runtimeDispatchEventInputFromState(state))
	events = appendCanonicalOrchestratorCheckpointEvents(events, state.session, checkpoints, canonicalEventSourceRuntimeService)
	found := false
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == "ORCHESTRATOR_CHECKPOINT_WRITTEN" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ORCHESTRATOR_CHECKPOINT_WRITTEN canonical event")
	}
}

func TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-phase-events-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		PhaseSummaries: []PhaseSummary{
			{Phase: "setup"}, {Phase: " "}, {Phase: "execute"},
		},
	}
	events := appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phases = %v, want %v", got, want)
	}

	session.Status = LifecycleStatusSucceeded
	events = appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phases = %v, want %v", got, want)
	}
	if got := appendCanonicalOrchestratorPhaseEvents(events, SessionReadResult{}, canonicalEventSourceRuntimeService); len(got) != len(events) {
		t.Fatalf("empty phase projection changed event count from %d to %d", len(events), len(got))
	}
}

func phaseEventStatuses(t *testing.T, events []json.RawMessage) []string {
	t.Helper()
	statuses := make([]string, 0, len(events))
	for _, raw := range events {
		var event struct {
			Context struct {
				PhaseID *string `json:"phaseId"`
			} `json:"context"`
			Payload struct {
				PhaseStatus string `json:"phaseStatus"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode phase event: %v", err)
		}
		if event.Context.PhaseID != nil && event.Payload.PhaseStatus != "" {
			statuses = append(statuses, *event.Context.PhaseID+":"+event.Payload.PhaseStatus)
		}
	}
	return statuses
}

func TestJavaScriptRuntimeService_FactoryEventObserverDeliversOnlyUnseenEvents(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-observer-events-001"
	session := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
	}
	state := &runtimeSessionState{
		session: session,
		result:  ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	service := &JavaScriptRuntimeService{sessions: map[string]*runtimeSessionState{sessionID: state}}
	var delivered []interfaces.FactoryEvent
	stop := service.observeFactoryEvents(state, func(events []interfaces.FactoryEvent) {
		delivered = append(delivered, events...)
	})
	service.presentCurrentFactoryEvents(sessionID)
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivered %d events after duplicate presentation, want %d", len(delivered), len(state.events))
	}
	stop()
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivery continued after observer stopped: got %d, want %d", len(delivered), len(state.events))
	}
	if stopNil := service.observeFactoryEvents(state, nil); stopNil == nil {
		t.Fatal("nil observer cleanup is nil")
	} else {
		stopNil()
	}
	service.unregisterFactoryEventConsumer("missing-session")
	service.presentCurrentFactoryEvents("missing-session")
}

func TestRuntimeRecordEvents_ReconcileAppendOnlyPhaseCheckpointPhaseHistory(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	const sessionID = "dur-sess-append-only-events-001"
	records := []factory.JavaScriptRuntimeRecord{
		{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "plan"}},
		{Sequence: 2, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-plan", Label: "plan-ready"}},
		{Sequence: 3, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
	}
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID: sessionID, Status: LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1", SourceHash: "sha256:append-only",
			Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID: sessionID, SessionStatus: LifecycleStatusRunning,
			ResultStatus: ResultStatusNotReady,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-plan", CreatedAt: startedAt.Add(time.Second),
		},
		runtimeRecords: append(append([]factory.JavaScriptRuntimeRecord(nil), records...), records...),
		eventConsumer:  func([]interfaces.FactoryEvent) {},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	running := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, running)
	if got, want := phaseEventStatuses(t, running), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phase transitions = %v, want %v", got, want)
	}

	state.events = running
	state.session.Status = LifecycleStatusSucceeded
	state.result.SessionStatus = LifecycleStatusSucceeded
	state.result.ResultStatus = ResultStatusFinal
	terminal := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, terminal)
	if got, want := phaseEventStatuses(t, terminal), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phase transitions = %v, want %v", got, want)
	}
	if len(terminal) <= len(running) {
		t.Fatalf("terminal events = %d, want append beyond %d running events", len(terminal), len(running))
	}
	for index := range running {
		if string(terminal[index]) != string(running[index]) {
			t.Fatalf("published event %d was mutated:\nrunning=%s\nterminal=%s", index, running[index], terminal[index])
		}
	}
}

func assertStrictCanonicalSequences(t *testing.T, events []json.RawMessage) {
	t.Helper()
	previousSequence := 0
	previousSessionSequence := -1
	for index, raw := range events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode event %d: %v", index, err)
		}
		if event.Context.Sequence <= previousSequence || event.Context.SessionSequence == nil ||
			*event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("event %d sequence context is not increasing: %#v", index, event.Context)
		}
		previousSequence = event.Context.Sequence
		previousSessionSequence = *event.Context.SessionSequence
	}
}

func TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-1234567890abcdef1234567890abcdef"
	observedAt := time.Date(2026, 7, 12, 16, 30, 0, 0, time.UTC)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions[sessionID] = &runtimeSessionState{
		session:        SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded, OrchestratorKind: interfaces.OrchestratorKindJavaScript, ResolvedSource: ResolvedSource{SourceRef: "workflow/audit.js"}, SourceHash: "sha256:" + strings.Repeat("1", 64), Policy: PolicyProjection{EffectiveHash: "sha256:" + strings.Repeat("2", 64)}},
		startRequest:   &StartRequest{Args: map[string]any{"customer": "north"}},
		artifacts:      []ArtifactSummary{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("3", 64), SizeBytes: 2, CreatedAt: &observedAt}},
		events:         []json.RawMessage{json.RawMessage(`{"id":"event-1","type":"SESSION_COMPLETED","context":{"sequence":0,"eventTime":"2026-07-12T16:30:00Z"},"payload":{"artifactIds":["artifact-1"]}}`)},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-secret", State: map[string]any{"secret": "raw-state"}}}},
	}
	path := filepath.Join(t.TempDir(), "session.recording.json")
	if err := service.WriteRecording(context.Background(), sessionID, path); err != nil {
		t.Fatalf("WriteRecording: %v", err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(encoded), "checkpoint-secret") || strings.Contains(string(encoded), "raw-state") {
		t.Fatalf("recording leaked runtime state: %s", encoded)
	}
	badPath := filepath.Join(t.TempDir(), "missing", "\x00invalid")
	err = service.WriteRecording(context.Background(), sessionID, badPath)
	var recordingErr *RecordingError
	if !errors.As(err, &recordingErr) || recordingErr.SessionID != sessionID || recordingErr.Path != badPath {
		t.Fatalf("WriteRecording failure = %#v", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("live session changed after recording failure: read=%#v err=%v", read, readErr)
	}
}
