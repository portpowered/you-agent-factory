package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type selectedWorkFactsLedger struct {
	recordings.RuntimeLedger
	facts  recordings.WorkerSessionWorkFacts
	err    error
	workID string
	reads  int
}

func (ledger *selectedWorkFactsLedger) CurrentWorkerSessionWorkFacts(_ context.Context, workID string) (recordings.WorkerSessionWorkFacts, error) {
	ledger.workID = workID
	ledger.reads++
	return ledger.facts, ledger.err
}

func (*selectedWorkFactsLedger) CanonicalEvents() []interfaces.FactoryEvent {
	panic("scoped list read canonical history")
}

func (ledger *selectedWorkFactsLedger) StreamGenerationID() string {
	return ledger.facts.StreamGenerationID
}

type scopedWorkHealthReader struct {
	recordings.WorkerRecordingReader
	reads  int
	onRead func()
	err    error
}

type scopedWorkLiveOwner struct {
	processLocalWorkerSessionService
}

func (owner *scopedWorkLiveOwner) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	result := workersessions.ListObservationsResult{Observations: make([]workersessions.Observation, len(owner.observationListResult.Observations))}
	for index, row := range owner.observationListResult.Observations {
		result.Observations[index] = row.Clone()
	}
	return result, nil
}

func (*scopedWorkHealthReader) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	panic("scoped list copied whole Worker recording")
}

func (reader *scopedWorkHealthReader) CurrentWorkerRecordingHealth(_ context.Context, _ string, ids []string) (recordings.WorkerRecordingSnapshot, error) {
	if len(ids) != 1 || ids[0] != "worker" {
		panic("scoped list selected unrelated or duplicate capture health")
	}
	reader.reads++
	if reader.onRead != nil {
		reader.onRead()
	}
	return recordings.WorkerRecordingSnapshot{RecordingID: "owned-recording"}, reader.err
}

func TestScopedWorkListSharesSelectedFactsAndHealthAcrossConfirmation(t *testing.T) {
	t.Parallel()
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	const generation = "owned-generation"
	ledger := &selectedWorkFactsLedger{RuntimeLedger: service.ledger, facts: recordings.WorkerSessionWorkFacts{
		KnownWork: true, StreamGenerationID: generation,
		Associations:    map[string]recordings.WorkerSessionAssociationFacts{"attempt": {WorkerSessionID: "worker"}},
		Requests:        map[string]interfaces.FactoryWorldDispatch{"attempt": {WorkItemIDs: []string{fixture.workID}}},
		ResponseCursors: map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: generation, Sequence: 7}},
		World: interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "attempt", WorkItemIDs: []string{fixture.workID}, Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
		}}},
	}}
	usage := 12
	live := &scopedWorkLiveOwner{processLocalWorkerSessionService: processLocalWorkerSessionService{observationListResult: workersessions.ListObservationsResult{
		Observations: []workersessions.Observation{{WorkerSessionID: "worker", AttemptID: "attempt", WorkIDs: []string{fixture.workID},
			State: workersessions.StateCompleted, TokenUsage: &workersessions.TokenUsage{TotalTokens: &usage}}},
	}}}
	health := &scopedWorkHealthReader{onRead: func() {
		// An accepted append after row selection belongs to the next request.
		ledger.facts.ResponseCursors = map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: generation, Sequence: 9}}
	}}
	watermark := &workerSessionWatermarkedLedger{available: true, watermark: recordings.CanonicalEventCursor{StreamGenerationID: generation, Sequence: 7}}
	service.ledger, service.Service, service.durability = ledger, live, watermark
	service.recordingReader, service.recordingID = health, "owned-recording"
	service.projector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		panic("scoped list rebuilt canonical history")
	}
	read := func(wantSequence int64, wantConfirmation workersessions.ConfirmationState) {
		t.Helper()
		beforeFacts, beforeHealth, beforeWatermarks := ledger.reads, health.reads, watermark.calls
		result, err := service.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: fixture.workID})
		if err != nil || len(result.Observations) != 1 {
			t.Fatalf("scoped list = %+v, %v", result, err)
		}
		row := result.Observations[0]
		if row.WorkerSessionID != "worker" || row.StateSequence != wantSequence || row.ConfirmationState != wantConfirmation ||
			row.TokenUsage == nil || *row.TokenUsage.TotalTokens != usage {
			t.Fatalf("selected terminal row = %+v", row)
		}
		if ledger.reads-beforeFacts != 1 || health.reads-beforeHealth != 1 || watermark.calls-beforeWatermarks != 1 {
			t.Fatalf("reads: selected=%d health=%d watermark=%d; want one each", ledger.reads-beforeFacts, health.reads-beforeHealth, watermark.calls-beforeWatermarks)
		}
		*row.TokenUsage.TotalTokens = -1
		if usage < 0 {
			t.Fatal("returned usage poisoned registry facts")
		}
	}
	read(7, workersessions.ConfirmationStateConfirmed)
	usage = 99
	read(9, workersessions.ConfirmationStateUnconfirmed)
	watermark.watermark.Sequence = 9
	read(9, workersessions.ConfirmationStateConfirmed)
	assertScopedWorkListFailures(t, service, ledger, health, fixture.workID)
	read(9, workersessions.ConfirmationStateConfirmed)
}

