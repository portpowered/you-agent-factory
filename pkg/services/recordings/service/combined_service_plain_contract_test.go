package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

type stubLedger struct {
	events         []factorydefinitions.FactoryEvent
	subscribeErr   error
	subscribeScope factorydefinitions.FactoryEventReconnectScope
}

func (ledger *stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *stubLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger.subscribeScope = scope
	if ledger.subscribeErr != nil {
		return factorydefinitions.FactoryEventStream{}, ledger.subscribeErr
	}
	return factorydefinitions.FactoryEventStream{}, nil
}

func (ledger *stubLedger) StreamGenerationID() string { return "gen-1" }

func (ledger *stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *stubLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	ledger.events = append(ledger.events, event)
}

func portableDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func samplePortableFacts() recordings.PortableRecordingCanonicalFacts {
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	return recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-service-1",
		Status:           "COMPLETED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/export.js",
		SourceHash:       portableDigest('1'),
		PolicyHash:       portableDigest('2'),
		Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
			ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC",
			Label: "Result", ContentHash: portableDigest('3'), SizeBytes: 21, CreatedAt: createdAt,
		}},
		Result: &recordings.PortableRecordingCanonicalResult{
			Status: "FINAL", Mode: "final",
			PrimaryResult: json.RawMessage(`{"answer":"ok"}`),
			ArtifactIDs:   []string{"artifact-result"},
			Availability: &recordings.PortableRecordingAvailability{
				Reason: "READY", Message: "available", Retryable: false,
			},
		},
	}
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	t.Parallel()
	if got := NewService(nil, NewProjectionService()); got != nil {
		t.Fatalf("NewService(nil, projection) = %#v, want nil", got)
	}
	if got := NewService(&stubLedger{}, nil); got != nil {
		t.Fatalf("NewService(ledger, nil) = %#v, want nil", got)
	}
}

func TestCombinedServicePlainSlices_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := NewService(ledger, NewProjectionService())
	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	assertAppendSubscribe(t, svc, ledger)
	assertProjectionQuery(t, svc)
	assertRecordingLifecycle(t, svc)
	assertReplay(t, svc)
	assertArtifactExport(t, svc)
}

func assertAppendSubscribe(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	event := factorydefinitions.FactoryEvent{Id: "evt-1", Type: factorydefinitions.FactoryEventTypeWorkRequest}
	_ = svc.Append(recordings.AppendRecordedEventRequest{Event: event})
	if len(ledger.events) != 1 || ledger.events[0].Id != "evt-1" {
		t.Fatalf("Append did not delegate: %#v", ledger.events)
	}

	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.EventReconnectScope{SessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("SubscribeFrom whitespace scope = %v, want ErrInvalidSubscribeScope", err)
	}

	ledger.subscribeErr = recordings.ErrReconnectCursorNotFound
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &recordings.EventReconnectCursor{AfterEventID: "missing"},
		Scope:  recordings.EventReconnectScope{SessionID: "session-1"},
	}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("SubscribeFrom stale cursor = %v, want ErrReconnectCursorNotFound", err)
	}
	ledger.subscribeErr = nil
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.EventReconnectScope{SessionID: "session-1"},
	}); err != nil {
		t.Fatalf("SubscribeFrom success path: %v", err)
	}
}

func assertProjectionQuery(t *testing.T, svc recordings.Service) {
	t.Helper()
	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		SelectedTick: -1,
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState negative tick = %v, want ErrInvalidProjectionInput", err)
	}
	world, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       nil,
		SelectedTick: 0,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState success path: %v", err)
	}
	_ = svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{WorldState: world.WorldState})
	_ = svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{WorldState: world.WorldState})
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{}); err != nil {
		t.Fatalf("ValidateReconnectReplayFrom empty: %v", err)
	}
}

