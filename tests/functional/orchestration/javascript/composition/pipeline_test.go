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
	pipelineStageOneLabel          = "edit-0"
	pipelineStageTwoLabel          = "review-0"
	pipelineStageOnePrompt         = "edit alpha"
	pipelineStageOneFailurePrompt  = "fail:edit rejected"
	pipelineStageFailureWorkflow = `return (async function () {
  const items = ["alpha"];
  const results = await pipeline(
    items,
    function (item, index) {
      return agent.run({
        prompt: "` + pipelineStageOneFailurePrompt + `",
        label: "` + pipelineStageOneLabel + `",
      });
    },
    function (editResult, item, index) {
      return agent.run({
        prompt: "review-after:" + editResult.dispatchId,
        label: "` + pipelineStageTwoLabel + `",
      });
    }
  );
  const failedItem = results[0];
  return {
    results: results,
    itemStatus: failedItem.status,
    stageCount: failedItem.stages.length,
    stageOneStatus: failedItem.stages[0].status,
    stageOneDiagnostic: failedItem.stages[0].diagnostic,
  };
})();`
	pipelineStageOutputWorkflow = `return (async function () {
  const items = ["alpha"];
  const results = await pipeline(
    items,
    function (item, index) {
      return agent.run({
        prompt: "` + pipelineStageOnePrompt + `",
        label: "` + pipelineStageOneLabel + `",
      });
    },
    function (editResult, item, index) {
      const stageTwoPrompt =
        "review-after:" + editResult.dispatchId + ":" + editResult.output.text;
      return agent.run({
        prompt: stageTwoPrompt,
        label: "` + pipelineStageTwoLabel + `",
      });
    }
  );
  const stageOneResult = results[0].stages[0].result;
  const stageTwoResult = results[0].stages[1].result;
  const stageTwoPrompt =
    "review-after:" + stageOneResult.dispatchId + ":" + stageOneResult.output.text;
  return {
    results: results,
    stageTwoPrompt: stageTwoPrompt,
    dependency: {
      priorDispatchId: stageOneResult.dispatchId,
      observedByStageTwo:
        stageTwoResult.output.text.indexOf(stageOneResult.dispatchId) !== -1,
    },
  };
})();`
)

// TestJavaScriptPipelinePassesStageOutputToNextStage proves a JavaScript
// Factory that runs a multi-stage pipeline completes with evidence that
// stage-one child output is passed into and observed by the next stage on
// the public primary result and Factory Session dispatch listing surfaces.
func TestJavaScriptPipelinePassesStageOutputToNextStage(t *testing.T) {
	t.Parallel()

	dir := scaffoldPipelineStageOutputWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startPipelineStageOutputWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listPipelineStageOutputDispatches(t, server.URL(), started.SessionId)
	stageOneDispatch, stageTwoDispatch := assertTwoCompletedPipelineChildDispatches(t, dispatches.Dispatches)
	assertPipelineStageOutputPrimaryResult(t, started.Result, stageOneDispatch.Id, stageTwoDispatch.Id)
}

// TestJavaScriptPipelineStopsAfterStageFailure proves a JavaScript Factory
// pipeline stops later child dispatch after an early stage fails and records
// the stage failure on the public primary result and dispatch listing surfaces.
func TestJavaScriptPipelineStopsAfterStageFailure(t *testing.T) {
	t.Parallel()

	dir := scaffoldPipelineStageFailureWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startPipelineStageFailureWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listPipelineStageFailureDispatches(t, server.URL(), started.SessionId)
	stageOneDispatch := assertSingleFailedPipelineStageOneDispatch(t, dispatches.Dispatches)
	assertNoLaterPipelineStageDispatch(t, dispatches.Dispatches, pipelineStageTwoLabel)
	assertPipelineStageFailurePrimaryResult(t, started.Result, stageOneDispatch.Id)
}

func scaffoldPipelineStageOutputWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-pipeline-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(pipelineStageOutputWorkflow), 0o600); err != nil {
		t.Fatalf("write pipeline stage-output workflow: %v", err)
	}
	return dir
}

func scaffoldPipelineStageFailureWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-pipeline-stage-failure"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(pipelineStageFailureWorkflow), 0o600); err != nil {
		t.Fatalf("write pipeline stage-failure workflow: %v", err)
	}
	return dir
}

func startPipelineStageOutputWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-pipeline-stage-output-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal pipeline stage-output workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build pipeline stage-output workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start pipeline stage-output workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start pipeline stage-output workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode pipeline stage-output workflow response: %v", err)
	}
	return started
}

func startPipelineStageFailureWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-pipeline-stage-failure-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal pipeline stage-failure workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build pipeline stage-failure workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start pipeline stage-failure workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start pipeline stage-failure workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode pipeline stage-failure workflow response: %v", err)
	}
	return started
}

func listPipelineStageOutputDispatches(
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

func listPipelineStageFailureDispatches(
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

func assertTwoCompletedPipelineChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want exactly 2 child dispatches", len(dispatches))
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

	stageOne, ok := byLabel[pipelineStageOneLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), pipelineStageOneLabel)
	}
	stageTwo, ok := byLabel[pipelineStageTwoLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), pipelineStageTwoLabel)
	}
	if stageOne.Id == stageTwo.Id {
		t.Fatalf("stage dispatch IDs are duplicated: %q", stageOne.Id)
	}
	return stageOne, stageTwo
}

