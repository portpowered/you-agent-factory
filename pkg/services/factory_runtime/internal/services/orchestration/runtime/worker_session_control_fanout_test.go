package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFanOutWorkerSessionControl_AttemptsEveryCapturedChildInStableOrder(t *testing.T) {
	boom := errors.New("worker session control failed")
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeUnsupported},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-c"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed},
			err:    boom,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := fanOutWorkerSessionControl(ctx, service, capturedWorkerSessionControlTargets{
		turnID:           "turn-captured",
		workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
	}, factory.WorkerSessionControlActionPause)

	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
	}
	wantChildren := []factory.WorkerSessionControlChildResult{
		{WorkerSessionID: "worker-a", DispatchID: "dispatch-a", Outcome: factory.WorkerSessionControlChildOutcomeApplied},
		{WorkerSessionID: "worker-b", DispatchID: "dispatch-b", Outcome: factory.WorkerSessionControlChildOutcomeUnsupported},
		{WorkerSessionID: "worker-c", DispatchID: "dispatch-c", Outcome: factory.WorkerSessionControlChildOutcomeFailed},
	}
	if !reflect.DeepEqual(result.Children, wantChildren) {
		t.Fatalf("child results = %#v, want %#v", result.Children, wantChildren)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-c"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
	if service.observedCanceledContext() {
		t.Fatal("Worker Sessions received canceled fan-out context")
	}
}

func TestRecordedWorkerStreamGenerationAndCursorHelpers(t *testing.T) {
	workerSessionID := "worker-generation"
	wantGeneration := "worker-recording/" + workerSessionID
	if got := recordedWorkerStreamGenerationForIdentity(" " + workerSessionID + " "); got != wantGeneration {
		t.Fatalf("recordedWorkerStreamGenerationForIdentity() = %q, want %q", got, wantGeneration)
	}
	if !durableWorkerCursor(&workersessions.ObservationCursor{StreamGenerationID: wantGeneration}) {
		t.Fatal("durableWorkerCursor() = false for durable generation")
	}
	if durableWorkerCursor(&workersessions.ObservationCursor{StreamGenerationID: "events/generation"}) {
		t.Fatal("durableWorkerCursor() = true for live generation")
	}

	wrapped := withRecordedWorkerStreamGeneration(workersessions.ObservationSubscription{
		NextFunc: func(context.Context) workersessions.ObservationDelivery {
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord}
		},
	}, workerSessionID)
	delivery := wrapped.Next(context.Background())
	if delivery.Event.Cursor.StreamGenerationID != wantGeneration {
		t.Fatalf("recorded stream cursor = %q, want %q", delivery.Event.Cursor.StreamGenerationID, wantGeneration)
	}
	if unchanged := withRecordedWorkerStreamGeneration(workersessions.ObservationSubscription{}, workerSessionID); unchanged.NextFunc != nil {
		t.Fatal("empty subscription unexpectedly gained a Next function")
	}
}

func TestRecordedWorkerSessionObservationUsesDurableWorkerIDFallback(t *testing.T) {
	adapter, service, workerSessionID := newDurableWorkerIDFallbackFixture()
	subscription, err := adapter.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID() error = %v", err)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord || got.Event.Cursor.StreamGenerationID != "worker-recording/"+workerSessionID {
		t.Fatalf("durable fallback delivery = %#v, want record with durable generation", got)
	}

	service.observation.ProviderSessionAvailable = false
	service.subscription = workersessions.ObservationSubscription{}
	service.streamErr = nil
	if _, err := adapter.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	}); err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID(provider-neutral fallback) error = %v", err)
	}
	show, err := adapter.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil || show.WorkerSessionID != workerSessionID {
		t.Fatalf("GetObservationByWorkerSessionID(durable fallback) = %#v, %v", show, err)
	}
}

