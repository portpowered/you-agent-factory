package submission_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	httpSubmitListGetRequestID = "work-http-submit-list-get"
	httpSubmitListGetWorkName  = "http-submit-list-get-task"
	httpSubmitListGetWorkID    = "work-http-submit-list-get-id"
)

// TestAPISubmitBatchThenListAndGetWork proves a successful public HTTP batch
// Work Request submission makes the submitted Work visible through list and get
// endpoints so automation can submit once and inspect the resulting Work identity
// and payload without a second transport.
func TestAPISubmitBatchThenListAndGetWork(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	workTypeName := batchInputsWorkType
	submitted := support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: httpSubmitListGetRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         httpSubmitListGetWorkName,
			WorkId:       stringPtr(httpSubmitListGetWorkID),
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "HTTP submit list get round-trip"},
		}},
	})

	if submitted.RequestId != httpSubmitListGetRequestID {
		t.Fatalf("PUT /work-requests requestId = %q, want %q", submitted.RequestId, httpSubmitListGetRequestID)
	}
	if submitted.TraceId == "" {
		t.Fatalf("PUT /work-requests traceId is empty, want customer-visible trace identity")
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkId != httpSubmitListGetWorkID {
		t.Fatalf(
			"PUT /work-requests works = %#v, want one work with id %q",
			submitted.Works,
			httpSubmitListGetWorkID,
		)
	}
	if submitted.Works[0].Name != httpSubmitListGetWorkName {
		t.Fatalf(
			"PUT /work-requests work name = %q, want %q",
			submitted.Works[0].Name,
			httpSubmitListGetWorkName,
		)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	item, ok := findListedWorkByNameAndID(listed, httpSubmitListGetWorkName, httpSubmitListGetWorkID)
	if !ok {
		t.Fatalf(
			"GET /work list missing submitted work name=%q workId=%q: %#v",
			httpSubmitListGetWorkName,
			httpSubmitListGetWorkID,
			listed.Results,
		)
	}
	if support.StringPointerValue(item.WorkTypeName) != batchInputsWorkType {
		t.Fatalf(
			"GET /work list workTypeName = %q, want %q for name=%q workId=%q",
			support.StringPointerValue(item.WorkTypeName),
			batchInputsWorkType,
			httpSubmitListGetWorkName,
			httpSubmitListGetWorkID,
		)
	}

	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work/"+httpSubmitListGetWorkID)
	got := support.GetJSON[factoryapi.Work](t, endpoint)
	if support.StringPointerValue(got.WorkId) != httpSubmitListGetWorkID {
		t.Fatalf(
			"GET /work/%s workId = %q, want %q",
			httpSubmitListGetWorkID,
			support.StringPointerValue(got.WorkId),
			httpSubmitListGetWorkID,
		)
	}
	if got.Name != httpSubmitListGetWorkName {
		t.Fatalf("GET /work/%s name = %q, want %q", httpSubmitListGetWorkID, got.Name, httpSubmitListGetWorkName)
	}
	if support.StringPointerValue(got.WorkTypeName) != batchInputsWorkType {
		t.Fatalf(
			"GET /work/%s workTypeName = %q, want %q",
			httpSubmitListGetWorkID,
			support.StringPointerValue(got.WorkTypeName),
			batchInputsWorkType,
		)
	}
}

func stringPtr(value string) *string {
	return &value
}
