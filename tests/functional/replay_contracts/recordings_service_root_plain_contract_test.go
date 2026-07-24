package replay_contracts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type recordingsRootLedgerStub struct {
	events       []factorydefinitions.FactoryEvent
	subscribeErr error
}

func (ledger *recordingsRootLedgerStub) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *recordingsRootLedgerStub) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	_ factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	if ledger.subscribeErr != nil {
		return factorydefinitions.FactoryEventStream{}, ledger.subscribeErr
	}
	return factorydefinitions.FactoryEventStream{}, nil
}

func (ledger *recordingsRootLedgerStub) StreamGenerationID() string { return "functional-gen" }

func (ledger *recordingsRootLedgerStub) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *recordingsRootLedgerStub) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
}

func (ledger *recordingsRootLedgerStub) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	ledger.events = append(ledger.events, event)
}

func recordingsRootDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func TestRecordingsServiceRootPlainContract_FunctionalCoverage(t *testing.T) {
	ledger := &recordingsRootLedgerStub{}
	svc := recordingservice.NewService(ledger, recordingservice.NewProjectionService())
	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	_ = svc.Append(recordings.AppendRecordedEventRequest{
		Event: factorydefinitions.FactoryEvent{Id: "functional-evt-1"},
	})
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.EventReconnectScope{SessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("SubscribeFrom invalid scope = %v", err)
	}
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.EventReconnectScope{SessionID: "session-functional"},
	}); err != nil {
		t.Fatalf("SubscribeFrom success: %v", err)
	}

	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{SelectedTick: -1}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState negative tick = %v", err)
	}
	world, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{SelectedTick: 0})
	if err != nil {
		t.Fatalf("ReconstructWorldState: %v", err)
	}
	_ = svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{WorldState: world.WorldState})
	_ = svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{WorldState: world.WorldState})
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{}); err != nil {
		t.Fatalf("ValidateReconnectReplayFrom: %v", err)
	}

	bound, err := svc.BindRecording(recordings.BindRecordingRequest{RecordPath: "functional.json"})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	if _, err := svc.StartRecording(context.Background(), recordings.StartRecordingRequest{RecordingID: bound.RecordingID}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       factorydefinitions.FactoryEvent{Id: "functional-rec-1"},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{RecordingID: bound.RecordingID}); err != nil {
		t.Fatalf("FlushRecording: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.RecordingID,
		FinishedAt:  time.Unix(1_700_000_200, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	if _, err := svc.StopRecording(recordings.StopRecordingRequest{RecordingID: bound.RecordingID}); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if _, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: bound.RecordingID}); err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       factorydefinitions.FactoryEvent{Id: "after-finish"},
	}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write = %v, want ErrRecordingWriteRejected", err)
	}
	failBound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "functional-flush-fail",
		RecordPath:  "fail.json",
	})
	if err != nil {
		t.Fatalf("BindRecording flush-fail: %v", err)
	}
	if _, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: failBound.RecordingID,
		Err:         errors.New("producer boom"),
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{RecordingID: failBound.RecordingID}); !errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("FlushRecording after error = %v", err)
	}

	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("LoadReplayArtifact missing = %v", err)
	}
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{Path: "missing.json"}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("LoadReplayArtifact unknown = %v", err)
	}
	if _, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &factorydefinitions.ReplayArtifact{SchemaVersion: "1"},
	}); err != nil {
		t.Fatalf("BindReplayExecution: %v", err)
	}

	createdAt := time.Unix(1_700_000_000, 0).UTC()
	facts := recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-functional-export",
		Status:           "COMPLETED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/export.js",
		SourceHash:       recordingsRootDigest('a'),
		PolicyHash:       recordingsRootDigest('b'),
		Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
			ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC",
			Label: "Result", ContentHash: recordingsRootDigest('c'), SizeBytes: 8, CreatedAt: createdAt,
		}},
		Result: &recordings.PortableRecordingCanonicalResult{
			Status: "FINAL", Mode: "final",
			PrimaryResult: json.RawMessage(`{"ok":true}`),
			ArtifactIDs:   []string{"artifact-result"},
		},
	}
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{Facts: facts})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{Recording: built.Recording}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	if _, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{Recording: built.Recording}); err != nil {
		t.Fatalf("SummarizePortableArtifact: %v", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{}); !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("DecodePortableArtifact empty = %v", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{Payload: []byte("{")}); !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("DecodePortableArtifact malformed = %v", err)
	}
	badFacts := facts
	badFacts.SourceHash = "bad"
	if _, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{Facts: badFacts}); !errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("BuildPortableArtifact bad digest = %v", err)
	}

	if recordingservice.NewService(nil, recordingservice.NewProjectionService()) != nil {
		t.Fatal("NewService(nil, projection) should be nil")
	}
	if recordingservice.NewReplayClock(nil) != nil {
		t.Fatal("NewReplayClock(nil) should be nil")
	}
	provider, runner, hooks, planner, err := recordingservice.NewReplayExecution(nil, nil, nil)
	if provider != nil || runner != nil || hooks != nil || planner != nil || err != nil {
		t.Fatalf("NewReplayExecution(nil) = unexpected non-nil values")
	}
	if _, err := recordingservice.NewRuntimeRecorder(nil, 0, nil, nil, "", nil); err != nil {
		t.Fatalf("NewRuntimeRecorder disabled: %v", err)
	}
	if _, err := recordingservice.NewRuntimeRecorder(nil, 0, nil, nil, "recording.json", nil); err == nil {
		t.Fatal("NewRuntimeRecorder enabled without clock should error")
	}
}
