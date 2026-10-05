package customer_journeys_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestRESTResourceCapacityChangesPreserveRevisionAndIdempotency(t *testing.T) {
	t.Parallel()
	config := liveRuntimePipelineConfig()
	config["resources"] = []map[string]any{{"id": "reviewers", "name": "Reviewers", "capacity": 1}}
	dir := support.ScaffoldFactory(t, config)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir, UseMockWorkers: true})
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	sessionID := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), sessionID) })
	endpoint := server.URL() + "/factory-sessions/" + sessionID + "/resources/reviewers/capacity"

	first := postCustomerCapacity(t, endpoint, "raise", 0, 2, http.StatusOK)
	if first.SessionId != sessionID || first.ResourceId != "reviewers" || first.EffectiveCapacity != 2 || first.Revision != 1 || first.ChangeId == "" {
		t.Fatalf("capacity change = %#v, want owned resource capacity 2 at revision 1", first)
	}
	replayed := postCustomerCapacity(t, endpoint, "raise", 0, 2, http.StatusOK)
	if replayed.ChangeId != first.ChangeId || replayed.Revision != first.Revision || replayed.EffectiveCapacity != first.EffectiveCapacity {
		t.Fatalf("repeated request = %#v, want original decision %#v", replayed, first)
	}
	postCustomerCapacity(t, endpoint, "raise", 0, 3, http.StatusConflict)
	postCustomerCapacity(t, endpoint, "stale", 0, 3, http.StatusConflict)
	unchanged := postCustomerCapacity(t, endpoint, "unchanged", 1, 2, http.StatusOK)
	if unchanged.EffectiveCapacity != 2 || unchanged.Revision != 1 {
		t.Fatalf("unchanged capacity = %#v, want capacity 2 without a revision increment", unchanged)
	}
	postCustomerCapacity(t, server.URL()+"/factory-sessions/"+sessionID+"/resources/missing/capacity", "missing", 1, 2, http.StatusNotFound)
	final := postCustomerCapacity(t, endpoint, "next", 1, 3, http.StatusOK)
	if final.EffectiveCapacity != 3 || final.Revision != 2 {
		t.Fatalf("next valid change = %#v, want capacity 3 at revision 2", final)
	}
	functionalevidence.Covers(t, "rest/setFactorySessionResourceCapacity")
}

func postCustomerCapacity(t *testing.T, endpoint, requestID string, revision, capacity, wantStatus int) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"requestId": requestID, "expectedRevision": revision, "capacity": capacity})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("capacity request %q status=%d, want %d: %s", requestID, response.StatusCode, wantStatus, body)
	}
	var result factoryapi.FactorySessionResourceCapacityResponse
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
