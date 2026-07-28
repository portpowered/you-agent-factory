package testhelpers

import (
	"context"
	"os"
	"testing"

	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// StaticRuntimeConfig is a runtime-config lookup fixture for workstation executor tests.
type StaticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

// WSMockExecutor records workstation execution requests for tests.
type WSMockExecutor struct {
	Dispatch workers.WorkstationExecutionRequest
	Called   bool
	Result   workers.WorkResult
	Err      error
}

func (m *WSMockExecutor) Execute(_ context.Context, request workers.WorkstationExecutionRequest) (workers.WorkResult, error) {
	m.Called = true
	m.Dispatch = request
	return m.Result, m.Err
}

// SetTestWorkingDirectory changes the process working directory for the duration of a test.
func SetTestWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
