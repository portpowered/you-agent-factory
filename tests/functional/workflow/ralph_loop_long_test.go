//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRalphLoop_TemplateFieldsResolvePerIteration(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		Payload:    []byte(`{"title": "template test"}`),
		Tags: map[string]string{
			"project":      "inventory-service",
			"branch":       "ralph/ralph-loop",
			"iteration_id": "iter-001",
		},
	})

	work := map[string][]workerexecution.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "missing error handling"},
			{Content: "looks good<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"story:complete": 1})

	if provider.CallCount("executor-worker") != 2 {
		t.Fatalf("expected at least 2 executor dispatches, got %d", provider.CallCount("executor-worker"))
	}

	for i, dispatch := range provider.Calls("executor-worker") {
		if dispatch.WorkingDirectory == "" {
			t.Errorf("dispatch %d: expected WorkingDirectory to be set, got empty", i)
		} else {
			expectedDir := support.ResolvedRuntimePath(dir, "/workspaces/ralph-loop-fixture/ralph/ralph-loop")
			if dispatch.WorkingDirectory != expectedDir {
				t.Errorf("dispatch %d: expected WorkingDirectory '%s', got '%s'", i, expectedDir, dispatch.WorkingDirectory)
			}
		}
		if dispatch.EnvVars["PROJECT"] != "ralph-loop-fixture" {
			t.Errorf("dispatch %d: expected env PROJECT=ralph-loop-fixture, got %s", i, dispatch.EnvVars["PROJECT"])
		}
		if dispatch.EnvVars["ITERATION_ID"] != "iter-001" {
			t.Errorf("dispatch %d: expected env ITERATION_ID=iter-001, got %s", i, dispatch.EnvVars["ITERATION_ID"])
		}
	}
}
