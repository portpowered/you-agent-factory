package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSubmitRuntimeWork_EmitsCanonicalTraceAwareBatchEvent(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true)

	submitted := server.SubmitRuntimeWork(t, work.SubmitRequest{
		Name:                   "trace-aware-submit",
		WorkTypeID:             "task",
		CurrentChainingTraceID: "trace-request",
		TraceID:                "trace-request",
		Payload:                []byte(`{"title":"explicit current"}`),
	})
	requestID := submitted[0].RequestID

	event := waitForRuntimeAPIWorkRequestEvent(t, server, requestID, 5*time.Second)
	if got := support.StringPointerValue(event.Context.RequestId); got != requestID {
		t.Fatalf("WORK_REQUEST context request ID = %q, want %q", got, requestID)
	}

	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("WORK_REQUEST payload type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}

	works := support.FactoryWorksValue(payload.Works)
	if len(works) != 1 {
		t.Fatalf("WORK_REQUEST payload work count = %d, want 1", len(works))
	}
	if works[0].Name != "trace-aware-submit" {
		t.Fatalf("work name = %q, want trace-aware-submit", works[0].Name)
	}
	if got := support.StringPointerValue(works[0].CurrentChainingTraceId); got != "trace-request" {
		t.Fatalf("work current chaining trace ID = %q, want trace-request", got)
	}
	if got := support.StringPointerValue(works[0].TraceId); got != "trace-request" {
		t.Fatalf("work trace ID = %q, want trace-request", got)
	}

	waitForGeneratedWorkIDsComplete(t, server.URL(), []string{submitted[0].WorkID}, 10*time.Second)

	listed := server.ListWork(t)
	if len(listed.Results) != 1 ||
		support.StringPointerValue(listed.Results[0].CurrentChainingTraceId) != "trace-request" {
		t.Fatalf("public work projection = %#v, want chaining trace identity", listed.Results)
	}
}

func waitForRuntimeAPIWorkRequestEvent(
	t *testing.T,
	server *functionalAPIServer,
	requestID string,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := server.GetFactoryEvents(t)
		for _, event := range events {
			if event.Type == factoryapi.FactoryEventTypeWorkRequest &&
				support.StringPointerValue(event.Context.RequestId) == requestID {
				return event
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for WORK_REQUEST event for %q", requestID)
	return factoryapi.FactoryEvent{}
}
