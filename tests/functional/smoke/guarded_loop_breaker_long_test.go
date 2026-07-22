//go:build functionallong

package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestIntegrationSmoke_GuardedLoopBreakerRoutesOverLimitExampleWorkToFailed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
	support.ClearSeedInputs(t, dir)
	assertFactoryHasNoTopLevelExhaustionRules(t, dir)

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
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

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "story",
		WorkID:     "guarded-loop-breaker-smoke",
		TraceID:    "trace-guarded-loop-breaker-smoke",
		Name:       "guarded loop breaker smoke",
		Payload:    []byte("prove guarded loop breaker"),
	})
	session := support.RunFactoryToCompletion(t, dir, provider, 15*time.Second)
	for placeID, want := range map[string]int{
		"story:failed":    1,
		"story:init":      0,
		"story:in-review": 0,
		"story:complete":  0,
	} {
		if got := support.SessionPlaceTokenCount(session, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if got := provider.CallCount("reviewer"); got != 3 {
		t.Fatalf("reviewer calls = %d, want 3 before guarded loop breaker", got)
	}
	if got := provider.CallCount("executor"); got < 3 {
		t.Fatalf("executor calls = %d, want at least 3 before guarded loop breaker", got)
	}

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
