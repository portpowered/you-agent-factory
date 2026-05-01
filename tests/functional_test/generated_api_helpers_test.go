package functional_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

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
