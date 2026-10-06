package factory_session_snapshots_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Keep the private checkpoint scale harness in the stress lane without adding
// production exports. Go's build overlay adds one test to its owning package;
// that package's existing helpers and TestMain still run unchanged.
func TestSnapshotGrowthStress(t *testing.T) {
	if testing.Short() {
		t.Skip("snapshot scale profile belongs to the stress lane")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate snapshot stress sources")
	}
	dir := filepath.Dir(source)
	root := filepath.Clean(filepath.Join(dir, "../../.."))
	target := filepath.Join(root, "pkg/services/factory_sessions/internal/execution/snapshot_growth_stress_test.go")
	fixture := filepath.Join(dir, "testdata/snapshot_growth_test.go.txt")
	overlay, err := json.Marshal(struct{ Replace map[string]string }{map[string]string{target: fixture}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "overlay.json")
	if err := os.WriteFile(path, overlay, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-p=2", "-vet=off", "-count=1", "-timeout=4m", "-v", "-overlay="+path,
		"-run=^(TestDurablePetriFailureHistorySnapshotGrowthStress|TestPersistSessionSnapshotWarningScale|TestCompactPetriTokenHistoryScale|TestPetriFeedbackSnapshotGrowthScale|TestPetriFeedbackActualCapRecoveryScale)$", "./pkg/services/factory_sessions/internal/execution")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	t.Logf("workload: 10/100/1000 retries and same-token 50 KiB feedback, 64/128 MiB limits and actual-cap recovery, 500/1000 terminal lifecycles; budget: five minutes; preserve failure-log bounds and constant same-token history\n%s", output)
	if err != nil {
		t.Fatalf("snapshot growth stress: %v", err)
	}
}
