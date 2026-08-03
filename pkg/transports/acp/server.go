package acp

import (
	"context"
	"io"
)

// Server serves one ACP JSON-RPC agent-side connection at a time over
// caller-owned stdio streams. Construction alone performs no reads, writes,
// goroutine starts, process starts, endpoint binding, session creation, or
// persistence; a Server begins connection I/O only when Serve is called.
//
// Production construction is owned by pkg/transports/acp/wire, which
// injects only explicit collaborators; this package never resolves a
// Server through a dependency bag, service locator, secondary injector, or
// process-global stdio lookup.
type Server interface {
	// Serve begins one connection-scoped invocation over caller-owned input
	// and output streams and returns once the connection terminates. Each
	// call is assigned its own connection-scoped identity, stable for that
	// call and distinct from every other invocation.
	Serve(ctx context.Context, in io.Reader, out io.Writer) error
}
