package workers

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	runtimefixtures "github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture

type wsMockExecutor struct {
	dispatch interfaces.WorkstationExecutionRequest
	called   bool
	result   interfaces.WorkResult
	err      error
}

func (m *wsMockExecutor) Execute(_ context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
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
