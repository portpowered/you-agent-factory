// Package composition holds customer functional scenarios for nested JavaScript
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
	nestedParallelAlphaLabel          = "nested-parallel-alpha"
	nestedParallelBetaLabel           = "nested-parallel-beta"
	nestedPipelineReviewLabel         = "nested-pipeline-review"
	nestedParallelAlphaPrompt         = "nested edit alpha"
	nestedParallelBetaPrompt          = "nested edit beta"
	nestedParallelBetaFailurePrompt   = "fail:nested edit beta rejected"
	nestedPipelineReviewPrompt        = "review-after-nested-parallel"
	nestedFailureNestedStageIndex     = 0
	nestedFailureChildDiagnosticToken = "nested edit beta rejected"
	nestedCompletionWorkflow          = `return (async function () {
  const items = ["alpha"];
  const results = await pipeline(
    items,
    function (item, index) {
      return parallel([
        { prompt: "` + nestedParallelAlphaPrompt + `", label: "` + nestedParallelAlphaLabel + `" },
        { prompt: "` + nestedParallelBetaPrompt + `", label: "` + nestedParallelBetaLabel + `" },
      ]);
    },
    function (stageOneResult, item, index) {
      const firstDispatchId = stageOneResult[0].dispatchId;
      return agent.run({
        prompt: "` + nestedPipelineReviewPrompt + `:" + firstDispatchId,
        label: "` + nestedPipelineReviewLabel + `",
      });
    }
  );
  const itemResult = results[0];
  const stageOneParallel = itemResult.stages[0].result;
  const nestedParallelLabels = [
    stageOneParallel[0].label,
    stageOneParallel[1].label,
  ];
  const stageOneParallelDispatchIds = [
    stageOneParallel[0].dispatchId,
    stageOneParallel[1].dispatchId,
  ];
  return {
    results: results,
    itemStatus: itemResult.status,
    stageCount: itemResult.stages.length,
    nestedParallelLabels: nestedParallelLabels,
    stageOneParallelDispatchIds: stageOneParallelDispatchIds,
    reviewDispatchFromStageTwo: itemResult.stages[1].result.dispatchId,
  };
})();`
	nestedFailureWorkflow = `return (async function () {
  const items = ["alpha"];
  const results = await pipeline(
    items,
    function (item, index) {
      return parallel([
        { prompt: "` + nestedParallelAlphaPrompt + `", label: "` + nestedParallelAlphaLabel + `" },
        { prompt: "` + nestedParallelBetaFailurePrompt + `", label: "` + nestedParallelBetaLabel + `" },
      ]);
    },
    function (stageOneResult, item, index) {
      const firstDispatchId = stageOneResult[0].dispatchId;
      return agent.run({
        prompt: "` + nestedPipelineReviewPrompt + `:" + firstDispatchId,
        label: "` + nestedPipelineReviewLabel + `",
      });
    }
  );
  const itemResult = results[0];
  const stageOneParallel = itemResult.stages[0].result;
  const failedNestedChild = stageOneParallel[1];
  return {
    results: results,
    itemStatus: itemResult.status,
    nestedFailureStageIndex: itemResult.stages[0].index,
    nestedFailureStageStatus: itemResult.stages[0].status,
    failedNestedChildLabel: failedNestedChild.label,
    failedNestedChildStatus: failedNestedChild.status,
    failedNestedChildDiagnostic: failedNestedChild.diagnostic,
  };
})();`
)

// TestJavaScriptNestedPipelineParallelCompositionCompletes proves a JavaScript
// Factory that nests parallel children inside a pipeline stage completes with
// inspectable nested child dispatches on the public primary result and Factory
// Session dispatch listing surfaces.
func TestJavaScriptNestedPipelineParallelCompositionCompletes(t *testing.T) {
	t.Parallel()

	dir := scaffoldNestedCompletionWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startNestedCompletionWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listNestedCompletionDispatches(t, server.URL(), started.SessionId)
	alphaDispatch, betaDispatch, reviewDispatch := assertThreeCompletedNestedChildDispatches(t, dispatches.Dispatches)
	assertNestedCompletionPrimaryResult(
		t,
		started.Result,
		alphaDispatch.Id,
		betaDispatch.Id,
		reviewDispatch.Id,
	)
}

