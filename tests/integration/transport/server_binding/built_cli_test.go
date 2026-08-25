package server_binding_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

const serverBindFailureProcessTimeout = time.Minute

var serverBindingCLIBinary struct {
	once     sync.Once
	tempDir  string
	path     string
	err      error
	buildLog []byte
}

// TestMain owns the reusable OS-boundary binary for this package. The bind
// failure test runs under -count=N; rebuilding the same binary for every
// repetition creates nested Go build processes and keeps the blocking terminal
// listener reserved while unrelated build work is still running.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if serverBindingCLIBinary.tempDir != "" {
		if err := os.RemoveAll(serverBindingCLIBinary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove reusable server-binding CLI directory %s: %v\n", serverBindingCLIBinary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func buildServerBindingBinary(t testing.TB, ctx context.Context, repoRoot string) string {
	t.Helper()
	serverBindingCLIBinary.once.Do(func() {
		serverBindingCLIBinary.tempDir, serverBindingCLIBinary.err = os.MkdirTemp("", "you-server-binding-")
		if serverBindingCLIBinary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		serverBindingCLIBinary.path = filepath.Join(serverBindingCLIBinary.tempDir, binaryName)
		build := exec.CommandContext(ctx, "go", "build", "-o", serverBindingCLIBinary.path, "./cmd/factory")
		build.Dir = repoRoot
		serverBindingCLIBinary.buildLog, serverBindingCLIBinary.err = build.CombinedOutput()
	})
	if serverBindingCLIBinary.err != nil {
		t.Fatalf("build you CLI: %v\n%s", serverBindingCLIBinary.err, serverBindingCLIBinary.buildLog)
	}
	return serverBindingCLIBinary.path
}