func assertSingleFailedPipelineStageOneDispatch(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()

	if len(dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want exactly 1 child dispatch after stage-one failure", len(dispatches))
	}
	stageOne := dispatches[0]
	if dispatchLabel(stageOne) != pipelineStageOneLabel {
		t.Fatalf("dispatch label = %q, want %q", dispatchLabel(stageOne), pipelineStageOneLabel)
	}
	if stageOne.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch %q status = %q, want FAILED", dispatchLabel(stageOne), stageOne.Status)
	}
	if stageOne.Javascript == nil || stageOne.Javascript.ExecutionMode == nil ||
		*stageOne.Javascript.ExecutionMode != "fake" {
		t.Fatalf("dispatch %q javascript projection = %#v, want fake execution mode", dispatchLabel(stageOne), stageOne.Javascript)
	}
	if stageOne.FailureDetail == nil || stageOne.FailureDetail.Message == "" {
		t.Fatalf("dispatch %q failure detail = %#v, want customer-readable failure message", dispatchLabel(stageOne), stageOne.FailureDetail)
	}
	if !strings.Contains(stageOne.FailureDetail.Message, "edit rejected") {
		t.Fatalf(
			"dispatch %q failure message = %q, want stage-one failure diagnostic",
			dispatchLabel(stageOne),
			stageOne.FailureDetail.Message,
		)
	}
	return stageOne
}

func assertNoLaterPipelineStageDispatch(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	laterStageLabel string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if dispatchLabel(dispatch) == laterStageLabel {
			t.Fatalf(
				"dispatch labels = %#v, want no later-stage dispatch %q after stage-one failure",
				dispatchLabels(dispatches),
				laterStageLabel,
			)
		}
	}
}

func assertPipelineStageOutputPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	stageOneDispatchID string,
	stageTwoDispatchID string,
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
		StageTwoPrompt string `json:"stageTwoPrompt"`
		Dependency     struct {
			PriorDispatchID    string `json:"priorDispatchId"`
			ObservedByStageTwo bool   `json:"observedByStageTwo"`
		} `json:"dependency"`
		Results []struct {
			Status string `json:"status"`
			Stages []struct {
				Index  int            `json:"index"`
				Status string         `json:"status"`
				Result map[string]any `json:"result"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode pipeline stage-output primary result: %v", err)
	}
	if evidence.Dependency.PriorDispatchID != stageOneDispatchID || !evidence.Dependency.ObservedByStageTwo {
		t.Fatalf(
			"dependency evidence = %#v, want stage-one dispatch %q observed by stage two",
			evidence.Dependency,
			stageOneDispatchID,
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
	if !strings.Contains(evidence.StageTwoPrompt, stageOneDispatchID) {
		t.Fatalf(
			"stage-two prompt = %q, want stage-one dispatch id %q",
			evidence.StageTwoPrompt,
			stageOneDispatchID,
		)
	}
	stageTwoResult := evidence.Results[0].Stages[1].Result
	reviewDispatchID, _ := stageTwoResult["dispatchId"].(string)
	if reviewDispatchID != stageTwoDispatchID {
		t.Fatalf("stage-two child dispatchId = %q, want %q", reviewDispatchID, stageTwoDispatchID)
	}
}

func assertPipelineStageFailurePrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	stageOneDispatchID string,
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
		ItemStatus         string `json:"itemStatus"`
		StageCount         int    `json:"stageCount"`
		StageOneStatus     string `json:"stageOneStatus"`
		StageOneDiagnostic string `json:"stageOneDiagnostic"`
		Results            []struct {
			Status string `json:"status"`
			Stages []struct {
				Index      int    `json:"index"`
				Status     string `json:"status"`
				Diagnostic string `json:"diagnostic"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode pipeline stage-failure primary result: %v", err)
	}
	if evidence.ItemStatus != "FAILED" || evidence.StageCount != 1 || evidence.StageOneStatus != "FAILED" {
		t.Fatalf(
			"pipeline failure evidence = %#v, want one failed stage-one item without later stages",
			evidence,
		)
	}
	if !strings.Contains(evidence.StageOneDiagnostic, "edit rejected") {
		t.Fatalf(
			"stage-one diagnostic = %q, want customer-readable failure for stage-one child",
			evidence.StageOneDiagnostic,
		)
	}
	if len(evidence.Results) != 1 || evidence.Results[0].Status != "FAILED" {
		t.Fatalf("pipeline item results = %#v, want one failed item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 1 {
		t.Fatalf("pipeline stage count = %d, want exactly 1 after stage-one failure", len(evidence.Results[0].Stages))
	}
	stage := evidence.Results[0].Stages[0]
	if stage.Index != 0 || stage.Status != "FAILED" || !strings.Contains(stage.Diagnostic, "edit rejected") {
		t.Fatalf("pipeline stage[0] = %#v, want failed stage-one diagnostic", stage)
	}
	if stageOneDispatchID == "" {
		t.Fatal("stage-one dispatch id is empty")
	}
}

func dispatchLabel(dispatch factoryapi.FactorySessionDispatchSummary) string {
	if dispatch.Label == nil {
		return ""
	}
	return *dispatch.Label
}

func dispatchLabels(dispatches []factoryapi.FactorySessionDispatchSummary) []string {
	labels := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
		labels = append(labels, dispatchLabel(dispatch))
	}
	return labels
}
