package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type transcriptCaptureFake struct {
	pages    []recordings.WorkerCapturedActivityPage
	requests []recordings.WorkerCapturedActivityRequest
}

func (f *transcriptCaptureFake) ReadWorkerCapturedActivity(_ context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	f.requests = append(f.requests, req)
	if req.WorkerSessionID != f.pages[0].Catalog.WorkerSessionID {
		return recordings.WorkerCapturedActivityPage{}, os.ErrNotExist
	}
	if req.NextToken == "" {
		return f.pages[0], nil
	}
	return f.pages[1], nil
}
func (f *transcriptCaptureFake) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("not used")
}
func (f *transcriptCaptureFake) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{Items: []recordings.WorkerCapturedCatalogItem{{Catalog: f.pages[0].Catalog, Terminal: f.pages[0].Terminal, MetadataRecords: []events.Record{f.pages[len(f.pages)-1].Records[len(f.pages[len(f.pages)-1].Records)-1].Record}}}}, nil
}

func transcriptCapturePage(id, scope, text string) recordings.WorkerCapturedActivityPage {
	ref := observationProviderRef()
	opening := workers.SessionPayload{WorkerSessionID: id, FactorySessionID: scope, AttemptID: "attempt-1", WorkIDs: []string{"work-1"}}
	terminal := opening
	terminal.Status = "COMPLETED"
	terminal.Continuation = &workers.SessionContinuation{Provider: string(ref.Provider), Kind: ref.Kind, ID: ref.ID}
	message := workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: text}}}
	drafts := []workers.Draft{
		{Kind: workers.KindSession, Phase: workers.PhaseStarted},
		{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ItemID: "message"},
		{Kind: workers.KindSession, Phase: workers.PhaseCompleted},
	}
	payloads := []any{opening, message, terminal}
	page := recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{
		WorkerSessionID: id, FactorySessionID: scope, RecordingGenerationID: "generation", CommittedPosition: 3,
	}, Health: recordings.WorkerRecordingStatusComplete, Terminal: &recordings.WorkerRecordingTerminal{Position: 3, Status: "COMPLETED"}}
	for index, draft := range drafts {
		draft.DispatchID, draft.TurnID = "attempt-1", "turn-1"
		draft.Payload, _ = json.Marshal(payloads[index])
		payload, _ := json.Marshal(draft)
		page.Records = append(page.Records, recordings.WorkerCapturedRecord{Record: events.Record{
			ID: events.RecordID{Topic: workersessions.Topic(id, scope), Position: events.AggregateSequence(index + 1)}, SourceID: "source", Payload: payload,
		}})
	}
	page.Opening = page.Records[0].Record
	return page
}

func attachTranscriptCapture(r *registry, id, scope, text string) *transcriptCaptureFake {
	fake := &transcriptCaptureFake{pages: []recordings.WorkerCapturedActivityPage{transcriptCapturePage(id, scope, text)}}
	r.logs = &LogReader{reader: fake, logger: logging.NoopLogger{}}
	return fake
}

func TestCapturedTranscriptPreservesLiveArchivedAndTupleCorrelation(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "factory"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			r := newObservationRegistry(projector, nil)
			r.sessions["worker"] = observationSession("worker", workersessions.StateCompleted)
			r.observations["worker"] = observationMetadata()
			r.observations["worker"].factorySessionID = scope
			fake := attachTranscriptCapture(r, "worker", scope, "captured answer")
			want, err := r.ReadTranscript(t.Context(), workersessions.ReadTranscriptRequest{WorkerSessionID: "worker", FactorySessionID: scope})
			if err != nil {
				t.Fatal(err)
			}

			text := "captured answer"
			expected := workersessions.ReadTranscriptResult{WorkerSessionID: "worker", ProviderSession: observationProviderRef(), AttemptID: "attempt-1", TurnID: "turn-1", WorkIDs: []string{"work-1"}, State: workersessions.StateCompleted,
				Entries: []workersessions.TranscriptEntry{{Type: workersessions.TranscriptAssistantMessage, Order: 1, Text: &text}}}
			if !reflect.DeepEqual(want, expected) {
				t.Fatalf("lost captured correlation: %+v, want %+v", want, expected)
			}
			// Simulate a fresh host with the same detached storage boundary; no
			// registry or control authority is recreated by historical reads.
			archive := newObservationRegistry(projector, nil)
			archive.logs = &LogReader{reader: fake, logger: logging.NoopLogger{}}
			for _, request := range []workersessions.ReadTranscriptRequest{
				{WorkerSessionID: "worker", FactorySessionID: scope}, {ProviderSession: observationProviderRef(), FactorySessionID: scope},
			} {
				got, err := archive.ReadTranscript(t.Context(), request)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("archived parity = %+v, %v; want %+v", got, err, want)
				}
			}
			got, err := archive.ReadTranscriptByWorkerSessionID(t.Context(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "worker", FactorySessionID: scope})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("keyed archive = %+v, %v", got, err)
			}
			if projector.calls != 0 || len(archive.sessions) != 0 {
				t.Fatal("transcript called native projection or restored live authority")
			}
			*got.Entries[0].Text = "changed"
			again, err := archive.ReadTranscript(t.Context(), workersessions.ReadTranscriptRequest{WorkerSessionID: "worker"})
			if err != nil || *again.Entries[0].Text != "captured answer" {
				t.Fatal("returned entries alias capture")
			}
			_, err = archive.ReadTranscript(t.Context(), workersessions.ReadTranscriptRequest{WorkerSessionID: "worker", FactorySessionID: "foreign"})
			if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
				t.Fatalf("foreign scope = %v", err)
			}
		})
	}
}

