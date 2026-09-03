package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// productionServeCommand builds the shared server family snapshot whose acp leaf hosts
// the process-composed ACP server over caller-owned stdio. It is bound
// through the manifest's stable you.server.acp handler ID via the same
// generic manifest constructor every other resolved command family uses
// (see the MCP child constructor), so its projected
// help, usage, and examples always match the authoritative manifest.
func productionServeCommand(options CommandFactory) *cobra.Command {
	serve, err := climanifestcobra.NewServeCommand(resolvedServeACPHandler(options))
	if err != nil {
		panic(fmt.Sprintf("build serve family command: %v", err))
	}
	if err := suppressUnrelatedServeACPFlags(serve); err != nil {
		panic(fmt.Sprintf("build serve family command: %v", err))
	}
	return serve
}

// serveACPUnrelatedFlagNames are inherited root or server-parent flags with no
// ACP-stdio meaning: --json promises structured command output on stdout,
// which is already reserved for ACP protocol frames, while --listen, --pprof,
// and --server configure an HTTP API endpoint this command never contacts (it starts no HTTP or
// dashboard listener).
var serveACPUnrelatedFlagNames = []string{"json", "listen", "pprof", "remote", "server"}

// suppressUnrelatedServeACPFlags hides the shared server family's unrelated
// flags from its rendered help and rejects them if explicitly
// supplied.
//
// Cobra flag inheritance is pointer-shared: (*cobra.Command).PersistentFlags
// registers one *pflag.Flag object on the root, and every descendant's
// InheritedFlags() (via mergePersistentFlags/parentsPflags) references that
// same object, so calling MarkHidden on it from here would hide --json and
// --server on every other command too. Instead this registers new, local
// flags of the same name directly on the acp leaf: (*cobra.Command).
// InheritedFlags() skips any parent flag whose name already has a local
// flag on the command (see cobra's own local.Lookup(f.Name) == nil guard),
// so the shared root flags stay fully visible and functional on every
// sibling command, and only acp's own local copies -- which are marked
// hidden -- are affected.
func suppressUnrelatedServeACPFlags(serve *cobra.Command) error {
	acpCmd, _, err := serve.Find([]string{"acp"})
	if err != nil {
		return fmt.Errorf("find acp leaf: %w", err)
	}
	var jsonShadow bool
	var listenShadow string
	var pprofShadow bool
	var remoteShadow bool
	var serverShadow string
	acpCmd.Flags().BoolVar(&jsonShadow, "json", false, "")
	acpCmd.Flags().StringVar(&listenShadow, "listen", "", "")
	acpCmd.Flags().BoolVar(&pprofShadow, "pprof", false, "")
	acpCmd.Flags().BoolVar(&remoteShadow, "remote", false, "")
	acpCmd.Flags().StringVar(&serverShadow, "server", "", "")
	for _, name := range serveACPUnrelatedFlagNames {
		if err := acpCmd.Flags().MarkHidden(name); err != nil {
			return fmt.Errorf("hide flag %q: %w", name, err)
		}
	}
	acpCmd.PreRunE = rejectServeACPUnrelatedFlags
	return nil
}

// rejectServeACPUnrelatedFlags fails the invocation if a caller explicitly
// passes an unrelated HTTP/server flag, rather than silently accepting and ignoring
// them.
func rejectServeACPUnrelatedFlags(cmd *cobra.Command, _ []string) error {
	for _, name := range serveACPUnrelatedFlagNames {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf(
				"server acp: --%s is not supported; ACP stdio reserves stdout for protocol frames and starts no HTTP server",
				name,
			)
		}
	}
	return nil
}

// resolvedServeACPHandler adapts the injected ACP server into the generic
// manifest constructor's ResolvedCobraHandler shape. you.server.acp declares
// no local inputs, so both resolved-input snapshots are unused.
func resolvedServeACPHandler(options CommandFactory) climanifestcobra.ResolvedCobraHandler {
	return func(cmd *cobra.Command, _ resolvedinput.Inputs, _ resolvedinput.Inputs) error {
		profile, err := resolveACPInvocationProfile(cmd, options)
		if err != nil {
			return err
		}
		cmd.SetContext(acp.WithInvocationProfile(cmd.Context(), profile))
		return runServeACP(cmd, options.acpServer)
	}
}

func resolveACPInvocationProfile(cmd *cobra.Command, options CommandFactory) (acp.InvocationProfile, error) {
	homeDir, err := resolveProcessHomeDirForCommand(cmd, options)
	if err != nil {
		return acp.InvocationProfile{}, err
	}
	var provider, model string
	if options.lookupEnv != nil {
		provider, _ = options.lookupEnv(operatorsettings.EnvDefaultWorkerModelProvider)
		model, _ = options.lookupEnv(operatorsettings.EnvDefaultWorkerModel)
	}
	return acp.InvocationProfile{
		HomeDir: strings.TrimSpace(homeDir), WorkerModelProvider: strings.TrimSpace(provider), WorkerModel: strings.TrimSpace(model),
	}, nil
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
		return errors.New("server acp: ACP server is required")
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
		return errors.New("server acp: connection ended with an error")
	}
}

// suppressUnrelatedServerProtocolListener hides HTTP-only server flags after
// a protocol child is attached to the production server command. The detached
// protocol constructors intentionally omit the runnable server handler, but
// Cobra still inherits the actual parent's local --listen and --pprof flags
// when the child is composed into the final tree.
func suppressUnrelatedServerProtocolListener(command *cobra.Command, label string) error {
	if command == nil {
		return fmt.Errorf("suppress %s listener: command is required", label)
	}
	var listen string
	var pprof bool
	if command.LocalNonPersistentFlags().Lookup("listen") == nil {
		command.Flags().StringVar(&listen, "listen", "", "")
	}
	if command.LocalNonPersistentFlags().Lookup("pprof") == nil {
		command.Flags().BoolVar(&pprof, "pprof", false, "")
	}
	for _, name := range []string{"listen", "pprof"} {
		if err := command.Flags().MarkHidden(name); err != nil {
			return fmt.Errorf("suppress %s listener: %w", label, err)
		}
	}
	previous := command.PreRunE
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		for _, name := range []string{"listen", "pprof"} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("server %s: --%s is not supported; protocol stdio does not start an HTTP listener", label, name)
			}
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return nil
}