func TestRecordedWorkerSessionObservationDurableReadRoutes(t *testing.T) {
	adapter, service, workerSessionID := newDurableWorkerIDFallbackFixture()
	show, err := adapter.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil || show.WorkerSessionID != workerSessionID {
		t.Fatalf("GetObservationByWorkerSessionID(durable fallback) = %#v, %v", show, err)
	}
	service.getErr = workersessions.ErrObservationSessionNotFound
	if _, err := adapter.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("GetObservationByWorkerSessionID(missing) error = %v", err)
	}
	service.getErr = nil
	service.transcript = workersessions.ReadTranscriptResult{WorkerSessionID: workerSessionID, AttemptID: "attempt", State: workersessions.StateCompleted}
	transcript, err := adapter.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: workerSessionID})
	if err != nil || transcript.WorkerSessionID != workerSessionID {
		t.Fatalf("ReadTranscript(durable fallback) = %#v, %v", transcript, err)
	}
	service.transcriptErr = errors.New("transcript unavailable")
	if _, err := adapter.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: workerSessionID}); !errors.Is(err, service.transcriptErr) {
		t.Fatalf("ReadTranscript(failed durable fallback) error = %v", err)
	}
}

func TestRecordedWorkerSessionObservationDurableHelperBranches(t *testing.T) {
	adapter, _, workerSessionID := newDurableWorkerIDFallbackFixture()
	if _, handled, err := adapter.recordedWorkerObservationIfAvailable(context.Background(), workersessions.Observation{}, workerSessionID); handled || err != nil {
		t.Fatalf("recordedWorkerObservationIfAvailable(no provider) = handled %v, error %v", handled, err)
	}
	if _, handled, err := adapter.recordedTranscriptIfAvailable(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: workerSessionID}, workersessions.ReadTranscriptResult{}); handled || err != nil {
		t.Fatalf("recordedTranscriptIfAvailable(no provider) = handled %v, error %v", handled, err)
	}
}

