//go:build functionallong

package smoke

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestStatelessExecutionSmoke_LoadedConfigDrivesExecution(t *testing.T) {
	support.SkipLongFunctional(t, "slow stateless-loaded-config sweep")
	originalDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, originalDir, "task", []byte(`{"item":"original-config"}`))

	originalProvider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)
	originalHarness := testutil.NewServiceTestHarness(t, originalDir,
		testutil.WithProvider(originalProvider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	originalHarness.RunUntilComplete(t, 10*time.Second)

	originalHarness.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	originalCalls := originalProvider.Calls()
	if len(originalCalls) != 2 {
		t.Fatalf("expected 2 provider calls for original config, got %d", len(originalCalls))
	}
	if !strings.Contains(originalCalls[1].UserMessage, "Step 2 workstation.") {
		t.Fatalf("expected original step2 prompt, got %q", originalCalls[1].UserMessage)
	}

	updatedDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	writeSmokeTestFile(t, filepath.Join(updatedDir, "workers", "agent", "AGENTS.md"), `---
type: MODEL_WORKER
model: test-model
stopToken: APPROVED
---
Process the work item.
`)
	writeSmokeTestFile(t, filepath.Join(updatedDir, "workstations", "step2", "AGENTS.md"), `---
type: MODEL_WORKSTATION
---
Updated Step 2 workstation.
`)
	testutil.WriteSeedFile(t, updatedDir, "task", []byte(`{"item":"updated-config"}`))

	updatedProvider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Stage 1 approved. APPROVED"},
		interfaces.InferenceResponse{Content: "Stage 2 approved. APPROVED"},
	)
	updatedHarness := testutil.NewServiceTestHarness(t, updatedDir,
		testutil.WithProvider(updatedProvider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	updatedHarness.RunUntilComplete(t, 10*time.Second)

	updatedHarness.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	updatedCalls := updatedProvider.Calls()
	if len(updatedCalls) != 2 {
		t.Fatalf("expected 2 provider calls for updated config, got %d", len(updatedCalls))
	}
	if !strings.Contains(updatedCalls[0].UserMessage, "Step 1 workstation.") {
		t.Fatalf("expected step1 prompt to remain unchanged, got %q", updatedCalls[0].UserMessage)
	}
	if !strings.Contains(updatedCalls[1].UserMessage, "Updated Step 2 workstation.") {
		t.Fatalf("expected updated step2 prompt, got %q", updatedCalls[1].UserMessage)
	}
}

func writeSmokeTestFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
