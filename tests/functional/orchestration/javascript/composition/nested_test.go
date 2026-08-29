// Package composition holds customer functional scenarios for nested JavaScript
// composition primitives through the public process and invocation boundary.
package composition_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
func runJavaScriptNestedPipelineParallelCompositionCompletes(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldNestedCompletionWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startNestedCompletionWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child execution", got, providerCalls)
	}

	dispatches := listNestedCompletionDispatches(t, fixture, started.SessionId)
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
func runJavaScriptNestedFailureNamesChildAndStage(t *testing.T, fixture *compositionFixture) {
	dir := scaffoldNestedFailureWorkflow(t)
	providerCalls := fixture.provider.callCount()
	started := startNestedFailureWorkflow(t, fixture, dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.provider.callCount(); got != providerCalls {
		t.Fatalf("provider call count = %d, want unchanged at %d for fake child failure edge", got, providerCalls)
	}

	dispatches := listNestedFailureDispatches(t, fixture, started.SessionId)
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
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-nested-pipeline-parallel-composition", dir)
}

func startNestedFailureWorkflow(
	t *testing.T,
	fixture *compositionFixture,
	dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	return fixture.startFakeSync(t, "javascript-nested-pipeline-parallel-failure", dir)
}

func listNestedCompletionDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
}

func listNestedFailureDispatches(
	t *testing.T,
	fixture *compositionFixture,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return fixture.fakeDispatches(t, sessionID)
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
