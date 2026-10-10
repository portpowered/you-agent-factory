package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
		token := strings.TrimPrefix(got.Target.Environment.SupervisedEnvironment[len(want)], "YOU_WORKER_SESSION_TOKEN=")
		output := coverageExecutionResult(request, workers.ExecutionOutcomeAccepted)
		output.Output.Primary = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "ordinary " + token}}
		output.ProposedOutputPresent = true
		return output, nil
	}}
	request := validStartRequest("child", "child-dispatch")
	request.Metadata = &workersessions.SessionMetadata{Requester: &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "replacement"}}
	request.Execution.Execution.ProcessEnvironment = []string{"PATH=explicit"}
	result, err := r.InvokeSession(t.Context(), request)
	if err != nil || result.Session.State != workersessions.StateCompleted {
		t.Fatalf("invocation = %#v, %v", result.Session, err)
	}
	token := assertIdentityTokenEnvironment(t, got.Target.Environment.SupervisedEnvironment, want)
	assertTokenPublicationAbsent(t, r, result.Session, token)
	if !reflect.DeepEqual(got.Target.Environment.SupervisedEnvironment[:len(want)], want) {
		t.Fatal("supervised environment changed reserved identity facts")
	}
	if !reflect.DeepEqual(got.Target.Environment.ProcessEnvironment, []string{"PATH=explicit"}) || !got.Target.Environment.SkipProcessInheritance {
		t.Fatal("identity binding changed ordinary explicit environment policy")
	}
	if !reflect.DeepEqual(result.Session.Metadata, metadata) || !reflect.DeepEqual(result.Session.IdentityEnvironment(), want) {
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
	assertIdentityTokenEnvironment(t, got, want)
	if !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatal("runtime identity changed public scoped facts")
	}
	admitted, err := r.Get(t.Context(), workersessions.GetRequest{ID: request.ID, FactorySessionID: request.Execution.Execution.FactorySessionID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(admitted.IdentityEnvironment(), want) {
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

func TestRuntimeCallerRetainsChildCorrelationAndOnlyVerifiedRequester(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"begin", "invoke"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			testRuntimeCallerMetadata(t, mode)
		})
	}
}

func testRuntimeCallerMetadata(t *testing.T, mode string) {
	t.Helper()
	r := newTestRegistry(t)
	caller := runningCaller(t, r, "source")
	metadata := &workersessions.SessionMetadata{
		Requester:   &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "untrusted"},
		Correlation: &workersessions.Correlation{WorkID: "child-work", FactorySessionID: "child-factory"},
		Labels:      []string{"tag:child=value"},
	}
	request := workersessions.RuntimeAttemptRequest{
		Key: workersessions.RuntimeAttemptKey{RuntimeID: "runtime-test", DispatchID: "caller-dispatch"},
		ID:  "caller-child", Execution: runtimeAttemptHandoff("caller-dispatch"), Metadata: metadata, Caller: caller,
	}
	var environment []string
	request.BindEnvironment = func(value []string) { environment = append([]string(nil), value...) }
	var attempt workersessions.RuntimeAttempt
	var err error
	if mode == "begin" {
		attempt, err = r.BeginRuntimeAttempt(t.Context(), request, r.execution, r.clock, r.scheduler, runtimeAttemptNoopCancellation)
	} else {
		executor := coverageExecution{execute: func(_ context.Context, input workers.ExecuteRequest) (workers.ExecuteResult, error) {
			environment = append([]string(nil), input.Target.Environment.SupervisedEnvironment...)
			token := strings.TrimPrefix(environment[len(environment)-1], "YOU_WORKER_SESSION_TOKEN=")
			output := coverageExecutionResult(input, workers.ExecutionOutcomeAccepted)
			output.Output.Primary = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "ordinary " + token}}
			output.ProposedOutputPresent = true
			return output, nil
		}}
		_, err = r.InvokeRuntimeSession(t.Context(), request, workersessions.RetryPolicy{}, executor, r.clock, r.scheduler)
	}
	if err != nil {
		t.Fatal(err)
	}
	want := metadata.Clone()
	want.Requester = &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "source", WorkID: "lane"}
	want.Labels = append(want.Labels, "parent:source")
	key := scopedWorkerAddress(request.ID, request.Execution.Execution.FactorySessionID)
	session, err := r.Get(t.Context(), workersessions.GetRequest{ID: request.ID, FactorySessionID: request.Execution.Execution.FactorySessionID})
	if err != nil || !reflect.DeepEqual(session.Metadata, want) {
		t.Fatalf("runtime child metadata = %+v, %v; want %+v", session.Metadata, err, want)
	}
	token := assertIdentityTokenEnvironment(t, environment, (workersessions.Session{ID: request.ID, Metadata: want}).IdentityEnvironment())
	encoded, err := json.Marshal(request)
	if err != nil || bytes.Contains(encoded, []byte(caller.Token)) {
		t.Fatal("runtime caller authority entered serialized request")
	}
	metadata.Correlation.WorkID = "mutated"
	metadata.Labels[0] = "mutated"
	if !reflect.DeepEqual(r.sessions[key].Metadata, want) || r.sessions["source"].Metadata.Correlation.WorkID != "lane" {
		t.Fatal("runtime admission changed or aliased caller/child facts")
	}
	if attempt != nil {
		result := runtimeAttemptCompletedDispatch("caller-dispatch")
		result.Result.Output = "ordinary " + token
		if err := attempt.Complete(t.Context(), result, nil); err != nil {
			t.Fatal(err)
		}
	}
	assertTokenPublicationAbsent(t, r, r.sessions[key], token)
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

