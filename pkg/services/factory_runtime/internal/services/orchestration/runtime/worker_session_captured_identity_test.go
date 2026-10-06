package runtime

import (
	"context"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type forbiddenCapturedProviderProjection struct{ providersessions.Service }

func (forbiddenCapturedProviderProjection) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	panic("canonical Worker ID read consulted provider files")
}

func TestCapturedFactoryIdentityUsesOnlyCommittedUsage(t *testing.T) {
	t.Parallel()
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	service.providerSessions = forbiddenCapturedProviderProjection{}
	service.recordingID = "captured-factory"
	reader := &scriptedWorkerRecordingReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: service.recordingID,
		Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "sibling", Status: recordings.WorkerRecordingStatusComplete,
				Records: []events.Record{{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"sibling"}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":999}}`)}}},
			{WorkerSessionID: fixture.workerSessionID, Status: recordings.WorkerRecordingStatusComplete,
				Records: []events.Record{{Payload: []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker-recorded-exact"}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"outputTokens":4,"cachedInputTokens":2,"reasoningOutputTokens":1}}`)},
					{Payload: []byte(`{"kind":"USAGE","phase":"STARTED","payload":{"inputTokens":999}}`)}}},
		},
	}}
	service.recordingReader = reader
	req := workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID}
	got, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkerSessionID != fixture.workerSessionID || got.State != workersessions.StateCompleted || !got.ProviderSessionAvailable || got.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		t.Fatalf("captured identity lost lifecycle/association or invented transcript: %+v", got)
	}
	usage := got.TokenUsage
	assertCapturedFactoryUsage(t, usage)
	*usage.OutputTokens = 999
	again, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || *again.TokenUsage.OutputTokens != 4 {
		t.Fatalf("captured usage aliases previous read: %+v %v", again, err)
	}
	reader.snapshot.Sessions = nil
	unknown, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || unknown.TokenUsage != nil {
		t.Fatalf("missing capture invented usage: %+v %v", unknown, err)
	}
	reader.err = recordings.ErrWorkerRecordingIncomplete
	prefix, err := service.GetObservationByWorkerSessionID(t.Context(), req)
	if err != nil || prefix.WorkerSessionID != fixture.workerSessionID || prefix.TokenUsage != nil {
		t.Fatalf("incomplete capture hid Factory identity: %+v %v", prefix, err)
	}
}

func assertCapturedFactoryUsage(t *testing.T, usage *workersessions.TokenUsage) {
	t.Helper()
	if usage == nil {
		t.Fatal("captured usage is absent")
	}
	for _, field := range []struct {
		name string
		got  *int
		want int
	}{
		{"input", usage.InputTokens, 0},
		{"output", usage.OutputTokens, 4},
		{"cached input", usage.CachedInputTokens, 2},
		{"reasoning output", usage.ReasoningOutputTokens, 1},
	} {
		if field.got == nil || *field.got != field.want {
			t.Fatalf("captured %s usage = %v, want %d", field.name, field.got, field.want)
		}
	}
	if usage.TotalTokens != nil {
		t.Fatal("capture invented an absent total")
	}
}

// The fake controls only the durable Worker Sessions boundary, never providers.
type capturedForceService struct {
	workersessions.Service
	observation workersessions.Observation
	err         error
	request     workersessions.GetObservationByWorkerSessionIDRequest
}

