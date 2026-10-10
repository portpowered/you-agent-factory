package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDirectIdentityHandoffUsesReservedMetadataWithoutChangingRestartInput(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	metadata := &workersessions.SessionMetadata{
		Requester:   &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "lead", WorkID: "project"},
		Correlation: &workersessions.Correlation{WorkID: "original-work", FactorySessionID: "original-factory"},
	}
	if _, err := r.Reserve(t.Context(), workersessions.ReserveRequest{ID: "child", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	want := (workersessions.Session{ID: "child", Metadata: metadata}).IdentityEnvironment()
	var got workers.ExecuteRequest
	r.execution = coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		got = request.Clone()
		request.Target.Environment.SupervisedEnvironment[0] = "mutated"
		return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
	}}
	request := validStartRequest("child", "child-dispatch")
	request.Metadata = &workersessions.SessionMetadata{Requester: &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "replacement"}}
	request.Execution.Execution.ProcessEnvironment = []string{"PATH=explicit"}
	result, err := r.InvokeSession(t.Context(), request)
	if err != nil || result.Session.State != workersessions.StateCompleted {
		t.Fatalf("invocation = %#v, %v", result.Session, err)
	}
	if !reflect.DeepEqual(got.Target.Environment.SupervisedEnvironment, want) {
		t.Fatalf("supervised environment = %#v, want reserved facts %#v", got.Target.Environment.SupervisedEnvironment, want)
	}
	if !reflect.DeepEqual(got.Target.Environment.ProcessEnvironment, []string{"PATH=explicit"}) || !got.Target.Environment.SkipProcessInheritance {
		t.Fatal("identity binding changed ordinary explicit environment policy")
	}
	if !reflect.DeepEqual(result.Session.Metadata, metadata) || !reflect.DeepEqual(r.executionIdentityEnvironment("child"), want) {
		t.Fatal("runner mutated admitted identity")
	}
	supervision := r.supervisions["child"]
	if supervision != nil && !reflect.DeepEqual(supervision.execution.Execution.ProcessEnvironment, []string{"PATH=explicit"}) {
		t.Fatal("identity overlay entered retained restart execution")
	}
}

func TestRuntimeIdentityHandoffIsDetachedScopedAndAbsentOnRejectedAdmission(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	request := workersessions.RuntimeAttemptRequest{
		Key: workersessions.RuntimeAttemptKey{RuntimeID: "runtime-test", DispatchID: "identity-dispatch"},
		ID:  "scoped-child", Execution: runtimeAttemptHandoff("identity-dispatch"),
		Metadata: &workersessions.SessionMetadata{
			Requester:   &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "exact-lead"},
			Correlation: &workersessions.Correlation{WorkID: "source-work", FactorySessionID: "source-factory"},
		},
	}
	want := (workersessions.Session{ID: request.ID, Metadata: request.Metadata}).IdentityEnvironment()
	var got []string
	request.BindEnvironment = func(environment []string) {
		got = append([]string(nil), environment...)
		environment[0] = "mutated"
	}
	attempt, err := r.BeginRuntimeAttempt(t.Context(), request, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime identity = %#v, want public scoped facts %#v", got, want)
	}
	key := scopedWorkerAddress(request.ID, request.Execution.Execution.FactorySessionID)
	if !reflect.DeepEqual(r.executionIdentityEnvironment(key), want) {
		t.Fatal("runtime callback mutated registry facts")
	}
	if err := attempt.Complete(t.Context(), runtimeAttemptCompletedDispatch("identity-dispatch"), nil); err != nil {
		t.Fatal(err)
	}
	got = nil
	request.ID = "rejected-child"
	request.Metadata.Requester.Kind = "OPERATOR"
	if _, err := r.BeginRuntimeAttempt(t.Context(), request, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation); err == nil || got != nil {
		t.Fatal("invalid admission bound an identity environment")
	}
}

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

			reader := &observationEventReaderFake{}
			registry := newObservationRegistry(reader)
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
			if reader.readCalls != 0 || reader.subscribeCalls != 0 {
				t.Fatalf("foreign reads reached events: %d/%d", reader.readCalls, reader.subscribeCalls)
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

			registry := newObservationRegistry(nil)
			registry.sessions["recorded-worker"] = observationSession("recorded-worker", workersessions.StateCompleted)
			metadata := observationMetadata()
			metadata.factorySessionID = "recording-session"
			registry.observations["recorded-worker"] = metadata
			attachTranscriptCapture(registry, "recorded-worker", "recording-session", "hello")
			result, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: scope,
			})
			if err != nil || result.WorkerSessionID != "recorded-worker" {
				t.Fatalf("selected transcript = %+v, %v", result, err)
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
			registry := newObservationRegistry(reader)
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
	registry := newObservationRegistry(nil)
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
			registry := newObservationRegistry(nil)
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

			topic := workersessions.Topic("worker-"+owner, owner)
			reader := &observationEventReaderFake{readResults: []events.ReadResult{{
				Outcome:  events.ReadOutcomeProgress,
				Records:  []events.Record{replayObservationRecord(topic, 1, "message")},
				Next:     events.Cursor{Topic: topic, Position: 1},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 1},
			}}}
			registry := newObservationRegistry(reader)
			for _, scope := range []string{"factory-a", "factory-b"} {
				id := "worker-" + scope
				registry.sessions[id] = observationSession(id, workersessions.StateCompleted)
				metadata := observationMetadata()
				metadata.factorySessionID = scope
				registry.observations[id] = metadata
			}
			attachTranscriptCapture(registry, "worker-"+owner, owner, "hello")
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
			assertForeignProviderReadsHaveNoEffects(t, registry, reader)
			got, err = registry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: observationProviderRef(), FactorySessionID: owner})
			if err != nil || got.WorkerSessionID != "worker-"+owner || got.State != workersessions.StateCompleted {
				t.Fatalf("optional transcript loss hid scoped identity = %+v, %v", got, err)
			}
		})
	}
}

func assertForeignProviderReadsHaveNoEffects(t *testing.T, registry *registry, reader *observationEventReaderFake) {
	t.Helper()
	ctx := context.Background()

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
	if reader.readCalls != 1 {
		t.Fatal("foreign lookup reached enrichment/history")
	}
}