func assertIdentityTokenEnvironment(t *testing.T, environment, facts []string) string {
	t.Helper()
	if len(environment) != len(facts)+1 {
		t.Fatal("identity handoff did not supply exactly one execution token")
	}
	if !reflect.DeepEqual(environment[:len(facts)], facts) {
		t.Fatal("identity handoff changed admitted facts")
	}
	token := strings.TrimPrefix(environment[len(facts)], "YOU_WORKER_SESSION_TOKEN=")
	entropy, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(entropy) != 32 {
		t.Fatal("execution token must encode 32 bytes without padding")
	}
	return token
}

func TestTokenEntropyFailureDoesNotLaunchExecution(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	r.tokenEntropy = bytes.NewReader(nil)
	calls := 0
	r.execution = coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		calls++
		return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
	}}
	result, err := r.InvokeSession(t.Context(), validStartRequest("entropy-failure", "dispatch"))
	if err != nil || calls != 0 || result.Session.State != workersessions.StateFailed {
		t.Fatalf("entropy failure: state %s, calls %d, error %v", result.Session.State, calls, err)
	}
	if len(r.executionTokens) != 0 || len(r.executionSecrets) != 0 {
		t.Fatal("failed entropy acquired authority or secret state")
	}
}

func TestTokenBindingFreshnessRevocationAndRedaction(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	token := bindTestSessionToken(t, r, "source")
	next := bindTestSessionToken(t, r, "successor")
	if token == next {
		t.Fatal("successor reused source token")
	}
	assertTokenFragmentRedaction(t, r, token)
	assertTokenResultRedaction(t, r, token)
	if _, committed := r.commitControlTerminal("source", workersessions.StateCanceled); !committed {
		t.Fatal("terminal transition rejected")
	}
	if r.executionTokens["source"] != "" || r.executionTokens["successor"] != next {
		t.Fatal("terminal transition did not revoke only its owner")
	}
	if _, err := r.bindExecutionIdentityEnvironment("source", nil); err == nil {
		t.Fatal("terminal session acquired another token")
	}
	safe, err := r.redactExecutionFragment("source", workers.ProgressFragment{Payload: token})
	if err != nil || strings.Contains(safe.Payload, token) {
		t.Fatal("late fragment lost secret classification")
	}
	session, err := r.Get(t.Context(), workersessions.GetRequest{ID: "source"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(session)
	if err != nil || bytes.Contains(encoded, []byte(token)) {
		t.Fatal("session representation published a token")
	}
}

func assertTokenResultRedaction(t *testing.T, r *registry, token string) {
	t.Helper()
	failure := errors.New("provider failed " + token)
	content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: token}}
	result := workers.WorkstationDispatchResult{Result: workers.WorkResult{Output: "ordinary " + token, Error: token, OutputContent: content}, ProposedOutput: &workers.ProposedOutput{Primary: content}}
	redacted, safeErr := r.redactExecutionResult("source", result, failure)
	if redacted.Result.OutputContent[0].Text != "[REDACTED]" || redacted.ProposedOutput.Primary[0].Text != "[REDACTED]" || content[0].Text != token {
		t.Fatal("transient content redaction lost detached output")
	}
	if redacted.Result.Output != "ordinary [REDACTED]" || strings.Contains(safeErr.Error(), token) || !errors.Is(safeErr, failure) {
		t.Fatal("result or diagnostic redaction lost safety or typed cause")
	}
}

