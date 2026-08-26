package submission_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	httpPayloadSizeRequestID         = "work-http-payload-size-rejection"
	httpPayloadSizeValidWorkID       = "work-http-payload-size-valid"
	httpPayloadSizeOversizedWorkID   = "work-http-payload-size-oversized"
	httpPayloadSizeValidWorkName     = "http-payload-size-valid"
	httpPayloadSizeOversizedName     = "http-payload-size-oversized"
	httpPayloadSizeBoundaryRequestID = "work-http-payload-size-boundary"
	httpPayloadSizeBoundaryWorkID    = "work-http-payload-size-boundary-id"
	httpPayloadSizeBoundaryWorkName  = "http-payload-size-boundary"
)

// TestAPIBatchUpsertRejectsOversizedWorkAtomically proves the public batch HTTP
// contract preserves the Work admission diagnostic and emits no request or
// dispatch observation for a mixed batch that contains an oversized Work.
func TestAPIBatchUpsertRejectsOversizedWorkAtomically(t *testing.T) {
	t.Parallel()
	server := startPayloadSizeHTTPServer(t)
	defer server.Stop(t)

	baseline := support.ListDefaultSessionWork(t, server.URL())
	body := marshalHTTPBatch(t, httpPayloadSizeRequestID, []map[string]any{
		httpBatchWork(t, httpPayloadSizeValidWorkName, httpPayloadSizeValidWorkID, json.RawMessage(`{"title":"valid sibling"}`)),
		httpBatchWork(t, httpPayloadSizeOversizedName, httpPayloadSizeOversizedWorkID, workPayloadJSONOfSize(t, 65537)),
	})

	status, responseBody := putHTTPBatch(t, server.URL(), httpPayloadSizeRequestID, body)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT oversized Work batch status = %d, want 400: %s", status, responseBody)
	}

	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		t.Fatalf("decode oversized Work batch error: %v\nbody: %s", err, responseBody)
	}
	if response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("oversized Work batch error = %#v, want BAD_REQUEST family and code", response)
	}
	for _, marker := range []string{
		"work_request:",
		`Work "http-payload-size-oversized"`,
		"payloadBytes=65537",
		"payloadLimitBytes=65536",
	} {
		if !strings.Contains(response.Message, marker) {
			t.Fatalf("oversized Work batch message = %q, want marker %q", response.Message, marker)
		}
	}
	if strings.Contains(response.Message, "valid sibling") || strings.Contains(response.Message, "data") {
		t.Fatalf("oversized Work batch message = %q, must not expose payload content", response.Message)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	if len(listed.Results) != len(baseline.Results) {
		t.Fatalf("public Work count after rejected batch = %d, want baseline %d", len(listed.Results), len(baseline.Results))
	}
	for _, item := range listed.Results {
		if item.Name == httpPayloadSizeValidWorkName || item.Name == httpPayloadSizeOversizedName {
			t.Fatalf("rejected batch Work appeared in public list: %#v", item)
		}
	}
	assertHTTPBatchHasNoPublicObservations(t, server, httpPayloadSizeRequestID, httpPayloadSizeValidWorkID, httpPayloadSizeOversizedWorkID)
}

