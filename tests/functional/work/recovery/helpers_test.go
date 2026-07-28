package recovery

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startRecoveryAPIServer(
	t *testing.T,
	factoryDir string,
	provider workerprovider.Provider,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            false,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
}

func postMoveWork(t *testing.T, baseURL, workID, stateName string) factoryapi.Work {
	t.Helper()

	body, err := json.Marshal(factoryapi.MoveWorkRequest{StateName: stateName})
	if err != nil {
		t.Fatalf("marshal move request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work/"+workID+"/move")
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, resp.StatusCode, string(payload))
	}
	var work factoryapi.Work
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		t.Fatalf("decode move response: %v", err)
	}
	return work
}

func waitForWorkIDsAtState(
	t *testing.T,
	baseURL string,
	workIDs []string,
	stateName string,
	timeout time.Duration,
) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := 0
		for _, item := range listed.Results {
			workID := support.StringPointerValue(item.WorkId)
			if want[workID] && workStateName(item.State) == stateName {
				found++
			}
		}
		if found == len(want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf("timed out waiting for work IDs %v at state %q; last listing: %#v", workIDs, stateName, listed.Results)
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
	t.Fatalf("timed out waiting for work IDs %v to complete; last listing: %#v", workIDs, listed.Results)
	return nil
}

func workStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func stringPtr(value string) *string {
	return &value
}
