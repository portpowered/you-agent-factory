//go:build windows

package recordingsprocess_test

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

var recordingProcessCLIBinary struct {
	once     sync.Once
	tempDir  string
	path     string
	err      error
	buildLog []byte
}

func buildYouBinary(t testing.TB, ctx context.Context, repoRoot string) string {
	t.Helper()
	recordingProcessCLIBinary.once.Do(func() {
		recordingProcessCLIBinary.tempDir, recordingProcessCLIBinary.err = os.MkdirTemp("", "you-cli-recordings-process-")
		if recordingProcessCLIBinary.err != nil {
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		recordingProcessCLIBinary.path = filepath.Join(recordingProcessCLIBinary.tempDir, binaryName)
		command := exec.CommandContext(ctx, "go", "build", "-o", recordingProcessCLIBinary.path, "./cmd/factory")
		command.Dir = repoRoot
		recordingProcessCLIBinary.buildLog, recordingProcessCLIBinary.err = command.CombinedOutput()
	})
	if recordingProcessCLIBinary.err != nil {
		t.Fatalf("build you CLI: %v\n%s", recordingProcessCLIBinary.err, recordingProcessCLIBinary.buildLog)
	}
	return recordingProcessCLIBinary.path
}

// TestMain builds the CLI once for the recordings process package.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if recordingProcessCLIBinary.tempDir != "" {
		if err := removeRecordingProcessCLIBinaryDir(recordingProcessCLIBinary.tempDir); err != nil && exitCode == 0 {
			fmt.Fprintf(os.Stderr, "remove recordings process CLI binary directory %s: %v\n", recordingProcessCLIBinary.tempDir, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func removeRecordingProcessCLIBinaryDir(path string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.RemoveAll(path)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		// Windows can retain an executable image briefly after Wait returns.
		time.Sleep(50 * time.Millisecond)
	}
}