func TestRecordedTranscriptProviderIdentity(t *testing.T) {
	for _, test := range []struct {
		name string
		ref  workersessions.ReadTranscriptResult
		want bool
	}{
		{name: "provider", ref: workersessions.ReadTranscriptResult{ProviderSession: providers.SessionRef{Provider: "codex"}}, want: true},
		{name: "kind", ref: workersessions.ReadTranscriptResult{ProviderSession: providers.SessionRef{Kind: "session_id"}}, want: true},
		{name: "id", ref: workersessions.ReadTranscriptResult{ProviderSession: providers.SessionRef{ID: "opaque"}}, want: true},
		{name: "empty", ref: workersessions.ReadTranscriptResult{}, want: false},
	} {
		t.Run("transcript identity "+test.name, func(t *testing.T) {
			if got := transcriptProviderSessionAvailable(test.ref); got != test.want {
				t.Fatalf("transcriptProviderSessionAvailable() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRecordedObservationReplaySummaryHelpers(t *testing.T) {
	workerSessionID := "worker-durable-fallback"
	if got := recordedObservationGeneration(nil, workerSessionID, false); got != "" {
		t.Fatalf("recordedObservationGeneration(no ledger) = %q, want empty", got)
	}
	if got := recordedObservationReplaySummary(recordedDispatchObservation{state: workersessions.StateRunning}, workerRecordingHealth{}); got == nil || got.Complete || got.Reason != "recording-incomplete" {
		t.Fatalf("recordedObservationReplaySummary(active) = %#v", got)
	}
	if got := recordedObservationReplaySummary(recordedDispatchObservation{state: workersessions.StateCompleted}, workerRecordingHealth{}); got == nil || !got.Complete || got.Reason != "recording-complete" {
		t.Fatalf("recordedObservationReplaySummary(completed) = %#v", got)
	}
	if got := recordedObservationReplaySummary(recordedDispatchObservation{state: workersessions.StateCompleted}, workerRecordingHealth{status: recordings.WorkerRecordingStatusDegraded, reason: "capture lost"}); got == nil || got.Complete || got.Reason != "capture lost" {
		t.Fatalf("recordedObservationReplaySummary(health) = %#v", got)
	}
	if got := observationStreamLimit(0); got != workersessions.DefaultObservationStreamLimit || observationStreamLimit(3) != 3 {
		t.Fatalf("observationStreamLimit() = %d/%d", got, observationStreamLimit(3))
	}
}

func TestRecordedWorkerSessionObservationDurableStreamErrors(t *testing.T) {
	adapter, service, workerSessionID := newDurableWorkerIDFallbackFixture()

	var nilAdapter *recordedWorkerSessionObservation
	if _, handled, err := nilAdapter.streamDurableWorkerSession(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: workerSessionID}); handled || err != nil {
		t.Fatalf("nil streamDurableWorkerSession() = handled %v, error %v, want false/nil", handled, err)
	}
	service.getErr = workersessions.ErrObservationSessionNotFound
	service.streamErr = workersessions.ErrObservationSessionNotFound
	if _, handled, err := adapter.streamDurableWorkerSession(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: workerSessionID}); handled || err != nil {
		t.Fatalf("missing durable stream = handled %v, error %v, want false/nil", handled, err)
	}
	service.streamErr = errors.New("stream unavailable")
	if _, handled, err := adapter.streamDurableWorkerSession(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: workerSessionID}); !handled || !errors.Is(err, service.streamErr) {
		t.Fatalf("failed durable stream = handled %v, error %v, want true/stream error", handled, err)
	}
}

func newDurableWorkerIDFallbackFixture() (*recordedWorkerSessionObservation, *durableWorkerSessionsService, string) {
	const workerSessionID = "worker-durable-fallback"
	deliveries := []workersessions.ObservationDelivery{{Kind: workersessions.ObservationDeliveryRecord}}
	service := &durableWorkerSessionsService{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{},
		observation: workersessions.Observation{
			WorkerSessionID:          workerSessionID,
			State:                    workersessions.StateCompleted,
			ProviderSessionAvailable: true,
		},
		subscription: workersessions.ObservationSubscription{
			NextFunc: func(context.Context) workersessions.ObservationDelivery {
				value := deliveries[0]
				deliveries = deliveries[1:]
				return value
			},
		},
	}
	return &recordedWorkerSessionObservation{Service: service}, service, workerSessionID
}

type durableWorkerSessionsService struct {
	*fakeWorkerSessionsService
	observation   workersessions.Observation
	subscription  workersessions.ObservationSubscription
	getErr        error
	streamErr     error
	transcript    workersessions.ReadTranscriptResult
	transcriptErr error
}

func (s *durableWorkerSessionsService) GetObservationByWorkerSessionID(
	context.Context,
	workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	return s.observation, s.getErr
}

func (s *durableWorkerSessionsService) StreamObservationsByWorkerSessionID(
	context.Context,
	workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, error) {
	return s.subscription, s.streamErr
}

func (s *durableWorkerSessionsService) ReadTranscript(
	context.Context,
	workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	return s.transcript, s.transcriptErr
}

func (*durableWorkerSessionsService) ListWorkerRecordingProjections(
	context.Context,
	recordings.WorkerRecordingListRequest,
) (recordings.WorkerRecordingListResult, error) {
	return recordings.WorkerRecordingListResult{}, nil
}

func (*durableWorkerSessionsService) LoadWorkerRecordingByWorkerSessionID(
	context.Context,
	string,
) (recordings.WorkerRecordingSnapshot, error) {
	return recordings.WorkerRecordingSnapshot{}, nil
}

var _ recordings.WorkerRecordingHistoryReader = (*durableWorkerSessionsService)(nil)

func TestFanOutWorkerSessionControl_PropagatesParentControlID(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"}: {result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeApplied}},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"}: {result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeNoop}},
	})

	result := fanOutWorkerSessionControl(context.Background(), service, capturedWorkerSessionControlTargets{
		turnID: "turn-parent", workerSessionIDs: []string{"worker-a", "worker-b"},
	}, factory.WorkerSessionControlActionPause, "control-parent")
	if result.Outcome != factory.WorkerSessionControlAggregateOutcomePartial {
		t.Fatalf("aggregate outcome = %q, want PARTIAL", result.Outcome)
	}
	if got, want := service.requestIDsSnapshot(), []string{"control-parent", "control-parent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("child request IDs = %v, want %v", got, want)
	}
}

