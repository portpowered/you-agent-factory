package functional_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/replay"
)

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

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()

	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewBufferString(`{"workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func writeWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func projectReplayInitialStructureFromEmbeddedConfig(t *testing.T, dir string) interfaces.InitialStructurePayload {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generatedFactory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	mapper := factoryconfig.ConfigMapper{}
	replayNet, err := mapper.Map(context.Background(), replayRuntimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	return projections.ProjectInitialStructure(replayNet, replayRuntimeCfg)
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

func assertGeneratedEventsStreamHasCanonicalHistory(t *testing.T, baseURL string) {
	t.Helper()

	stream := openFactoryEventHTTPStream(t, baseURL+"/events")
	runRequest, initialStructure := requireFunctionalEventStreamPrelude(t, stream)
	assertFunctionalEventsUseCanonicalVocabulary(t, []factoryapi.FactoryEvent{runRequest, initialStructure},
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
	)
}

var retiredFunctionalFactoryEventTypes = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}
