package runtime_api

import (
	"context"
	"sort"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSubmitRuntimeWork_EmitsCanonicalTraceAwareBatchEvent(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	const requestID = "request-functional-runtime-trace-batch"
	workTypeName := "task"
	explicitWorkID := "work-runtime-explicit-current"
	explicitCurrentTraceID := "chain-request-current"
	legacyWorkID := "work-runtime-legacy-fallback"
	legacyTraceID := "trace-work-legacy"
	response := putGeneratedWorkRequest(t, host.Endpoint(), requestID, factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:                   "explicit-current",
				WorkId:                 &explicitWorkID,
				WorkTypeName:           &workTypeName,
				CurrentChainingTraceId: &explicitCurrentTraceID,
				TraceId:                &explicitCurrentTraceID,
				Payload:                map[string]string{"title": "explicit current"},
			},
			{
				Name:         "legacy-fallback",
				WorkId:       &legacyWorkID,
				WorkTypeName: &workTypeName,
				TraceId:      &legacyTraceID,
				Payload:      map[string]string{"title": "legacy fallback"},
			},
		},
	})
	if response.RequestId != requestID || len(response.Works) != 2 {
		t.Fatalf("PUT /work-requests response = %#v, want request ID %q and two works", response, requestID)
	}

	event := waitForRuntimeAPIWorkRequestEvent(t, stream, requestID, 5*time.Second)
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

	works := append([]factoryapi.Work(nil), support.FactoryWorksValue(payload.Works)...)
	if len(works) != 2 {
		t.Fatalf("WORK_REQUEST payload work count = %d, want 2", len(works))
	}
	sort.Slice(works, func(i, j int) bool { return works[i].Name < works[j].Name })

	explicit := works[0]
	if explicit.Name != "explicit-current" {
		t.Fatalf("first work name = %q, want explicit-current", explicit.Name)
	}
	if got := support.StringPointerValue(explicit.CurrentChainingTraceId); got != explicitCurrentTraceID {
		t.Fatalf("explicit work current chaining trace ID = %q, want %q", got, explicitCurrentTraceID)
	}
	if got := support.StringPointerValue(explicit.TraceId); got != explicitCurrentTraceID {
		t.Fatalf("explicit work trace ID = %q, want %q", got, explicitCurrentTraceID)
	}

	legacyFallback := works[1]
	if legacyFallback.Name != "legacy-fallback" {
		t.Fatalf("second work name = %q, want legacy-fallback", legacyFallback.Name)
	}
	if got := support.StringPointerValue(legacyFallback.CurrentChainingTraceId); got != legacyTraceID {
		t.Fatalf("legacy-fallback current chaining trace ID = %q, want %q", got, legacyTraceID)
	}
	if got := support.StringPointerValue(legacyFallback.TraceId); got != legacyTraceID {
		t.Fatalf("legacy-fallback trace ID = %q, want %q", got, legacyTraceID)
	}

	completed := waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{explicitWorkID, legacyWorkID}, 10*time.Second)
	assertRuntimeAPIWorkRead(t, completed[0], explicitCurrentTraceID)
	assertRuntimeAPIWorkRead(t, completed[1], legacyTraceID)
}

func waitForRuntimeAPIWorkRequestEvent(
	t *testing.T,
	stream *factoryEventHTTPStream,
	requestID string,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type == factoryapi.FactoryEventTypeWorkRequest &&
			support.StringPointerValue(event.Context.RequestId) == requestID {
			return event
		}
	}

	t.Fatalf("timed out waiting for WORK_REQUEST event for %q", requestID)
	return factoryapi.FactoryEvent{}
}

func assertRuntimeAPIWorkRead(t *testing.T, work factoryapi.Work, currentTraceID string) {
	t.Helper()

	if got := support.StringPointerValue(work.CurrentChainingTraceId); got != currentTraceID {
		t.Fatalf("GET /work current chaining trace ID = %q, want %q", got, currentTraceID)
	}
}
