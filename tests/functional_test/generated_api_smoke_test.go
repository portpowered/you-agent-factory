package functional_test

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
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	submitcli "github.com/portpowered/agent-factory/pkg/cli/submit"
	"github.com/portpowered/agent-factory/pkg/factory"
)

var retiredFunctionalFactoryEventTypes = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

func TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "generated API integration smoke",
		},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace_id")
	}

	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	token := work.Results[0]
	if token.TraceId != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", token.TraceId, traceID)
	}
	if token.WorkType != "task" {
		t.Fatalf("GET /work work_type = %q, want task", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Fatalf("GET /work place_id = %q, want task:complete", token.PlaceId)
	}

	workDetail := getGeneratedJSON[factoryapi.TokenResponse](t, server.URL()+"/work/"+url.PathEscape(token.Id))
	if workDetail.Id != token.Id {
		t.Fatalf("GET /work/{id} id = %q, want %q", workDetail.Id, token.Id)
	}
	if workDetail.History == nil {
		t.Fatal("GET /work/{id} omitted generated token history")
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
	dir := scaffoldFactory(t, simplePipelineConfig())
	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())

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

	token := waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	if token.WorkType != "task" {
		t.Fatalf("CLI-submitted work_type = %q, want task", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Fatalf("CLI-submitted place_id = %q, want task:complete", token.PlaceId)
	}
}

func TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())

	firstWorkID := "work-generated-api-batch-first"
	secondWorkID := "work-generated-api-batch-second"
	requiredState := "complete"
	workTypeName := "task"
	request := factoryapi.WorkRequest{
		RequestId: "request-generated-api-batch",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "first",
				WorkId:       &firstWorkID,
				WorkTypeName: &workTypeName,
				Payload:      map[string]string{"step": "first"},
			},
			{
				Name:         "second",
				WorkId:       &secondWorkID,
				WorkTypeName: &workTypeName,
				Payload:      map[string]string{"step": "second"},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "second",
			TargetWorkName: "first",
			RequiredState:  &requiredState,
		}},
	}

	resp := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
	if resp.RequestId != request.RequestId {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", resp.RequestId, request.RequestId)
	}
	if resp.TraceId == "" {
		t.Fatal("PUT /work-requests returned empty trace_id")
	}

	tokens := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{firstWorkID, secondWorkID}, 10*time.Second)
	for _, token := range tokens {
		if token.WorkType != "task" {
			t.Fatalf("generated batch token %s work_type = %q, want task", token.Id, token.WorkType)
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
			if token.Color.WorkTypeID != "task" {
				t.Fatalf("first runtime WorkTypeID = %q, want task", token.Color.WorkTypeID)
			}
		case secondWorkID:
			secondSeen = true
			if token.Color.WorkTypeID != "task" {
				t.Fatalf("second runtime WorkTypeID = %q, want task", token.Color.WorkTypeID)
			}
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
		for _, token := range work.Results {
			if token.TraceId == traceID && token.PlaceId == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
}

func waitForGeneratedWorkTypeComplete(t *testing.T, baseURL string, workType string, timeout time.Duration) factoryapi.TokenResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, token := range work.Results {
			if token.WorkType == workType && token.PlaceId == workType+":complete" {
				return token
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work type %q; last work response: %#v", workType, work)
	return factoryapi.TokenResponse{}
}

func waitForGeneratedWorkIDsComplete(t *testing.T, baseURL string, workIDs []string, timeout time.Duration) []factoryapi.TokenResponse {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		found := make(map[string]factoryapi.TokenResponse, len(want))
		for _, token := range work.Results {
			if want[token.WorkId] && strings.HasSuffix(token.PlaceId, ":complete") {
				found[token.WorkId] = token
			}
		}
		if len(found) == len(want) {
			tokens := make([]factoryapi.TokenResponse, 0, len(workIDs))
			for _, workID := range workIDs {
				tokens = append(tokens, found[workID])
			}
			return tokens
		}
		time.Sleep(100 * time.Millisecond)
	}

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work IDs %v; last work response: %#v", workIDs, work)
	return nil
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