func TestFanOutWorkerSessionControl_ClassifiesFullNoOpUnsupportedAndPartialOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []workersessions.ControlOutcome
		want     factory.WorkerSessionControlAggregateOutcome
	}{
		{name: "all applied", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeApplied}, want: factory.WorkerSessionControlAggregateOutcomeApplied},
		{name: "all terminal no-op", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeNoop, workersessions.ControlOutcomeNoop}, want: factory.WorkerSessionControlAggregateOutcomeNoOp},
		{name: "all unsupported", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeUnsupported, workersessions.ControlOutcomeUnsupported}, want: factory.WorkerSessionControlAggregateOutcomeUnsupported},
		{name: "mixed outcomes", outcomes: []workersessions.ControlOutcome{workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeNoop, workersessions.ControlOutcomeUnsupported}, want: factory.WorkerSessionControlAggregateOutcomePartial},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := make(map[workerSessionControlCall]workerSessionControlResponse, len(test.outcomes))
			targets := make([]string, 0, len(test.outcomes))
			for index, outcome := range test.outcomes {
				id := string(rune('a' + index))
				targets = append(targets, id)
				responses[workerSessionControlCall{action: factory.WorkerSessionControlActionResume, id: id}] = workerSessionControlResponse{
					result: workersessions.ControlResult{Outcome: outcome},
				}
			}

			result := fanOutWorkerSessionControl(context.Background(), newWorkerSessionControlSpy(responses), capturedWorkerSessionControlTargets{
				turnID: "turn-resume", workerSessionIDs: targets,
			}, factory.WorkerSessionControlActionResume)
			if result.Outcome != test.want {
				t.Fatalf("aggregate outcome = %q, want %q", result.Outcome, test.want)
			}
		})
	}
}

