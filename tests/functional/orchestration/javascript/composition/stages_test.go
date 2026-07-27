// Package composition holds customer functional scenarios for JavaScript
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	namedStageDraftLabel  = "stage-draft"
	namedStageReviewLabel = "stage-review"
	namedStageDraftPhase  = "stage-draft"
	namedStageReviewPhase = "stage-review"
	namedStagesWorkflow   = `return (async function () {
  phase("` + namedStageDraftPhase + `");
  const items = ["alpha"];
  const results = await pipeline(
    items,
    function (item, index) {
      return agent.run({
        prompt: "draft alpha",
        label: "` + namedStageDraftLabel + `",
      });
    },
    function (draftResult, item, index) {
      phase("` + namedStageReviewPhase + `");
      return agent.run({
        prompt: "review-after:" + draftResult.dispatchId,
        label: "` + namedStageReviewLabel + `",
      });
    }
  );
  const stageLabels = results[0].stages.map(function (stage) {
    return stage.result && stage.result.label ? stage.result.label : null;
  });
  return {
    results: results,
    orderedStageLabels: stageLabels,
  };
})();`

	emptyStagesWorkerLabel = "empty-stage-worker"
	emptyStagesNextLabel   = "empty-stage-next"
	emptyStagesWorkflow    = `return (async function () {
  const results = await pipeline(
    [],
    function (item, index) {
      return agent.run({
        prompt: "should-not-run",
        label: "` + emptyStagesWorkerLabel + `",
      });
    },
    function (workerResult, item, index) {
      return agent.run({
        prompt: "should-not-run-next",
        label: "` + emptyStagesNextLabel + `",
      });
    }
  );
  return {
    pipelineResults: results,
    resultKind: results.length === 0 ? "empty-ordered-per-item" : "unexpected",
  };
})();`
)

// TestJavaScriptNamedStagesExposeOrderedProgress proves a JavaScript Factory
// with distinct named pipeline stages completes with ordered progress on public
// Factory Event and session projection surfaces that preserve each stage's
// identity and documented execution order.
func TestJavaScriptNamedStagesExposeOrderedProgress(t *testing.T) {
	t.Parallel()

	dir := scaffoldNamedStagesWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startNamedStagesWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}

	dispatches := listNamedStagesDispatches(t, server.URL(), started.SessionId)
	stageDraftDispatch, stageReviewDispatch := assertTwoCompletedNamedStageDispatches(t, dispatches.Dispatches)
	assertNamedStagesOrderedDispatchProgress(t, dispatches.Dispatches)
	assertNamedStagesOrderedPrimaryResult(
		t,
		started.Result,
		stageDraftDispatch.Id,
		stageReviewDispatch.Id,
	)

	events := listNamedStagesFactoryEvents(t, server.URL(), started.SessionId)
	assertNamedStagesOrderedPhaseProgress(t, events)
}

// TestJavaScriptEmptyStageProducesDocumentedResult proves an empty JavaScript
// pipeline stage path completes with the documented empty ordered per-item
// public result and does not invent child Dispatches when external effects are
// substituted only through edges.Edges.
func TestJavaScriptEmptyStageProducesDocumentedResult(t *testing.T) {
	t.Parallel()

	dir := scaffoldEmptyStagesWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startEmptyStagesWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for empty stage path", runner.CallCount())
	}

	dispatches := listEmptyStagesDispatches(t, server.URL(), started.SessionId)
	assertEmptyStageNoChildDispatches(t, dispatches.Dispatches)
	assertEmptyStageDocumentedPrimaryResult(t, started.Result)
}

func scaffoldNamedStagesWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-named-stages-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(namedStagesWorkflow), 0o600); err != nil {
		t.Fatalf("write named stages workflow: %v", err)
	}
	return dir
}

func scaffoldEmptyStagesWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "javascript-empty-stages-composition"})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(emptyStagesWorkflow), 0o600); err != nil {
		t.Fatalf("write empty stages workflow: %v", err)
	}
	return dir
}

func startNamedStagesWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-named-stages-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal named stages workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build named stages workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start named stages workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start named stages workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode named stages workflow response: %v", err)
	}
	return started
}

func startEmptyStagesWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, "workflow.js")
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-empty-stages-composition",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal empty stages workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build empty stages workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start empty stages workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start empty stages workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode empty stages workflow response: %v", err)
	}
	return started
}

func listNamedStagesDispatches(
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

func listEmptyStagesDispatches(
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

func listNamedStagesFactoryEvents(
	t *testing.T,
	serverURL, sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build named stages factory events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET named stages factory events: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		t.Fatalf("GET named stages factory events status = %d", response.StatusCode)
	}

	events := make(chan factoryapi.FactoryEvent, 256)
	errs := make(chan error, 1)
	go func() {
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event factoryapi.FactoryEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				errs <- fmt.Errorf("decode factory event: %w", err)
				return
			}
			events <- event
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()

	var collected []factoryapi.FactoryEvent
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if quiet == nil {
				quiet = time.NewTimer(25 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(25 * time.Millisecond)
			}
			quietC = quiet.C
		case err := <-errs:
			t.Fatalf("read named stages factory events: %v", err)
		case <-quietC:
			return collected
		case <-deadline.C:
			return collected
		}
	}
}

func assertTwoCompletedNamedStageDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) (factoryapi.FactorySessionDispatchSummary, factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want exactly 2 named stage dispatches", len(dispatches))
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

	stageDraft, ok := byLabel[namedStageDraftLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), namedStageDraftLabel)
	}
	stageReview, ok := byLabel[namedStageReviewLabel]
	if !ok {
		t.Fatalf("dispatch labels = %#v, want %q", dispatchLabels(dispatches), namedStageReviewLabel)
	}
	if stageDraft.Id == stageReview.Id {
		t.Fatalf("named stage dispatch IDs are duplicated: %q", stageDraft.Id)
	}
	return stageDraft, stageReview
}

