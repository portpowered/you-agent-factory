package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type replayCaptureReader struct {
	recordings.WorkerSessionRecordingService
	snapshot recordings.WorkerRecordingSnapshot
}

// Prepare selected peer facts from the component fixture, independently of
// LoadWorkerRecording. Production readers maintain metadata slots at append.
func (f replayCaptureReader) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerCapturedSummary{}, err
	}
	for _, session := range f.snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		item := recordings.WorkerCapturedCatalogItem{Catalog: recordings.WorkerSessionCatalogEntry{
			WorkerSessionID: id, RecordingID: f.snapshot.RecordingID, CommittedPosition: ^uint64(0),
		}}
		for _, record := range session.Records {
			item.MetadataRecords = append(item.MetadataRecords, record.Detached())
		}
		return recordings.WorkerCapturedSummary{Capture: item}, nil
	}
	return recordings.WorkerCapturedSummary{}, nil
}

type continuationPageReader struct {
	capturedActivityFake
	next recordings.WorkerCapturedActivityPage
}

func (f *continuationPageReader) ReadWorkerCapturedActivity(_ context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	if req.NextToken == "next" {
		return f.next, f.err
	}
	return f.page, f.err
}

func TestContinuationCapturedStreamPagesAndFailsClosed(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"complete", "generation", "gap", "missing-terminal", "false-terminal", "read-failure", "canceled", "closed"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			reader := continuationStreamPages(t)
			r := newContinuationSource(t, continuationReservationRequest())
			r.logs = &LogReader{reader: reader}
			stream, err := r.StreamObservationsByWorkerSessionID(t.Context(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "archived-successor", Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if first := stream.Next(t.Context()); first.Kind != workersessions.ObservationDeliveryRecord || first.Event.Position != 1 {
				t.Fatalf("opening delivery = %+v", first)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := configureContinuationStreamCell(cell, reader, stream, cancel)
			got := stream.Next(ctx)
			if got.Kind != want {
				t.Fatalf("next delivery = %+v, want %s", got, want)
			}
			if cell == "complete" && (got.Event.Position != 2 || got.Summary == nil || !got.Summary.Complete || !json.Valid(got.Event.Payload)) {
				t.Fatalf("terminal replay lost captured evidence: %+v", got)
			}
			if _, exists := r.sessions["archived-successor"]; exists || len(r.supervisions) != 0 {
				t.Fatal("captured stream restored live authority")
			}
		})
	}
}

func configureContinuationStreamCell(cell string, reader *continuationPageReader, stream workersessions.ObservationSubscription, cancel context.CancelFunc) workersessions.ObservationDeliveryKind {
	switch cell {
	case "complete":
		return workersessions.ObservationDeliveryTerminalReplay
	case "generation":
		reader.next.Catalog.RecordingGenerationID = "changed"
	case "gap":
		reader.next.Records[0].Record.ID.Position = 3
	case "missing-terminal":
		reader.next.Terminal = nil
	case "false-terminal":
		reader.next.Records[0].Record.SourceEventID = "progress"
	case "read-failure":
		reader.err = context.DeadlineExceeded
	case "canceled":
		cancel()
		return workersessions.ObservationDeliveryCanceled
	case "closed":
		stream.Close()
		return workersessions.ObservationDeliveryClosed
	}
	return workersessions.ObservationDeliverySourceFailure
}

func continuationStreamPages(t *testing.T) *continuationPageReader {
	t.Helper()
	_, plan, target := retainedContinuationFixture(t)
	opening := openingSessionPayload("archived-successor", "successor-attempt", newContinuationSource(t, plan.request).clock.Now(),
		plan.execution.Execution, &workers.SessionLineage{PredecessorWorkerSessionID: target.WorkerSessionID,
			PreviousAttemptID: target.ExpectedAttemptID, PreviousDispatchID: target.ExpectedAttemptID})
	payload, _ := json.Marshal(opening)
	encoded, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: payload})
	first := replayObservationRecord(workersessions.Topic("archived-successor"), 1, "opening")
	first.Payload = encoded
	terminal := replayObservationRecord(first.ID.Topic, 2, "terminal")
	terminal.SourceType, terminal.SourceSequence, terminal.SourceEventID = lifecycleSourceType, terminalSourceSequence, terminalSourceEventID
	terminal.Payload = []byte(`{"kind":"SESSION","phase":"COMPLETED","payload":{"status":"COMPLETED"}}`)
	page := recordings.WorkerCapturedActivityPage{
		Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: "archived-successor", RecordingGenerationID: "generation", CommittedPosition: 2},
		Opening: first, Health: recordings.WorkerRecordingStatusComplete,
		Terminal: &recordings.WorkerRecordingTerminal{Status: "COMPLETED", Position: 2},
		Records:  []recordings.WorkerCapturedRecord{{Record: first}}, NextToken: "next",
	}
	next := page
	next.Records, next.NextToken = []recordings.WorkerCapturedRecord{{Record: terminal}}, ""
	return &continuationPageReader{capturedActivityFake: capturedActivityFake{page: page}, next: next}
}

func TestContinuationCapturedStreamRefusesForeignOrIncompleteHistory(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"foreign-scope", "foreign-worker", "non-terminal", "missing-opening", "cursor"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			reader := continuationStreamPages(t)
			r := newContinuationSource(t, continuationReservationRequest())
			r.logs = &LogReader{reader: reader}
			req := workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "archived-successor"}
			switch cell {
			case "foreign-scope":
				req.FactorySessionID = "foreign"
			case "foreign-worker":
				reader.page.Catalog.WorkerSessionID = "foreign"
			case "non-terminal":
				reader.page.Terminal.Status = "RUNNING"
			case "missing-opening":
				reader.page.Opening.Payload = nil
			case "cursor":
				req.Cursor = &workersessions.ObservationCursor{Position: 1}
			}
			_, err := r.StreamObservationsByWorkerSessionID(t.Context(), req)
			if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
				t.Fatalf("unsafe captured history opened: %v", err)
			}
		})
	}
}