func TestFanOutWorkerSessionControl_MultipleFailuresStillReachEveryChild(t *testing.T) {
	firstFailure := errors.New("first worker failure")
	secondFailure := errors.New("second worker failure")
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeFailed}, err: firstFailure,
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-c"}: {
			result: workersessions.ControlResult{Outcome: workersessions.ControlOutcomeFailed}, err: secondFailure,
		},
	})

	result := fanOutWorkerSessionControl(context.Background(), service, capturedWorkerSessionControlTargets{
		turnID:           "turn-failures",
		workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
	}, factory.WorkerSessionControlActionResume)

	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-c"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestFactoryControls_FanOutCapturedTurnOnceAndRetainOriginalEvidence(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeUnsupported},
		},
		{action: factory.WorkerSessionControlActionResume, id: "worker-late"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-late", Outcome: workersessions.ControlOutcomeNoop},
		},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-duplicate", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 40, "association-other-turn", "turn-replacement", "worker-replacement"),
	)
	control := factoryInstance.(factory.Service)

	paused, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if paused.Outcome != factory.ControlOutcomeAccepted || paused.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomePartial {
		t.Fatalf("pause result = %#v, want accepted partial fan-out", paused)
	}
	if got, want := workerSessionIDsFromResults(paused.WorkerSessionControl.Children), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paused worker session IDs = %v, want %v", got, want)
	}

	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 50, "association-late", "turn-captured", "worker-late"))
	repeatedPause, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("repeated ControlPause: %v", err)
	}
	if repeatedPause.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeatedPause.WorkerSessionControl, paused.WorkerSessionControl) {
		t.Fatalf("repeated pause = %#v, want retained no-op result %#v", repeatedPause, paused.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 2 {
		t.Fatalf("calls after repeated pause = %#v, want exactly the first two child calls", got)
	}

	resumed, err := control.ControlResume(context.Background(), factory.ResumeRequest{TurnID: "turn-captured", ControlID: "resume-intent"})
	if err != nil {
		t.Fatalf("ControlResume: %v", err)
	}
	if resumed.Outcome != factory.ControlOutcomeAccepted || resumed.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomePartial {
		t.Fatalf("resume result = %#v, want accepted partial fan-out", resumed)
	}
	if got, want := workerSessionIDsFromResults(resumed.WorkerSessionControl.Children), []string{"worker-a", "worker-b", "worker-late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resumed worker session IDs = %v, want %v", got, want)
	}

	repeatedResume, err := control.ControlResume(context.Background(), factory.ResumeRequest{TurnID: "turn-captured", ControlID: "resume-intent"})
	if err != nil {
		t.Fatalf("repeated ControlResume: %v", err)
	}
	if repeatedResume.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeatedResume.WorkerSessionControl, resumed.WorkerSessionControl) {
		t.Fatalf("repeated resume = %#v, want retained no-op result %#v", repeatedResume, resumed.WorkerSessionControl)
	}
	// A committed control may be retried after a later lifecycle transition.
	// Its ControlID must retain the old captured target set rather than
	// including worker-late, which joined this Factory turn after pause first
	// linearized.
	retriedAfterResume, err := control.ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-captured", ControlID: "pause-intent"})
	if err != nil {
		t.Fatalf("ControlPause retry after resume: %v", err)
	}
	if retriedAfterResume.Outcome != factory.ControlOutcomeAccepted || !reflect.DeepEqual(retriedAfterResume.WorkerSessionControl, paused.WorkerSessionControl) {
		t.Fatalf("pause retry after resume = %#v, want accepted retained evidence %#v", retriedAfterResume, paused.WorkerSessionControl)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{
		{action: factory.WorkerSessionControlActionPause, id: "worker-a"},
		{action: factory.WorkerSessionControlActionPause, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-a"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-b"},
		{action: factory.WorkerSessionControlActionResume, id: "worker-late"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
}

func TestFactoryControls_EmptyCapturedTurnDoesNotCallWorkerSessions(t *testing.T) {
	service := newWorkerSessionControlSpy(nil)
	factoryInstance, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	paused, err := factoryInstance.(factory.Service).ControlPause(context.Background(), factory.PauseRequest{TurnID: "turn-without-children"})
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if paused.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomeNoOp || len(paused.WorkerSessionControl.Children) != 0 {
		t.Fatalf("empty fan-out result = %#v, want no-op with no children", paused.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 0 {
		t.Fatalf("Worker Sessions calls = %#v, want none", got)
	}
}

func TestFactoryControls_ReplayHistoryWinsOverLiveLedgerForCapturedTurn(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionPause, id: "worker-replayed"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-replayed", Outcome: workersessions.ControlOutcomeApplied},
		},
		{action: factory.WorkerSessionControlActionPause, id: "worker-live"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-live", Outcome: workersessions.ControlOutcomeApplied},
		},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 2, "live-association", "turn-replay", "worker-live"),
	)
	impl, ok := factoryInstance.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", factoryInstance)
	}
	impl.SetReplayEvents([]interfaces.FactoryEvent{
		workerSessionAssociationEvent(t, 4, "replay-association", "turn-replay", "worker-replayed"),
	})

	paused, err := factoryInstance.(factory.Service).ControlPause(context.Background(), factory.PauseRequest{
		TurnID: "turn-replay", ControlID: "pause-from-replay",
	})
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if got, want := workerSessionIDsFromResults(paused.WorkerSessionControl.Children), []string{"worker-replayed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("replayed Worker Session IDs = %v, want %v", got, want)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{{action: factory.WorkerSessionControlActionPause, id: "worker-replayed"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
}

func TestFanOutWorkerSessionControl_StopActionsReachEveryCapturedChild(t *testing.T) {
	for _, action := range []factory.WorkerSessionControlAction{
		factory.WorkerSessionControlActionCancel,
		factory.WorkerSessionControlActionTerminate,
	} {
		t.Run(string(action), func(t *testing.T) {
			boundaryFailure := errors.New("worker session boundary unavailable")
			service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
				{action: action, id: "worker-a"}: {result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied}},
				{action: action, id: "worker-b"}: {result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop}},
				{action: action, id: "worker-c"}: {result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed}, err: boundaryFailure},
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			result := fanOutWorkerSessionControl(ctx, service, capturedWorkerSessionControlTargets{
				turnID: "turn-stop", workerSessionIDs: []string{"worker-a", "worker-b", "worker-c"},
			}, action)

			if result.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
				t.Fatalf("aggregate outcome = %q, want FAILED", result.Outcome)
			}
			if got, want := service.callsSnapshot(), []workerSessionControlCall{
				{action: action, id: "worker-a"}, {action: action, id: "worker-b"}, {action: action, id: "worker-c"},
			}; !reflect.DeepEqual(got, want) {
				t.Fatalf("control calls = %#v, want %#v", got, want)
			}
			if service.observedCanceledContext() {
				t.Fatal("Worker Sessions received canceled stop fan-out context")
			}
		})
	}
}

