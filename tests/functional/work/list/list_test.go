package list_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each parallel leaf owns its Factory Sessions and input directories. One
// root-built host serves all leaves; public completion responses advance reads.
// Existing CLI journey cells own mixed-state order/filter/supersession parity.
func TestWorkListReadBoundaries(t *testing.T) {
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: listFactory(t)})
	for _, scenario := range []struct {
		name string
		run  func(*testing.T, string)
	}{
		{"empty and absent cursor", testEmptyAndAbsentCursor},
		{"admission move and malformed cursor", testAdmissionMoveAndMalformedCursor},
		{"session isolation and unknown session", testSessionIsolation},
	} {
		t.Run(scenario.name, func(t *testing.T) { t.Parallel(); scenario.run(t, server.URL()) })
	}
}

func listFactory(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{"name": "list-boundaries", "workTypes": []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}}, "workers": []any{}, "workstations": []any{}})
}

func openListSession(t *testing.T, baseURL string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, listFactory(t))
	id := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, id) })
	return baseURL + "/factory-sessions/" + id
}

func testEmptyAndAbsentCursor(t *testing.T, baseURL string) {
	endpoint := openListSession(t, baseURL)
	assertEmptyList(t, listPage(t, endpoint, "counts=true"), true, 0)
	assertEmptyList(t, listPage(t, endpoint, ""), false, 0)
	submitListWork(t, endpoint, "one")
	assertEmptyList(t, listPage(t, endpoint, "counts=true&name=missing"), true, 0)
	assertEmptyList(t, listPage(t, endpoint, "name=missing"), false, 0)
	query := url.Values{"counts": {"true"}, "nextToken": {base64.StdEncoding.EncodeToString([]byte("absent"))}}
	assertEmptyList(t, listPage(t, endpoint, query.Encode()), true, 1)
}

func testAdmissionMoveAndMalformedCursor(t *testing.T, baseURL string) {
	endpoint := openListSession(t, baseURL)
	firstID := submitListWork(t, endpoint, "first")
	before := listPage(t, endpoint, "counts=true&includeSuperseded=true")
	submitListWork(t, endpoint, "second")
	body, _ := json.Marshal(factoryapi.MoveWorkRequest{StateName: "complete"})
	listRequest(t, http.MethodPost, endpoint+"/work/"+firstID+"/move", body, http.StatusOK)
	after := listPage(t, endpoint, "counts=true&includeSuperseded=true")
	if before.Counts == nil || before.Counts.Total != 1 || len(before.Results) != 1 || before.Results[0].State == nil || before.Results[0].State.Name != "init" {
		t.Fatal("prior response changed")
	}
	assertListIdentitySet(t, after, 2, firstID)
	for _, item := range after.Results {
		if item.WorkId != nil && *item.WorkId == firstID && (item.State == nil || item.State.Name != "complete") {
			t.Fatal("next read did not observe move")
		}
	}
	data := listRequest(t, http.MethodGet, endpoint+"/work?counts=true&includeSuperseded=true&nextToken=%25%25%25", nil, http.StatusBadRequest)
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(data, &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "BAD_REQUEST" || failure.Family != "BAD_REQUEST" || failure.Message != "nextToken is invalid" {
		t.Fatalf("malformed cursor: %#v", failure)
	}
	assertListIdentitySet(t, listPage(t, endpoint, "counts=true&includeSuperseded=true"), 2, firstID)
}

func testSessionIsolation(t *testing.T, baseURL string) {
	first, second := openListSession(t, baseURL), openListSession(t, baseURL)
	firstID, secondID := submitListWork(t, first, "first"), submitListWork(t, second, "second")
	for _, cell := range []struct{ endpoint, id string }{{first, firstID}, {second, secondID}} {
		t.Run(cell.id, func(t *testing.T) {
			t.Parallel()
			assertListIdentitySet(t, listPage(t, cell.endpoint, "counts=true"), 1, cell.id)
		})
	}
	data := listRequest(t, http.MethodGet, baseURL+"/factory-sessions/"+uuid.NewString()+"/work", nil, http.StatusNotFound)
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(data, &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "NOT_FOUND" || failure.Family != "NOT_FOUND" {
		t.Fatalf("unknown session: %#v", failure)
	}
}

func submitListWork(t *testing.T, endpoint, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "workTypeName": "task", "payload": map[string]string{"message": "synthetic"}})
	data := listRequest(t, http.MethodPost, endpoint+"/work", body, http.StatusCreated)
	var result factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.WorkId == nil {
		t.Fatalf("admission: %#v", result)
	}
	return *result.WorkId
}

func listPage(t *testing.T, endpoint, query string) factoryapi.ListWorkResponse {
	t.Helper()
	data := listRequest(t, http.MethodGet, endpoint+"/work?"+query, nil, http.StatusOK)
	var result factoryapi.ListWorkResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func listRequest(t *testing.T, method, endpoint string, body []byte, wantStatus int) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s HTTP %d want %d: %s", method, response.StatusCode, wantStatus, data)
	}
	return data
}

func assertEmptyList(t *testing.T, result factoryapi.ListWorkResponse, counts bool, total int) {
	t.Helper()
	if len(result.Results) != 0 || result.PaginationContext == nil || result.PaginationContext.NextToken != nil {
		t.Fatalf("empty pagination: %#v", result)
	}
	if counts {
		if result.Counts == nil || result.Counts.Total != total {
			t.Fatalf("empty count: %#v", result.Counts)
		}
	} else if result.Counts != nil {
		t.Fatal("unrequested counts returned")
	}
}

func assertListIdentitySet(t *testing.T, result factoryapi.ListWorkResponse, total int, requiredID string) {
	t.Helper()
	if result.Counts == nil || result.Counts.Total != total || len(result.Results) != total {
		t.Fatalf("cardinality: %#v", result)
	}
	found := false
	for _, item := range result.Results {
		if item.WorkId != nil && *item.WorkId == requiredID {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing selected-session Work %q", requiredID)
	}
}
