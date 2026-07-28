package submission_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
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

	httpUnknownWorkID = "work-http-unknown-missing-id"
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

// TestAPIUnknownWorkReturnsTypedNotFound proves GET for a Work identity that
// does not exist in the running Factory Session returns a typed not-found public
// error outcome (structured 404 with NOT_FOUND family/code) rather than an opaque
// 500 or unstructured failure body.
func TestAPIUnknownWorkReturnsTypedNotFound(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work/"+httpUnknownWorkID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	assertAPIUnknownWorkTypedNotFoundHTTPResponse(t, response, httpUnknownWorkID)
}

func assertAPIUnknownWorkTypedNotFoundHTTPResponse(
	t *testing.T,
	response *http.Response,
	workID string,
) {
	t.Helper()

	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET unknown Work %q status = %d, want 404: %s",
			workID,
			response.StatusCode,
			payload,
		)
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET unknown Work %q Content-Type = %q, want application/json structured error body: %s",
			workID,
			contentType,
			payload,
		)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("GET unknown Work %q read body: %v", workID, err)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("GET unknown Work %q decode structured error: %v\nbody: %s", workID, err, body)
	}
	if errResp.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf(
			"GET unknown Work %q error family = %q, want %q: %#v",
			workID,
			errResp.Family,
			factoryapi.ErrorFamilyNotFound,
			errResp,
		)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf(
			"GET unknown Work %q error code = %q, want %q: %#v",
			workID,
			errResp.Code,
			factoryapi.ErrorResponseCodeNOTFOUND,
			errResp,
		)
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatalf(
			"GET unknown Work %q error message is empty, want customer-readable not-found text: %#v",
			workID,
			errResp,
		)
	}
	if !strings.Contains(strings.ToLower(errResp.Message), "not found") {
		t.Fatalf(
			"GET unknown Work %q error message = %q, want not-found guidance: %#v",
			workID,
			errResp.Message,
			errResp,
		)
	}

	var shown factoryapi.Work
	if json.Unmarshal(body, &shown) == nil && strings.TrimSpace(shown.Name) != "" {
		t.Fatalf("GET unknown Work %q must not emit a success Work payload: %#v", workID, shown)
	}
}

func stringPtr(value string) *string {
	return &value
}
