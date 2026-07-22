package process

import (
	"bytes"
	"context"
	"io"

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
	Observer   OutputChunkObserver
	Logger     logging.Logger
	Clock      Clock
	NewCommand CommandFactory
}

// Run executes the command with process-tree cancellation, capturing stdout and
// stderr while forwarding incremental chunks to Observer when configured.
func (r StreamingExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	runner := ExecCommandRunner{Logger: r.Logger, Clock: r.Clock, NewCommand: r.NewCommand}
	return runner.run(ctx, req, r.Observer)
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
