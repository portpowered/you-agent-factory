package runtime_api

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDriven_RESTAPISubmitAndQuery(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-driven runtime API submit/query smoke")

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

	const requestID = "request-config-driven-rest-submit"
	const workID = "work-config-driven-rest-submit"
	const workTypeName = "task"
	response := putGeneratedWorkRequest(t, host.Endpoint(), requestID, factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "rest-submit",
			WorkId:       stringPointer(workID),
			WorkTypeName: stringPointer(workTypeName),
			Payload:      map[string]string{"title": "REST submit"},
		}},
	})
	if response.RequestId != requestID || response.TraceId == "" {
		t.Fatalf("PUT /work-requests response = %#v, want request ID %q and trace ID", response, requestID)
	}

	event := waitForRuntimeAPIWorkRequestEvent(t, stream, requestID, 5*time.Second)
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode public WORK_REQUEST payload: %v", err)
	}
	works := support.FactoryWorksValue(payload.Works)
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || len(works) != 1 {
		t.Fatalf("public WORK_REQUEST payload = %#v, want one-work FACTORY_REQUEST_BATCH", payload)
	}
	if got := support.StringPointerValue(works[0].WorkId); got != workID {
		t.Fatalf("public WORK_REQUEST work ID = %q, want %q", got, workID)
	}

	completed := waitForGeneratedWorkIDsComplete(t, host.Endpoint(), []string{workID}, 10*time.Second)
	assertConfigDrivenSubmittedWork(t, completed[0], workID, workTypeName, response.TraceId)
}

func assertConfigDrivenSubmittedWork(
	t *testing.T,
	work factoryapi.Work,
	workID string,
	workTypeName string,
	traceID string,
) {
	t.Helper()

	if got := support.StringPointerValue(work.WorkId); got != workID {
		t.Fatalf("GET /work ID = %q, want %q", got, workID)
	}
	if got := support.StringPointerValue(work.WorkTypeName); got != workTypeName {
		t.Fatalf("GET /work work type = %q, want %q", got, workTypeName)
	}
	if got := support.StringPointerValue(work.TraceId); got != traceID {
		t.Fatalf("GET /work trace ID = %q, want %q", got, traceID)
	}
	if generatedWorkStateName(work.State) != "complete" || generatedWorkStateType(work.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", work.State)
	}
}
