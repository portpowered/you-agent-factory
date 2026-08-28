package runtime_api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func commandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	if len(request.Args) > 0 {
		return request.Args[len(request.Args)-1]
	}
	return ""
}

func assertGeneratedEventsStreamHasCanonicalHistory(t *testing.T, baseURL string) {
	t.Helper()
	assertGeneratedEventsStreamHasCanonicalHistoryAt(t, support.DefaultSessionEventsURL(baseURL))
}

func assertGeneratedEventsStreamHasCanonicalHistoryAt(t *testing.T, endpoint string) {
	t.Helper()
	stream := openFactoryEventHTTPStream(t, endpoint)
	runRequest, initialStructure := requireFunctionalEventStreamPrelude(t, stream)
	assertFunctionalEventsUseCanonicalVocabulary(t, []factoryapi.FactoryEvent{runRequest, initialStructure},
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
	)
}

func assertGeneratedEventsStreamHasCanonicalHistoryForServer(t *testing.T, server *functionalAPIServer) {
	t.Helper()
	stream := server.openEventStream(t)
	runRequest, initialStructure := requireFunctionalEventStreamPrelude(t, stream)
	assertFunctionalEventsUseCanonicalVocabulary(t, []factoryapi.FactoryEvent{runRequest, initialStructure},
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
	)
}

func submitGeneratedWork(t *testing.T, baseURL string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	return submitGeneratedWorkAt(t, support.DefaultSessionWorkURL(baseURL, "/work"), req)
}

func submitGeneratedWorkAt(t *testing.T, endpoint string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	if req.Name == nil || *req.Name == "" {
		req.Name = stringPtr("generated-api-submit")
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
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

func stringPtr(value string) *string {
	return &value
}

func putGeneratedWorkRequest(t *testing.T, baseURL string, requestID string, req factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	return putGeneratedWorkRequestAt(
		t,
		support.DefaultSessionWorkURL(baseURL, "/work-requests/"+url.PathEscape(requestID)),
		req,
	)
}

func putGeneratedWorkRequestAt(t *testing.T, endpoint string, req factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated work request: %v", err)
	}
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
	return waitForGeneratedWorkAtEndpoint(t, support.DefaultSessionWorkURL(baseURL, "/work"), traceID, placeID, timeout)
}

func waitForGeneratedWorkAtEndpoint(t *testing.T, endpoint string, traceID string, placeID string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
	t.Fatalf("timed out waiting for trace %q at %s; last work response: %#v", traceID, placeID, work)
	return factoryapi.ListWorkResponse{}
}

func waitForGeneratedWorkIDsComplete(t *testing.T, baseURL string, workIDs []string, timeout time.Duration) []factoryapi.Work {
	t.Helper()
	return waitForGeneratedWorkIDsCompleteAtEndpoint(t, support.DefaultSessionWorkURL(baseURL, "/work"), workIDs, timeout)
}

func waitForGeneratedWorkIDsCompleteAtEndpoint(t *testing.T, endpoint string, workIDs []string, timeout time.Duration) []factoryapi.Work {
	t.Helper()
	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
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
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, endpoint)
	t.Fatalf("timed out waiting for completed work IDs %v; last work response: %#v", workIDs, work)
	return nil
}

func generatedWorkPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return stringPointerValue(work.WorkTypeName) + ":"
	}
	return stringPointerValue(work.WorkTypeName) + ":" + work.State.Name
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
