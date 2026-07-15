package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

const (
	OutputStreamStdout = "stdout"
	OutputStreamStderr = "stderr"
)

// OutputChunkObserver receives incremental stdout or stderr bytes while a
// subprocess is still running.
type OutputChunkObserver func(stream string, chunk []byte)

// StreamingExecCommandRunner executes subprocesses and emits stdout/stderr
// chunks through an observer before the command exits.
type StreamingExecCommandRunner struct {
	Observer OutputChunkObserver
	Logger   logging.Logger
}

// Run executes the command with process-tree cancellation, capturing stdout and
// stderr while forwarding incremental chunks to Observer when configured.
func (r StreamingExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}

	cmd := exec.Command(req.Command, req.Args...)
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	stdout := &observedBuffer{stream: OutputStreamStdout, observer: r.Observer}
	stderr := &observedBuffer{stream: OutputStreamStderr, observer: r.Observer}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	configureCommandProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return CommandResult{}, err
	}

	tree, _ := attachCommandProcessTree(cmd)
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	cleanupLogger := logging.EnsureLogger(r.Logger)
	cancelCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonCancel)
	postRunCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonPostRun)

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		_ = terminateCommandProcessTree(cmd, tree, cancelCleanup)
		<-waitCh
		closeCommandProcessTree(cmd, tree, postRunCleanup)
		return CommandResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
		}, ctx.Err()
	}
	closeCommandProcessTree(cmd, tree, postRunCleanup)

	result := CommandResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	return result, nil
}

type observedBuffer struct {
	stream   string
	observer OutputChunkObserver
	buf      bytes.Buffer
}

func (b *observedBuffer) Write(p []byte) (int, error) {
	n, err := b.buf.Write(p)
	if err != nil {
		return n, err
	}
	if b.observer != nil && n > 0 {
		chunk := append([]byte(nil), p[:n]...)
		b.observer(b.stream, chunk)
	}
	return n, nil
}

func (b *observedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

var _ io.Writer = (*observedBuffer)(nil)
var _ CommandRunner = StreamingExecCommandRunner{}
