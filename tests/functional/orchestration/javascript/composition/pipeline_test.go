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
	pipelineStageOneLabel         = "edit-0"
	pipelineStageTwoLabel         = "review-0"
	pipelineStageOnePrompt        = "edit alpha"
	pipelineStageOneFailurePrompt = "fail:edit rejected"
	pipelineStageFailureWorkflow  = `return (async function () {
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
func runJavaScriptPipelinePassesStageOutputToNextStage(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldPipelineStageOutputWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startPipelineStageOutputWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child execution", got, providerCalls)
	}

	dispatches := listPipelineStageOutputDispatches(t, fixture, started.SessionId)
	stageOneDispatch, stageTwoDispatch := assertTwoCompletedPipelineChildDispatches(t, dispatches.Dispatches)
	assertPipelineStageOutputPrimaryResult(t, started.Result, stageOneDispatch.Id, stageTwoDispatch.Id)
	assertPipelineFactoryEventProjection(t, fixture.fakeEvents(t, started.SessionId),
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded, stageOneDispatch.Id, stageTwoDispatch.Id)
}

// TestJavaScriptPipelineStopsAfterStageFailure proves a JavaScript Factory
// pipeline stops later child dispatch after an early stage fails and records
// the stage failure on the public primary result and dispatch listing surfaces.
func runJavaScriptPipelineStopsAfterStageFailure(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldPipelineStageFailureWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startPipelineStageFailureWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child execution", got, providerCalls)
	}

	dispatches := listPipelineStageFailureDispatches(t, fixture, started.SessionId)
	stageOneDispatch := assertSingleFailedPipelineStageOneDispatch(t, dispatches.Dispatches)
	assertNoLaterPipelineStageDispatch(t, dispatches.Dispatches, pipelineStageTwoLabel)
	assertPipelineStageFailurePrimaryResult(t, started.Result, stageOneDispatch.Id)
	assertPipelineFactoryEventProjection(t, fixture.fakeEvents(t, started.SessionId),
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded, "edit rejected")
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
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-pipeline-stage-output-composition", dir)
}

func startPipelineStageFailureWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-pipeline-stage-failure-composition", dir)
}

func listPipelineStageOutputDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
}

func listPipelineStageFailureDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
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

func assertPipelineFactoryEventProjection(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantFinalStatus factoryapi.FactorySessionDurableLifecycleStatus,
	wantEvidence ...string,
) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("factory events = empty, want retained pipeline lifecycle projection")
	}
	sawResultUpdated := false
	sawSessionCompleted := false
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionResultUpdated:
			payload, err := event.Payload.AsSessionResultUpdatedEventPayload()
			if err != nil {
				t.Fatalf("decode pipeline SESSION_RESULT_UPDATED payload: %v", err)
			}
			if payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal || payload.ResultSummary == nil {
				t.Fatalf("pipeline result update payload = %#v, want FINAL result summary", payload)
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("encode pipeline Factory Event: %v", err)
			}
			for _, evidence := range wantEvidence {
				if !strings.Contains(string(encoded), evidence) {
					t.Fatalf("pipeline result update event = %s, want evidence %q", encoded, evidence)
				}
			}
			sawResultUpdated = true
		case factoryapi.FactoryEventTypeSessionCompleted:
			payload, err := event.Payload.AsSessionCompletedEventPayload()
			if err != nil {
				t.Fatalf("decode pipeline SESSION_COMPLETED payload: %v", err)
			}
			if payload.FinalStatus != wantFinalStatus || payload.ResultStatus == nil || *payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal {
				t.Fatalf("pipeline session completed payload = %#v, want %q with FINAL result", payload, wantFinalStatus)
			}
			sawSessionCompleted = true
		}
	}
	if !sawResultUpdated {
		t.Fatalf("factory events = %#v, want SESSION_RESULT_UPDATED for pipeline result", events)
	}
	if !sawSessionCompleted {
		t.Fatalf("factory events = %#v, want SESSION_COMPLETED for pipeline result", events)
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