// TestJavaScriptNestedFailureNamesChildAndStage proves a nested parallel child
// failure inside a pipeline stage names the failing child and stage index on public
// dispatch diagnostics and primary result evidence without private VM stack frames.
func TestJavaScriptNestedFailureNamesChildAndStage(t *testing.T) {
	t.Parallel()

	dir := scaffoldNestedFailureWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startNestedFailureWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child failure edge", runner.CallCount())
	}

	dispatches := listNestedFailureDispatches(t, server.URL(), started.SessionId)
	alphaDispatch, betaDispatch, reviewDispatch := assertNestedFailureChildDispatches(
		t,
		dispatches.Dispatches,
	)
	assertNestedFailurePrimaryResult(
		t,
		started.Result,
		alphaDispatch.Id,
		betaDispatch.Id,
		reviewDispatch.Id,
	)
}

func scaffoldNestedCompletionWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-nested-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(nestedCompletionWorkflow), 0o600); err != nil {
		t.Fatalf("write nested completion workflow: %v", err)
	}
	return dir
}

func scaffoldNestedFailureWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-nested-composition-failure"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(nestedFailureWorkflow), 0o600); err != nil {
		t.Fatalf("write nested failure workflow: %v", err)
	}
	return dir
}

func startNestedCompletionWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-nested-pipeline-parallel-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal nested completion workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build nested completion workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start nested completion workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start nested completion workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode nested completion workflow response: %v", err)
	}
	return started
}

func startNestedFailureWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-nested-pipeline-parallel-failure",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal nested failure workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build nested failure workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start nested failure workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start nested failure workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode nested failure workflow response: %v", err)
	}
	return started
}

func listNestedCompletionDispatches(
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

func listNestedFailureDispatches(
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

func assertThreeCompletedNestedChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	if len(dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want exactly 3 nested child dispatches", len(dispatches))
	}

	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %q status = %q, want COMPLETED", dispatchLabel(dispatch), dispatch.Status)
		}
		if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
			*dispatch.Javascript.ExecutionMode != "fake" {
			t.Fatalf("dispatch %q javascript projection = %#v, want fake execution mode", dispatchLabel(dispatch), dispatch.Javascript)
		}
		byLabel[dispatchLabel(dispatch)] = dispatch
	}

	alpha, ok := byLabel[nestedParallelAlphaLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedParallelAlphaLabel)
	}
	beta, ok := byLabel[nestedParallelBetaLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedParallelBetaLabel)
	}
	review, ok := byLabel[nestedPipelineReviewLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedPipelineReviewLabel)
	}
	if alpha.Id == beta.Id || alpha.Id == review.Id || beta.Id == review.Id {
		t.Fatalf("nested dispatch IDs are duplicated: alpha=%q beta=%q review=%q", alpha.Id, beta.Id, review.Id)
	}
	return alpha, beta, review
}

func assertNestedFailureChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	if len(dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want exactly 3 nested child dispatches", len(dispatches))
	}

	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
			*dispatch.Javascript.ExecutionMode != "fake" {
			t.Fatalf("dispatch %q javascript projection = %#v, want fake execution mode", dispatchLabel(dispatch), dispatch.Javascript)
		}
		byLabel[dispatchLabel(dispatch)] = dispatch
	}

	alpha, ok := byLabel[nestedParallelAlphaLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedParallelAlphaLabel)
	}
	beta, ok := byLabel[nestedParallelBetaLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedParallelBetaLabel)
	}
	review, ok := byLabel[nestedPipelineReviewLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), nestedPipelineReviewLabel)
	}
	if alpha.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch %q status = %q, want COMPLETED", dispatchLabel(alpha), alpha.Status)
	}
	if beta.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch %q status = %q, want FAILED", dispatchLabel(beta), beta.Status)
	}
	if review.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch %q status = %q, want COMPLETED", dispatchLabel(review), review.Status)
	}
	if beta.FailureDetail == nil || beta.FailureDetail.Message == "" {
		t.Fatalf(
			"dispatch %q failure detail = %#v, want customer-readable failure message",
			dispatchLabel(beta),
			beta.FailureDetail,
		)
	}
	if !strings.Contains(beta.FailureDetail.Message, nestedFailureChildDiagnosticToken) {
		t.Fatalf(
			"dispatch %q failure message = %q, want nested child failure diagnostic %q",
			dispatchLabel(beta),
			beta.FailureDetail.Message,
			nestedFailureChildDiagnosticToken,
		)
	}
	for _, leaked := range []string{"goja", "runtimeGlobals", "heap", "stack"} {
		if strings.Contains(strings.ToLower(beta.FailureDetail.Message), leaked) {
			t.Fatalf(
				"dispatch %q failure message leaked non-customer detail %q: %q",
				dispatchLabel(beta),
				leaked,
				beta.FailureDetail.Message,
			)
		}
	}
	if alpha.Id == beta.Id || alpha.Id == review.Id || beta.Id == review.Id {
		t.Fatalf("nested dispatch IDs are duplicated: alpha=%q beta=%q review=%q", alpha.Id, beta.Id, review.Id)
	}
	return alpha, beta, review
}

func assertNestedCompletionPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	alphaDispatchID string,
	betaDispatchID string,
	reviewDispatchID string,
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
		ItemStatus                  string   `json:"itemStatus"`
		StageCount                  int      `json:"stageCount"`
		NestedParallelLabels        []string `json:"nestedParallelLabels"`
		StageOneParallelDispatchIDs []string `json:"stageOneParallelDispatchIds"`
		ReviewDispatchFromStageTwo  string   `json:"reviewDispatchFromStageTwo"`
		Results                     []struct {
			Status string `json:"status"`
			Stages []struct {
				Index  int             `json:"index"`
				Status string          `json:"status"`
				Result json.RawMessage `json:"result"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode nested completion primary result: %v", err)
	}
	if evidence.ItemStatus != "COMPLETED" || evidence.StageCount != 2 {
		t.Fatalf(
			"nested completion evidence = %#v, want one completed item with two pipeline stages",
			evidence,
		)
	}
	wantParallelLabels := []string{nestedParallelAlphaLabel, nestedParallelBetaLabel}
	if len(evidence.NestedParallelLabels) != len(wantParallelLabels) {
		t.Fatalf("nested parallel labels = %#v, want %#v", evidence.NestedParallelLabels, wantParallelLabels)
	}
	for index, wantLabel := range wantParallelLabels {
		if evidence.NestedParallelLabels[index] != wantLabel {
			t.Fatalf(
				"nested parallel labels[%d] = %q, want %q",
				index,
				evidence.NestedParallelLabels[index],
				wantLabel,
			)
		}
	}
	if len(evidence.StageOneParallelDispatchIDs) != 2 {
		t.Fatalf(
			"stage-one parallel dispatch ids = %#v, want two nested parallel child ids",
			evidence.StageOneParallelDispatchIDs,
		)
	}
	if evidence.StageOneParallelDispatchIDs[0] != alphaDispatchID ||
		evidence.StageOneParallelDispatchIDs[1] != betaDispatchID {
		t.Fatalf(
			"stage-one parallel dispatch ids = %#v, want alpha=%q beta=%q",
			evidence.StageOneParallelDispatchIDs,
			alphaDispatchID,
			betaDispatchID,
		)
	}
	if evidence.ReviewDispatchFromStageTwo != reviewDispatchID {
		t.Fatalf(
			"stage-two review dispatch id = %q, want %q",
			evidence.ReviewDispatchFromStageTwo,
			reviewDispatchID,
		)
	}
	if len(evidence.Results) != 1 || evidence.Results[0].Status != "COMPLETED" {
		t.Fatalf("pipeline item results = %#v, want one completed item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want exactly 2", len(evidence.Results[0].Stages))
	}
	for index, stage := range evidence.Results[0].Stages {
		if stage.Index != index || stage.Status != "COMPLETED" {
			t.Fatalf("pipeline stage[%d] = %#v, want completed stage index %d", index, stage, index)
		}
	}
	var stageOneParallel []struct {
		Label      string `json:"label"`
		Status     string `json:"status"`
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(evidence.Results[0].Stages[0].Result, &stageOneParallel); err != nil {
		t.Fatalf("decode stage-one parallel results: %v", err)
	}
	if len(stageOneParallel) != 2 {
		t.Fatalf("stage-one parallel results = %#v, want two nested parallel child results", stageOneParallel)
	}
	for index, wantLabel := range wantParallelLabels {
		child := stageOneParallel[index]
		if child.Label != wantLabel || child.Status != "COMPLETED" {
			t.Fatalf(
				"stage-one parallel[%d] = label=%q status=%q, want label=%q status=COMPLETED",
				index,
				child.Label,
				child.Status,
				wantLabel,
			)
		}
	}
}

func assertNestedFailurePrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	alphaDispatchID string,
	_ string,
	reviewDispatchID string,
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
		ItemStatus                  string `json:"itemStatus"`
		NestedFailureStageIndex     int    `json:"nestedFailureStageIndex"`
		NestedFailureStageStatus    string `json:"nestedFailureStageStatus"`
		FailedNestedChildLabel      string `json:"failedNestedChildLabel"`
		FailedNestedChildStatus     string `json:"failedNestedChildStatus"`
		FailedNestedChildDiagnostic string `json:"failedNestedChildDiagnostic"`
		Results                     []struct {
			Status string `json:"status"`
			Stages []struct {
				Index  int             `json:"index"`
				Status string          `json:"status"`
				Result json.RawMessage `json:"result"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode nested failure primary result: %v", err)
	}
	if evidence.NestedFailureStageIndex != nestedFailureNestedStageIndex {
		t.Fatalf(
			"nested failure stage index = %d, want pipeline stage %d",
			evidence.NestedFailureStageIndex,
			nestedFailureNestedStageIndex,
		)
	}
	if evidence.NestedFailureStageStatus != "COMPLETED" {
		t.Fatalf(
			"nested failure stage status = %q, want COMPLETED parallel stage with one failed nested child",
			evidence.NestedFailureStageStatus,
		)
	}
	if evidence.FailedNestedChildLabel != nestedParallelBetaLabel {
		t.Fatalf(
			"failed nested child label = %q, want %q",
			evidence.FailedNestedChildLabel,
			nestedParallelBetaLabel,
		)
	}
	if evidence.FailedNestedChildStatus != "FAILED" {
		t.Fatalf(
			"failed nested child status = %q, want FAILED",
			evidence.FailedNestedChildStatus,
		)
	}
	if !strings.Contains(evidence.FailedNestedChildDiagnostic, nestedFailureChildDiagnosticToken) {
		t.Fatalf(
			"failed nested child diagnostic = %q, want customer-readable token %q",
			evidence.FailedNestedChildDiagnostic,
			nestedFailureChildDiagnosticToken,
		)
	}
	if len(evidence.Results) != 1 {
		t.Fatalf("pipeline item results = %#v, want one item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want exactly 2 after nested parallel failure", len(evidence.Results[0].Stages))
	}
	stageZero := evidence.Results[0].Stages[0]
	if stageZero.Index != nestedFailureNestedStageIndex || stageZero.Status != "COMPLETED" {
		t.Fatalf(
			"pipeline stage[0] = index=%d status=%q, want index=%d status=COMPLETED",
			stageZero.Index,
			stageZero.Status,
			nestedFailureNestedStageIndex,
		)
	}
	var stageOneParallel []struct {
		Label      string `json:"label"`
		Status     string `json:"status"`
		Diagnostic string `json:"diagnostic"`
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(stageZero.Result, &stageOneParallel); err != nil {
		t.Fatalf("decode stage-one parallel results: %v", err)
	}
	if len(stageOneParallel) != 2 {
		t.Fatalf("stage-one parallel results = %#v, want two nested parallel child results", stageOneParallel)
	}
	if stageOneParallel[0].Label != nestedParallelAlphaLabel || stageOneParallel[0].Status != "COMPLETED" {
		t.Fatalf(
			"stage-one parallel[0] = label=%q status=%q, want label=%q status=COMPLETED",
			stageOneParallel[0].Label,
			stageOneParallel[0].Status,
			nestedParallelAlphaLabel,
		)
	}
	if stageOneParallel[0].DispatchID != alphaDispatchID {
		t.Fatalf(
			"stage-one parallel alpha dispatch id = %q, want %q",
			stageOneParallel[0].DispatchID,
			alphaDispatchID,
		)
	}
	failedChild := stageOneParallel[1]
	if failedChild.Label != nestedParallelBetaLabel || failedChild.Status != "FAILED" {
		t.Fatalf(
			"stage-one parallel[1] = label=%q status=%q, want label=%q status=FAILED",
			failedChild.Label,
			failedChild.Status,
			nestedParallelBetaLabel,
		)
	}
	if !strings.Contains(failedChild.Diagnostic, nestedFailureChildDiagnosticToken) {
		t.Fatalf(
			"stage-one parallel beta diagnostic = %q, want token %q",
			failedChild.Diagnostic,
			nestedFailureChildDiagnosticToken,
		)
	}
	stageOne := evidence.Results[0].Stages[1]
	if stageOne.Index != 1 || stageOne.Status != "COMPLETED" {
		t.Fatalf(
			"pipeline stage[1] = index=%d status=%q, want index=1 status=COMPLETED after nested failure",
			stageOne.Index,
			stageOne.Status,
		)
	}
	var stageTwoResult struct {
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(stageOne.Result, &stageTwoResult); err != nil {
		t.Fatalf("decode stage-two review result: %v", err)
	}
	if stageTwoResult.DispatchID != reviewDispatchID {
		t.Fatalf(
			"stage-two review dispatch id = %q, want %q",
			stageTwoResult.DispatchID,
			reviewDispatchID,
		)
	}
}