// TestAPIBatchUpsertAcceptsPayloadAtInclusiveLimit proves a compact JSON Work
// payload of exactly 65,536 bytes reaches the public session-scoped Work and
// Factory Event observations.
func TestAPIBatchUpsertAcceptsPayloadAtInclusiveLimit(t *testing.T) {
	t.Parallel()
	server := startPayloadSizeHTTPServer(t)
	defer server.Stop(t)

	body := marshalHTTPBatch(t, httpPayloadSizeBoundaryRequestID, []map[string]any{
		httpBatchWork(t, httpPayloadSizeBoundaryWorkName, httpPayloadSizeBoundaryWorkID, workPayloadJSONOfSize(t, 65536)),
	})
	status, responseBody := putHTTPBatch(t, server.URL(), httpPayloadSizeBoundaryRequestID, body)
	if status != http.StatusCreated {
		t.Fatalf("PUT at-limit Work batch status = %d, want 201: %s", status, responseBody)
	}

	var response factoryapi.UpsertWorkRequestResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		t.Fatalf("decode at-limit Work batch response: %v\nbody: %s", err, responseBody)
	}
	if response.RequestId != httpPayloadSizeBoundaryRequestID ||
		len(response.Works) != 1 || response.Works[0].WorkId != httpPayloadSizeBoundaryWorkID {
		t.Fatalf("at-limit Work batch response = %#v, want accepted boundary Work", response)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	item, ok := findListedWorkByNameAndID(listed, httpPayloadSizeBoundaryWorkName, httpPayloadSizeBoundaryWorkID)
	if !ok {
		t.Fatalf("at-limit Work missing from public session Work list: %#v", listed.Results)
	}
	if support.StringPointerValue(item.WorkId) != httpPayloadSizeBoundaryWorkID {
		t.Fatalf("at-limit public Work ID = %q, want %q", support.StringPointerValue(item.WorkId), httpPayloadSizeBoundaryWorkID)
	}
	support.AssertSingleWorkRequestEvent(
		t,
		server.GetFactoryEvents(t),
		httpPayloadSizeBoundaryRequestID,
		httpPayloadSizeBoundaryWorkID,
		batchInputsWorkType,
	)
}

func startPayloadSizeHTTPServer(t *testing.T) *support.FunctionalAPIServer {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, payloadSizeHTTPFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "provider-worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("payload size HTTP dispatch COMPLETE"),
	})
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
}

func payloadSizeHTTPFactoryConfig() map[string]any {
	config := batchInputsFactoryConfig()
	config["workers"] = []map[string]string{{"name": "provider-worker"}}
	workstations := config["workstations"].([]map[string]any)
	workstations[0]["worker"] = "provider-worker"
	config["workstations"] = workstations
	return config
}

func httpBatchWork(t *testing.T, name, workID string, payload json.RawMessage) map[string]any {
	t.Helper()
	return map[string]any{
		"name":         name,
		"workId":       workID,
		"workTypeName": batchInputsWorkType,
		"payload":      payload,
	}
}

func marshalHTTPBatch(t *testing.T, requestID string, works []map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works":     works,
	})
	if err != nil {
		t.Fatalf("marshal HTTP Work batch: %v", err)
	}
	return body
}

func putHTTPBatch(t *testing.T, baseURL, requestID string, body []byte) (int, []byte) {
	t.Helper()
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work-requests/"+requestID)
	request, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build PUT %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read PUT %s response: %v", endpoint, err)
	}
	return response.StatusCode, responseBody
}

func workPayloadJSONOfSize(t *testing.T, size int) json.RawMessage {
	t.Helper()
	const prefix = `{"data":"`
	const suffix = `"}`
	valueSize := size - len(prefix) - len(suffix)
	if valueSize < 0 {
		t.Fatalf("payload size %d is too small for test JSON envelope", size)
	}
	payload := []byte(prefix + strings.Repeat("x", valueSize) + suffix)
	if len(payload) != size || !json.Valid(payload) {
		t.Fatalf("constructed payload length = %d, valid = %t, want %d", len(payload), json.Valid(payload), size)
	}
	return json.RawMessage(payload)
}

func assertHTTPBatchHasNoPublicObservations(
	t *testing.T,
	server *support.FunctionalAPIServer,
	requestID string,
	workIDs ...string,
) {
	t.Helper()
	for _, event := range server.GetFactoryEvents(t) {
		if support.StringPointerValue(event.Context.RequestId) == requestID {
			t.Fatalf("rejected HTTP batch emitted public event: %#v", event)
		}
		for _, workID := range workIDs {
			if event.Context.WorkIds == nil {
				continue
			}
			for _, eventWorkID := range *event.Context.WorkIds {
				if eventWorkID == workID {
					t.Fatalf("rejected HTTP batch emitted public event for Work %q: %#v", workID, event)
				}
			}
		}
	}
}
