package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDriven_RESTAPISubmitAndQuery(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-driven runtime API submit/query smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	defer server.Stop(t)

	postWorkViaAPI(t, server.URL())
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	assertListWorkResponse(t, server.URL())
}

func postWorkViaAPI(t *testing.T, baseURL string) {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+support.DefaultSessionWorkPath("/work"),
		bytes.NewBufferString(`{"name":"rest-submit","workTypeName": "task", "payload": {"title": "REST submit"}}`),
	)
	if err != nil {
		t.Fatalf("build POST /work request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Errorf("POST /work: expected status 201, got %d", response.StatusCode)
	}

	var submitResp factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitResp); err != nil {
		t.Fatalf("POST /work: failed to decode response: %v", err)
	}
	if submitResp.TraceId == "" {
		t.Error("POST /work: expected non-empty trace_id")
	}
}

func assertListWorkResponse(t *testing.T, baseURL string) {
	t.Helper()

	listResp := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+support.DefaultSessionWorkPath("/work"),
	)
	if len(listResp.Results) != 1 {
		t.Fatalf("GET /work: expected 1 result, got %d", len(listResp.Results))
	}

	work := listResp.Results[0]
	if stringPointerValue(work.WorkTypeName) != "task" {
		t.Errorf("GET /work: expected work type 'task', got %q", stringPointerValue(work.WorkTypeName))
	}
	if generatedWorkStateName(work.State) != "complete" || generatedWorkStateType(work.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Errorf("GET /work: expected state complete/TERMINAL, got %#v", work.State)
	}
}
