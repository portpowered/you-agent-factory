package runtime

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRecordedWorkerSessionObservationCursorResumePreservesDurableHistoryBeyondLiveLimit(t *testing.T) {
	dispatchID := "dispatch-recorded-long-resume"
	workerSessionID := "worker-recorded-long-resume"
	events := make([]interfaces.FactoryEvent, 66)
	for sequence := 1; sequence <= len(events); sequence++ {
		eventType := interfaces.FactoryEventTypeDispatchQueued
		sourceID := "recorded-long-" + strconv.Itoa(sequence)
		if sequence == 1 {
			eventType = interfaces.FactoryEventTypeDispatchRequest
			sourceID = "recorded-long-request"
		}
		if sequence == 2 {
			eventType = interfaces.FactoryEventTypeDispatchWorkerSessionAssoc
			sourceID = "recorded-long-association"
		}
		if sequence == len(events) {
			eventType = interfaces.FactoryEventTypeDispatchResponse
			sourceID = "recorded-long-terminal"
		}
		events[sequence-1] = interfaces.FactoryEvent{
			Context: interfaces.FactoryEventContext{
				Sequence:   sequence,
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Id:   sourceID,
			Type: eventType,
		}
	}
	events[1].Payload = mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	service := newRecordedWorkerSessionObservation(nil, ledger, nil, platformclock.Real{}, nil).(*recordedWorkerSessionObservation)

	subscription, _, err := service.streamRecordedFact(
		context.Background(),
		recordedDispatchObservation{dispatchID: dispatchID, workerSessionID: workerSessionID},
		workersessions.DefaultObservationStreamLimit,
		false,
		&workersessions.ObservationCursor{WorkerSessionID: workerSessionID, Position: 1},
	)
	if err != nil {
		t.Fatalf("streamRecordedFact() error = %v", err)
	}
	defer subscription.Close()

	for wantPosition := uint64(2); wantPosition <= uint64(len(events)); wantPosition++ {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != workersessions.ObservationDeliveryRecord && delivery.Kind != workersessions.ObservationDeliveryTerminalReplay {
			t.Fatalf("durable delivery at position %d = %#v, want a record", wantPosition, delivery)
		}
		if delivery.Event.Position != wantPosition {
			t.Fatalf("durable delivery at index %d = %#v, want position %d", wantPosition-2, delivery.Event, wantPosition)
		}
	}
	if closed := subscription.Next(context.Background()); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after long durable resume = %#v, want CLOSED", closed)
	}
}

func TestRecordedWorkerSessionObservationReplayOnlyDrainsActiveHighWater(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	dispatchID := "dispatch-recorded-replay-only-active"
	workerSessionID := "worker-recorded-replay-only-active"
	workID := "work-recorded-replay-only-active"
	event := func(sequence int, id string, eventType interfaces.FactoryEventType) interfaces.FactoryEvent {
		return interfaces.FactoryEvent{
			Context: interfaces.FactoryEventContext{
				Sequence:   sequence,
				EventTime:  base.Add(time.Duration(sequence) * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Id:   id,
			Type: eventType,
		}
	}
	events := []interfaces.FactoryEvent{
		event(1, "replay-only-request", interfaces.FactoryEventTypeDispatchRequest),
		event(2, "replay-only-association", interfaces.FactoryEventTypeDispatchWorkerSessionAssoc),
		event(3, "replay-only-queued", interfaces.FactoryEventTypeDispatchQueued),
	}
	events[1].Payload = mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
				dispatchID: {DispatchID: dispatchID, StartedAt: base, WorkItemIDs: []string{workID}},
			}}, nil
		},
		platformclock.Real{},
		nil,
	)

	subscription, err := service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
		Limit:           1,
		ReplayOnly:      true,
	})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID(replay-only active) error = %v", err)
	}
	defer subscription.Close()
	for index, wantID := range []string{"replay-only-request", "replay-only-association", "replay-only-queued"} {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.SourceID != wantID {
			t.Fatalf("active replay-only delivery %d = %#v, want record %q", index, delivery, wantID)
		}
	}
	summary := subscription.Next(context.Background())
	if summary.Kind != workersessions.ObservationDeliveryReplaySummary || summary.Summary == nil || summary.Summary.Complete {
		t.Fatalf("active replay-only summary = %#v, want finite incomplete summary", summary)
	}
	if closed := subscription.Next(context.Background()); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after active replay-only summary = %#v, want CLOSED", closed)
	}
}