func assertRecordingLifecycle(t *testing.T, svc recordings.Service) {
	t.Helper()
	if _, err := svc.BindRecording(recordings.BindRecordingRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("BindRecording empty path = %v, want ErrMissingRecordingTarget", err)
	}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{RecordPath: "recording.json"})
	if err != nil || bound.RecordingID == "" {
		t.Fatalf("BindRecording success = (%#v, %v)", bound, err)
	}
	if _, err := svc.StartRecording(context.Background(), recordings.StartRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       factorydefinitions.FactoryEvent{Id: "rec-evt-1"},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{RecordingID: bound.RecordingID}); err != nil {
		t.Fatalf("FlushRecording: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       factorydefinitions.FactoryEvent{Id: "after-finish"},
	}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write = %v, want ErrRecordingWriteRejected", err)
	}

	boundFail, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "flush-fail",
		RecordPath:  "fail.json",
	})
	if err != nil {
		t.Fatalf("BindRecording flush-fail: %v", err)
	}
	if _, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: boundFail.RecordingID,
		Err:         errors.New("producer boom"),
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: boundFail.RecordingID,
	}); !errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("FlushRecording after error = %v, want ErrRecordingFlushFailed", err)
	}
	if _, err := svc.StopRecording(recordings.StopRecordingRequest{RecordingID: boundFail.RecordingID}); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	status, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: boundFail.RecordingID})
	if err != nil || !status.Stopped || status.Err == nil {
		t.Fatalf("QueryRecordingStatus = (%#v, %v)", status, err)
	}
	if _, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("QueryRecordingStatus missing = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertReplay(t *testing.T, svc recordings.Service) {
	t.Helper()
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("LoadReplayArtifact missing = %v, want ErrMissingReplayArtifact", err)
	}
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{Path: "missing.json"}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("LoadReplayArtifact unknown = %v, want ErrInvalidReplayArtifact", err)
	}
	if _, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{}); !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("BindReplayExecution empty = %v, want ErrUnsupportedReplayBinding", err)
	}
	bound, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &factorydefinitions.ReplayArtifact{SchemaVersion: "1"},
	})
	if err != nil || bound.Hooks == nil {
		t.Fatalf("BindReplayExecution success = (%#v, %v)", bound, err)
	}

	combined, ok := svc.(*combinedService)
	if !ok {
		t.Fatalf("service type = %T, want *combinedService", svc)
	}
	artifact := &factorydefinitions.ReplayArtifact{SchemaVersion: "1"}
	combined.replayByKey["artifact-1"] = artifact
	loaded, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{ArtifactID: "artifact-1"})
	if err != nil || loaded.Artifact == nil || loaded.Artifact.SchemaVersion != "1" {
		t.Fatalf("LoadReplayArtifact seeded = (%#v, %v)", loaded, err)
	}
}

func assertArtifactExport(t *testing.T, svc recordings.Service) {
	t.Helper()
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{Facts: samplePortableFacts()})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: built.Recording,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	summary, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Recording: built.Recording,
	})
	if err != nil || summary.SessionID != "session-service-1" {
		t.Fatalf("SummarizePortableArtifact = (%#v, %v)", summary, err)
	}
	if _, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{}); !errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("SummarizePortableArtifact empty = %v, want ErrInvalidRecordingSummary", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{}); !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("DecodePortableArtifact empty = %v, want ErrInvalidRecordingDecode", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte("{"),
	}); !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidRecordingDecode", err)
	}

	badFacts := samplePortableFacts()
	badFacts.SourceHash = "not-a-digest"
	if _, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{Facts: badFacts}); !errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("BuildPortableArtifact bad digest = %v, want ErrInvalidRecordingDigest", err)
	}
	invalid := built.Recording
	invalid.Session.ID = ""
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{Recording: invalid}); !errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("ValidatePortableArtifact empty session = %v, want ErrInvalidRecordingSummary", err)
	}
}

func TestProjectionServiceDelegates(t *testing.T) {
	t.Parallel()
	projection := NewProjectionService()
	state, err := projection.ReconstructFactoryWorldState(nil, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	_ = projection.SimpleDashboardRenderData(state)
	_ = projection.ProjectActiveThrottlePauses(factorydefinitions.InitialStructurePayload{}, nil)
	_ = projection.ProjectWorkstationRequests(state)
	if err := projection.ValidateReconnectReplay(nil, factorydefinitions.FactoryEventReconnectCursor{}, factorydefinitions.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("ValidateReconnectReplay: %v", err)
	}
}

func TestNewReplayClockAndExecutionNilArtifact(t *testing.T) {
	t.Parallel()
	if got := NewReplayClock(nil); got != nil {
		t.Fatalf("NewReplayClock(nil) = %#v, want nil", got)
	}
	provider, runner, hooks, planner, err := NewReplayExecution(nil, nil, nil)
	if provider != nil || runner != nil || hooks != nil || planner != nil || err != nil {
		t.Fatalf("NewReplayExecution(nil) = (%v,%v,%v,%v,%v), want nils", provider, runner, hooks, planner, err)
	}
}
