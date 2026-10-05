package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type logsServiceFake struct {
	page workersessions.LogPage
	err  error
}

func (f logsServiceFake) ReadLogs(context.Context, workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	return f.page, f.err
}

func TestCapturedLogsHTTPRetainsCommitTimeAndLegacyOmission(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 10, 5, 12, 0, 0, 123, time.UTC)
	page := workersessions.LogPage{WorkerSessionID: "worker", RecordingGenerationID: "generation", CommittedPosition: 2, Health: "COMPLETE", Events: []workersessions.ObservationEvent{
		{Position: 1, CapturedAt: &stamp, Payload: []byte(`{"kind":"SESSION"}`)},
		{Position: 2, Payload: []byte(`{"kind":"MESSAGE"}`)},
	}}
	handler := NewHandler((&Adapter{}).WithLogsService(logsServiceFake{page: page}), zap.NewNop())
	response := httptest.NewRecorder()
	handler.ReadWorkerSessionLogs(response, httptest.NewRequest(http.MethodGet, "/", nil), "worker", factoryapi.ReadWorkerSessionLogsParams{})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", response.Code, response.Body.String())
	}
	var result factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || result.Events[0].Event.CapturedAt == nil || !result.Events[0].Event.CapturedAt.Equal(stamp) || result.Events[1].Event.CapturedAt != nil {
		t.Fatalf("commit times lost: %+v", result.Events)
	}
	if strings.Count(response.Body.String(), `"capturedAt"`) != 1 {
		t.Fatal("legacy capturedAt must be absent")
	}
}

func TestCapturedLogsHTTPErrorMeanings(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name   string
		err    error
		status int
	}{
		{"cursor", workersessions.ErrInvalidLogsRequest, 400},
		{"unknown", workersessions.ErrObservationSessionNotFound, 404},
		{"recording", workersessions.ErrLogsUnavailable, 503},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			handler := NewHandler((&Adapter{}).WithLogsService(logsServiceFake{err: cell.err}), zap.NewNop())
			response := httptest.NewRecorder()
			handler.ReadWorkerSessionLogs(response, httptest.NewRequest(http.MethodGet, "/", nil), "worker", factoryapi.ReadWorkerSessionLogsParams{})
			if response.Code != cell.status {
				t.Fatalf("status = %d, want %d", response.Code, cell.status)
			}
		})
	}
}
