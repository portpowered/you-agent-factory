// Package composition holds customer functional scenarios for JavaScript
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	forEachItemAlphaLabel  = "for-each-item-alpha"
	forEachItemBetaLabel   = "for-each-item-beta"
	forEachItemAlphaPrompt = "process alpha"
	forEachItemBetaPrompt  = "process beta"
	forEachCardinalityWorkflow = `return (async function () {
  const items = ["alpha", "beta"];
  const results = await pipeline(
    items,
    function (item, index) {
      if (item === "alpha") {
        return agent.run({
          prompt: "` + forEachItemAlphaPrompt + `",
          label: "` + forEachItemAlphaLabel + `",
        });
      }
      return agent.run({
        prompt: "` + forEachItemBetaPrompt + `",
        label: "` + forEachItemBetaLabel + `",
      });
    }
  );
  return {
    inputCount: items.length,
    results: results,
  };
})();`
)

// TestJavaScriptForEachDispatchesEveryInputOnce proves a JavaScript Factory
// that runs single-stage pipeline iteration over a non-empty input collection
// dispatches exactly one child per input, observable on the public Factory
// Session dispatch listing and primary result surfaces without inspecting
// private runtime state.
func TestJavaScriptForEachDispatchesEveryInputOnce(t *testing.T) {
	t.Parallel()

	dir := scaffoldForEachCardinalityWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startForEachCardinalityWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listForEachCardinalityDispatches(t, server.URL(), started.SessionId)
	assertExactlyOneDispatchPerForEachInput(t, dispatches.Dispatches)
	assertForEachCardinalityPrimaryResult(t, started.Result)
}

func scaffoldForEachCardinalityWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-for-each-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(forEachCardinalityWorkflow), 0o600); err != nil {
		t.Fatalf("write for-each cardinality workflow: %v", err)
	}
	return dir
}

func startForEachCardinalityWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-for-each-cardinality-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal for-each cardinality workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build for-each cardinality workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start for-each cardinality workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start for-each cardinality workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode for-each cardinality workflow response: %v", err)
	}
	return started
}

func listForEachCardinalityDispatches(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
}

func assertExactlyOneDispatchPerForEachInput(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	wantLabels := []string{forEachItemAlphaLabel, forEachItemBetaLabel}
	if len(dispatches) != len(wantLabels) {
		t.Fatalf("dispatch count = %d, want exactly %d child dispatches (one per input)", len(dispatches), len(wantLabels))
	}

	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		label := dispatchLabel(dispatch)
		if label == "" {
			t.Fatalf("dispatch %q missing label, want labeled child dispatches", dispatch.Id)
		}
		if _, exists := byLabel[label]; exists {
			t.Fatalf("dispatch label %q appears more than once: %#v", label, dispatchLabels(dispatches))
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %q status = %q, want COMPLETED", label, dispatch.Status)
		}
		if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
			*dispatch.Javascript.ExecutionMode != "fake" {
			t.Fatalf("dispatch %q javascript projection = %#v, want fake execution mode", label, dispatch.Javascript)
		}
		byLabel[label] = dispatch
	}

	for _, wantLabel := range wantLabels {
		if _, ok := byLabel[wantLabel]; !ok {
			t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), wantLabel)
		}
	}
}

func assertForEachCardinalityPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode primary result JSON: %v", err)
	}
	var evidence struct {
		InputCount int `json:"inputCount"`
		Results    []struct {
			Index  int    `json:"index"`
			Item   string `json:"item"`
			Status string `json:"status"`
			Stages []struct {
				Index  int    `json:"index"`
				Status string `json:"status"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode for-each cardinality primary result: %v", err)
	}
	if evidence.InputCount != 2 {
		t.Fatalf("inputCount = %d, want 2 distinct inputs", evidence.InputCount)
	}
	if len(evidence.Results) != 2 {
		t.Fatalf("pipeline item count = %d, want exactly one result per input", len(evidence.Results))
	}
	wantItems := []string{"alpha", "beta"}
	for index, itemResult := range evidence.Results {
		if itemResult.Index != index || itemResult.Status != "COMPLETED" {
			t.Fatalf("pipeline item[%d] = %#v, want completed item at index %d", index, itemResult, index)
		}
		if itemResult.Item != wantItems[index] {
			t.Fatalf("pipeline item[%d].item = %q, want %q", index, itemResult.Item, wantItems[index])
		}
		if len(itemResult.Stages) != 1 {
			t.Fatalf("pipeline item[%d] stage count = %d, want exactly 1 per-item stage", index, len(itemResult.Stages))
		}
		stage := itemResult.Stages[0]
		if stage.Index != 0 || stage.Status != "COMPLETED" {
			t.Fatalf("pipeline item[%d] stage[0] = %#v, want one completed stage", index, stage)
		}
	}
}