func assertNamedStagesOrderedDispatchProgress(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	wantLabels := []string{namedStageDraftLabel, namedStageReviewLabel}
	if len(dispatches) != len(wantLabels) {
		t.Fatalf("dispatch count = %d, want %d ordered named stages", len(dispatches), len(wantLabels))
	}
	for index, wantLabel := range wantLabels {
		gotLabel := dispatchLabel(dispatches[index])
		if gotLabel != wantLabel {
			t.Fatalf(
				"dispatch[%d] label = %q, want ordered named stage %q; labels = %#v",
				index,
				gotLabel,
				wantLabel,
				dispatchLabels(dispatches),
			)
		}
	}
}

func assertNamedStagesOrderedPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	stageDraftDispatchID string,
	stageReviewDispatchID string,
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
		OrderedStageLabels []string `json:"orderedStageLabels"`
		Results            []struct {
			Status string `json:"status"`
			Stages []struct {
				Index  int            `json:"index"`
				Status string         `json:"status"`
				Result map[string]any `json:"result"`
			} `json:"stages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode named stages primary result: %v", err)
	}
	wantLabels := []string{namedStageDraftLabel, namedStageReviewLabel}
	if len(evidence.OrderedStageLabels) != len(wantLabels) {
		t.Fatalf("orderedStageLabels = %#v, want %v", evidence.OrderedStageLabels, wantLabels)
	}
	for index, wantLabel := range wantLabels {
		if evidence.OrderedStageLabels[index] != wantLabel {
			t.Fatalf(
				"orderedStageLabels[%d] = %q, want %q preserving named stage identity",
				index,
				evidence.OrderedStageLabels[index],
				wantLabel,
			)
		}
	}
	if len(evidence.Results) != 1 || evidence.Results[0].Status != "COMPLETED" {
		t.Fatalf("pipeline item results = %#v, want one completed item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want exactly 2 named stages", len(evidence.Results[0].Stages))
	}
	for index, stage := range evidence.Results[0].Stages {
		if stage.Index != index || stage.Status != "COMPLETED" {
			t.Fatalf("pipeline stage[%d] = %#v, want completed stage index %d", index, stage, index)
		}
		label, _ := stage.Result["label"].(string)
		if label != wantLabels[index] {
			t.Fatalf(
				"pipeline stage[%d] label = %q, want ordered named stage %q",
				index,
				label,
				wantLabels[index],
			)
		}
	}
	draftDispatchID, _ := evidence.Results[0].Stages[0].Result["dispatchId"].(string)
	reviewDispatchID, _ := evidence.Results[0].Stages[1].Result["dispatchId"].(string)
	if draftDispatchID != stageDraftDispatchID || reviewDispatchID != stageReviewDispatchID {
		t.Fatalf(
			"pipeline stage dispatch ids = [%q, %q], want public dispatch ids [%q, %q]",
			draftDispatchID,
			reviewDispatchID,
			stageDraftDispatchID,
			stageReviewDispatchID,
		)
	}
}

func assertEmptyStageNoChildDispatches(t *testing.T, dispatches []factoryapi.FactorySessionDispatchSummary) {
	t.Helper()

	if len(dispatches) != 0 {
		t.Fatalf(
			"dispatch count = %d labels = %#v, want no child dispatches for empty pipeline stage path",
			len(dispatches),
			dispatchLabels(dispatches),
		)
	}
}

func assertEmptyStageDocumentedPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
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
		ResultKind      string `json:"resultKind"`
		PipelineResults []any  `json:"pipelineResults"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode empty stage primary result: %v", err)
	}
	if evidence.ResultKind != "empty-ordered-per-item" {
		t.Fatalf(
			"resultKind = %q, want documented empty ordered per-item pipeline result",
			evidence.ResultKind,
		)
	}
	if len(evidence.PipelineResults) != 0 {
		t.Fatalf(
			"pipelineResults = %#v, want empty ordered per-item stage results for pipeline([])",
			evidence.PipelineResults,
		)
	}
}

func assertNamedStagesOrderedPhaseProgress(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	want := []struct {
		phaseName string
		status    factoryapi.OrchestratorPhaseStatus
	}{
		{namedStageDraftPhase, factoryapi.ACTIVE},
		{namedStageDraftPhase, factoryapi.COMPLETED},
		{namedStageReviewPhase, factoryapi.ACTIVE},
		{namedStageReviewPhase, factoryapi.COMPLETED},
	}

	var phaseEvents []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeOrchestratorPhaseChanged {
			continue
		}
		phaseEvents = append(phaseEvents, event)
	}
	if len(phaseEvents) != len(want) {
		t.Fatalf("phase event count = %d, want %d: %#v", len(phaseEvents), len(want), phaseEvents)
	}
	for index, expected := range want {
		event := phaseEvents[index]
		if event.Context.PhaseName == nil || *event.Context.PhaseName != expected.phaseName {
			t.Fatalf(
				"phase event[%d] name = %#v, want %q in documented order",
				index,
				event.Context.PhaseName,
				expected.phaseName,
			)
		}
		payload, err := event.Payload.AsOrchestratorPhaseChangedEventPayload()
		if err != nil {
			t.Fatalf("decode phase event[%d] payload: %v", index, err)
		}
		if payload.PhaseStatus != expected.status {
			t.Fatalf(
				"phase event[%d] status = %q, want %q for phase %q",
				index,
				payload.PhaseStatus,
				expected.status,
				expected.phaseName,
			)
		}
	}
}
