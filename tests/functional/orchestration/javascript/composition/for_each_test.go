// Package composition holds customer functional scenarios for JavaScript
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	forEachItemAlphaLabel  = "for-each-item-alpha"
	forEachItemBetaLabel   = "for-each-item-beta"
	forEachItemAlphaPrompt = "process alpha"
	forEachItemBetaPrompt  = "process beta"

	forEachCorrelateGammaLabel  = "for-each-correlate-gamma"
	forEachCorrelateDeltaLabel  = "for-each-correlate-delta"
	forEachCorrelateGammaPrompt = "correlate gamma input"
	forEachCorrelateDeltaPrompt = "correlate delta input"
	forEachCorrelationWorkflow  = `return (async function () {
  const items = ["delta", "gamma"];
  const pipelineItems = await pipeline(
    items,
    function (item, index) {
      if (item === "gamma") {
        return agent.run({
          prompt: "` + forEachCorrelateGammaPrompt + `",
          label: "` + forEachCorrelateGammaLabel + `",
        });
      }
      return agent.run({
        prompt: "` + forEachCorrelateDeltaPrompt + `",
        label: "` + forEachCorrelateDeltaLabel + `",
      });
    }
  );
  return {
    inputOrder: items,
    itemResults: pipelineItems,
  };
})();`

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

	forEachEmptyChildLabel    = "for-each-empty-child"
	forEachEmptyChildPrompt   = "should not dispatch"
	forEachEmptyInputWorkflow = `return (async function () {
  const items = [];
  const results = await pipeline(
    items,
    function (item, index) {
      return agent.run({
        prompt: "` + forEachEmptyChildPrompt + `",
        label: "` + forEachEmptyChildLabel + `",
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
func runJavaScriptForEachDispatchesEveryInputOnce(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldForEachCardinalityWorkflow(t)
	providerCalls := fixture.runner.callCount()
	started := startForEachCardinalityWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.runner.callCount(); got <= providerCalls {
		t.Fatalf("provider command call count = %d, want provider executions after baseline %d", got, providerCalls)
	}

	dispatches := listForEachCardinalityDispatches(t, fixture, started.SessionId)
	assertExactlyOneDispatchPerForEachInput(t, dispatches.Dispatches)
	assertForEachCardinalityPrimaryResult(t, started.Result)
}

// TestJavaScriptForEachPreservesInputResultCorrelation proves a JavaScript
// Factory that runs single-stage pipeline iteration keeps each child result
// correlated to its originating input identity on the public primary result
// and Factory Session dispatch listing surfaces, even when input order would
// otherwise make completion-order guessing unreliable.
func runJavaScriptForEachPreservesInputResultCorrelation(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldForEachCorrelationWorkflow(t)
	providerCalls := fixture.runner.callCount()
	started := startForEachCorrelationWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.runner.callCount(); got <= providerCalls {
		t.Fatalf("provider command call count = %d, want provider executions after baseline %d", got, providerCalls)
	}

	dispatches := listForEachCorrelationDispatches(t, fixture, started.SessionId)
	assertForEachInputResultCorrelation(t, dispatches.Dispatches, started.Result)
}

// TestJavaScriptForEachEmptyInputDoesNotDispatch proves a JavaScript Factory
// that runs single-stage pipeline iteration over an empty input collection
// completes without child dispatch on the public Factory Session dispatch
// listing and primary result surfaces, without unresolved hangs or private
// runtime leakage in public diagnostics.
func runJavaScriptForEachEmptyInputDoesNotDispatch(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldForEachEmptyInputWorkflow(t)
	providerCalls := fixture.runner.callCount()
	started := startForEachEmptyInputWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.runner.callCount(); got != providerCalls {
		t.Fatalf("provider command call count = %d, want no provider execution after baseline %d", got, providerCalls)
	}

	dispatches := listForEachEmptyInputDispatches(t, fixture, started.SessionId)
	assertNoForEachChildDispatches(t, dispatches.Dispatches)
	assertForEachEmptyInputPrimaryResult(t, started.Result)
}

func scaffoldForEachCardinalityWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-for-each-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(forEachCardinalityWorkflow), 0o600); err != nil {
		t.Fatalf("write for-each cardinality workflow: %v", err)
	}
	return dir
}

func scaffoldForEachCorrelationWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-for-each-correlation-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(forEachCorrelationWorkflow), 0o600); err != nil {
		t.Fatalf("write for-each correlation workflow: %v", err)
	}
	return dir
}

func scaffoldForEachEmptyInputWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-for-each-empty-input-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(forEachEmptyInputWorkflow), 0o600); err != nil {
		t.Fatalf("write for-each empty-input workflow: %v", err)
	}
	return dir
}

func startForEachCardinalityWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startPublicSync(t, "javascript-for-each-cardinality-composition", dir)
}

func startForEachCorrelationWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startPublicSync(t, "javascript-for-each-correlation-composition", dir)
}

func startForEachEmptyInputWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startPublicSync(t, "javascript-for-each-empty-input-composition", dir)
}

func listForEachCardinalityDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.publicDispatches(t, sessionID)
}

func listForEachCorrelationDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.publicDispatches(t, sessionID)
}

func listForEachEmptyInputDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.publicDispatches(t, sessionID)
}

func assertNoForEachChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 0 {
		t.Fatalf("dispatch count = %d, want 0 child dispatches for empty for-each input: %#v", len(dispatches), dispatchLabels(dispatches))
	}
}

func assertForEachEmptyInputPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.FailureDetail != nil {
		assertNoPrivateRuntimeLeakage(t, result.FailureDetail.Message)
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
		InputCount int             `json:"inputCount"`
		Results    json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode for-each empty-input primary result: %v", err)
	}
	if evidence.InputCount != 0 {
		t.Fatalf("inputCount = %d, want 0 for empty collection", evidence.InputCount)
	}
	var results []json.RawMessage
	if err := json.Unmarshal(evidence.Results, &results); err != nil {
		t.Fatalf("decode for-each empty-input pipeline results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("pipeline result count = %d, want empty results array", len(results))
	}
}

func assertNoPrivateRuntimeLeakage(t *testing.T, message string) {
	t.Helper()

	for _, leaked := range []string{"stack", "heap", "goja", "VM"} {
		if strings.Contains(message, leaked) {
			t.Fatalf("public diagnostic leaked non-customer detail %q: %q", leaked, message)
		}
	}
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
			*dispatch.Javascript.ExecutionMode != "live-provider" {
			t.Fatalf("dispatch %q javascript projection = %#v, want live-provider execution mode", label, dispatch.Javascript)
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

func assertForEachInputResultCorrelation(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	result *factoryapi.FactorySessionResult,
) {
	t.Helper()

	wantItems := []string{"delta", "gamma"}
	wantLabels := []string{forEachCorrelateDeltaLabel, forEachCorrelateGammaLabel}
	wantPrompts := []string{forEachCorrelateDeltaPrompt, forEachCorrelateGammaPrompt}

	if len(dispatches) != len(wantItems) {
		t.Fatalf("dispatch count = %d, want exactly %d child dispatches", len(dispatches), len(wantItems))
	}
	dispatchByLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		label := dispatchLabel(dispatch)
		if label == "" {
			t.Fatalf("dispatch %q missing label, want labeled child dispatches", dispatch.Id)
		}
		if _, exists := dispatchByLabel[label]; exists {
			t.Fatalf("dispatch label %q appears more than once: %#v", label, dispatchLabels(dispatches))
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %q status = %q, want COMPLETED", label, dispatch.Status)
		}
		dispatchByLabel[label] = dispatch
	}

	evidence := decodeForEachCorrelationPrimaryResult(t, result)
	if len(evidence.InputOrder) != len(wantItems) {
		t.Fatalf("inputOrder length = %d, want %d declared inputs", len(evidence.InputOrder), len(wantItems))
	}
	if len(evidence.ItemResults) != len(wantItems) {
		t.Fatalf("item result count = %d, want one entry per input", len(evidence.ItemResults))
	}

	for index, wantItem := range wantItems {
		if evidence.InputOrder[index] != wantItem {
			t.Fatalf("inputOrder[%d] = %q, want %q", index, evidence.InputOrder[index], wantItem)
		}

		itemResult := evidence.ItemResults[index]
		if itemResult.Index != index || itemResult.Item != wantItem || itemResult.Status != "COMPLETED" {
			t.Fatalf("itemResults[%d] = %#v, want completed item %q at index %d", index, itemResult, wantItem, index)
		}
		if len(itemResult.Stages) != 1 || itemResult.Stages[0].Status != "COMPLETED" {
			t.Fatalf("itemResults[%d] stages = %#v, want one completed stage", index, itemResult.Stages)
		}

		stageResult := itemResult.Stages[0].Result
		wantLabel := wantLabels[index]
		if stageResult.Label != wantLabel {
			t.Fatalf("itemResults[%d] stage label = %q, want %q", index, stageResult.Label, wantLabel)
		}
		if !strings.Contains(stageResult.Output.Text, wantPrompts[index]) {
			t.Fatalf(
				"itemResults[%d] stage output = %q, want child output containing prompt %q",
				index,
				stageResult.Output.Text,
				wantPrompts[index],
			)
		}
		if strings.TrimSpace(stageResult.DispatchID) == "" {
			t.Fatalf("itemResults[%d] stage dispatchId is empty, want completed child dispatch id", index)
		}

		dispatch, ok := dispatchByLabel[wantLabel]
		if !ok {
			t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), wantLabel)
		}
		if dispatch.Id != stageResult.DispatchID {
			t.Fatalf(
				"itemResults[%d] stage dispatchId = %q, want public dispatch listing id %q for label %q",
				index,
				stageResult.DispatchID,
				dispatch.Id,
				wantLabel,
			)
		}
	}
}

type forEachCorrelationEvidence struct {
	InputOrder  []string `json:"inputOrder"`
	ItemResults []struct {
		Index  int    `json:"index"`
		Item   string `json:"item"`
		Status string `json:"status"`
		Stages []struct {
			Index  int    `json:"index"`
			Status string `json:"status"`
			Result struct {
				Label      string `json:"label"`
				DispatchID string `json:"dispatchId"`
				Output     struct {
					Text string `json:"text"`
				} `json:"output"`
			} `json:"result"`
		} `json:"stages"`
	} `json:"itemResults"`
}

func decodeForEachCorrelationPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
) forEachCorrelationEvidence {
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
	var evidence forEachCorrelationEvidence
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode for-each correlation primary result: %v", err)
	}
	return evidence
}