func assertTokenPublicationAbsent(t *testing.T, r *registry, session workersessions.Session, token string) {
	t.Helper()
	payload, err := json.Marshal(session)
	if err != nil || bytes.Contains(payload, []byte(token)) {
		t.Fatal("terminal session did not publish sanitized ordinary output")
	}
	sink := r.events.(*internalTestEventsService)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	records := sink.records[r.observationTopic(session.ID)]
	if len(records) < 2 {
		t.Fatal("missing opening and terminal records")
	}
	var observed []byte
	for _, record := range records {
		observed = append(observed, record.Payload...)
		if bytes.Contains(record.Payload, []byte(token)) {
			t.Fatal("execution token escaped into Events")
		}
	}
	if !bytes.Contains(observed, []byte("ordinary [REDACTED]")) {
		t.Fatal("Events lost ordinary sanitized output")
	}
	if r.executionTokens[session.ID] != "" {
		t.Fatal("completed execution retained token authority")
	}
}

func assertTokenFragmentRedaction(t *testing.T, r *registry, token string) {
	t.Helper()
	payload := workers.ProgressFragment{Payload: "echo " + token, Metadata: map[string]string{"failure": token}}
	safe, err := r.redactExecutionFragment("source", payload)
	if err != nil || safe.Payload != "echo [REDACTED]" || safe.Metadata["failure"] != "[REDACTED]" {
		t.Fatal("execution fragment was not sanitized")
	}
	if payload.Metadata["failure"] != token {
		t.Fatal("sanitizer mutated producer metadata")
	}
}

func bindTestSessionToken(t *testing.T, r *registry, id string) string {
	t.Helper()
	if _, err := r.Reserve(t.Context(), workersessions.ReserveRequest{ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.transitionToStarting(id); err != nil {
		t.Fatal(err)
	}
	environment, err := r.bindExecutionIdentityEnvironment(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	return assertIdentityTokenEnvironment(t, environment, []string{"YOU_WORKER_SESSION_ID=" + id})
}

func TestExecutionEndpointBindingsRemainExecutionOnlyAndReleaseIndependently(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	first := r.bindExecutionEndpoint("http://127.0.0.1:7438")
	second := r.bindExecutionEndpoint("http://[::1]:7439")
	t.Cleanup(first)
	t.Cleanup(second)
	assertEndpoint := func(id, want string) {
		t.Helper()
		if _, err := r.Reserve(t.Context(), workersessions.ReserveRequest{ID: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.transitionToStarting(id); err != nil {
			t.Fatal(err)
		}
		environment, err := r.bindExecutionIdentityEnvironment(id, nil)
		if err != nil {
			t.Fatal(err)
		}
		var endpoint string
		for _, entry := range environment {
			if value, found := strings.CutPrefix(entry, "YOU_SERVER="); found {
				endpoint = value
			}
		}
		if endpoint != want {
			t.Fatalf("execution endpoint = %q, want %q", endpoint, want)
		}
		session, err := r.Get(t.Context(), workersessions.GetRequest{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(session)
		if err != nil || strings.Contains(string(payload), "http://") || strings.Contains(string(payload), "YOU_SERVER") {
			t.Fatal("execution endpoint entered retained session")
		}
	}
	assertEndpoint("newest", "http://[::1]:7439")
	second()
	second()
	assertEndpoint("remaining", "http://127.0.0.1:7438")
	first()
	assertEndpoint("unhosted", "")
	third := r.bindExecutionEndpoint("http://127.0.0.1:7440")
	t.Cleanup(third)
	first()
	assertEndpoint("rebound", "http://127.0.0.1:7440")
}
