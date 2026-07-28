package submission_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func simplePipelineFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func competingPipelineFactoryConfig() map[string]any {
	config := simplePipelineFactoryConfig()
	config["workers"] = []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}}
	config["workstations"] = append(config["workstations"].([]map[string]any), map[string]any{
		"name":      "process-alternate",
		"worker":    "worker-b",
		"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	return config
}

func postSubmitWork(t *testing.T, baseURL string, body []byte) factoryapi.SubmitWorkResponse {
	t.Helper()

	endpoint := support.DefaultSessionWorkURL(baseURL, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 201: %s", endpoint, response.StatusCode, payload)
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return submitted
}

func postSubmitWorkExpectStatus(t *testing.T, baseURL string, body []byte, wantStatus int) {
	t.Helper()

	endpoint := support.DefaultSessionWorkURL(baseURL, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want %d: %s", endpoint, response.StatusCode, wantStatus, payload)
	}
}

func waitForWorkByTraceAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		for _, item := range listed.Results {
			if support.StringPointerValue(item.TraceId) == traceID &&
				workCustomerPlaceID(item) == placeID {
				return listed
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"timed out waiting for trace %q at %s; last work response: %#v",
		traceID,
		placeID,
		listed.Results,
	)
	return factoryapi.ListWorkResponse{}
}

func waitForWorkByTraceComplete(
	t *testing.T,
	baseURL string,
	traceID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()
	return waitForWorkByTraceAtPlace(t, baseURL, traceID, "task:complete", timeout)
}

func waitForWorkIDsComplete(
	t *testing.T,
	baseURL string,
	workIDs []string,
	timeout time.Duration,
) []factoryapi.Work {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := make(map[string]factoryapi.Work, len(want))
		for _, item := range listed.Results {
			workID := support.StringPointerValue(item.WorkId)
			if want[workID] && workStateName(item.State) == "complete" {
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
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"timed out waiting for completed work IDs %v; last work response: %#v",
		workIDs,
		listed.Results,
	)
	return nil
}

func waitForWorkTypeComplete(
	t *testing.T,
	baseURL string,
	workType string,
	timeout time.Duration,
) factoryapi.Work {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		for _, item := range listed.Results {
			if support.StringPointerValue(item.WorkTypeName) == workType &&
				workStateName(item.State) == "complete" {
				return item
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"timed out waiting for completed work type %q; last work response: %#v",
		workType,
		listed.Results,
	)
	return factoryapi.Work{}
}

func requireWorkByTrace(t *testing.T, listed factoryapi.ListWorkResponse, traceID string) factoryapi.Work {
	t.Helper()

	for _, item := range listed.Results {
		if support.StringPointerValue(item.TraceId) == traceID {
			return item
		}
	}
	t.Fatalf("trace %q missing from work list: %#v", traceID, listed.Results)
	return factoryapi.Work{}
}

func workStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func workCustomerPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return support.StringPointerValue(work.WorkTypeName) + ":"
	}
	return support.StringPointerValue(work.WorkTypeName) + ":" + work.State.Name
}

func functionalServerBaseURL(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse functional server URL %q: %v", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("functional server URL %q missing scheme or host", rawURL)
	}
	return strings.TrimSuffix(rawURL, "/")
}
