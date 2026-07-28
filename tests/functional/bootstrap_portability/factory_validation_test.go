package bootstrap_portability

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/root"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryValidation rejects factories whose workstation wiring references
// undeclared workers before runtime bootstrap succeeds.
func TestFactoryValidation_RejectsWorkstationWithNonexistentWorker(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "invalid_worker_reference"))

	support.SetWorkingDirectory(t, dir)

	fakeEnv := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", "./factory.json"})
	homeDir := t.TempDir()
	fakeEnv.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	// Act

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		Clock: clock.Ensure(nil),
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(fakeEnv.Input)

	if err == nil {
		t.Fatal("expected Wire graph construction to fail for workstation referencing non-existent worker")
	}

	if !strings.Contains(err.Error(), "invalid named factory") {
		t.Errorf("expected load-boundary invalid factory error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Errorf("expected blocking structural validation summary, got: %v", err)
	}
	if !strings.Contains(err.Error(), "factory.worker.danglingReference") {
		t.Errorf("expected dangling worker reference diagnostic, got: %v", err)
	}
	if fakeEnv.Stdout() != "" {
		t.Errorf("expected no stdout before validation failed, got: %q", fakeEnv.Stdout())
	}
	if !strings.Contains(fakeEnv.Stderr(), "factory.worker.danglingReference") {
		t.Errorf("expected validation diagnostic on stderr, got: %q", fakeEnv.Stderr())
	}
}
