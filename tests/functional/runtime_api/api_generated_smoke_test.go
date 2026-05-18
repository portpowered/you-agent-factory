package runtime_api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated API and live runtime alignment smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated API integration smoke"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace_id")
	}

	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	item := work.Results[0]
	if stringPointerValue(item.TraceId) != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", stringPointerValue(item.TraceId), traceID)
	}
	if stringPointerValue(item.WorkTypeName) != "task" {
		t.Fatalf("GET /work work type = %q, want task", stringPointerValue(item.WorkTypeName))
	}
	if generatedWorkStateName(item.State) != "complete" || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", item.State)
	}

	statusRead := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if statusRead.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", statusRead.TotalTokens)
	}
	if statusRead.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", statusRead.Categories.Terminal)
	}

	assertGeneratedEventsStreamHasCanonicalHistory(t, server.URL())
}

func TestGeneratedAPIIntegrationSmoke_CLIWorkTypeNameReachesLiveAPIHandler(t *testing.T) {
	support.SkipLongFunctional(t, "slow CLI submit generated API smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("ship name based CLI submit"), 0o644); err != nil {
		t.Fatalf("write CLI submit payload: %v", err)
	}

	if err := submitcli.Submit(submitcli.SubmitConfig{
		WorkTypeName: "task",
		Payload:      payloadPath,
		Port:         functionalServerPort(t, server.URL()),
	}); err != nil {
		t.Fatalf("agent-factory submit --work-type-name: %v", err)
	}

	item := waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	if stringPointerValue(item.WorkTypeName) != "task" || generatedWorkStateName(item.State) != "complete" {
		t.Fatalf("CLI-submitted work = %#v, want task in complete state", item)
	}
}

func TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow batch generated API normalization smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	firstWorkID := "work-generated-api-batch-first"
	secondWorkID := "work-generated-api-batch-second"
	requiredState := "complete"
	workTypeName := "task"
	request := factoryapi.WorkRequest{
		RequestId: "request-generated-api-batch",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{Name: "first", WorkId: &firstWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "first"}},
			{Name: "second", WorkId: &secondWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "second"}},
		},
		Relations: &[]factoryapi.Relation{{Type: factoryapi.RelationTypeDependsOn, SourceWorkName: "second", TargetWorkName: "first", RequiredState: &requiredState}},
	}

	resp := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
	if resp.RequestId != request.RequestId || resp.TraceId == "" {
		t.Fatalf("PUT /work-requests response = %#v, want request id and trace id", resp)
	}

	items := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{firstWorkID, secondWorkID}, 10*time.Second)
	for _, item := range items {
		if stringPointerValue(item.WorkTypeName) != "task" {
			t.Fatalf("generated batch work %s work type = %q, want task", stringPointerValue(item.WorkId), stringPointerValue(item.WorkTypeName))
		}
	}

	snapshot := server.GetEngineStateSnapshot(t)
	var firstSeen bool
	var secondSeen bool
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		switch token.Color.WorkID {
		case firstWorkID:
			firstSeen = true
		case secondWorkID:
			secondSeen = true
			if len(token.Color.Relations) != 1 {
				t.Fatalf("second runtime relations = %d, want 1", len(token.Color.Relations))
			}
			relation := token.Color.Relations[0]
			if relation.TargetWorkID != firstWorkID || relation.RequiredState != "complete" {
				t.Fatalf("second runtime relation = %#v, want dependency on first completion", relation)
			}
		}
	}
	if !firstSeen || !secondSeen {
		t.Fatalf("runtime snapshot missing generated API batch tokens: first=%v second=%v", firstSeen, secondSeen)
	}
}

func assertGeneratedEventsStreamHasCanonicalHistory(t *testing.T, baseURL string) {
	t.Helper()
	stream := openFactoryEventHTTPStream(t, baseURL+"/events")
	runRequest, initialStructure := requireFunctionalEventStreamPrelude(t, stream)
	assertFunctionalEventsUseCanonicalVocabulary(t, []factoryapi.FactoryEvent{runRequest, initialStructure},
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
	)
}

func submitGeneratedWork(t *testing.T, baseURL string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(baseURL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201", resp.StatusCode)
	}
	var out factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated submit response: %v", err)
	}
	return out.TraceId
}

func putGeneratedWorkRequest(t *testing.T, baseURL string, requestID string, req factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated work request: %v", err)
	}
	endpoint := baseURL + "/work-requests/" + url.PathEscape(requestID)
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
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var out factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated work request response: %v", err)
	}
	return out
}

func getGeneratedJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", endpoint, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s as %T: %v", endpoint, out, err)
	}
	return out
}

func waitForGeneratedWorkComplete(t *testing.T, baseURL string, traceID string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	return waitForGeneratedWorkAtPlace(t, baseURL, traceID, "task:complete", timeout)
}

func waitForGeneratedWorkAtPlace(t *testing.T, baseURL string, traceID string, placeID string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
}

func waitForGeneratedWorkTypeComplete(t *testing.T, baseURL string, workType string, timeout time.Duration) factoryapi.Work {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, item := range work.Results {
			if stringPointerValue(item.WorkTypeName) == workType && generatedWorkStateName(item.State) == "complete" {
				return item
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work type %q; last work response: %#v", workType, work)
	return factoryapi.Work{}
}

func waitForGeneratedWorkIDsComplete(t *testing.T, baseURL string, workIDs []string, timeout time.Duration) []factoryapi.Work {
	t.Helper()
	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		found := make(map[string]factoryapi.Work, len(want))
		for _, item := range work.Results {
			workID := stringPointerValue(item.WorkId)
			if want[workID] && generatedWorkStateName(item.State) == "complete" {
				found[workID] = item
			}
		}
		if len(found) == len(want) {
			items := make([]factoryapi.Work, 0, len(workIDs))
			for _, workID := range workIDs {
				items = append(items, found[workID])
			}
			return items
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work IDs %v; last work response: %#v", workIDs, work)
	return nil
}

func generatedWorkPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return stringPointerValue(work.WorkTypeName) + ":"
	}
	return stringPointerValue(work.WorkTypeName) + ":" + work.State.Name
}

func functionalServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse functional server URL %q: %v", rawURL, err)
	}
	_, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("parse functional server host %q: %v", parsed.Host, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse functional server port %q: %v", portText, err)
	}
	return port
}

func assertFunctionalEventsUseCanonicalVocabulary(t *testing.T, events []factoryapi.FactoryEvent, required ...factoryapi.FactoryEventType) {
	t.Helper()
	seen := make(map[factoryapi.FactoryEventType]int, len(events))
	for _, event := range events {
		seen[event.Type]++
		for _, retired := range retiredFunctionalFactoryEventTypes {
			if string(event.Type) == retired {
				t.Fatalf("event %s reintroduced retired public event type %q", event.Id, retired)
			}
		}
	}
	for _, eventType := range required {
		if seen[eventType] == 0 {
			t.Fatalf("events %v missing canonical event type %s", functionalEventTypes(events), eventType)
		}
	}
}