func TestRecordedWorkerSessionObservation_UsesCanonicalFactsForExactQueries(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	requireRecordedExactObservationList(t, fixture)
	show := requireRecordedExactObservationShow(t, fixture)
	requireRecordedExactObservationTranscript(t, fixture, show)
	requireRecordedExactObservationStream(t, fixture)
	requireRecordedExactObservationWorkerID(t, fixture, show)
}

type scriptedWorkerRecordingReader struct {
	snapshot recordings.WorkerRecordingSnapshot
	err      error
}

func (reader *scriptedWorkerRecordingReader) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	if reader.err != nil {
		return recordings.WorkerRecordingSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}

func TestRecordedWorkerSessionObservationProjectsDurableRecordingHealth(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	service := fixture.service.(*recordedWorkerSessionObservation)
	service.recordingID = "recording-health"
	reader := &scriptedWorkerRecordingReader{}
	service.recordingReader = reader

	for _, testCase := range []struct {
		name        string
		status      recordings.WorkerRecordingStatus
		failure     string
		interrupted string
		wantReason  string
	}{
		{name: "complete", status: recordings.WorkerRecordingStatusComplete},
		{name: "degraded", status: recordings.WorkerRecordingStatusDegraded, failure: "PERSISTENCE_FAILED", wantReason: "PERSISTENCE_FAILED"},
		{name: "incomplete", status: recordings.WorkerRecordingStatusIncomplete, interrupted: recordings.WorkerRecordingInterruptionProcessStopped, wantReason: recordings.WorkerRecordingInterruptionProcessStopped},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reader.snapshot = recordings.WorkerRecordingSnapshot{
				RecordingID: "recording-health",
				Sessions: []recordings.WorkerSessionRecordingSnapshot{{
					WorkerSessionID:    fixture.workerSessionID,
					Status:             testCase.status,
					Failure:            testCase.failure,
					InterruptionReason: testCase.interrupted,
				}},
			}
			observation, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID})
			if err != nil {
				t.Fatalf("GetObservationByWorkerSessionID() error = %v", err)
			}
			if observation.RecordingHealth != testCase.status || observation.RecordingHealthReason != testCase.wantReason {
				t.Fatalf("recording health = %q/%q, want %q/%q", observation.RecordingHealth, observation.RecordingHealthReason, testCase.status, testCase.wantReason)
			}
		})
	}

	reader.err = recordings.ErrWorkerRecordingReplay
	if _, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID}); !errors.Is(err, workersessions.ErrObservationRecordingCorrupt) {
		t.Fatalf("corrupt recording error = %v, want ErrObservationRecordingCorrupt", err)
	}
	reader.err = errors.New("recording storage is offline")
	if _, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID}); !errors.Is(err, workersessions.ErrObservationRecordingUnavailable) {
		t.Fatalf("unavailable recording error = %v, want ErrObservationRecordingUnavailable", err)
	}
}

type recordedExactObservationFixture struct {
	service         workersessions.Service
	ref             providers.SessionRef
	workerSessionID string
	workID          string
	inputTokens     int
	transcriptText  string
}

