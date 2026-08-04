package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// productionServeCommand builds the you serve family, whose acp leaf hosts
// the process-composed ACP server over caller-owned stdio. It is bound
// through the manifest's stable you.serve.acp handler ID via the same
// generic manifest constructor every other resolved command family uses
// (see you mcp serve's climanifestcobra.NewMCPCommand), so its projected
// help, usage, and examples always match the authoritative manifest.
func productionServeCommand(options CommandFactory) *cobra.Command {
	serve, err := climanifestcobra.NewServeCommand(resolvedServeACPHandler(options))
	if err != nil {
		panic(fmt.Sprintf("build serve family command: %v", err))
	}
	return serve
}

// resolvedServeACPHandler adapts the injected ACP server into the generic
// manifest constructor's ResolvedCobraHandler shape. you.serve.acp declares
// no local inputs, so both resolved-input snapshots are unused.
func resolvedServeACPHandler(options CommandFactory) climanifestcobra.ResolvedCobraHandler {
	return func(cmd *cobra.Command, _ resolvedinput.Inputs, _ resolvedinput.Inputs) error {
		return runServeACP(cmd, options.acpServer)
	}
}

// runServeACP invokes Serve on the exact acp.Server instance Wire composed
// from the process's canonical Chat Sessions and Factory Sessions authority
// -- no command-local ACP server, service graph, or secondary injector is
// constructed, and no HTTP/dashboard listener is started.
//
// stdio.Server.Serve only ever checks the supplied context for cancellation
// between reads; it cannot itself unblock a read that is already parked
// waiting for the next input line (see
// pkg/transports/acp/internal/stdio/server_test.go's
// TestServeReturnsContextErrorOnMidReadCancellation, which requires the
// caller to close the stream on cancellation). Production stdin is a live
// stream that may sit idle indefinitely between protocol frames, so this
// command closes it itself once the invocation's context is cancelled,
// exactly the responsibility that test assigns to the caller.
func runServeACP(cmd *cobra.Command, server acp.Server) error {
	if server == nil {
		return errors.New("serve acp: ACP server is required")
	}
	ctx := cmd.Context()
	in := cmd.InOrStdin()
	if closer, ok := in.(io.Closer); ok {
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	return sanitizeServeACPError(server.Serve(ctx, in, cmd.OutOrStdout()))
}

// sanitizeServeACPError bounds every non-cancellation Serve outcome to a
// fixed, payload-free diagnostic before it can reach stderr. A Serve error
// can originate from arbitrary decoded request content (an unmarshal
// failure that echoes the offending JSON, for example), so nothing about
// its original message is safe to print verbatim; only the two well-known
// cancellation sentinels are ever returned unchanged, preserving Cobra's and
// cmd/factory/main.go's own errors.Is(err, context.Canceled) exit
// classification.
func sanitizeServeACPError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return errors.New("serve acp: connection ended with an error")
	}
}
