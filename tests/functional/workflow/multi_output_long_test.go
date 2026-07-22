//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMultiOutput_WithStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output stop-word workflow sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output with stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Here is the plan and tasks. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"plan:complete": 1, "task:complete": 1, "request:init": 0, "request:failed": 0,
	})
}

func TestMultiOutput_WithoutStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output failure routing sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output without stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "I tried but could not finish"},
	)
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"request:failed": 1, "plan:init": 0, "task:init": 0, "plan:complete": 0, "task:complete": 0,
	})
}

func TestMultiOutput_NoStopWordsConfigured(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output no-stopword harness sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_no_stopwords_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Multi-output no stop words"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "planner output COMPLETE"},
		workerexecution.InferenceResponse{Content: "finisher output COMPLETE"},
		workerexecution.InferenceResponse{Content: "finisher output COMPLETE"},
	)
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"plan:complete": 1, "task:complete": 1, "request:init": 0, "request:failed": 0,
	})
}

func TestMultiOutput_SecondStopWord(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output alternate stop-word sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))
	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "Second stop word"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "All tasks generated. DONE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"plan:complete": 1, "task:complete": 1})
}

func TestMultiOutput_OutputTokensInheritInputLineage(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output lineage propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_dir"))

	inputTraceID := "trace-lineage-test"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "request",
		Payload:    []byte(`{"title": "Lineage test"}`),
		TraceID:    inputTraceID,
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Plan generated. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finished. COMPLETE"},
	)
	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"plan:complete": 1, "task:complete": 1})
	assertListedLineage(t, listedWork, map[string]string{"plan": inputTraceID, "task": inputTraceID})
}

func assertListedLineage(t *testing.T, response factoryapi.ListWorkResponse, wants map[string]string) {
	t.Helper()
	seen := map[string]bool{}
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != "complete" || item.WorkTypeName == nil {
			continue
		}
		want, ok := wants[*item.WorkTypeName]
		if !ok {
			continue
		}
		if item.TraceId == nil || *item.TraceId != want {
			t.Errorf("%s complete trace ID = %#v, want %q", *item.WorkTypeName, item.TraceId, want)
		}
		seen[*item.WorkTypeName] = true
	}
	for workType := range wants {
		if !seen[workType] {
			t.Errorf("listed Work missing %s:complete", workType)
		}
	}
}
