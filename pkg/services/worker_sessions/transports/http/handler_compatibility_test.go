package http

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartWorkerSessionAcceptsUnknownFieldsWithWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	service := &fakeObservationService{}
	handler := NewHandler(NewAdapterWithStart(service, service, workServiceStub{}), zap.New(core))
	recorder := httptest.NewRecorder()
	body := `{"requestId":"request-1","workerSessionId":"worker-1","futureRoot":"secret-root","execution":{"workstationName":"swe","futureExecution":{"value":"secret-execution"},"dispatch":{"dispatchId":"dispatch-1","workstationName":"swe","futureDispatch":true}}}`

	handler.StartWorkerSession(recorder, httptest.NewRequest(http.MethodPost, "/worker-sessions", strings.NewReader(body)))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", recorder.Code, recorder.Body.String())
	}
	assertKnownStartWorkerSessionFields(t, service)
	assertCompatibilityWarningHeader(t, recorder)
	assertCompatibilityWarningLog(t, logs, recorder)
}

func assertKnownStartWorkerSessionFields(t *testing.T, service *fakeObservationService) {
	t.Helper()
	if !service.startCalled || service.startRequest.RequestID != "request-1" ||
		service.startRequest.Execution.WorkstationName != "swe" ||
		service.startRequest.Execution.Execution.Dispatch.DispatchID != "dispatch-1" {
		t.Fatalf("start request = %#v, want known fields preserved", service.startRequest)
	}
}

func assertCompatibilityWarningHeader(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	warning := recorder.Header().Get("Warning")
	for _, path := range []string{"$.execution.dispatch.futureDispatch", "$.execution.futureExecution", "$.futureRoot"} {
		if !strings.Contains(warning, path) {
			t.Fatalf("Warning = %q, want %s", warning, path)
		}
	}
}

func assertCompatibilityWarningLog(t *testing.T, logs *observer.ObservedLogs, recorder *httptest.ResponseRecorder) {
	t.Helper()
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) ||
		fields["boundary"] != "worker_sessions.http" ||
		fields["operation"] != "start_worker_session" {
		t.Fatalf("warning fields = %#v, want HTTP compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{
		"$.execution.dispatch.futureDispatch", "$.execution.futureExecution", "$.futureRoot",
	}) {
		t.Fatalf("json_paths = %#v, want sorted ignored paths", fields["json_paths"])
	}
	if strings.Contains(entries[0].Message, "secret") || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("compatibility diagnostics exposed an ignored field value")
	}
}
