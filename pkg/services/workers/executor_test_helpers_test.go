package workers_test

import (
	"context"
	"os"
	"testing"

	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

type wsMockExecutor struct {
	dispatch workers.WorkstationExecutionRequest
	called   bool
	result   workers.WorkResult
	err      error
}

func (m *wsMockExecutor) Execute(_ context.Context, request workers.WorkstationExecutionRequest) (workers.WorkResult, error) {
	m.called = true
	m.dispatch = request
	return m.result, m.err
}

func setTestWorkingDirectory(t *testing.T, dir string) {
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
