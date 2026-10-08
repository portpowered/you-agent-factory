package bootstrap_portability

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/clock"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryValidation rejects factories whose workstation wiring references
// undeclared workers before runtime bootstrap succeeds.
func TestFactoryValidation_RejectsWorkstationWithNonexistentWorker(t *testing.T) {
	t.Parallel()
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "invalid_worker_reference"))

	homeDir := t.TempDir()
	fakeEnv := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", filepath.Join(dir, "factory.json")})
	fakeEnv.Input.WorkingDirectory = dir
	fakeEnv.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	// Act

	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
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
	support.RequireSafeCLIDiagnostic(t, fakeEnv.Stderr(), true)
	assertFlattenFailuresAreCustomerVisible(t, process, dir, homeDir)
}

func assertFlattenFailuresAreCustomerVisible(t *testing.T, process support.Process, invalidDir, homeDir string) {
	t.Helper()
	invalid := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "flatten", invalidDir})
	invalid.Input.WorkingDirectory = invalidDir
	invalid.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	if err := process.Execute(invalid.Input); err == nil || !strings.Contains(err.Error(), "inline factory definition is incomplete") {
		t.Fatalf("flatten invalid Factory error = %v, want structural validation", err)
	}
	if invalid.Stdout() != "" {
		t.Fatalf("invalid Factory produced canonical output: %q", invalid.Stdout())
	}
	missing := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "flatten", filepath.Join(homeDir, "missing-factory")})
	missing.Input.WorkingDirectory = invalidDir
	missing.Input.Env = invalid.Input.Env
	if err := process.Execute(missing.Input); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("flatten missing Factory error = %v, want missing config diagnostic", err)
	}
	if missing.Stdout() != "" {
		t.Fatalf("missing Factory produced canonical output: %q", missing.Stdout())
	}
	validDir := support.ScaffoldFactory(t, map[string]any{"name": "flatten-output", "workTypes": []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}}}}})
	failure := errors.New("customer output stream closed")
	output := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "flatten", validDir})
	output.Input.WorkingDirectory = validDir
	output.Input.Env = invalid.Input.Env
	output.Input.Stdout = rejectedFactoryOutput{err: failure}
	if err := process.Execute(output.Input); !errors.Is(err, failure) || !strings.Contains(err.Error(), "write canonical factory config") {
		t.Fatalf("flatten output error = %v, want customer stream failure", err)
	}
}

type rejectedFactoryOutput struct{ err error }

func (writer rejectedFactoryOutput) Write([]byte) (int, error) { return 0, writer.err }
