// Package stdio implements the ACP agent-side JSON-RPC stdio server:
// caller-owned stream serving, one-connection lifecycle, and
// connection-scoped identity assignment. Framing, request dispatch, and
// shutdown semantics beyond clean EOF are added by later stories in this
// slice. It is internal to pkg/transports/acp; callers use the package
// root's exported operations instead of this package directly.
package stdio

import (
	"bufio"
	"context"
	"errors"
	"io"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// ErrStreamsRequired marks a Serve call that is missing a required input or
// output stream.
var ErrStreamsRequired = errors.New("acp: stdio serve requires input and output streams")

// Server serves one ACP JSON-RPC agent-side connection at a time over
// caller-owned stdio streams. Construction performs no I/O and starts no
// goroutine, process, listener, session, or persistence; each Serve call
// begins one connection-scoped invocation over the streams the caller
// supplies for that call.
type Server struct {
	logger logging.Logger
}

// New constructs an inert stdio Server. Construction alone performs no
// reads, writes, goroutine starts, process starts, endpoint binding,
// session creation, or persistence.
func New(logger logging.Logger) *Server {
	return &Server{logger: logging.EnsureLogger(logger)}
}

// Serve begins one connection-scoped serving invocation over caller-owned
// input and output streams and returns once the connection terminates.
// Starting the invocation creates one connection-scoped identity, minted
// via identity.NewConnectionID, that is stable for the duration of this
// call and distinct from every other invocation. Start and terminal
// outcomes are logged as structured, payload-free diagnostics carrying only
// the connection id and a bounded outcome label.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if s == nil {
		return errors.New("acp: stdio server is required")
	}
	if in == nil || out == nil {
		return ErrStreamsRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	connectionID := identity.NewConnectionID()
	logger := logging.EnsureLogger(s.logger)
	logger.Info("acp stdio connection started", "connectionId", string(connectionID))

	err := drainFrames(in)

	logger.Info("acp stdio connection terminated",
		"connectionId", string(connectionID),
		"outcome", terminalOutcomeLabel(err),
	)
	return err
}

// terminalOutcomeLabel classifies a Serve result into a bounded, safe label
// for diagnostics. It never includes the error's message text, so a cause
// that happens to mention sensitive request content can never reach a log
// record through this label.
func terminalOutcomeLabel(err error) string {
	if err == nil {
		return "eof"
	}
	return "error"
}

// drainFrames reads line-delimited input to a clean EOF. It establishes the
// one-connection read lifecycle this story owns; decoding lines into
// JSON-RPC envelopes and dispatching them is added by a later story in this
// slice.
func drainFrames(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
	}
	return scanner.Err()
}