func assertScopedWorkListFailures(t *testing.T, service *recordedWorkerSessionObservation, ledger *selectedWorkFactsLedger, health *scopedWorkHealthReader, workID string) {
	t.Helper()
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "selected failure", err: errors.New("owned selected read failed"), want: workersessions.ErrObservationProjectionUnavailable},
		{name: "selected cancellation", err: context.Canceled, want: workersessions.ErrObservationCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ledger.err = test.err
			result, err := service.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: workID})
			if !errors.Is(err, test.want) || len(result.Observations) != 0 {
				t.Fatalf("selected failure returned partial success: %+v, %v", result, err)
			}
		})
	}
	ledger.err = nil
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "corrupt health", err: recordings.ErrWorkerRecordingReplay, want: workersessions.ErrObservationRecordingCorrupt},
		{name: "unavailable health", err: errors.New("owned recording unavailable"), want: workersessions.ErrObservationRecordingUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			health.err = test.err
			result, err := service.ListObservations(t.Context(), workersessions.ListObservationsRequest{WorkID: workID})
			if !errors.Is(err, test.want) || len(result.Observations) != 0 {
				t.Fatalf("health failure returned partial success: %+v, %v", result, err)
			}
		})
	}
	health.err = nil
	ctx, cancel := context.WithCancel(t.Context())
	health.onRead = cancel
	if result, err := service.ListObservations(ctx, workersessions.ListObservationsRequest{WorkID: workID}); !errors.Is(err, workersessions.ErrObservationCanceled) || len(result.Observations) != 0 {
		t.Fatalf("canceled read returned partial success: %+v, %v", result, err)
	}
	health.onRead = nil
}

func TestRecordedListWorkerSessionWorkUsesPreparedFacts(t *testing.T) {
	t.Parallel()
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	when := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	ledger := &selectedWorkFactsLedger{RuntimeLedger: service.ledger, facts: recordings.WorkerSessionWorkFacts{
		KnownWork: true, StreamGenerationID: "selected-generation",
		Associations: map[string]recordings.WorkerSessionAssociationFacts{"selected": {WorkerSessionID: "worker-selected", AssociatedAt: when}},
		Requests:     map[string]interfaces.FactoryWorldDispatch{"selected": {WorkItemIDs: []string{fixture.workID}, StartedAt: when}},
		StateCursors: map[string]recordings.CanonicalEventCursor{"selected": {StreamGenerationID: "selected-generation", Sequence: 7}},
		World:        interfaces.FactoryWorldState{ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{"selected": {WorkItemIDs: []string{fixture.workID}, StartedAt: when}}},
	}}
	service.ledger = ledger
	service.projector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
		panic("scoped list rebuilt the world")
	}
	rows, known, _, err := service.projectListedWorkSnapshot(t.Context(), fixture.workID, nil)
	if err != nil || !known || len(rows) != 1 || rows[0].WorkerSessionID != "worker-selected" || rows[0].State != workersessions.StateRunning || rows[0].StateSequence != 7 || rows[0].StreamGenerationID != "selected-generation" || ledger.workID != fixture.workID {
		t.Fatalf("selected list = %+v, known=%v, err=%v, selector=%q", rows, known, err, ledger.workID)
	}
	ledger.facts.Associations = nil
	rows, known, _, err = service.projectListedWorkSnapshot(t.Context(), fixture.workID, nil)
	if err != nil || !known || len(rows) != 0 {
		t.Fatalf("known empty Work = %+v, %v, %v", rows, known, err)
	}
	ledger.err = context.Canceled
	if _, _, _, err := service.projectListedWorkSnapshot(t.Context(), fixture.workID, nil); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("selected cancellation = %v", err)
	}
	ledger.err = errors.New("selected projection unavailable")
	if _, _, _, err := service.projectListedWorkSnapshot(t.Context(), fixture.workID, nil); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("selected read error = %v", err)
	}
}

type forbiddenCapturedProviderProjection struct{ providersessions.Service }

type selectedHealthResponse struct {
	recordings.WorkerRecordingReader
	snapshot recordings.WorkerRecordingSnapshot
	ids      []string
}

func (reader *selectedHealthResponse) CurrentWorkerRecordingHealth(_ context.Context, _ string, ids []string) (recordings.WorkerRecordingSnapshot, error) {
	reader.ids = append([]string(nil), ids...)
	return reader.snapshot, nil
}

func (*selectedHealthResponse) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	panic("selected health loaded full capture")
}

