//go:build functionallong

package smoke

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestIntegrationSmoke_GuardedLoopBreakerRoutesOverLimitExampleWorkToFailed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
	support.ClearSeedInputs(t, dir)
	assertFactoryHasNoTopLevelExhaustionRules(t, dir)

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {
			{Content: "<result>ACCEPTED</result>"},
			{Content: "<result>ACCEPTED</result>"},
			{Content: "<result>ACCEPTED</result>"},
		},
		"reviewer": {
			{Content: "<result>REJECTED</result>\nneeds revision"},
			{Content: "<result>REJECTED</result>\nstill blocked"},
			{Content: "<result>REJECTED</result>\nmissing acceptance criteria"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: "story",
		WorkID:     "guarded-loop-breaker-smoke",
		TraceID:    "trace-guarded-loop-breaker-smoke",
		Name:       "guarded loop breaker smoke",
		Payload:    []byte("prove guarded loop breaker"),
	}})
	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("story:failed", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:in-review").
		HasNoTokenInPlace("story:complete")

	if got := provider.CallCount("reviewer"); got != 3 {
		t.Fatalf("reviewer calls = %d, want 3 before guarded loop breaker", got)
	}
	if got := provider.CallCount("executor"); got < 3 {
		t.Fatalf("executor calls = %d, want at least 3 before guarded loop breaker", got)
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstation(t, snapshot.DispatchHistory, "review-loop-breaker", "story:failed", "guarded-loop-breaker-smoke")
}

func assertFactoryHasNoTopLevelExhaustionRules(t *testing.T, dir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse factory.json: %v", err)
	}

	if rules, ok := config["exhaustion_rules"]; ok {
		t.Fatalf("factory.json unexpectedly includes top-level exhaustion_rules: %#v", rules)
	}
}

func assertDispatchHistoryContainsWorkstation(
	t *testing.T,
	history []interfaces.CompletedDispatch,
	workstationName string,
	terminalPlace string,
	workID string,
) {
	t.Helper()

	for _, dispatch := range history {
		if dispatch.WorkstationName != workstationName {
			continue
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.ToPlace != terminalPlace || mutation.Token == nil {
				continue
			}
			if mutation.Token.Color.WorkID == workID {
				return
			}
		}
	}

	t.Fatalf(
		"dispatch history missing %q route to %q for work %q: %#v",
		workstationName,
		terminalPlace,
		workID,
		history,
	)
}
