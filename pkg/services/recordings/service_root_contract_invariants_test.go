package recordings_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
	return &peerRootServiceFake{
		streamGenerationID: "generation-seal",
		dashboardData: recordings.SimpleDashboardRenderData{
			InFlightDispatchCount: 1,
		},
	}
}

func assertSealAppendSubscribe(t *testing.T, service recordings.Service) {
	t.Helper()
	ctx := context.Background()
	first, err := service.Append(recordings.AppendRecordedEventRequest{
		Event: rootAppendEvent("event-1", "WORK_REQUEST"),
	})
	if err != nil {
		t.Fatalf("append/subscribe first append: %v", err)
	}
	if _, err := service.Append(recordings.AppendRecordedEventRequest{
		Event: rootAppendEvent("event-2", "WORK_STATE_CHANGE"),
	}); err != nil {
		t.Fatalf("append/subscribe second append: %v", err)
	}
	subscribed, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &first.Event.Cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-seal"},
	})
	if err != nil {
		t.Fatalf("append/subscribe success: %v", err)
	}
	outcome := subscribed.Subscription.Next(ctx)
	if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != "event-2" {
		t.Fatalf("append/subscribe outcome = %#v, want event-2 newer than cursor", outcome)
	}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-seal",
			Sequence:           99,
		},
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-seal"},
	})
	if !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("append/subscribe typed failure = %v, want ErrReconnectCursorExpired", err)
	}
}

func assertSealProjectionQuery(t *testing.T, service recordings.Service) {
	t.Helper()
	world, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events: []recordings.CanonicalEvent{
			projectionEvent("event-1", 0, recordings.CanonicalEventScope{}, "WORK_REQUEST"),
		},
		SelectedTick: 3,
	})
	if err != nil {
		t.Fatalf("projection-query success: %v", err)
	}
	if world.WorldState.SelectedTick != 3 {
		t.Fatalf("projection-query world tick = %d, want 3", world.WorldState.SelectedTick)
	}
	dashboard, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	})
	if err != nil {
		t.Fatalf("projection-query dashboard: %v", err)
	}
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
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-seal"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:seal",
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("recording-lifecycle bind: %v", err)
	}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       lifecycleEvent("event-1", 0, scope),
	}); err != nil {
		t.Fatalf("recording-lifecycle record: %v", err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("recording-lifecycle flush success: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("recording-lifecycle finish: %v", err)
	}
	_, err = service.BindRecording(recordings.BindRecordingRequest{})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("recording-lifecycle typed failure = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertSealReplay(t *testing.T, service recordings.Service) {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-replay-seal"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:replay-seal",
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("replay bind recording: %v", err)
	}
	event := lifecycleEvent("replay-event-1", 0, scope)
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("replay record event: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("replay finish recording: %v", err)
	}
	loaded, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("replay load success: %v", err)
	}
	planned, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
		SelectedTick:  3,
	})
	if err != nil {
		t.Fatalf("replay plan success: %v", err)
	}
	observed, err := service.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil || observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("replay observation: result=%#v err=%v", observed, err)
	}
	_, err = service.ObserveReplay(recordings.ObserveReplayRequest{Plan: "missing"})
	if !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("replay typed failure = %v, want ErrReplayPlanNotFound", err)
	}
}

func assertSealArtifactExport(t *testing.T, service recordings.Service) {
	t.Helper()
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-seal",
		Artifact:    "artifact:seal-export",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-seal",
		},
	})
	if err != nil {
		t.Fatalf("artifact-export bind: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("artifact-export finish: %v", err)
	}
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("artifact-export build success: %v", err)
	}
	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("artifact-export validate success: %v", err)
	}
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("artifact-export encode success: bytes=%d err=%v", len(encoded.Payload), err)
	}
	summary, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || summary.Summary.Scope.FactorySessionID != "session-seal" {
		t.Fatalf("artifact-export summarize success: result=%#v err=%v", summary, err)
	}
	_, err = service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	})
	if !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("artifact-export typed failure = %v, want ErrInvalidPortableArtifact", err)
	}
}