func TestScopedWorkHealthRejectsForeignAndMalformedSelections(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		snapshot recordings.WorkerRecordingSnapshot
	}{
		{name: "foreign recording", snapshot: recordings.WorkerRecordingSnapshot{RecordingID: "foreign"}},
		{name: "foreign worker", snapshot: recordings.WorkerRecordingSnapshot{Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: "foreign"}}}},
		{name: "duplicate worker", snapshot: recordings.WorkerRecordingSnapshot{Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "selected", Status: recordings.WorkerRecordingStatusComplete},
			{WorkerSessionID: "selected", Status: recordings.WorkerRecordingStatusComplete},
		}}},
		{name: "invalid health", snapshot: recordings.WorkerRecordingSnapshot{Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: "selected", Status: "INVALID"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reader := &selectedHealthResponse{snapshot: test.snapshot}
			service := &recordedWorkerSessionObservation{recordingReader: reader, recordingID: "owned"}
			rows := []workersessions.Observation{{WorkerSessionID: "selected"}}
			if result, err := service.selectedRecordingHealth(t.Context(), rows, rows); !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) || len(result) != 0 {
				t.Fatalf("invalid selected health = %+v, %v", result, err)
			}
			if len(reader.ids) != 1 || reader.ids[0] != "selected" {
				t.Fatalf("capture selectors = %v", reader.ids)
			}
		})
	}
	reader := &selectedHealthResponse{snapshot: recordings.WorkerRecordingSnapshot{RecordingID: "owned"}}
	service := &recordedWorkerSessionObservation{recordingReader: reader, recordingID: "owned"}
	if result, err := service.selectedRecordingHealth(t.Context(), nil, nil); err != nil || len(result) != 0 || len(reader.ids) != 0 {
		t.Fatalf("empty selected health = %+v, %v, selectors=%v", result, err, reader.ids)
	}
	service.recordingReader = &scriptedWorkerRecordingReader{}
	if _, err := service.selectedRecordingHealth(t.Context(), nil, nil); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("missing prepared-health capability = %v", err)
	}
}

func TestScopedTerminalConfirmationUsesSelectedResponseCursor(t *testing.T) {
	t.Parallel()
	const generation = "selected-generation"
	ledger := &selectedWorkFactsLedger{facts: recordings.WorkerSessionWorkFacts{
		StreamGenerationID: generation,
		Associations:       map[string]recordings.WorkerSessionAssociationFacts{"attempt": {WorkerSessionID: "worker"}},
		ResponseCursors:    map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: generation, Sequence: 7}},
		StateCursors:       map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: generation, Sequence: 9}},
		World: interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "attempt", Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
		}}},
	}}
	sample := completedFlushWatermarkSample{generationID: generation, available: true,
		watermark: recordings.CanonicalEventCursor{StreamGenerationID: generation, Sequence: 7}}
	for _, test := range []struct {
		name      string
		mutate    func(*recordings.WorkerSessionWorkFacts, *workersessions.Observation)
		confirmed bool
	}{
		{name: "matching response", confirmed: true},
		{name: "different generation", mutate: func(f *recordings.WorkerSessionWorkFacts, _ *workersessions.Observation) {
			f.StreamGenerationID = "other"
		}},
		{name: "different response generation", mutate: func(f *recordings.WorkerSessionWorkFacts, _ *workersessions.Observation) {
			f.ResponseCursors = map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: "other", Sequence: 7}}
		}},
		{name: "response beyond watermark", mutate: func(f *recordings.WorkerSessionWorkFacts, _ *workersessions.Observation) {
			f.ResponseCursors = map[string]recordings.CanonicalEventCursor{"attempt": {StreamGenerationID: generation, Sequence: 8}}
		}},
		{name: "different worker", mutate: func(_ *recordings.WorkerSessionWorkFacts, o *workersessions.Observation) { o.WorkerSessionID = "other" }},
		{name: "different attempt", mutate: func(_ *recordings.WorkerSessionWorkFacts, o *workersessions.Observation) { o.AttemptID = "other" }},
		{name: "different outcome", mutate: func(_ *recordings.WorkerSessionWorkFacts, o *workersessions.Observation) {
			o.State = workersessions.StateFailed
		}},
		{name: "active", mutate: func(_ *recordings.WorkerSessionWorkFacts, o *workersessions.Observation) {
			o.State = workersessions.StateRunning
		}},
		{name: "no response", mutate: func(f *recordings.WorkerSessionWorkFacts, _ *workersessions.Observation) { f.ResponseCursors = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ledger := &selectedWorkFactsLedger{facts: ledger.facts}
			rows := []workersessions.Observation{{WorkerSessionID: "worker", AttemptID: "attempt", State: workersessions.StateCompleted}}
			if test.mutate != nil {
				test.mutate(&ledger.facts, &rows[0])
			}
			if err := applySelectedWorkConfirmation(t.Context(), rows, ledger.facts, sample); err != nil {
				t.Fatal(err)
			}
			if got := rows[0].ConfirmationState == workersessions.ConfirmationStateConfirmed; got != test.confirmed {
				t.Fatalf("confirmation = %+v, want confirmed=%v", rows[0], test.confirmed)
			}
			if test.confirmed && (rows[0].StateSequence != 7 || !rows[0].StateSequenceKnown) {
				t.Fatalf("selected response cursor = %+v, selector=%q", rows[0], ledger.workID)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := applySelectedWorkConfirmation(ctx, nil, ledger.facts, sample); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("selected cancellation = %v", err)
	}
}

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
	service.ledger = service.ledger.(*preparedScopedTestLedger).RuntimeLedger
	prepareScopedTestFacts(service)
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
