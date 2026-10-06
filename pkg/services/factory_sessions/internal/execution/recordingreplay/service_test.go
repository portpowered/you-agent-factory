package recordingreplay

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestServiceLegacyFactoryViewsRemainDetachedAcrossEqualIDOpenings(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 10, 6, 12, 0, 0, 123, time.FixedZone("recorded", 3600))
	snapshot := factorydefinitions.FactorySnapshot(`{"unknown":{"retained":true}}`)
	state := recording.FactoryWorldState{
		EventTime: stamp, Factory: &snapshot,
		WorkItemsByID: map[string]work.FactoryWorkItem{"work": {
			ID: "work", Tags: map[string]string{"selected": "original"},
			StructuredResult: map[string]any{"number": json.Number("9007199254740993"), "items": []any{"original"}},
			Content:          []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "original", Metadata: map[string]any{"selected": "original"}}},
		}},
		PlaceOccupancyByID: map[string]factorydefinitions.FactoryPlaceOccupancy{"place": {WorkItemIDs: []string{"work"}}},
		ScriptRequestsByDispatchID: map[string]map[string]factorydefinitions.FactoryWorldScriptRequest{
			"dispatch": {"request": {Args: []string{"original"}}},
		},
		Artifacts: []factorydefinitions.FactorySessionArtifactState{{ID: "artifact", CaptureMetadata: map[string]string{"selected": "original"}}},
		SessionBracket: &factorydefinitions.FactoryWorldSessionBracketState{
			SessionID: "same-recorded-id", StartedAt: stamp, ArtifactIDs: []string{"artifact"},
			ResultSummary: []work.WorkContentPart{{Text: "original", JSON: json.RawMessage(`{"selected":true}`)}},
		},
	}
	projection, err := ReplayLegacyRecording(recording.ReplayArtifact{}, "requested", state)
	if err != nil {
		t.Fatal(err)
	}
	behavior := NewBehavior()
	first, peer := behavior.Acquire(projection, nil), behavior.Acquire(projection, nil)
	expected := peer.Inspection().FactoryProjection.State
	for _, view := range []*recording.FactoryWorldState{&state, projection.FactoryProjection, first.Inspection().FactoryProjection.State} {
		(*view.Factory)[0] = ' '
		view.WorkItemsByID["work"].Tags["selected"] = "mutated"
		view.WorkItemsByID["work"].StructuredResult.(map[string]any)["items"].([]any)[0] = "mutated"
		view.WorkItemsByID["work"].Content[0].Metadata["selected"] = "mutated"
		view.PlaceOccupancyByID["place"].WorkItemIDs[0] = "mutated"
		view.ScriptRequestsByDispatchID["dispatch"]["request"].Args[0] = "mutated"
		view.Artifacts[0].CaptureMetadata["selected"] = "mutated"
		view.SessionBracket.ArtifactIDs[0] = "mutated"
		view.SessionBracket.ResultSummary[0].JSON[0] = ' '
		view.SessionBracket.StartedAt = time.Time{}
	}
	for _, scope := range []*Scope{first, peer} {
		actual := scope.Inspection().FactoryProjection.State
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("selected Factory facts changed after mutation: %#v", actual)
		}
		if !actual.EventTime.Equal(stamp) || actual.EventTime.Location() != stamp.Location() {
			t.Fatalf("recorded timestamp changed: %v", actual.EventTime)
		}
	}
}

