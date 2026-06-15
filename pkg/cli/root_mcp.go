package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand(_ *cliDiagnosticsOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve repo-owned MCP tools over stdio",
		Long: "Serve repo-owned MCP tools for dynamic workflow preview validation.\n\n" +
			"Subcommands:\n" +
			"  serve   start the canonical stdio MCP server for Factory preview tools",
	}
	cmd.AddCommand(newMCPServeCommand())
	return cmd
}

func newMCPServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve workflow preview MCP tools over stdio",
		Long: "Start the canonical repo-owned MCP server that exposes Factory preview " +
			"validate and start-preview tools for JavaScript orchestrator sources.\n\n" +
			"The server communicates over stdio using newline-delimited JSON and is " +
			"intended for MCP host configuration rather than direct interactive use.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return mcpcli.Serve(ctx, mcpcli.ServeConfig{
				Input:  cmd.InOrStdin(),
				Output: cmd.OutOrStdout(),
			})
		},
	}
	return cmd
}