func (s *capturedForceService) GetCapturedObservation(_ context.Context, request workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	s.request = request
	return s.observation, s.err
}
func (*capturedForceService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

type forceReplayIdentityPlanner struct{ fixedCompletionDeliveryPlanner }

func (forceReplayIdentityPlanner) WorkerSessionIDForDispatch(dispatch work.WorkDispatch) (string, bool) {
	return "worker-recorded-exact", dispatch.DispatchID == "recorded-dispatch"
}
func TestReplayForceDispositionUsesExactCommittedCapture(t *testing.T) {
	t.Parallel()
	cause := "OPERATOR_KILL"
	readErr := errors.New("capture unavailable")
	for _, tc := range []struct {
		name    string
		state   workersessions.State
		cause   *string
		factory string
		err     error
		forced  bool
		fails   bool
	}{
		{"committed kill", workersessions.StateTerminated, &cause, "factory", nil, true, false},
		{"uncommitted termination", workersessions.StateTerminated, nil, "factory", nil, false, false},
		{"graceful cancellation", workersessions.StateCanceled, nil, "factory", nil, false, false},
		{"natural completion", workersessions.StateCompleted, &cause, "factory", nil, false, false},
		{"foreign capture", workersessions.StateTerminated, &cause, "foreign", nil, false, true},
		{"missing legacy capture", "", nil, "", workersessions.ErrObservationSessionNotFound, false, false},
		{"unreadable capture", "", nil, "", readErr, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &capturedForceService{observation: workersessions.Observation{WorkerSessionID: "worker-recorded-exact", FactorySessionID: tc.factory, State: tc.state, TerminalCause: tc.cause}, err: tc.err}
			cfg := &runtimeConfig{workerSessions: sessions, publicSessionID: "factory", completionDeliveryPlanner: forceReplayIdentityPlanner{}, attempts: &attemptLifecycle{forced: map[string]bool{}}}
			err := cfg.recoverReplayForce(t.Context(), work.WorkDispatch{DispatchID: "recorded-dispatch"})
			if (err != nil) != tc.fails || cfg.attempts.wasForced("recorded-dispatch") != tc.forced || cfg.attempts.wasForced("worker-recorded-exact") {
				t.Fatalf("force recovery: err=%v forced=%v", err, cfg.attempts.wasForced("recorded-dispatch"))
			}
			if sessions.request.FactorySessionID != "factory" || sessions.request.WorkerSessionID != "worker-recorded-exact" {
				t.Fatalf("capture read escaped exact scope: %+v", sessions.request)
			}
			sessions.request = workersessions.GetObservationByWorkerSessionIDRequest{}
			if err := cfg.recoverReplayForce(t.Context(), work.WorkDispatch{DispatchID: "unrelated"}); err != nil || sessions.request.WorkerSessionID != "" {
				t.Fatal("unrecorded dispatch consulted capture")
			}
		})
	}
}
func TestRecordedForceObservationRetainsCapturedTerminalTruth(t *testing.T) {
	t.Parallel()
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	service.factorySessionID = "factory"
	service.projector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{DispatchID: "dispatch-recorded-exact", WorkItemIDs: []string{fixture.workID}, Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeCanceled), Cancellation: &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}}}}}, nil
	}
	cause := "OPERATOR_KILL"
	service.Service = &capturedForceService{observation: workersessions.Observation{WorkerSessionID: fixture.workerSessionID, FactorySessionID: "factory", AttemptID: "physical", State: workersessions.StateTerminated, TerminalCause: &cause}}
	got, err := service.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID, FactorySessionID: "factory"})
	if err != nil || got.State != workersessions.StateTerminated || got.TerminalCause == nil || *got.TerminalCause != cause || got.AttemptID != "physical" {
		t.Fatalf("recorded force truth = %+v, %v", got, err)
	}
	listed, err := service.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: fixture.workID})
	if err != nil || len(listed.Observations) != 1 || listed.Observations[0].State != workersessions.StateTerminated || listed.Observations[0].TerminalCause == nil || *listed.Observations[0].TerminalCause != cause {
		t.Fatalf("recorded force list = %+v, %v", listed, err)
	}
	*got.TerminalCause = "changed"
	again, err := service.GetObservationByWorkerSessionID(t.Context(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID, FactorySessionID: "factory"})
	if err != nil || again.TerminalCause == nil || *again.TerminalCause != cause {
		t.Fatal("returned capture aliases durable facts")
	}
}

func (*capturedForceService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
}
