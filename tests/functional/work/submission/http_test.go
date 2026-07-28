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

	httpUpsertCanonicalRequestID = "work-http-upsert-canonical"
	httpUpsertCanonicalWorkName  = "http-upsert-canonical-task"
	httpUpsertCanonicalWorkID    = "work-http-upsert-canonical-id"
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

// TestAPIUpsertWorkRequestUsesCanonicalIdentity proves repeat upserts of the
// same logical Work Request through the public HTTP API keep canonical Work
// Request and Work identities so retries and idempotent clients do not create
// divergent identities for the same upsert key.
func TestAPIUpsertWorkRequestUsesCanonicalIdentity(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	workTypeName := batchInputsWorkType
	batchRequest := factoryapi.WorkRequest{
		RequestId: httpUpsertCanonicalRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         httpUpsertCanonicalWorkName,
			WorkId:       stringPtr(httpUpsertCanonicalWorkID),
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "HTTP upsert canonical identity"},
		}},
	}

	first := support.UpsertDefaultSessionWorkRequest(t, server.URL(), batchRequest)
	if first.RequestId != httpUpsertCanonicalRequestID {
		t.Fatalf("first PUT /work-requests requestId = %q, want %q", first.RequestId, httpUpsertCanonicalRequestID)
	}
	if first.TraceId == "" {
		t.Fatalf("first PUT /work-requests traceId is empty, want customer-visible trace identity")
	}
	if len(first.Works) != 1 || first.Works[0].WorkId != httpUpsertCanonicalWorkID {
		t.Fatalf(
			"first PUT /work-requests works = %#v, want one work with id %q",
			first.Works,
			httpUpsertCanonicalWorkID,
		)
	}

	second := support.UpsertDefaultSessionWorkRequest(t, server.URL(), batchRequest)
	if second.RequestId != first.RequestId {
		t.Fatalf(
			"repeat PUT /work-requests requestId = %q, want canonical %q",
			second.RequestId,
			first.RequestId,
		)
	}
	if second.TraceId != first.TraceId {
		t.Fatalf(
			"repeat PUT /work-requests traceId = %q, want canonical %q",
			second.TraceId,
			first.TraceId,
		)
	}
	if len(second.Works) != 1 || second.Works[0].WorkId != first.Works[0].WorkId {
		t.Fatalf(
			"repeat PUT /work-requests works = %#v, want canonical work id %q",
			second.Works,
			first.Works[0].WorkId,
		)
	}

	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work/"+httpUpsertCanonicalWorkID)
	got := support.GetJSON[factoryapi.Work](t, endpoint)
	if support.StringPointerValue(got.WorkId) != httpUpsertCanonicalWorkID {
		t.Fatalf(
			"GET /work/%s workId = %q, want canonical %q",
			httpUpsertCanonicalWorkID,
			support.StringPointerValue(got.WorkId),
			httpUpsertCanonicalWorkID,
		)
	}
	if got.Name != httpUpsertCanonicalWorkName {
		t.Fatalf(
			"GET /work/%s name = %q, want %q",
			httpUpsertCanonicalWorkID,
			got.Name,
			httpUpsertCanonicalWorkName,
		)
	}
}

func stringPtr(value string) *string {
	return &value
}
