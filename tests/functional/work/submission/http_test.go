package submission_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
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

// TestAPIBatchUpsertAcceptsWorksContent proves batch upsert through PUT
// /work-requests accepts canonical works content and projects ordered content
// parts. Its top-level name is retained because the functional-evidence
// registry owns this public endpoint coverage identity.
func TestAPIBatchUpsertAcceptsWorksContent(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, submissionInputPreservingFactoryConfig())
	configureSubmissionCodexWorkers(t, factoryDir, "worker-a")
	server := support.StartFunctionalAPIServer(t, submissionServerConfig(factoryDir, submissionInputPreservingProviderRunner()))
	defer server.Stop(t)

	assertAPIBatchUpsertAcceptsWorksContent(t, server)
	functionalevidence.Covers(t, "rest/upsertWorkRequestBySessionId")
}

// assertAPIPOSTSubmitAndQueryWork proves REST POST /work submission and GET
// /work query expose the submitted Work through the public HTTP surface after
// completion.
func assertAPIPOSTSubmitAndQueryWork(t *testing.T, server *support.FunctionalAPIServer) {
	submitted := postWorkViaRESTAPI(t, server.URL())
	assertListedWorkCompleteTask(t, server.URL(), submitted.TraceId)
}

// assertAPIBatchUpsertAcceptsWorksContent proves batch upsert through PUT
// /work-requests accepts canonical works content and projects ordered content
// parts.
func assertAPIBatchUpsertAcceptsWorksContent(t *testing.T, server *support.FunctionalAPIServer) {
	const (
		workID    = "work-http-batch-content"
		requestID = "request-http-batch-content"
	)
	body, err := json.Marshal(map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "content-batch-work",
			"workId":       workID,
			"workTypeName": "task",
			"content": []map[string]any{
				{"type": "text", "text": "Batch canonical content."},
				{"type": "text", "text": "Second batch part."},
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(
		server.URL(),
		"/work-requests/"+url.PathEscape(requestID),
	)
	httpReq, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build PUT /work-requests request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("PUT /work-requests: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", resp.StatusCode, payload)
	}
	var upserted factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&upserted); err != nil {
		t.Fatalf("decode work request response: %v", err)
	}
	if upserted.RequestId != requestID || upserted.TraceId == "" {
		t.Fatalf("PUT /work-requests response = %#v, want request id and trace id", upserted)
	}
	if len(upserted.Works) != 1 || upserted.Works[0].WorkId != workID {
		t.Fatalf("PUT /work-requests works = %#v, want one accepted work with id %q", upserted.Works, workID)
	}

	items := waitForWorkIDsComplete(t, server.URL(), []string{workID}, 10*time.Second)
	content := items[0].Content
	if content == nil || len(*content) != 2 {
		t.Fatalf("GET /work content = %#v, want two ordered batch content parts", content)
	}
	firstPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode first batch content part: %v", err)
	}
	secondPart, err := (*content)[1].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode second batch content part: %v", err)
	}
	if firstPart.Text != "Batch canonical content." || secondPart.Text != "Second batch part." {
		t.Fatalf("GET /work batch content = %#v, want ordered batch text parts", content)
	}
}

// assertCLIWorkTypeNameReachesLiveAPIHandler proves CLI submit with an
// explicit work type name reaches the live public HTTP handler and completes
// the Work.
func assertCLIWorkTypeNameReachesLiveAPIHandler(
	t *testing.T,
	server *support.FunctionalAPIServer,
	factoryDir string,
) {
	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("ship name based CLI submit"), 0o644); err != nil {
		t.Fatalf("write CLI submit payload: %v", err)
	}

	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server", functionalServerBaseURL(t, server.URL()),
		"submit",
		"--name", "  cli-live-api-name  ",
		"--work-type-name", "task",
		"--payload", payloadPath,
	})
	inputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(you submit --work-type-name) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	item := waitForWorkByNameComplete(t, server.URL(), "cli-live-api-name", "task", 10*time.Second)
	if item.Name != "cli-live-api-name" {
		t.Fatalf("CLI-submitted work name = %q, want cli-live-api-name", item.Name)
	}
	if support.StringPointerValue(item.WorkTypeName) != "task" ||
		workStateName(item.State) != "complete" {
		t.Fatalf("CLI-submitted work = %#v, want task in complete state", item)
	}
}

// assertAPISubmitBatchThenListAndGetWork proves a successful public HTTP batch
// Work Request submission makes the submitted Work visible through list and
// get endpoints so automation can submit once and inspect the resulting Work
// identity and payload without a second transport.
func assertAPISubmitBatchThenListAndGetWork(t *testing.T, server *support.FunctionalAPIServer) {
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

// assertAPIUpsertWorkRequestUsesCanonicalIdentity proves repeat upserts of the
// same logical Work Request through the public HTTP API keep canonical Work
// Request and Work identities so retries and idempotent clients do not create
// divergent identities for the same upsert key.
func assertAPIUpsertWorkRequestUsesCanonicalIdentity(t *testing.T, server *support.FunctionalAPIServer) {
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

// assertAPIUnknownWorkReturnsTypedNotFound proves GET for a Work identity that
// does not exist in the running Factory Session returns a typed not-found public
// error outcome (structured 404 with NOT_FOUND family/code) rather than an
// opaque 500 or unstructured failure body.
func assertAPIUnknownWorkReturnsTypedNotFound(t *testing.T, server *support.FunctionalAPIServer) {
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

func postWorkViaRESTAPI(t *testing.T, baseURL string) factoryapi.SubmitWorkResponse {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+support.DefaultSessionWorkPath("/work"),
		bytes.NewBufferString(`{"name":"rest-submit","workTypeName": "task", "payload": {"title": "REST submit"}}`),
	)
	if err != nil {
		t.Fatalf("build POST /work request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Errorf("POST /work: expected status 201, got %d", response.StatusCode)
	}

	var submitResp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitResp); err != nil {
		t.Fatalf("POST /work: failed to decode response: %v", err)
	}
	if submitResp.TraceId == "" {
		t.Error("POST /work: expected non-empty trace_id")
	}
	return submitResp
}

func assertListedWorkCompleteTask(t *testing.T, baseURL, traceID string) {
	t.Helper()

	listResp := waitForWorkByTraceComplete(t, baseURL, traceID, 10*time.Second)
	work := requireWorkByTrace(t, listResp, traceID)
	if support.StringPointerValue(work.WorkTypeName) != "task" {
		t.Errorf("GET /work: expected work type 'task', got %q", support.StringPointerValue(work.WorkTypeName))
	}
	if workStateName(work.State) != "complete" ||
		work.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Errorf("GET /work: expected state complete/TERMINAL, got %#v", work.State)
	}
}

func stringPtr(value string) *string {
	return &value
}