func TestServiceAcquisitionAndReturnedReadsDoNotLendProjectionStorage(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	projection.Session.ResolvedSource.Metadata = map[string]string{"selected": "original"}
	projection.Session.ResolvedSource.ResolutionOrder = []string{"selected"}
	projection.Session.ResolvedSource.Agents = map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent{"child": {Preset: "original"}}
	projection.Session.ResolvedSource.ArgsSchema = json.RawMessage(`{"type":"object"}`)
	projection.Session.ResolvedSource.DefaultPolicy = json.RawMessage(`{"limit":1}`)
	projection.Session.Policy.Requested = map[string]any{"nested": map[string]any{"items": []any{"original", json.Number("1")}}}
	projection.Session.Policy.Effective = map[string]any{"nested": []any{"original"}}
	projection.Session.PhaseSummaries = []fse.PhaseSummary{{Phase: "original"}}
	projection.Session.LatestCheckpoint = &fse.CheckpointRef{ID: "original"}
	projection.Session.Progress = &fse.ProgressCounts{TotalDispatches: 1}
	projection.Session.Budgets = &fse.SessionBudgets{MaxAgents: 1}
	projection.Session.Usage.Resources = []fse.ResourceUsage{{Name: "original"}}
	projection.Session.Failure = &fse.FailureSummary{Reason: "original"}
	projection.Session.Lifecycle = &fse.LifecycleTimestamps{StartedAt: &stamp}
	projection.Result.Failure = &fse.FailureSummary{Reason: "original"}
	projection.Result.Availability = &fse.ResultAvailabilityDetail{Reason: "original"}
	projection.Artifacts.Artifacts[0].CreatedAt = &stamp
	projection.Artifacts.Artifacts[0].RedactionCounts = &fse.ArtifactRedactionCounts{Secrets: 1}
	projection.Artifacts.Artifacts[0].RetrievalRef = &fse.ArtifactRetrievalRef{Href: "original"}
	projection.Checkpoint = &CheckpointReadModel{ID: "original", Timestamp: stamp}
	projection.WorkerHistory = recording.PortableRecordingWorkerHistory{
		Availability: recording.PortableRecordingWorkerHistoryAvailable,
		WorkerPortableRecording: &recording.WorkerPortableRecording{
			Records:     []recording.WorkerPortableRecord{{Payload: json.RawMessage(`{"original":true}`)}},
			Correlation: recording.WorkerPortableRecordingCorrelation{WorkIDs: []string{"original"}},
			Lifecycle:   recording.WorkerPortableRecordingLifecycle{OpeningTimestamp: &stamp},
		},
	}
	behavior := NewBehavior()
	first, peer := behavior.Acquire(projection, nil), behavior.Acquire(projection, nil)
	expected := peer.Inspection()
	// Freeze facts before mutating either the acquisition input or a returned view.
	baseline, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	assertRetained := func() {
		t.Helper()
		for _, scope := range []*Scope{first, peer} {
			actual, marshalErr := json.Marshal(scope.Inspection())
			if marshalErr != nil || string(actual) != string(baseline) {
				t.Fatalf("historical facts changed after caller mutation: %s; %v", actual, marshalErr)
			}
		}
	}
	mutateReplayViews(factorysessions.HistoricalReplayInspection{
		Session: projection.Session, Result: projection.Result, Events: projection.Events,
		Artifacts: projection.Artifacts, WorkerHistory: projection.WorkerHistory,
	})
	projection.Checkpoint.ID = "mutated"
	assertRetained()
	mutateReplayViews(first.Inspection())
	assertRetained()

	id, ctx := first.Inspection().Session.SessionID, t.Context()
	session, err := first.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	result, err := first.GetResult(ctx, id, fse.ResultRequest{Mode: fse.ResultModeFinal, IncludeArtifacts: true})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := first.ListArtifacts(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	events, err := first.ReadEvents(ctx, id, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	mutateReplayViews(factorysessions.HistoricalReplayInspection{Session: session, Result: result, Artifacts: artifacts, Events: events})
	assertRetained()
	detail, err := first.GetArtifact(ctx, id, expected.Artifacts.Artifacts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	detail.RetrievalRef.Href = "mutated"
	*detail.CreatedAt = time.Time{}
	assertRetained()
	listed, err := first.ListSessions(ctx, fse.ListSessionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	listed.DurableSessions[0].ResolvedSource.Metadata["selected"] = "mutated"
	listed.DurableSessions[0].Policy.Effective["nested"].([]any)[0] = "mutated"
	listed.DurableSessions[0].ResultSummary.Summary = "mutated"
	*listed.DurableSessions[0].Lifecycle.StartedAt = time.Time{}
	assertRetained()
	if number := peer.Inspection().Session.Policy.Requested["nested"].(map[string]any)["items"].([]any)[1]; !reflect.DeepEqual(number, json.Number("1")) {
		t.Fatalf("copy changed native JSON number type: %#v", number)
	}
}

func mutateReplayViews(value factorysessions.HistoricalReplayInspection) {
	value.Session.ResolvedSource.Metadata["selected"] = "mutated"
	value.Session.ResolvedSource.ResolutionOrder[0] = "mutated"
	value.Session.ResolvedSource.Agents["child"] = factorydefinitions.FactoryOrchestratorJavaScriptAgent{Preset: "mutated"}
	value.Session.ResolvedSource.ArgsSchema[0] = ' '
	value.Session.ResolvedSource.DefaultPolicy[0] = ' '
	value.Session.Policy.Requested["nested"].(map[string]any)["items"].([]any)[0] = "mutated"
	value.Session.Policy.Effective["nested"].([]any)[0] = "mutated"
	value.Session.PhaseSummaries[0].Phase = "mutated"
	value.Session.LatestCheckpoint.ID = "mutated"
	value.Session.Progress.TotalDispatches = 42
	value.Session.Budgets.MaxAgents = 42
	value.Session.Usage.Resources[0].Name = "mutated"
	value.Session.ResultSummary.Summary = "mutated"
	value.Session.ArtifactRefs[0].ID = "mutated"
	value.Session.Failure.Reason = "mutated"
	*value.Session.Lifecycle.StartedAt = time.Time{}
	value.Result.PrimaryResult[0] = ' '
	value.Result.ArtifactIDs[0] = "mutated"
	value.Result.ArtifactRefs[0].ID = "mutated"
	value.Result.Failure.Reason = "mutated"
	value.Result.Availability.Reason = "mutated"
	value.Artifacts.Artifacts[0].ID = "mutated"
	value.Artifacts.Artifacts[0].RetrievalRef.Href = "mutated"
	value.Artifacts.Artifacts[0].RedactionCounts.Secrets = 42
	*value.Artifacts.Artifacts[0].CreatedAt = time.Time{}
	value.Events.Events[0][0] = ' '
	if value.WorkerHistory.WorkerPortableRecording != nil {
		value.WorkerHistory.Records[0].Payload[0] = ' '
		value.WorkerHistory.Correlation.WorkIDs[0] = "mutated"
		*value.WorkerHistory.Lifecycle.OpeningTimestamp = time.Time{}
	}
	if value.Checkpoint != nil {
		value.Checkpoint.ID = "mutated"
	}
}

func TestServiceExposesRecordedSessionResultAndEvents(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewBehavior().Acquire(projection, nil)
	ctx := context.Background()

	session, err := service.GetSession(ctx, projection.Session.SessionID)
	if err != nil || session.SessionID != projection.Session.SessionID {
		t.Fatalf("GetSession = %#v, %v", session, err)
	}
	result, err := service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: fse.ResultModeFinal, IncludeArtifacts: true})
	if err != nil || len(result.ArtifactRefs) != 1 || !result.IncludeArtifacts {
		t.Fatalf("GetResult with artifacts = %#v, %v", result, err)
	}
	result, err = service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: fse.ResultModePartial})
	if err != nil || result.Mode != fse.ResultModePartial || len(result.ArtifactRefs) != 0 {
		t.Fatalf("GetResult without artifacts = %#v, %v", result, err)
	}
	if _, err := service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: "invalid"}); err == nil {
		t.Fatal("GetResult invalid mode succeeded")
	}

	events, err := service.ReadEvents(ctx, session.SessionID, fse.EventReconnectRequest{})
	if err != nil || len(events.Events) != len(projection.Events.Events) {
		t.Fatalf("ReadEvents = %#v, %v", events, err)
	}
}