func TestFanOutWorkerSessionControl_ContinuesPastSynchronousProductionChild(t *testing.T) {
	execution := newSynchronousFanOutExecution("dispatch-a", "dispatch-b")
	starts := make(chan workers.ExecuteResult, 2)
	startErrs := make(chan error, 2)
	for _, identity := range []struct{ dispatchID string }{
		{dispatchID: "dispatch-a"},
		{dispatchID: "dispatch-b"},
	} {
		go func(dispatchID string) {
			started, startErr := execution.Execute(context.Background(), workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{DispatchID: dispatchID, AttemptID: dispatchID},
				Target:      workers.ExecutionTarget{WorkstationName: "review", WorkerType: "mock"},
				Input: workers.ExecutionInput{Dispatch: work.WorkDispatch{
					DispatchID: dispatchID, WorkstationName: "review",
				}},
			})
			starts <- started
			startErrs <- startErr
		}(identity.dispatchID)
	}
	<-execution.admitted("dispatch-a")
	<-execution.admitted("dispatch-b")

	workerSessions := &synchronousFanOutWorkerSessions{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{},
		execution:                 execution,
		dispatchIDs: map[string]string{
			"worker-a": "dispatch-a",
			"worker-b": "dispatch-b",
		},
	}
	result := fanOutWorkerSessionControl(context.Background(), workerSessions, capturedWorkerSessionControlTargets{
		turnID: "turn-synchronous", workerSessionIDs: []string{"worker-a", "worker-b"},
	}, factory.WorkerSessionControlActionTerminate)
	if result.Outcome != factory.WorkerSessionControlAggregateOutcomeApplied {
		t.Fatalf("fan-out outcome = %q, want APPLIED", result.Outcome)
	}
	if got, want := workerSessionIDsFromResults(result.Children), []string{"worker-a", "worker-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fan-out children = %v, want %v", got, want)
	}
	cancellations := make(map[string]struct{}, 2)
	for range 2 {
		cancellation := <-execution.cancelCalls
		if _, duplicate := cancellations[cancellation.DispatchID]; duplicate {
			t.Fatalf("duplicate Workers context cancellation for dispatch %q", cancellation.DispatchID)
		}
		cancellations[cancellation.DispatchID] = struct{}{}
	}
	for _, wantDispatchID := range []string{"dispatch-a", "dispatch-b"} {
		if _, ok := cancellations[wantDispatchID]; !ok {
			t.Fatalf("Workers context cancellations = %#v, want dispatch %q", cancellations, wantDispatchID)
		}
	}
	for range 2 {
		started := <-starts
		if startErr := <-startErrs; !errors.Is(startErr, workers.ErrWorkstationDispatchCanceled) {
			t.Fatalf("synchronous dispatch error = %v, want cancellation", startErr)
		}
		if started.Outcome != workers.ExecutionOutcomeCanceled {
			t.Fatalf("synchronous execution result = %#v, want canceled result", started)
		}
	}
}

func TestFactoryTerminate_FansOutCapturedCancelAndRetainsOriginalEvidence(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionCancel, id: "worker-a"}: {result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied}},
		{action: factory.WorkerSessionControlActionCancel, id: "worker-b"}: {result: workersessions.ControlResult{DispatchID: "dispatch-b", Outcome: workersessions.ControlOutcomeNoop}},
		{action: factory.WorkerSessionControlActionCancel, id: "worker-c"}: {result: workersessions.ControlResult{DispatchID: "dispatch-c", Outcome: workersessions.ControlOutcomeFailed}, err: errors.New("boundary unavailable")},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events,
		workerSessionAssociationEvent(t, 20, "association-b", "turn-captured", "worker-b"),
		workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"),
		workerSessionAssociationEvent(t, 30, "association-c", "turn-captured", "worker-c"),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	control := factoryInstance.(factory.Service)
	terminated, err := control.ControlTerminate(ctx, factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "cancel-intent", WorkerSessionAction: factory.WorkerSessionControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlTerminate: %v", err)
	}
	if terminated.Outcome != factory.ControlOutcomeAccepted || terminated.WorkerSessionControl.Outcome != factory.WorkerSessionControlAggregateOutcomeFailed {
		t.Fatalf("terminate result = %#v, want accepted failed fan-out evidence", terminated)
	}
	if got, want := workerSessionIDsFromResults(terminated.WorkerSessionControl.Children), []string{"worker-a", "worker-b", "worker-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("controlled Worker Session IDs = %v, want %v", got, want)
	}

	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 40, "association-late", "turn-captured", "worker-late"))
	repeated, err := control.ControlTerminate(context.Background(), factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "cancel-intent", WorkerSessionAction: factory.WorkerSessionControlActionCancel,
	})
	if err != nil {
		t.Fatalf("repeated ControlTerminate: %v", err)
	}
	if repeated.Outcome != factory.ControlOutcomeNoOp || !reflect.DeepEqual(repeated.WorkerSessionControl, terminated.WorkerSessionControl) {
		t.Fatalf("repeated terminate = %#v, want retained no-op evidence %#v", repeated, terminated.WorkerSessionControl)
	}
	if got := service.callsSnapshot(); len(got) != 3 {
		t.Fatalf("Worker Sessions calls after retry = %#v, want original three calls", got)
	}
}