func newRecordedExactObservationFixture(t *testing.T) recordedExactObservationFixture {
	t.Helper()
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-exact"
	dispatchID := "dispatch-recorded-exact"
	providerMetadata := &workers.ProviderSessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-exact"}
	ref := providerSessionRef(*providerMetadata)
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base, DispatchID: stringPointerForRecordedTest(dispatchID), WorkIDs: stringSliceForRecordedTest([]string{workID})}, Id: "exact-request", Type: interfaces.FactoryEventTypeDispatchRequest, Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{TransitionID: "review"})},
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 2, EventTime: base.Add(time.Second), DispatchID: stringPointerForRecordedTest(dispatchID), RequestID: stringPointerForRecordedTest("turn-recorded-exact")}, Id: "exact-association", Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-recorded-exact"})},
		{Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 3, EventTime: base.Add(3 * time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)}, Id: "exact-response", Type: interfaces.FactoryEventTypeDispatchResponse},
	}
	inputTokens, outputTokens := 7, 5
	transcriptText := "historical transcript"
	providerProjection := &historicalProviderSessions{result: providersessions.ProjectResult{
		Session: ref,
		Detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{Provider: providersessions.ProviderCodex, Kind: providers.SessionIDKind, ID: ref.ID},
			Parse:           providersessions.ParseSummary{EventCount: 3, TokenUsage: &providersessions.TokenUsage{InputTokens: &inputTokens, OutputTokens: &outputTokens}},
			Transcript:      []providersessions.TranscriptEntry{{Order: 0, Type: providersessions.TranscriptAssistantMessage, Text: &transcriptText}},
		},
	}}
	service := newRecordedWorkerSessionObservation(nil, &recordingfixtures.ScriptedRuntimeLedger{Events: events}, func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(3 * time.Second), WorkItemIDs: []string{workID},
			Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)}, ProviderSession: providerMetadata,
		}}}, nil
	}, platformclock.NewDeterministic(base.Add(10*time.Second), time.Second), providerProjection)
	return recordedExactObservationFixture{service: service, ref: ref, workerSessionID: "worker-recorded-exact", workID: workID, inputTokens: inputTokens, transcriptText: transcriptText}
}

func requireRecordedExactObservationList(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	listed, err := fixture.service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: fixture.workID})
	if err != nil || len(listed.Observations) != 1 {
		t.Fatalf("ListObservations() = %#v, %v; want one recorded observation", listed, err)
	}
}

func requireRecordedExactObservationShow(t *testing.T, fixture recordedExactObservationFixture) workersessions.Observation {
	t.Helper()
	show, err := fixture.service.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("GetObservation() error = %v", err)
	}
	if show.WorkerSessionID != "worker-recorded-exact" || show.WorkIDs[0] != fixture.workID || show.State != workersessions.StateCompleted || show.Duration == nil || *show.Duration != 3*time.Second {
		t.Fatalf("GetObservation() = %#v, want canonical completed correlation and duration", show)
	}
	if show.TokenUsage == nil || show.TokenUsage.InputTokens == nil || *show.TokenUsage.InputTokens != fixture.inputTokens || show.Transcript != workersessions.TranscriptAvailabilityAvailable || show.Parse.EventCount != 3 {
		t.Fatalf("GetObservation() detail = %#v, want Provider Sessions enrichment", show)
	}
	return show
}

func requireRecordedExactObservationTranscript(t *testing.T, fixture recordedExactObservationFixture, show workersessions.Observation) {
	t.Helper()
	transcript, err := fixture.service.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	if transcript.WorkerSessionID != show.WorkerSessionID || transcript.WorkIDs[0] != fixture.workID || transcript.State != workersessions.StateCompleted || len(transcript.Entries) != 1 || transcript.Entries[0].Text == nil || *transcript.Entries[0].Text != fixture.transcriptText {
		t.Fatalf("ReadTranscript() = %#v, want canonical identity and normalized entry", transcript)
	}
}

func requireRecordedExactObservationStream(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	subscription, err := fixture.service.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()
	for index, want := range []workersessions.ObservationDeliveryKind{workersessions.ObservationDeliveryRecord, workersessions.ObservationDeliveryRecord, workersessions.ObservationDeliveryTerminalReplay} {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != want || delivery.Event.SourceID == "" || delivery.Event.SourceSequence != uint64(index+1) {
			t.Fatalf("historical delivery %d = %#v, want %s", index, delivery, want)
		}
	}
}

func requireRecordedExactObservationWorkerID(t *testing.T, fixture recordedExactObservationFixture, providerObservation workersessions.Observation) {
	t.Helper()
	requireRecordedWorkerObservation(t, fixture, providerObservation)
	requireRecordedWorkerTranscript(t, fixture)
	requireRecordedWorkerStream(t, fixture)
}

