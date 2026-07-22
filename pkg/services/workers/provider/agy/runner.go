package agy

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

// PTYRunner executes Agy invocations through the native PTY boundary instead of
// piped subprocess IO.
type PTYRunner struct {
	Allocator agypty.PTYAllocator
	Config    agypty.SessionConfig
}

// NewPTYRunner constructs a runner that allocates platform or mock PTY sessions.
func NewPTYRunner(allocator agypty.PTYAllocator, cfg agypty.SessionConfig) (PTYRunner, error) {
	if allocator == nil {
		return PTYRunner{}, errors.New("agy: PTY allocator is required")
	}
	return PTYRunner{Allocator: allocator, Config: cfg}, nil
}

// Run implements adapter.StreamingCommandRunner.
func (r PTYRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(adapter.Observation) error,
) (workerprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return workerprocess.CommandResult{}, err
	}
	argv := append([]string{req.Command}, req.Args...)
	if err := agypty.ValidateArgv(argv); err != nil {
		return workerprocess.CommandResult{}, err
	}
	launch := agypty.ProcessLaunch{
		Executable: req.Command,
		Argv:       argv,
		WorkDir:    req.WorkDir,
		Env:        req.Env,
	}
	session, err := r.Allocator.Allocate(ctx, launch, r.Config)
	if err != nil {
		return workerprocess.CommandResult{}, err
	}
	defer session.Close()

	result, err := session.Run(ctx)
	if err != nil {
		return workerprocess.CommandResult{}, err
	}
	cleaned := cleanedPTYText(result)
	commandResult := workerprocess.CommandResult{
		Stdout:   []byte(cleaned),
		ExitCode: result.ExitCode,
	}
	if result.TimedOut {
		commandResult.ExitCode = 124
		return commandResult, fmt.Errorf("%w", agypty.ErrSessionTimedOut)
	}
	if len(commandResult.Stdout) > 0 {
		if observeErr := observe(adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: commandResult.Stdout}); observeErr != nil {
			return commandResult, observeErr
		}
	}
	if commandResult.ExitCode != 0 {
		return commandResult, fmt.Errorf("%w: exit code %d", agypty.ErrNonzeroExit, commandResult.ExitCode)
	}
	return commandResult, err
}

func cleanedPTYText(result agypty.SessionResult) string {
	if len(result.RawBytes) > 0 {
		return agypty.CleanTerminal(result.RawBytes)
	}
	return result.CleanedText
}