func (f replayCaptureReader) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return f.snapshot, nil
}

func TestCapturedAtReplayKeepsStoredTimeAndOmitsUnknown(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	topic := workersessions.Topic("worker-1")
	reader := &observationEventReaderFake{readResults: []events.ReadResult{{
		Outcome:  events.ReadOutcomeProgress,
		Records:  []events.Record{replayObservationRecord(topic, 1, "one"), replayObservationRecord(topic, 2, "two")},
		Next:     events.Cursor{Topic: topic, Position: 2},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 2},
	}}}
	r := newObservationRegistry(reader)
	r.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	r.publications = map[string]*publication{"worker-1": {recordingID: "recording"}}
	times := map[string]time.Time{"1": stamp}
	r.recording = replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: "worker-1", CapturedAt: times}},
	}}
	stream, err := r.StreamObservationsByWorkerSessionID(t.Context(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1", ReplayOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	times["1"] = time.Time{}
	first := stream.Next(t.Context())
	if first.Event.CapturedAt == nil || !first.Event.CapturedAt.Equal(stamp) {
		t.Fatalf("replay lost detached committed timestamp: %+v", first.Event)
	}
	second := stream.Next(t.Context())
	if second.Event.CapturedAt != nil {
		t.Fatal("replay invented a timestamp for an unknown record")
	}
}

func TestArchivedOrdinaryReplay(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"complete", "incomplete", "owner-lost", "active", "storage-failure", "unknown"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			reader := ordinaryReplayPages(t, cell)
			r := newContinuationSource(t, continuationReservationRequest())
			r.logs = &LogReader{reader: reader}
			// Compare the exact detached public events, including captured timestamps.
			stamp := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
			reader.page.Records[0].CapturedAt = &stamp
			expected := projectObservationEvent(reader.page.Records[0].Record, "archived-successor")
			expected.CapturedAt = &stamp
			if cell == "unknown" || cell == "storage-failure" {
				_, streamErr := r.StreamObservationsByWorkerSessionID(t.Context(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "archived-successor", ReplayOnly: true})
				want := workersessions.ErrObservationSourceUnavailable
				if cell == "unknown" {
					want = workersessions.ErrObservationSessionNotFound
				}
				if !errors.Is(streamErr, want) {
					t.Fatalf("stream error = %v, want %v", streamErr, want)
				}
				return
			}
			stream, err := r.StreamObservationsByWorkerSessionID(t.Context(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "archived-successor", ReplayOnly: true, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			first := stream.Next(t.Context())
			if first.Kind != workersessions.ObservationDeliveryRecord || !reflect.DeepEqual(first.Event, expected) {
				t.Fatalf("replay differs from logs: %+v %+v", first, expected)
			}
			assertOrdinaryReplaySummary(t, stream, cell)
			if stream.Next(t.Context()).Kind != workersessions.ObservationDeliveryClosed {
				t.Fatal("archive replay did not close")
			}
			if _, exists := r.sessions["archived-successor"]; exists || len(r.supervisions) != 0 {
				t.Fatal("archive restored live authority")
			}
		})
	}
}

func assertOrdinaryReplaySummary(t *testing.T, stream workersessions.ObservationSubscription, cell string) {
	t.Helper()
	last := stream.Next(t.Context())
	if cell == "complete" {
		if last.Kind != workersessions.ObservationDeliveryTerminalReplay || last.Summary == nil || !last.Summary.Complete {
			t.Fatalf("complete replay = %+v", last)
		}
	} else {
		if cell == "incomplete" {
			if last.Kind != workersessions.ObservationDeliveryRecord || last.Event.Position != 2 {
				t.Fatalf("incomplete committed terminal = %+v", last)
			}
			last = stream.Next(t.Context())
		}
		if last.Kind != workersessions.ObservationDeliveryReplaySummary || last.Summary == nil || last.Summary.Complete {
			t.Fatalf("prefix presented as complete/live: %+v", last)
		}
	}
}

func ordinaryReplayPages(t *testing.T, cell string) *continuationPageReader {
	t.Helper()
	reader := continuationStreamPages(t)
	var draft workers.Draft
	var opening workers.SessionPayload
	if err := json.Unmarshal(reader.page.Opening.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(draft.Payload, &opening); err != nil {
		t.Fatal(err)
	}
	opening.Lineage, opening.Continuation = nil, nil
	draft.Payload, _ = json.Marshal(opening)
	reader.page.Opening.Payload, _ = json.Marshal(draft)
	reader.page.Records[0].Record = reader.page.Opening
	switch cell {
	case "incomplete":
		reader.page.Health = recordings.WorkerRecordingStatusIncomplete
	case "owner-lost", "active":
		reader.page.Catalog.CommittedPosition = 1
		reader.page.NextToken = ""
		reader.page.Terminal = nil
		if cell == "owner-lost" {
			reader.page.Health = recordings.WorkerRecordingStatusIncomplete
			reader.page.Terminal = &recordings.WorkerRecordingTerminal{Status: "CANCELED", Position: 0}
		}
	case "storage-failure":
		reader.err = recordings.ErrWorkerRecordingPersistence
	case "unknown":
		reader.err = os.ErrNotExist
	}
	reader.next.Health = reader.page.Health
	return reader
}
