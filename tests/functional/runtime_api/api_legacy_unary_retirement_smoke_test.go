package runtime_api

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestLegacyUnaryRetirementSmoke_DirectRESTSubmitPathsStayBatchOnly(t *testing.T) {
	support.SkipLongFunctional(t, "slow legacy unary retirement boundary smoke")

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

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "direct post canonical submit"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}
	assertTerminalDispatchForTrace(t, stream, traceID)

	workTypeName := "task"
	workID := "work-retired-unary-put"
	request := factoryapi.WorkRequest{
		RequestId: "request-retired-unary-put",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "idempotent-put",
			WorkId:       &workID,
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "idempotent put canonical submit"},
		}},
	}
	first := putGeneratedWorkRequest(t, host.Endpoint(), request.RequestId, request)
	retry := putGeneratedWorkRequest(t, host.Endpoint(), request.RequestId, request)
	if retry.TraceId != first.TraceId {
		t.Fatalf("idempotent PUT trace_id changed: first=%q retry=%q", first.TraceId, retry.TraceId)
	}
	event := waitForRuntimeAPIWorkRequestEvent(t, stream, request.RequestId, 5*time.Second)
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode public WORK_REQUEST payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("public WORK_REQUEST type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}
	works := support.FactoryWorksValue(payload.Works)
	if len(works) != 1 || support.StringPointerValue(works[0].WorkId) != workID {
		t.Fatalf("public WORK_REQUEST works = %#v, want one work ID %q", works, workID)
	}
	assertTerminalDispatchForTrace(t, stream, first.TraceId)

	workList := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work"))
	assertLegacyUnaryCompletedWork(t, requireGeneratedWorkByTrace(t, workList, traceID), traceID)
	assertLegacyUnaryCompletedWork(t, requireGeneratedWorkByTrace(t, workList, first.TraceId), first.TraceId)
}

func assertLegacyUnaryCompletedWork(t *testing.T, item factoryapi.Work, traceID string) {
	t.Helper()

	if got := support.StringPointerValue(item.TraceId); got != traceID {
		t.Fatalf("GET /work trace ID = %q, want %q", got, traceID)
	}
	if generatedWorkStateName(item.State) != "complete" || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", item.State)
	}
}