func TestServiceExposesRecordedArtifactsAndEmptyDispatches(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewBehavior().Acquire(projection, nil)
	ctx := context.Background()
	sessionID := projection.Session.SessionID

	artifacts, err := service.ListArtifacts(ctx, sessionID)
	if err != nil || len(artifacts.Artifacts) != 1 {
		t.Fatalf("ListArtifacts = %#v, %v", artifacts, err)
	}
	artifact, err := service.GetArtifact(ctx, sessionID, artifacts.Artifacts[0].ID)
	if err != nil || artifact.ID != artifacts.Artifacts[0].ID {
		t.Fatalf("GetArtifact = %#v, %v", artifact, err)
	}
	if _, err := service.GetArtifact(ctx, sessionID, "missing"); !errors.Is(err, fse.ErrArtifactNotFound) {
		t.Fatalf("GetArtifact missing error = %v", err)
	}
	dispatches, err := service.ListDispatches(ctx, sessionID)
	if err != nil || dispatches.Dispatches == nil || len(dispatches.Dispatches) != 0 {
		t.Fatalf("ListDispatches = %#v, %v", dispatches, err)
	}
	if _, err := service.GetDispatch(ctx, sessionID, "missing"); !errors.Is(err, fse.ErrDispatchNotFound) {
		t.Fatalf("GetDispatch error = %v", err)
	}

	sessions, err := service.ListSessions(ctx, fse.ListSessionsRequest{})
	if err != nil || len(sessions.DurableSessions) != 1 || sessions.DurableSessions[0].SessionID != sessionID {
		t.Fatalf("ListSessions = %#v, %v", sessions, err)
	}
}

