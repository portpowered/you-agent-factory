package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestKeyedRuntimeControlsRejectForeignFactoryScopeBeforeEffects(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionPause, workersessions.ControlActionResume, workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, scope := range []string{"foreign-session", "   "} {
			t.Run(fmt.Sprintf("%s/%q", action, scope), func(t *testing.T) {
				t.Parallel()
				sink := &perRuntimeAppendCapture{EventsAppender: newEventsAppender()}
				owner := newPerRuntimeAttemptFixture(t, "scoped-control", sink)
				before := assertPerRuntimeAttemptState(t, owner, workersessions.StateRunning)
				appends := sink.requestsFor("")
				request := workersessions.ControlRequest{ID: owner.request.ID, FactorySessionID: scope, RequestID: "foreign-control"}
				control := scopedRuntimeControl(owner.service, action)
				result, err := control(context.Background(), request)
				wantErr := workersessions.ErrSessionNotFound
				if scope == "   " {
					wantErr = workersessions.ErrInvalidSessionID
				}
				if !errors.Is(err, wantErr) || result.Action != action || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.ID != "" {
					t.Fatalf("foreign control = %+v, %v; want FAILED/%v", result, err, wantErr)
				}
				_, getErr := owner.service.Get(context.Background(), workersessions.GetRequest{ID: owner.request.ID, FactorySessionID: scope})
				if !errors.Is(getErr, wantErr) {
					t.Fatalf("foreign Get = %v, want %v", getErr, wantErr)
				}
				select {
				case <-owner.control.invoked:
					t.Fatal("foreign control invoked cancellation")
				default:
				}
				after := assertPerRuntimeAttemptState(t, owner, workersessions.StateRunning)
				if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(appends, sink.requestsFor("")) {
					t.Fatal("foreign control changed owner state or published history")
				}
			})
		}
	}
}

func scopedRuntimeControl(registry *registry, action workersessions.ControlAction) func(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	switch action {
	case workersessions.ControlActionPause:
		return registry.Pause
	case workersessions.ControlActionResume:
		return registry.Resume
	case workersessions.ControlActionTerminate:
		return registry.Terminate
	default:
		return registry.Cancel
	}
}

func TestKeyedRuntimeControlsRetainSelectedFactoryScopeAndDirectCompatibility(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "factory-scoped-control", " factory-scoped-control "} {
		t.Run(fmt.Sprintf("%q", scope), func(t *testing.T) {
			t.Parallel()
			owner := newPerRuntimeAttemptFixture(t, "scoped-control", newEventsAppender())
			if err := owner.attempt.Complete(context.Background(), runtimeAttemptCompletedDispatch(perRuntimeLogicalDispatchID), nil); err != nil {
				t.Fatal(err)
			}
			before, err := owner.service.Get(context.Background(), workersessions.GetRequest{ID: owner.request.ID, FactorySessionID: scope})
			if err != nil || before.ID != owner.request.ID || before.State != workersessions.StateCompleted {
				t.Fatalf("selected Get = %+v, %v", before, err)
			}
			result, err := owner.service.Cancel(context.Background(), workersessions.ControlRequest{ID: owner.request.ID, FactorySessionID: scope})
			if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || !reflect.DeepEqual(result.Session, before) || result.DispatchID != perRuntimeLogicalDispatchID {
				t.Fatalf("selected terminal control = %+v, %v; before=%+v", result, err, before)
			}
		})
	}
}

func TestKeyedRuntimeObservationReadsRejectForeignFactoryScopeBeforeEffects(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"replay-session", "   "} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			reader := &observationEventReaderFake{}
			registry := newObservationRegistry(projector, reader)
			registry.sessions["recorded-worker"] = observationSession("recorded-worker", workersessions.StateCompleted)
			metadata := observationMetadata()
			metadata.factorySessionID = "recording-session"
			registry.observations["recorded-worker"] = metadata
			ctx := context.Background()
			before, err := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: "recording-session",
			})
			if err != nil {
				t.Fatal(err)
			}
			wantErr := workersessions.ErrObservationSessionNotFound
			if scope == "   " {
				wantErr = workersessions.ErrInvalidObservationFactorySessionID
			}
			assertForeignObservationReads(t, registry, scope, wantErr)
			if projector.calls != 0 || reader.readCalls != 0 || reader.subscribeCalls != 0 {
				t.Fatalf("foreign reads reached provider/events: %d/%d/%d", projector.calls, reader.readCalls, reader.subscribeCalls)
			}
			after, err := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: "recording-session",
			})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("retained recording observation changed: before=%+v after=%+v err=%v", before, after, err)
			}
		})
	}
}

func assertForeignObservationReads(t *testing.T, registry *registry, scope string, wantErr error) {
	t.Helper()
	ctx := context.Background()
	_, getErr := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, transcriptErr := registry.ReadTranscriptByWorkerSessionID(ctx, workersessions.ReadTranscriptByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, canonicalTranscriptErr := registry.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, providerTranscriptErr := registry.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		ProviderSession: observationProviderRef(), FactorySessionID: scope,
	})
	_, streamErr := registry.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope, ReplayOnly: true,
	})
	_, liveErr := registry.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	for _, err := range []error{getErr, transcriptErr, canonicalTranscriptErr, providerTranscriptErr, streamErr, liveErr} {
		if !errors.Is(err, wantErr) {
			t.Errorf("foreign scope read error = %v, want %v", err, wantErr)
		}
	}
}

func TestKeyedRuntimeObservationReadsRetainSelectedScopeAndDirectCompatibility(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "recording-session", " recording-session "} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			registry := newObservationRegistry(projector, nil)
			registry.sessions["recorded-worker"] = observationSession("recorded-worker", workersessions.StateCompleted)
			metadata := observationMetadata()
			metadata.factorySessionID = "recording-session"
			registry.observations["recorded-worker"] = metadata
			result, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: scope,
			})
			if err != nil || result.WorkerSessionID != "recorded-worker" || projector.calls != 1 || projector.request.Session != observationProviderRef() {
				t.Fatalf("selected transcript = %+v, %v; provider calls=%d request=%+v", result, err, projector.calls, projector.request)
			}
		})
	}
}
