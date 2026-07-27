package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestParameterizedFields_WorkingDirectoryResolvesFromTags(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1})
	calls := provider.Calls("exec-worker")
	if len(calls) == 0 {
		t.Fatal("exec-worker provider was never called")
	}
	call := calls[0]
	if call.Dispatch.WorkstationName == "" {
		t.Error("expected WorkstationName to be set on dispatch")
	}
	if len(call.Dispatch.InputTokens) == 0 {
		t.Fatal("expected at least one input token")
	}
	tags := firstInputToken(call.Dispatch.InputTokens).Color.Tags
	if tags["branch"] != "feature-abc" {
		t.Errorf("expected tag branch=feature-abc, got %q", tags["branch"])
	}
}

func TestParameterizedFields_UnresolvedTemplateRoutesToFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "parameterized_failure"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "unresolved template test"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Should not reach COMPLETE"},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:complete": 0})

	if provider.CallCount() != 0 {
		t.Errorf("expected provider called 0 times (template error before invocation), got %d", provider.CallCount())
	}
}