func TestServiceHistoricalDispatchQueriesRemainEmptyForEveryFilter(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewBehavior().Acquire(projection, nil)
	sessionID := projection.Session.SessionID
	for _, filters := range []fse.DispatchFilters{
		{},
		{Phase: "omitted-phase"},
		{Status: fse.DispatchStatusCompleted},
	} {
		filtered, filterErr := service.QueryDispatches(context.Background(), fse.DispatchQueryRequest{
			SessionID: sessionID,
			Filters:   filters,
		})
		if filterErr != nil || filtered.SessionID != sessionID || filtered.Dispatches == nil || len(filtered.Dispatches) != 0 {
			t.Fatalf("QueryDispatches filters=%#v = %#v, %v; want a non-nil empty result", filters, filtered, filterErr)
		}
	}
}

func TestServiceRejectsUnknownSessionsAndLiveOperations(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewBehavior().Acquire(projection, nil)
	ctx := context.Background()

	if _, err := service.GetSession(ctx, "missing"); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("GetSession missing error = %v", err)
	}
	if _, err := service.ListDispatches(ctx, "missing"); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("ListDispatches missing error = %v", err)
	}
	if _, err := service.QueryDispatches(ctx, fse.DispatchQueryRequest{SessionID: "missing"}); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("QueryDispatches missing error = %v", err)
	}
	var nilService *Scope
	if _, err := nilService.GetSession(ctx, projection.Session.SessionID); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("nil GetSession error = %v", err)
	}

	operations := []struct {
		name string
		run  func() error
	}{
		{"start async", func() error { _, err := service.StartAsync(ctx, fse.StartRequest{}); return err }},
		{"start sync", func() error { _, err := service.StartSync(ctx, fse.StartRequest{}); return err }},
		{"resume interrupted", func() error {
			_, err := service.ResumeInterruptedSession(ctx, projection.Session.SessionID, fse.ResumeSessionRequest{})
			return err
		}},
		{"pause", func() error {
			_, err := service.Pause(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"resume", func() error {
			_, err := service.Resume(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"cancel", func() error {
			_, err := service.Cancel(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"terminate", func() error {
			_, err := service.Terminate(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"approve", func() error {
			_, err := service.Approve(ctx, projection.Session.SessionID, fse.ApproveRequest{})
			return err
		}},
		{"retry dispatch", func() error {
			_, err := service.RetryDispatch(ctx, projection.Session.SessionID, fse.RetryDispatchRequest{})
			return err
		}},
		{"interrupt dispatch", func() error {
			_, err := service.InterruptDispatch(ctx, projection.Session.SessionID, fse.InterruptDispatchRequest{})
			return err
		}},
	}
	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			if err := operation.run(); !errors.Is(err, ErrNonLiveReplay) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func terminalResult() *recording.PortableRecordingCanonicalResult {
	return &recording.PortableRecordingCanonicalResult{
		Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-result"},
	}
}
