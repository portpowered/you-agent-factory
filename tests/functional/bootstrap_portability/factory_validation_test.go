package bootstrap_portability

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryValidation rejects factories whose workstation wiring references
// undeclared workers before runtime bootstrap succeeds.
func TestFactoryValidation_RejectsWorkstationWithNonexistentWorker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "invalid_worker_reference"))

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "factory", "config", "validate", dir)
	if err == nil {
		t.Fatalf("expected factory config validation to fail for workstation referencing non-existent worker: result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("factory config validation exit code = 0, want non-zero")
	}

	output := result.Stdout + result.Stderr

	if !strings.Contains(output, "Factory validation failed") {
		t.Errorf("expected factory validation failure summary, got: %s", output)
	}
	if !strings.Contains(output, "factory.worker.danglingReference") {
		t.Errorf("expected dangling-worker validation code, got: %s", output)
	}
	if !strings.Contains(output, "non-existent worker \"ghost-worker\"") {
		t.Errorf("expected dangling-worker validation detail, got: %s", output)
	}
}
