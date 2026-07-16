package bootstrap_portability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/testutil"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
)

// TestInitFactory_AgentSlotResourceAlignmentRunsWithMockWorkers proves the default
// init scaffold loads and runs when processor worker resources agree with the
// embedded canonical factory.json agent-slot pool.
func TestInitFactory_AgentSlotResourceAlignmentRunsWithMockWorkers(t *testing.T) {
	dir := t.TempDir()

	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	factoryJSON, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if !strings.Contains(string(factoryJSON), `"name": "agent-slot"`) {
		t.Fatalf("expected embedded factory.json to declare agent-slot resources pool; got:\n%s", factoryJSON)
	}
	if normalizeFactoryJSON(t, string(factoryJSON)) != normalizeFactoryJSON(t, initcmd.DefaultFactoryJSON()) {
		t.Fatalf("written factory.json does not match embedded canonical document")
	}

	workerPath := filepath.Join(dir, "workers", "processor", "AGENTS.md")
	workerBody, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read processor AGENTS.md: %v", err)
	}
	if !strings.Contains(string(workerBody), "agent-slot") {
		t.Fatalf("expected processor worker to declare agent-slot resources; got:\n%s", workerBody)
	}

	validateInitFactoryThroughCLI(t, dir)

	testutil.WriteSeedFile(t, dir, initcmd.DefaultFactoryInputType, []byte(`{"title": "agent-slot alignment e2e test"}`))

	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace(initcmd.DefaultFactoryInputType+":complete").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType+":init").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType+":failed").
		PlaceTokenCount(initcmd.DefaultFactoryInputType+":complete", 1).
		PlaceTokenCount("agent-slot:available", 1)

	if provider.CallCount("processor") != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount("processor"))
	}
}

func validateInitFactoryThroughCLI(t *testing.T, dir string) {
	t.Helper()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "factory", "config", "validate", dir)
	session.RequireSuccess(t, "init-agent-slot-factory-validation", result, err)
}

func normalizeFactoryJSON(t *testing.T, raw string) string {
	t.Helper()

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("normalizeFactoryJSON: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("normalizeFactoryJSON marshal: %v", err)
	}
	return string(encoded)
}
