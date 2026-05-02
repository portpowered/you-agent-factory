package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/api"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestConfigDriven_RESTAPISubmitAndQuery(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow config-driven runtime API submit/query smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "API test"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)
	h.Assert().HasTokenInPlace("task:complete").TokenCount(1)

	snap := h.Marking()
	mockFactory := &testutil.MockFactory{Marking: snap}
	srv := api.NewServer(mockFactory, 0, zap.NewNop())

	postWorkViaAPI(t, srv)
	assertListWorkResponse(t, srv)
}

func postWorkViaAPI(t *testing.T, srv *api.Server) {
	t.Helper()

	req := httptest.NewRequest("POST", "/work", bytes.NewBufferString(`{"workTypeName": "task", "payload": {"title": "REST submit"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("POST /work: expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var submitResp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&submitResp); err != nil {
		t.Fatalf("POST /work: failed to decode response: %v", err)
	}
	if submitResp.TraceId == "" {
		t.Error("POST /work: expected non-empty trace_id")
	}
}

func assertListWorkResponse(t *testing.T, srv *api.Server) {
	t.Helper()

	req := httptest.NewRequest("GET", "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work: expected status 200, got %d", rec.Code)
	}

	var listResp factoryapi.ListWorkResponse
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("GET /work: failed to decode response: %v", err)
	}
	if len(listResp.Results) != 1 {
		t.Fatalf("GET /work: expected 1 result, got %d", len(listResp.Results))
	}

	token := listResp.Results[0]
	if token.WorkType != "task" {
		t.Errorf("GET /work: expected work_type 'task', got %q", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Errorf("GET /work: expected place_id 'task:complete', got %q", token.PlaceId)
	}
}
