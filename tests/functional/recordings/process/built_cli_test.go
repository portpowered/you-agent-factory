//go:build windows

package recordingsprocess_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const recordingProcessCLIHelperEnv = "YOU_RECORDING_PROCESS_CLI_HELPER"

func buildYouBinary(t testing.TB, _ context.Context, _ string) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve functional test executable: %v", err)
	}
	return path
}

func recordingProcessCLICommand(binaryPath string, args ...string) *exec.Cmd {
	helperArgs := []string{"-test.run=^TestRecordingProcessCLIHelper$", "--", "you"}
	helperArgs = append(helperArgs, args...)
	return exec.Command(binaryPath, helperArgs...)
}

func TestRecordingProcessCLIHelper(t *testing.T) {
	if os.Getenv(recordingProcessCLIHelperEnv) != "1" {
		return
	}
	os.Exit(runRecordingProcessCLI())
}

func runRecordingProcessCLI() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	process, err := support.BuildProcessWithContext(ctx, serviceedges.Edges{})
	if err != nil {
		return 1
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = process.Close(closeCtx)
	}()

	workingDirectory, err := os.Getwd()
	if err != nil {
		return 1
	}
	stdinTTY := false
	stdoutTTY := false
	err = process.Execute(root.Input{
		Args: recordingProcessCLIArgs(os.Args),
		Env:  os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Context: ctx, WorkingDirectory: workingDirectory,
		StdinIsTTY: &stdinTTY, StdoutIsTTY: &stdoutTTY,
	})
	if ctx.Err() != nil {
		return 130
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return 1
	}
	return 0
}

func recordingProcessCLIArgs(args []string) []string {
	for index, arg := range args {
		if arg == "--" && index+1 < len(args) {
			return append([]string(nil), args[index+1:]...)
		}
	}
	return []string{"you"}
}
