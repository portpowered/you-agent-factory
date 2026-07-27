package runtime_api_test

import (
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBlockedDispatchConcurrentBatchIngressRegression proves accepted batch ingress
// stays HTTP-observable (WORK_REQUEST plus Work list/get) while an unrelated
// dispatch remains blocked, and same-request-ID replay stays idempotent.
func TestBlockedDispatchConcurrentBatchIngressRegression(t *testing.T) {
	t.Helper()

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	dispatchRelease := make(chan struct{})
	server := StartFunctionalServer(
		t,
		dir,
		false,
		withProvider(&serviceModeBlockingProvider{release: dispatchRelease}),
	)

	traceID := server.SubmitWork(t, "task", []byte(`{"title":"blocked dispatch for batch ingress regression"}`))
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}
	waitForPublicFactorySession(t, server, 10*time.Second, serviceModeSessionActive)

	const (
		requestID = "request-http-batch-ingress-regression"
		workID    = "work-http-batch-ingress-regression"
	)
	workTypeName := "task"
	workIDPtr := workID
	traceIDPtr := "trace-http-batch-ingress-regression"
	batchRequest := factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "http-ingress-regression",
			WorkId:       &workIDPtr,
			WorkTypeName: &workTypeName,
			TraceId:      &traceIDPtr,
			Payload:      map[string]string{"title": "concurrent batch ingress regression"},
		}},
	}

	first := support.UpsertDefaultSessionWorkRequest(t, server.URL(), batchRequest)
	if first.RequestId != requestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", first.RequestId, requestID)
	}
	if len(first.Works) != 1 || first.Works[0].WorkId != workID {
		t.Fatalf("PUT /work-requests works = %#v, want one work with id %q", first.Works, workID)
	}

	support.AssertSingleWorkRequestEvent(t, server.GetFactoryEvents(t), requestID, workID, workTypeName)
	assertPublicWorkListAndGetVisible(t, server, workID)

	replayed := support.UpsertDefaultSessionWorkRequest(t, server.URL(), batchRequest)
	if replayed.RequestId != first.RequestId || replayed.TraceId != first.TraceId {
		t.Fatalf("idempotent PUT identity changed: first=%#v replay=%#v", first, replayed)
	}
	support.AssertSingleWorkRequestEvent(t, server.GetFactoryEvents(t), requestID, workID, workTypeName)

	select {
	case <-dispatchRelease:
		t.Fatal("blocked dispatch released before ingress regression assertions finished")
	default:
	}

	close(dispatchRelease)
	server.Stop(t)
}

func assertPublicWorkListAndGetVisible(t *testing.T, server *FunctionalServer, workID string) {
	t.Helper()

	listed := server.ListWork(t)
	if !publicWorkListingContainsID(listed, workID) {
		t.Fatalf("work %q missing from public Work list before blocked dispatch completed; listed=%#v", workID, listed.Results)
	}

	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work/"+workID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200 while blocked dispatch continues", endpoint, response.StatusCode)
	}

	work := support.GetJSON[factoryapi.Work](t, endpoint)
	if support.StringPointerValue(work.WorkId) != workID {
		t.Fatalf("GET /work/%s workId = %q, want %q", workID, support.StringPointerValue(work.WorkId), workID)
	}
}

func publicWorkListingContainsID(listed factoryapi.ListWorkResponse, workID string) bool {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return true
		}
	}
	return false
}