func TestFactoryTerminate_DefaultsToWorkerSessionTerminate(t *testing.T) {
	service := newWorkerSessionControlSpy(map[workerSessionControlCall]workerSessionControlResponse{
		{action: factory.WorkerSessionControlActionTerminate, id: "worker-a"}: {
			result: workersessions.ControlResult{DispatchID: "dispatch-a", Outcome: workersessions.ControlOutcomeApplied},
		},
	})
	factoryInstance, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()), withInlineDispatch(), withWorkerSessions(service),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ledger.Events = append(ledger.Events, workerSessionAssociationEvent(t, 10, "association-a", "turn-captured", "worker-a"))

	terminated, err := factoryInstance.(factory.Service).ControlTerminate(context.Background(), factory.TerminateRequest{
		TurnID: "turn-captured", ControlID: "terminate-intent",
	})
	if err != nil {
		t.Fatalf("ControlTerminate: %v", err)
	}
	if terminated.WorkerSessionControl.Action != factory.WorkerSessionControlActionTerminate {
		t.Fatalf("Worker Sessions action = %q, want TERMINATE", terminated.WorkerSessionControl.Action)
	}
	if got, want := service.callsSnapshot(), []workerSessionControlCall{{action: factory.WorkerSessionControlActionTerminate, id: "worker-a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Worker Sessions calls = %#v, want %#v", got, want)
	}
}

func TestTerminateWorkerSessionControlAction_RejectsNonStopActions(t *testing.T) {
	_, err := terminateWorkerSessionControlAction(factory.WorkerSessionControlActionPause)
	if !errors.Is(err, factory.ErrInvalidLifecycleTransition) {
		t.Fatalf("pause action error = %v, want invalid lifecycle transition", err)
	}
}

func workerSessionIDsFromResults(results []factory.WorkerSessionControlChildResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.WorkerSessionID)
	}
	return ids
}

type workerSessionControlCall struct {
	action factory.WorkerSessionControlAction
	id     string
}

type workerSessionControlResponse struct {
	result workersessions.ControlResult
	err    error
}

type workerSessionControlSpy struct {
	*fakeWorkerSessionsService
	mu              sync.Mutex
	responses       map[workerSessionControlCall]workerSessionControlResponse
	calls           []workerSessionControlCall
	requestIDs      []string
	canceledContext bool
}

func newWorkerSessionControlSpy(responses map[workerSessionControlCall]workerSessionControlResponse) *workerSessionControlSpy {
	return &workerSessionControlSpy{
		fakeWorkerSessionsService: &fakeWorkerSessionsService{},
		responses:                 responses,
	}
}

func (s *workerSessionControlSpy) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionPause, req)
}

func (s *workerSessionControlSpy) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionResume, req)
}

func (s *workerSessionControlSpy) Cancel(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionCancel, req)
}

func (s *workerSessionControlSpy) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.control(ctx, factory.WorkerSessionControlActionTerminate, req)
}

