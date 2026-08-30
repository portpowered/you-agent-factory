package composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parallelConcurrentDispatchWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-alpha" },
    { prompt: "summarize beta", label: "child-beta" },
  ]);
  return { results };
})();`

const parallelDeclaredResultOrderingWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "child-alpha", label: "child-alpha" },
    { prompt: "child-beta", label: "child-beta" },
    { prompt: "child-gamma", label: "child-gamma" },
  ]);
  return { results };
})();`

var parallelDeclaredResultOrderingLabels = []string{
	"child-alpha",
	"child-beta",
	"child-gamma",
}

const parallelPartialFailureWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "child-alpha", label: "child-alpha" },
    { prompt: "child-beta and force provider failure", label: "child-beta" },
    { prompt: "child-gamma", label: "child-gamma" },
  ]);
  return { results };
})();`

// TestJavaScriptParallelDispatchesChildrenConcurrently proves JavaScript parallel
// keeps more than one external child call in flight at the same time through the
// public Factory Session and dispatch surfaces, using controllable provider edges
// instead of wall-clock sleeps to observe concurrency.
func runJavaScriptParallelDispatchesChildrenConcurrently(t *testing.T, fixture *compositionFixture) {
	fixture.runner.beginConcurrentCase()
	defer fixture.runner.releaseConcurrent()

	started := startParallelCompositionWorkflowAsync(
		t,
		fixture,
		"parallel-composition-concurrent-dispatch",
		parallelConcurrentDispatchWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionID),
	)
	defer responseStream.Close()

	waitContext, cancel := context.WithTimeout(t.Context(), compositionFixtureTimeout)
	defer cancel()
	if err := fixture.runner.waitForConcurrent(waitContext); err != nil {
		t.Fatalf("wait for concurrent provider command calls: %v", err)
	}

	fixture.runner.releaseConcurrent()
	if err := fixture.runner.waitForConcurrentCompletion(waitContext); err != nil {
		t.Fatalf("wait for concurrent provider command completion: %v", err)
	}
	responseStream.WaitClosed(compositionFixtureTimeout)
	completed := readParallelCompositionSession(t, fixture.baseURL, sessionID)
	if completed.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded ||
		completed.ResultSummary == nil || completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session = %#v, want SUCCEEDED with FINAL result", completed)
	}

	dispatches := fixture.publicDispatches(t, sessionID)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 public child dispatches", len(dispatches.Dispatches))
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
	}
	if fixture.runner.peakActive() < 2 {
		t.Fatalf("provider peak active child calls = %d, want at least 2 concurrent external calls", fixture.runner.peakActive())
	}
}

// TestJavaScriptParallelPreservesDeclaredResultOrdering proves JavaScript parallel
// returns child results in declared input order on the public Factory Session result
// surface even when controllable external edges complete children in a different order.
func runJavaScriptParallelPreservesDeclaredResultOrdering(t *testing.T, fixture *compositionFixture) {
	fixture.runner.beginOrderingCase(parallelDeclaredResultOrderingLabels)
	defer func() {
		for _, label := range parallelDeclaredResultOrderingLabels {
			fixture.runner.releaseLabel(label)
		}
	}()

	started := startParallelCompositionWorkflowAsync(
		t,
		fixture,
		"parallel-composition-declared-ordering",
		parallelDeclaredResultOrderingWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionID),
	)
	defer responseStream.Close()

	waitContext, cancel := context.WithTimeout(t.Context(), compositionFixtureTimeout)
	defer cancel()
	if err := fixture.runner.waitForOrderingStarted(waitContext); err != nil {
		t.Fatalf("wait for ordered provider command calls: %v", err)
	}
	for _, label := range []string{"child-gamma", "child-beta", "child-alpha"} {
		fixture.runner.releaseLabel(label)
		if err := fixture.runner.waitForLabel(waitContext, label); err != nil {
			t.Fatalf("wait for provider label %q: %v", label, err)
		}
	}
	responseStream.WaitClosed(compositionFixtureTimeout)

	completed := readParallelCompositionSession(t, fixture.baseURL, sessionID)
	if completed.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded ||
		completed.ResultSummary == nil || completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session = %#v, want SUCCEEDED with FINAL result", completed)
	}

	wantCompletionOrder := []string{"child-gamma", "child-beta", "child-alpha"}
	if got := fixture.runner.completionOrder(); !reflect.DeepEqual(got, wantCompletionOrder) {
		t.Fatalf("provider completion order = %v, want %v", got, wantCompletionOrder)
	}

	resultPayload := readParallelCompositionFinalResult(t, fixture.baseURL, sessionID)
	assertParallelCompositionResultLabels(t, resultPayload, parallelDeclaredResultOrderingLabels)

	dispatches := fixture.publicDispatches(t, sessionID)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 3 public child dispatches", len(dispatches.Dispatches))
	}
	assertParallelCompositionDispatchLabels(t, dispatches.Dispatches, parallelDeclaredResultOrderingLabels)
}

// TestJavaScriptParallelPartialFailureUsesDocumentedPolicy proves JavaScript parallel
// surfaces one failed child as an explicit failed result with a stable diagnostic while
// successful siblings still complete, matching the documented partial-failure policy rather
// than aborting the whole workflow call.
func runJavaScriptParallelPartialFailureUsesDocumentedPolicy(t *testing.T, fixture *compositionFixture) {
	baselineCalls := fixture.runner.callCount()
	started := startParallelCompositionWorkflowAsync(
		t,
		fixture,
		"parallel-composition-partial-failure",
		parallelPartialFailureWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionID),
	)
	defer responseStream.Close()

	waitContext, cancel := context.WithTimeout(t.Context(), compositionFixtureTimeout)
	defer cancel()
	if err := fixture.runner.waitForCallCount(waitContext, baselineCalls+3); err != nil {
		t.Fatalf("wait for partial-failure provider command calls: %v", err)
	}
	responseStream.WaitClosed(compositionFixtureTimeout)
	completed := readParallelCompositionSession(t, fixture.baseURL, sessionID)
	if completed.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded ||
		completed.ResultSummary == nil || completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("session = %#v, want SUCCEEDED with FINAL result", completed)
	}
	if completed.Progress == nil ||
		intValueOrZero(completed.Progress.TotalDispatches) != 3 ||
		intValueOrZero(completed.Progress.CompletedDispatches) != 2 ||
		intValueOrZero(completed.Progress.FailedDispatches) != 1 {
		t.Fatalf("progress = %#v, want three dispatches with two completed and one failed", completed.Progress)
	}

	resultPayload := readParallelCompositionFinalResult(t, fixture.baseURL, sessionID)
	assertParallelCompositionPartialFailureResults(t, resultPayload, parallelDeclaredResultOrderingLabels)

	dispatches := fixture.publicDispatches(t, sessionID)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 3 public child dispatches", len(dispatches.Dispatches))
	}
	assertParallelCompositionPartialFailureDispatches(t, dispatches.Dispatches, parallelDeclaredResultOrderingLabels)
}

func parallelCompositionFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeParallelCompositionGlobalConfig(t *testing.T, homeDir string) {
	t.Helper()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "openai", "workerModel": "default-model"}
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
}

func startParallelCompositionWorkflowAsync(
	t *testing.T,
	fixture *compositionFixture,
	requestID, workflowSource string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: fixture.nextRequestID(requestID),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   workflowSource,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal parallel workflow request: %v", err)
	}

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/async",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build async parallel workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start async parallel workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start async parallel workflow status = %d: %s", response.StatusCode, body.String())
	}

	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode async parallel workflow response: %v", err)
	}
	return started
}

func readParallelCompositionSession(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	return support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
}

func intValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func readParallelCompositionFinalResult(
	t *testing.T,
	baseURL, sessionID string,
) map[string]any {
	t.Helper()

	result := support.GetJSON[factoryapi.FactorySessionResult](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/results?mode=final",
	)
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result json part: %v", err)
	}
	payload, ok := part.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary json payload = %#v, want object", part.Json)
	}
	return payload
}

func assertParallelCompositionResultLabels(
	t *testing.T,
	payload map[string]any,
	wantLabels []string,
) {
	t.Helper()

	results, ok := payload["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("result.results = %#v, want %d entries", payload["results"], len(wantLabels))
	}
	for index, wantLabel := range wantLabels {
		child, ok := results[index].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object child result", index, results[index])
		}
		gotLabel, _ := child["label"].(string)
		gotStatus, _ := child["status"].(string)
		if gotLabel != wantLabel || gotStatus != "COMPLETED" {
			t.Fatalf(
				"results[%d] = label=%q status=%q, want label=%q status=COMPLETED",
				index,
				gotLabel,
				gotStatus,
				wantLabel,
			)
		}
	}
}

func assertParallelCompositionPartialFailureResults(
	t *testing.T,
	payload map[string]any,
	wantLabels []string,
) {
	t.Helper()

	results, ok := payload["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("result.results = %#v, want %d entries", payload["results"], len(wantLabels))
	}
	for index, wantLabel := range wantLabels {
		child, ok := results[index].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object child result", index, results[index])
		}
		gotLabel, _ := child["label"].(string)
		if gotLabel != wantLabel {
			t.Fatalf("results[%d].label = %q, want %q", index, gotLabel, wantLabel)
		}
		if wantLabel == "child-beta" {
			gotStatus, _ := child["status"].(string)
			if gotStatus != "FAILED" {
				t.Fatalf("results[%d].status = %q, want FAILED", index, gotStatus)
			}
			diagnostic, _ := child["diagnostic"].(string)
			if strings.TrimSpace(diagnostic) == "" {
				t.Fatalf("results[%d].diagnostic is empty, want non-empty public diagnostic", index)
			}
			if child["artifactRef"] != nil {
				t.Fatalf("results[%d].artifactRef = %#v, want absent on failed child", index, child["artifactRef"])
			}
			continue
		}
		gotStatus, _ := child["status"].(string)
		if gotStatus != "COMPLETED" {
			t.Fatalf("results[%d].status = %q, want COMPLETED", index, gotStatus)
		}
	}
}

func assertParallelCompositionPartialFailureDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	wantLabels []string,
) {
	t.Helper()

	gotByLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		gotByLabel[*dispatch.Label] = dispatch
	}
	for _, wantLabel := range wantLabels {
		dispatch, ok := gotByLabel[wantLabel]
		if !ok {
			t.Fatalf("dispatch labels = %v, missing %q", gotByLabel, wantLabel)
		}
		if wantLabel == "child-beta" {
			if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED {
				t.Fatalf("dispatch %s status = %q, want FAILED", dispatch.Id, dispatch.Status)
			}
			if dispatch.FailureDetail == nil || strings.TrimSpace(dispatch.FailureDetail.Message) == "" {
				t.Fatalf("dispatch %s failureDetail = %#v, want non-empty public diagnostic", dispatch.Id, dispatch.FailureDetail)
			}
			if dispatch.OutputArtifactIds != nil && len(*dispatch.OutputArtifactIds) > 0 {
				t.Fatalf("dispatch %s outputArtifactIds = %#v, want none on failed child", dispatch.Id, dispatch.OutputArtifactIds)
			}
			continue
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		if dispatch.FailureDetail != nil {
			t.Fatalf("dispatch %s failureDetail = %#v, want nil on successful sibling", dispatch.Id, dispatch.FailureDetail)
		}
	}
}

func assertParallelCompositionDispatchLabels(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	wantLabels []string,
) {
	t.Helper()

	gotLabels := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		gotLabels[*dispatch.Label] = dispatch
	}
	for _, wantLabel := range wantLabels {
		if _, ok := gotLabels[wantLabel]; !ok {
			t.Fatalf("dispatch labels = %v, missing %q", gotLabels, wantLabel)
		}
	}
}