func TestCapturedTranscriptRejectsIncompleteMismatchedAndDamagedPages(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"incomplete", "no terminal", "no ref", "gap", "generation", "head", "scope", "topic", "malformed", "attempt", "truncated"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			page := transcriptCapturePage("worker", "factory", "answer")
			second := page
			page.Records, second.Records = page.Records[:2], second.Records[2:]
			page.NextToken = "next"
			want := workersessions.ErrObservationTranscriptProjectionUnavailable
			switch name {
			case "incomplete":
				page.Health = recordings.WorkerRecordingStatusIncomplete
				want = workersessions.ErrObservationTranscriptUnavailable
			case "no terminal":
				page.Terminal = nil
				want = workersessions.ErrObservationTranscriptUnavailable
			case "no ref":
				second.Records[0].Record.Payload = []byte(`{"kind":"SESSION","phase":"COMPLETED","dispatchId":"attempt-1","turnId":"turn-1","payload":{"workerSessionId":"worker","factorySessionId":"factory","attemptId":"attempt-1","status":"COMPLETED"}}`)
				want = workersessions.ErrObservationTranscriptUnavailable
			case "gap":
				page.Records = page.Records[:1]
			case "generation":
				second.Catalog.RecordingGenerationID = "other"
			case "head":
				second.Catalog.CommittedPosition++
			case "scope":
				second.Catalog.FactorySessionID = "other"
			case "topic":
				second.Records[0].Record.ID.Topic = "foreign"
			case "malformed":
				second.Records[0].Record.Payload = []byte(`{`)
			case "attempt":
				second.Records[0].Record.Payload = []byte(`{"kind":"SESSION","payload":{"workerSessionId":"worker","attemptId":"foreign"}}`)
			case "truncated":
				second.Records[0].Truncated = true
			}
			reader := &LogReader{reader: &transcriptCaptureFake{pages: []recordings.WorkerCapturedActivityPage{page, second}}}
			_, err := reader.capturedTranscript(t.Context(), "worker", "factory")
			if !errors.Is(err, want) {
				t.Fatalf("unsafe capture = %v, want %v", err, want)
			}
		})
	}
}

func TestCapturedTranscriptKeepsRetryAttemptsSeparate(t *testing.T) {
	t.Parallel()
	for _, valid := range []bool{true, false} {
		t.Run(strconv.FormatBool(valid), func(t *testing.T) {
			t.Parallel()
			page := transcriptCapturePage("worker", "", "first attempt")
			terminal := page.Records[2]
			page.Records = page.Records[:2]
			previous := "attempt-1"
			if !valid {
				previous = "foreign"
			}
			retry := workers.SessionPayload{WorkerSessionID: "worker", AttemptID: "attempt-2", DispatchID: "attempt-2", AttemptReason: workers.AttemptReasonRetry,
				Lineage: &workers.SessionLineage{PreviousAttemptID: previous, PreviousDispatchID: previous}}
			payload, _ := json.Marshal(retry)
			draft := workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseUpdated, DispatchID: "attempt-2", Payload: payload}
			terminal.Record.ID.Position = 3
			terminal.Record.Payload, _ = json.Marshal(draft)
			page.Records = append(page.Records, terminal)
			draft = workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, DispatchID: "attempt-2", ItemID: "message", TurnID: "turn-1", Payload: []byte(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"second attempt"}]}`)}
			terminal.Record.ID.Position = 4
			terminal.Record.Payload, _ = json.Marshal(draft)
			page.Records = append(page.Records, terminal)
			ref := observationProviderRef()
			payload, _ = json.Marshal(terminalSessionPayload{Status: "COMPLETED", Continuation: &workers.SessionContinuation{Provider: string(ref.Provider), Kind: ref.Kind, ID: ref.ID}})
			draft = workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseCompleted, DispatchID: "attempt-2", TurnID: "turn-1", Payload: payload}
			terminal.Record.ID.Position = 5
			terminal.Record.Payload, _ = json.Marshal(draft)
			page.Records = append(page.Records, terminal)
			page.Catalog.CommittedPosition, page.Terminal.Position = 5, 5
			reader := &LogReader{reader: &transcriptCaptureFake{pages: []recordings.WorkerCapturedActivityPage{page}}}
			result, err := reader.capturedTranscript(t.Context(), "worker", "")
			if !valid {
				if !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
					t.Fatalf("foreign retry lineage accepted: %+v %v", result, err)
				}
				return
			}
			if err != nil || result.AttemptID != "attempt-2" || len(result.Entries) != 2 {
				t.Fatalf("retry correlation lost: %+v %v", result, err)
			}
			if *result.Entries[0].Text != "first attempt" || *result.Entries[1].Text != "second attempt" {
				t.Fatalf("retry messages merged: %+v", result.Entries)
			}
		})
	}
}