func requireRecordedWorkerObservation(t *testing.T, fixture recordedExactObservationFixture, providerObservation workersessions.Observation) {
	t.Helper()
	show, err := fixture.service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID})
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID() error = %v", err)
	}
	if show.WorkerSessionID != fixture.workerSessionID || show.ProviderSession != providerObservation.ProviderSession || show.State != providerObservation.State {
		t.Fatalf("GetObservationByWorkerSessionID() = %#v, want canonical Worker identity and provider enrichment", show)
	}
}

func requireRecordedWorkerTranscript(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	transcript, err := fixture.service.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: fixture.workerSessionID})
	if err != nil {
		t.Fatalf("ReadTranscript(WorkerSessionID) error = %v", err)
	}
	if transcript.WorkerSessionID != fixture.workerSessionID || transcript.ProviderSession != fixture.ref || len(transcript.Entries) != 1 || transcript.Entries[0].Text == nil || *transcript.Entries[0].Text != fixture.transcriptText {
		t.Fatalf("ReadTranscript(WorkerSessionID) = %#v, want normalized durable transcript", transcript)
	}
}

func requireRecordedWorkerStream(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	subscription, err := fixture.service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: fixture.workerSessionID})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID() error = %v", err)
	}
	defer subscription.Close()
	for index, want := range []workersessions.ObservationDeliveryKind{
		workersessions.ObservationDeliveryRecord,
		workersessions.ObservationDeliveryRecord,
		workersessions.ObservationDeliveryTerminalReplay,
	} {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != want || delivery.Event.SourceID == "" || delivery.Event.SourceSequence != uint64(index+1) {
			t.Fatalf("Worker-ID historical delivery %d = %#v, want %s at sequence %d", index, delivery, want, index+1)
		}
	}
}

func TestRecordedWorkerSessionObservationResumesExclusivelyAndClassifiesCursors(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	adapter := fixture.service.(*recordedWorkerSessionObservation)
	ledger := adapter.ledger.(*recordingfixtures.ScriptedRuntimeLedger)
	ledger.GenerationID = "generation-1"
	requireRecordedCursorResume(t, fixture)
	requireRecordedCursorErrors(t, fixture, ledger)
}

func requireRecordedCursorResume(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	subscription, err := fixture.service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: fixture.workerSessionID,
		Cursor:          &workersessions.ObservationCursor{WorkerSessionID: fixture.workerSessionID, Position: 1, StreamGenerationID: "generation-1"},
	})
	if err != nil {
		t.Fatalf("cursor resume = %v", err)
	}
	defer subscription.Close()
	first := subscription.Next(context.Background())
	assertRecordedResumedRecord(t, first)
	terminal := subscription.Next(context.Background())
	assertRecordedResumedTerminal(t, fixture, terminal)

	postTerminal, err := fixture.service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: fixture.workerSessionID,
		Cursor:          &workersessions.ObservationCursor{WorkerSessionID: fixture.workerSessionID, Position: 3, StreamGenerationID: "generation-1"},
	})
	if err != nil {
		t.Fatalf("post-terminal cursor resume = %v", err)
	}
	postTerminalSummary := postTerminal.Next(context.Background())
	postTerminal.Close()
	assertRecordedPostTerminalSummary(t, postTerminalSummary)
}

func assertRecordedResumedRecord(t *testing.T, delivery workersessions.ObservationDelivery) {
	t.Helper()
	if delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.SourceID != "exact-association" || delivery.Event.Position != 2 {
		t.Fatalf("first resumed delivery = %#v, want association at position 2", delivery)
	}
}

