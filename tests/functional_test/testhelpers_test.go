package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

var workingDirectoryMu sync.Mutex

type blockingExecutor struct {
	releaseCh <-chan struct{}
	mu        *sync.Mutex
	calls     *int
}

func (e *blockingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	*e.calls++
	e.mu.Unlock()

	<-e.releaseCh

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}

func fixtureDir(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", name)
}

func resolvedRuntimePath(factoryDir, configuredPath string) string {
	return filepath.Clean(filepath.Join(factoryDir, filepath.FromSlash(configuredPath)))
}

func setWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	workingDirectoryMu.Lock()
	originalDir, err := os.Getwd()
	if err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		workingDirectoryMu.Unlock()
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
		workingDirectoryMu.Unlock()
	})
}
