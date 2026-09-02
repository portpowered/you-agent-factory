package restart_test

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestBoardPersistenceCLIProcessHelper crosses the restart boundary with the
// already-built functional test executable while invoking the production root
// and customer CLI contract.
func TestBoardPersistenceCLIProcessHelper(t *testing.T) {
	if os.Getenv(boardPersistenceCLIHelperEnv) != "1" {
		return
	}
	os.Exit(runBoardPersistenceCLIProcess())
}

func runBoardPersistenceCLIProcess() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	process, err := root.BuildProcess(ctx, serviceedges.Edges{
		BrowserOpener: func(context.Context, string) error { return nil },
	})
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
		Args: cliHelperArgs(os.Args),
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

func cliHelperArgs(args []string) []string {
	for index, arg := range args {
		if arg == "--" && index+1 < len(args) {
			return append([]string(nil), args[index+1:]...)
		}
	}
	return []string{"you"}
}