func assertRecordedResumedTerminal(t *testing.T, fixture recordedExactObservationFixture, delivery workersessions.ObservationDelivery) {
	t.Helper()
	if delivery.Kind != workersessions.ObservationDeliveryTerminalReplay || delivery.Event.SourceID != "exact-response" || delivery.Event.Position != 3 {
		t.Fatalf("terminal resumed delivery = %#v, want response at position 3", delivery)
	}
	if delivery.Summary == nil || !delivery.Summary.Complete || delivery.Summary.EventsEmitted != 2 {
		t.Fatalf("terminal summary = %#v, want complete two-event durable summary", delivery.Summary)
	}
	if delivery.Event.Cursor.WorkerSessionID != fixture.workerSessionID || delivery.Event.Cursor.StreamGenerationID != "generation-1" {
		t.Fatalf("terminal cursor = %#v, want Worker Session and generation identity", delivery.Event.Cursor)
	}
}

func assertRecordedPostTerminalSummary(t *testing.T, delivery workersessions.ObservationDelivery) {
	t.Helper()
	if delivery.Kind != workersessions.ObservationDeliveryReplaySummary || delivery.Summary == nil || !delivery.Summary.Complete || delivery.Summary.EventsEmitted != 0 {
		t.Fatalf("post-terminal summary = %#v, want immediate complete zero-event summary", delivery)
	}
}

func requireRecordedCursorErrors(t *testing.T, fixture recordedExactObservationFixture, ledger *recordingfixtures.ScriptedRuntimeLedger) {
	t.Helper()
	otherDispatch := "other-dispatch"
	ledger.Events = append(ledger.Events, interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 5, DispatchID: &otherDispatch},
		Id:      "other-event",
		Type:    interfaces.FactoryEventTypeDispatchResponse,
	})

	for _, testCase := range []struct {
		name   string
		cursor workersessions.ObservationCursor
		want   error
	}{
		{name: "foreign", cursor: workersessions.ObservationCursor{Position: 5, StreamGenerationID: "generation-1"}, want: workersessions.ErrObservationCursorForeign},
		{name: "future", cursor: workersessions.ObservationCursor{Position: 99, StreamGenerationID: "generation-1"}, want: workersessions.ErrObservationCursorFuture},
		{name: "stale", cursor: workersessions.ObservationCursor{Position: 4, StreamGenerationID: "generation-1"}, want: workersessions.ErrObservationCursorStale},
		{name: "generation", cursor: workersessions.ObservationCursor{Position: 1, StreamGenerationID: "generation-2"}, want: workersessions.ErrObservationCursorUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := fixture.service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
				WorkerSessionID: fixture.workerSessionID,
				Cursor:          &testCase.cursor,
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("cursor error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestRecordedWorkerSessionObservationDeduplicatesDurableLiveOverlap(t *testing.T) {
	dispatchID := "dispatch-overlap"
	workerSessionID := "worker-overlap"
	opening := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 1, DispatchID: stringPointerForRecordedTest(dispatchID)},
		Id:      "overlap-opening",
		Type:    interfaces.FactoryEventTypeDispatchRequest,
	}
	association := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 2, DispatchID: stringPointerForRecordedTest(dispatchID)},
		Id:      "overlap-association",
		Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}
	terminal := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 3, DispatchID: stringPointerForRecordedTest(dispatchID)},
		Id:      "overlap-terminal",
		Type:    interfaces.FactoryEventTypeDispatchResponse,
	}
	live := make(chan interfaces.FactoryEvent, 2)
	live <- association
	live <- terminal
	close(live)
	subscription := newRecordedObservationSubscriptionWithSummary(
		interfaces.FactoryEventStream{
			StreamGenerationID: "generation-overlap",
			History:            []interfaces.FactoryEvent{opening, association},
			Events:             live,
		},
		dispatchID, false, nil, context.Background(), false, nil, workerSessionID,
	)
	defer subscription.Close()

	first := subscription.Next(context.Background())
	second := subscription.Next(context.Background())
	third := subscription.Next(context.Background())
	if first.Event.Position != 1 || second.Event.Position != 2 || third.Event.Position != 3 || third.Kind != workersessions.ObservationDeliveryTerminal {
		t.Fatalf("overlap deliveries = %#v, %#v, %#v, want positions 1,2,3", first, second, third)
	}
	if third.Event.Cursor.WorkerSessionID != workerSessionID {
		t.Fatalf("overlap terminal cursor = %#v, want Worker identity", third.Event.Cursor)
	}
}
