package bootstrap_portability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestInitFactory_AgentSlotResourceAlignmentRunsWithMockWorkers proves the default
// init scaffold loads and runs when processor worker resources agree with the
// embedded canonical factory.json agent-slot pool.
func TestInitFactory_AgentSlotResourceAlignmentRunsWithMockWorkers(t *testing.T) {
	dir := t.TempDir()

	support.RunInitCommand(t, dir)

	factoryJSON, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if !strings.Contains(string(factoryJSON), `"name": "agent-slot"`) {
		t.Fatalf("expected embedded factory.json to declare agent-slot resources pool; got:\n%s", factoryJSON)
	}
	referenceDir := t.TempDir()
	support.RunInitCommand(t, referenceDir)
	referenceFactoryJSON, err := os.ReadFile(filepath.Join(referenceDir, "factory.json"))
	if err != nil {
		t.Fatalf("read reference factory.json: %v", err)
	}
	if normalizeFactoryJSON(t, string(factoryJSON)) != normalizeFactoryJSON(t, string(referenceFactoryJSON)) {
		t.Fatalf("two customer init commands produced different canonical factory documents")
	}

	workerPath := filepath.Join(dir, "workers", "processor", "AGENTS.md")
	workerBody, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read processor AGENTS.md: %v", err)
	}
	if !strings.Contains(string(workerBody), "agent-slot") {
		t.Fatalf("expected processor worker to declare agent-slot resources; got:\n%s", workerBody)
	}

	testutil.WriteSeedFile(t, dir, defaultInitFactoryWorkType, []byte(`{"title": "agent-slot alignment e2e test"}`))

	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	for placeID, want := range map[string]int{
		defaultInitFactoryWorkType + ":complete": 1,
		defaultInitFactoryWorkType + ":init":     0,
		defaultInitFactoryWorkType + ":failed":   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	for _, usage := range session.Runtime.Usage.Resources {
		if usage.Name == "agent-slot" {
			if usage.Available != 1 || usage.Total != 1 {
				t.Errorf("agent-slot resource usage = %#v, want 1 available and total", usage)
			}
			break
		}
	}

	if provider.CallCount("processor") != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount("processor"))
	}
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
