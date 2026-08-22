package process_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var processCLIBinary struct {
	once     sync.Once
	tempDir  string
	path     string
	err      error
	buildLog []byte
}

// TestMain owns the one reusable OS-boundary binary for this package. The
// process-package tests run under -count=N, so rebuilding the same binary for
// every repetition needlessly creates nested Go build processes that can
// contend with the functional children and the build cache.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if processCLIBinary.tempDir != "" {
		if err := os.RemoveAll(processCLIBinary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove reusable CLI binary directory %s: %v\n", processCLIBinary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func buildYouBinary(t testing.TB, ctx context.Context, repoRoot string) string {
	t.Helper()
	processCLIBinary.once.Do(func() {
		processCLIBinary.tempDir, processCLIBinary.err = os.MkdirTemp("", "you-cli-process-package-")
		if processCLIBinary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		processCLIBinary.path = filepath.Join(processCLIBinary.tempDir, binaryName)
		command := exec.CommandContext(ctx, "go", "build", "-o", processCLIBinary.path, "./cmd/factory")
		command.Dir = repoRoot
		processCLIBinary.buildLog, processCLIBinary.err = command.CombinedOutput()
	})
	if processCLIBinary.err != nil {
		t.Fatalf("build you CLI: %v\n%s", processCLIBinary.err, processCLIBinary.buildLog)
	}
	return processCLIBinary.path
}
