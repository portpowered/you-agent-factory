package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
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
				before := assertPerRuntimeAttemptState(t, t.Context(), owner, workersessions.StateRunning)
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
				after := assertPerRuntimeAttemptState(t, t.Context(), owner, workersessions.StateRunning)
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
			ownProjectionCalls := projector.calls
			wantErr := workersessions.ErrObservationSessionNotFound
			if scope == "   " {
				wantErr = workersessions.ErrInvalidObservationFactorySessionID
			}
			assertForeignObservationReads(t, registry, scope, wantErr)
			if projector.calls != ownProjectionCalls || reader.readCalls != 0 || reader.subscribeCalls != 0 {
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

// Topic addressing is component evidence, not proof of shared-registry replay
// admission. Each case observes the retained-reader effect and public cursor.
func TestKeyedRuntimeScopedTopicReplayPreservesWorkerIdentity(t *testing.T) {
	t.Parallel()
	const workerID = "recorded/worker"
	for _, owner := range []string{"recording/session", "replay/session"} {
		t.Run(owner, func(t *testing.T) {
			t.Parallel()
			topic := workersessions.Topic(workerID, owner)
			other := workersessions.Topic(workerID, "other/session")
			if topic == other || topic == workersessions.Topic(workerID) {
				t.Fatal("Factory ownership did not separate source topics")
			}
			record := replayObservationRecord(topic, 1, "message")
			reader := &observationEventReaderFake{readResults: []events.ReadResult{{
				Outcome:  events.ReadOutcomeProgress,
				Records:  []events.Record{record},
				Next:     events.Cursor{Topic: topic, Position: 1},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 1},
			}}}
			registry := newObservationRegistry(nil, reader)
			registry.sessions[workerID] = observationSession(workerID, workersessions.StateRunning)
			metadata := observationMetadata()
			metadata.factorySessionID = owner
			registry.observations[workerID] = metadata
			subscription, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
				WorkerSessionID: workerID, FactorySessionID: owner, ReplayOnly: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer subscription.Close()
			delivery := subscription.Next(context.Background())
			if delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.Cursor.WorkerSessionID != workerID || delivery.Event.Position != 1 {
				t.Fatalf("scoped delivery changed recorded identity: %+v", delivery)
			}
			if reader.readCalls != 1 || reader.readRequests[0].Topic != topic || reader.readRequests[0].From.Topic != topic {
				t.Fatalf("retained read selected foreign topic: %+v", reader.readRequests)
			}
			if got := observationWorkerSessionIDFromTopic(topic); got != workerID {
				t.Fatalf("scoped topic Worker identity = %q", got)
			}
		})
	}
}

func TestKeyedRuntimeDirectTopicKeepsCompatibility(t *testing.T) {
	t.Parallel()
	registry := newObservationRegistry(nil, nil)
	registry.observations["direct-worker"] = &observation{direct: true, factorySessionID: "direct-context"}
	if got := registry.observationTopic("direct-worker"); got != workersessions.Topic("direct-worker") {
		t.Fatalf("direct topic = %s", got)
	}
}

func TestKeyedRuntimeDirectReadAcceptsOnlyDefaultCompatibilityScope(t *testing.T) {
	t.Parallel()
	for _, state := range []workersessions.State{workersessions.StateRunning, workersessions.StateCompleted} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			registry := newObservationRegistry(nil, nil)
			registry.sessions["direct-worker"] = observationSession("direct-worker", state)
			registry.observations["direct-worker"] = &observation{direct: true, attemptID: "direct-attempt"}
			for _, scope := range []string{"", "~default", "foreign-session"} {
				got, err := registry.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{
					WorkerSessionID: "direct-worker", FactorySessionID: scope,
				})
				if scope == "foreign-session" {
					if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
						t.Fatalf("foreign read = %+v, %v", got, err)
					}
				} else if err != nil || got.WorkerSessionID != "direct-worker" || got.State != state || !got.Direct || got.FactorySessionID != "" {
					t.Fatalf("direct read (%s) = %+v, %v", scope, got, err)
				}
			}
		})
	}
}

// The same provider reference is retained independently in each Factory Session.
func TestKeyedRuntimeProviderReferenceReadsSelectScopeBeforeEnrichment(t *testing.T) {
	t.Parallel()
	for _, owner := range []string{"factory-a", "factory-b"} {
		t.Run(owner, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			topic := workersessions.Topic("worker-"+owner, owner)
			reader := &observationEventReaderFake{readResults: []events.ReadResult{{
				Outcome:  events.ReadOutcomeProgress,
				Records:  []events.Record{replayObservationRecord(topic, 1, "message")},
				Next:     events.Cursor{Topic: topic, Position: 1},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 1},
			}}}
			registry := newObservationRegistry(projector, reader)
			for _, scope := range []string{"factory-a", "factory-b"} {
				id := "worker-" + scope
				registry.sessions[id] = observationSession(id, workersessions.StateCompleted)
				metadata := observationMetadata()
				metadata.factorySessionID = scope
				registry.observations[id] = metadata
			}
			ctx := context.Background()
			got, err := registry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: observationProviderRef(), FactorySessionID: owner})
			if err != nil || got.WorkerSessionID != "worker-"+owner || got.FactorySessionID != owner {
				t.Fatalf("scoped provider observation = %+v, %v", got, err)
			}
			transcript, err := registry.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{ProviderSession: observationProviderRef(), FactorySessionID: owner})
			if err != nil || transcript.WorkerSessionID != got.WorkerSessionID {
				t.Fatalf("scoped provider transcript = %+v, %v", transcript, err)
			}
			stream, err := registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{ProviderSession: observationProviderRef(), FactorySessionID: owner, ReplayOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			delivery := stream.Next(ctx)
			if delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.Cursor.WorkerSessionID != got.WorkerSessionID || reader.readRequests[0].Topic != topic {
				t.Fatalf("scoped provider history = %+v; reads=%+v", delivery, reader.readRequests)
			}
			assertForeignProviderReadsHaveNoEffects(t, registry, projector, reader)
			registry.providerSessions = nil
			got, err = registry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: observationProviderRef(), FactorySessionID: owner})
			if err != nil || got.WorkerSessionID != "worker-"+owner || got.State != workersessions.StateCompleted {
				t.Fatalf("optional transcript loss hid scoped identity = %+v, %v", got, err)
			}
		})
	}
}

func assertForeignProviderReadsHaveNoEffects(t *testing.T, registry *registry, projector *trackingObservationProjector, reader *observationEventReaderFake) {
	t.Helper()
	ctx := context.Background()
	calls := projector.calls
	for _, foreign := range []string{"foreign", "   "} {
		wantErr := workersessions.ErrObservationSessionNotFound
		if foreign == "   " {
			wantErr = workersessions.ErrInvalidObservationFactorySessionID
		}
		_, err := registry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: observationProviderRef(), FactorySessionID: foreign})
		if !errors.Is(err, wantErr) {
			t.Fatalf("foreign provider observation = %v", err)
		}
		_, err = registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{ProviderSession: observationProviderRef(), FactorySessionID: foreign, ReplayOnly: true})
		if !errors.Is(err, wantErr) {
			t.Fatalf("foreign provider history = %v", err)
		}
	}
	if projector.calls != calls || reader.readCalls != 1 {
		t.Fatal("foreign lookup reached enrichment/history")
	}
}
