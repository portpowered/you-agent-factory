package recordings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestRootContractInvariants_AllSlicesThroughSingularService seals the
// Recordings root-contract packet for IMP-REC unlock: every published slice
// (append/subscribe, projection query, recording lifecycle, replay, artifact
// export) is reachable through one named recordings.Service, a peer-shaped
// consumer can exercise success and typed-failure paths using only the root
// package (no events/, projections/, replay/, artifacts/, or service/
// imports), and no second peer-facing Recordings authority is required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	service := newSealRootService()
	assertSealAppendSubscribe(t, service)
	assertSealProjectionQuery(t, service)
	assertSealRecordingLifecycle(t, service)
	assertSealReplay(t, service)
	assertSealArtifactExport(t, service)
}

func newSealRootService() recordings.Service {
	seedArtifact := &interfaces.ReplayArtifact{
		SchemaVersion: "factory-event-log/v1",
		RecordedAt:    time.Unix(1_700_000_000, 0).UTC(),
		Events:        []interfaces.FactoryEvent{{Id: "event-1", Type: "WORK_REQUEST"}},
	}
	return &peerRootServiceFake{
		streamGenerationID: "generation-seal",
		reconstructState:   interfaces.FactoryWorldState{Tick: 3},
		dashboardData: recordings.SimpleDashboardRenderData{
			InFlightDispatchCount: 1,
		},
		replayByKey: map[string]*interfaces.ReplayArtifact{
			"seal.replay.json": seedArtifact,
		},
		replayBindingHooks: []recordings.ReplayHook{},
	}
}

func assertSealAppendSubscribe(t *testing.T, service recordings.Service) {
	t.Helper()
	ctx := context.Background()
	_ = service.Append(recordings.AppendRecordedEventRequest{
		Event: interfaces.FactoryEvent{Id: "event-1", Type: "WORK_REQUEST"},
	})
	_ = service.Append(recordings.AppendRecordedEventRequest{
		Event: interfaces.FactoryEvent{Id: "event-2", Type: "WORK_STATE_CHANGE"},
	})
	subscribed, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &recordings.EventReconnectCursor{AfterEventID: "event-1"},
		Scope:  recordings.EventReconnectScope{SessionID: "session-seal"},
	})
	if err != nil {
		t.Fatalf("append/subscribe success: %v", err)
	}
	if len(subscribed.Stream.History) != 1 || subscribed.Stream.History[0].Id != "event-2" {
		t.Fatalf("append/subscribe history = %#v, want event-2 newer than cursor", subscribed.Stream.History)
	}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &recordings.EventReconnectCursor{AfterEventID: "missing"},
		Scope:  recordings.EventReconnectScope{SessionID: "session-seal"},
	})
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("append/subscribe typed failure = %v, want ErrReconnectCursorNotFound", err)
	}
}

func assertSealProjectionQuery(t *testing.T, service recordings.Service) {
	t.Helper()
	world, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       []interfaces.FactoryEvent{{Id: "event-1", Type: "WORK_REQUEST"}},
		SelectedTick: 3,
	})
	if err != nil {
		t.Fatalf("projection-query success: %v", err)
	}
	if world.WorldState.Tick != 3 {
		t.Fatalf("projection-query world tick = %d, want 3", world.WorldState.Tick)
	}
	dashboard := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	})
	if dashboard.Data.InFlightDispatchCount != 1 {
		t.Fatalf("projection-query dashboard = %#v, want InFlightDispatchCount 1", dashboard.Data)
	}
	_, err = service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		SelectedTick: -1,
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("projection-query typed failure = %v, want ErrInvalidProjectionInput", err)
	}
}

func assertSealRecordingLifecycle(t *testing.T, service recordings.Service) {
	t.Helper()
	ctx := context.Background()
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordPath: "seal.replay.json",
	})
	if err != nil {
		t.Fatalf("recording-lifecycle bind: %v", err)
	}
	if _, err := service.StartRecording(ctx, recordings.StartRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("recording-lifecycle start: %v", err)
	}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       interfaces.FactoryEvent{Id: "event-1", Type: "WORK_REQUEST"},
	}); err != nil {
		t.Fatalf("recording-lifecycle record: %v", err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("recording-lifecycle flush success: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("recording-lifecycle finish: %v", err)
	}
	_, err = service.BindRecording(recordings.BindRecordingRequest{RecordPath: ""})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("recording-lifecycle typed failure = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertSealReplay(t *testing.T, service recordings.Service) {
	t.Helper()
	loaded, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "seal.replay.json",
	})
	if err != nil || loaded.Artifact == nil {
		t.Fatalf("replay load success: result=%#v err=%v", loaded, err)
	}
	boundReplay, err := service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: loaded.Artifact,
	})
	if err != nil || boundReplay.Hooks == nil {
		t.Fatalf("replay bind success: result=%#v err=%v", boundReplay, err)
	}
	_, err = service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{})
	if !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("replay typed failure = %v, want ErrMissingReplayArtifact", err)
	}
}

func assertSealArtifactExport(t *testing.T, service recordings.Service) {
	t.Helper()
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		Facts: recordings.PortableRecordingCanonicalFacts{
			SessionID:        "session-seal",
			Status:           "COMPLETED",
			OrchestratorKind: "JAVASCRIPT",
			SourceRef:        "workflow/seal.js",
			SourceHash:       peerRootDigest('a'),
			PolicyHash:       peerRootDigest('b'),
			Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
				ID: "artifact-seal", Kind: "RESULT", Visibility: "PUBLIC",
				Label: "Result", ContentHash: peerRootDigest('c'), SizeBytes: 4,
				CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
			}},
		},
	})
	if err != nil {
		t.Fatalf("artifact-export build success: %v", err)
	}
	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: built.Recording,
	}); err != nil {
		t.Fatalf("artifact-export validate success: %v", err)
	}
	summary, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Recording: built.Recording,
	})
	if err != nil || summary.SessionID != "session-seal" {
		t.Fatalf("artifact-export summarize success: result=%#v err=%v", summary, err)
	}
	_, err = service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("artifact-export typed failure = %v, want ErrInvalidRecordingDecode", err)
	}
}