func (s *workerSessionControlSpy) control(ctx context.Context, action factory.WorkerSessionControlAction, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := workerSessionControlCall{action: action, id: req.ID}
	s.calls = append(s.calls, call)
	s.requestIDs = append(s.requestIDs, req.RequestID)
	if ctx.Err() != nil {
		s.canceledContext = true
	}
	response := s.responses[call]
	return response.result, response.err
}

func (s *workerSessionControlSpy) callsSnapshot() []workerSessionControlCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workerSessionControlCall(nil), s.calls...)
}

func (s *workerSessionControlSpy) requestIDsSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requestIDs...)
}

func (s *workerSessionControlSpy) observedCanceledContext() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceledContext
}

var _ workersessions.Service = (*workerSessionControlSpy)(nil)

// synchronousFanOutWorkerSessions is an owner-local Worker Sessions seam for
// this runtime test. It preserves the production effect under test—each
// accepted child must cancel its exact Workers dispatch—without constructing
// sibling services or their wire packages in a runtime unit test.
type synchronousFanOutWorkerSessions struct {
	*fakeWorkerSessionsService
	execution   *synchronousFanOutExecution
	dispatchIDs map[string]string
}

func (s *synchronousFanOutWorkerSessions) Terminate(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	dispatchID := s.dispatchIDs[req.ID]
	err := s.execution.cancel(ctx, dispatchID)
	result := workersessions.ControlResult{
		Action:     workersessions.ControlActionTerminate,
		DispatchID: dispatchID,
	}
	if err != nil {
		result.Outcome = workersessions.ControlOutcomeFailed
		return result, err
	}
	result.Outcome = workersessions.ControlOutcomeApplied
	return result, nil
}

var _ workersessions.Service = (*synchronousFanOutWorkerSessions)(nil)

// synchronousFanOutExecution models two request-scoped Workers executions.
// Each execution can finish only by exact context cancellation, so the test
// observes the control path without sleeps or an artificial completion race.
type synchronousFanOutExecution struct {
	workers.ModelInvoker
	mu                     sync.Mutex
	dispatches             map[string]*synchronousFanOutDispatch
	cancelCalls            chan workers.WorkstationDispatchCancelRequest
	canceledControlContext bool
}

type synchronousFanOutDispatch struct {
	admitted     chan struct{}
	admittedOnce sync.Once
	cancel       context.CancelFunc
}

var _ workers.Service = (*synchronousFanOutExecution)(nil)

func newSynchronousFanOutExecution(dispatchIDs ...string) *synchronousFanOutExecution {
	execution := &synchronousFanOutExecution{
		dispatches:  make(map[string]*synchronousFanOutDispatch),
		cancelCalls: make(chan workers.WorkstationDispatchCancelRequest, len(dispatchIDs)),
	}
	for _, dispatchID := range dispatchIDs {
		execution.dispatches[dispatchID] = &synchronousFanOutDispatch{admitted: make(chan struct{})}
	}
	return execution
}

func (e *synchronousFanOutExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	dispatchID := request.Correlation.DispatchID
	e.mu.Lock()
	dispatch := e.dispatches[dispatchID]
	e.mu.Unlock()
	if dispatch == nil {
		return workers.ExecuteResult{}, workers.ErrUnknownWorkstationDispatch
	}
	attemptContext, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	dispatch.cancel = cancel
	e.mu.Unlock()
	dispatch.admittedOnce.Do(func() { close(dispatch.admitted) })
	<-attemptContext.Done()
	e.cancelCalls <- workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID}
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, workers.ErrWorkstationDispatchCanceled
}

func (e *synchronousFanOutExecution) cancel(ctx context.Context, dispatchID string) error {
	e.mu.Lock()
	dispatch := e.dispatches[dispatchID]
	if ctx.Err() != nil {
		e.canceledControlContext = true
	}
	e.mu.Unlock()
	if dispatch == nil {
		return workers.ErrUnknownWorkstationDispatch
	}
	e.mu.Lock()
	cancel := dispatch.cancel
	e.mu.Unlock()
	if cancel == nil {
		return workers.ErrUnknownWorkstationDispatch
	}
	cancel()
	return nil
}

func (e *synchronousFanOutExecution) admitted(dispatchID string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dispatches[dispatchID].admitted
}

func (e *synchronousFanOutExecution) observedCanceledControlContext() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.canceledControlContext
}
