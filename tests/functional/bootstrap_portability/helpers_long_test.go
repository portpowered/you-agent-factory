//go:build functionallong

package bootstrap_portability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func (fs *functionalAPIServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()

	req := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeID,
		Payload:      payload,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}

	resp, err := http.Post(fs.httpSrv.URL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

func (fs *functionalAPIServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
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
		t.Fatalf("decode %s: %v", endpoint, err)
	}
	return out
}

func waitForGeneratedWorkAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
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
