package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type capturedActivityFake struct {
	page    recordings.WorkerCapturedActivityPage
	err     error
	request recordings.WorkerCapturedActivityRequest
}

func (f *capturedActivityFake) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("finite logs must use the atomic page read")
}
func (f *capturedActivityFake) ReadWorkerCapturedActivity(_ context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	f.request = req
	return f.page, f.err
}

func TestCapturedLogsRetainCommittedIdentityAndDetachedTime(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 10, 5, 12, 0, 0, 123, time.UTC)
	fake := &capturedActivityFake{page: recordings.WorkerCapturedActivityPage{
		Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: "worker", RecordingGenerationID: "generation", CommittedPosition: 3},
		Health:  recordings.WorkerRecordingStatusIncomplete, NextToken: "next",
		Opening: events.Record{Payload: []byte(`{"kind":"SESSION","payload":{"factorySessionId":"factory","workIds":["work"]}}`)},
		Records: []recordings.WorkerCapturedRecord{{Record: events.Record{
			ID: events.RecordID{Position: 2}, Payload: []byte(`{"kind":"MESSAGE"}`),
		}, CapturedAt: &stamp}},
	}}
	service, err := NewWithCapturedActivity(nil, nil, logging.NoopLogger{}, nil, nil, nil, nil, fake)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.ReadLogs(t.Context(), workersessions.ReadLogsRequest{WorkerSessionID: "worker", Limit: 1, NextToken: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedLogPageIdentity(t, page)
	if len(page.Events) != 1 || page.Events[0].Position != 2 || page.Events[0].CapturedAt == nil || !page.Events[0].CapturedAt.Equal(stamp) {
		t.Fatalf("event lost committed time: %+v", page.Events)
	}
	*page.Events[0].CapturedAt = time.Time{}
	page.Events[0].Payload[0] = 'x'
	if stamp.IsZero() || fake.page.Records[0].Record.Payload[0] != '{' {
		t.Fatal("page aliases captured metadata")
	}
	if fake.request.Limit != 1 || fake.request.NextToken != "previous" {
		t.Fatalf("cursor not delegated: %+v", fake.request)
	}
	fake.page.Records[0].CapturedAt = nil
	legacy, err := service.ReadLogs(t.Context(), workersessions.ReadLogsRequest{WorkerSessionID: "worker"})
	if err != nil || legacy.Events[0].CapturedAt != nil {
		t.Fatalf("legacy time synthesized: %v", err)
	}
}

func assertCapturedLogPageIdentity(t *testing.T, page workersessions.LogPage) {
	t.Helper()
	if page.WorkerSessionID != "worker" || page.RecordingGenerationID != "generation" || page.CommittedPosition != 3 || page.NextToken != "next" || page.Health != "INCOMPLETE" || page.FactorySessionID != "factory" || len(page.WorkIDs) != 1 {
		t.Fatalf("page lost capture identity: %+v", page)
	}
}

func TestCapturedLogsReturnSafeTypedStorageOutcomes(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name         string
		source, want error
	}{
		{"missing", os.ErrNotExist, workersessions.ErrObservationSessionNotFound},
		{"cursor", recordings.ErrInvalidWorkerRecordingRequest, workersessions.ErrInvalidLogsRequest},
		{"unreadable", errors.New("private path and payload"), workersessions.ErrLogsUnavailable},
		{"canceled", context.Canceled, context.Canceled},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			service, err := NewWithCapturedActivity(nil, nil, logging.NoopLogger{}, nil, nil, nil, nil, &capturedActivityFake{err: cell.source})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.ReadLogs(t.Context(), workersessions.ReadLogsRequest{WorkerSessionID: "worker"})
			if !errors.Is(err, cell.want) {
				t.Fatalf("error = %v, want %v", err, cell.want)
			}
		})
	}
}
