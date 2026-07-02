package mcpcli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
	"github.com/spf13/cobra"
)

// ServeConfig holds CLI inputs for the stdio MCP server.
type ServeConfig struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	FactoryDir         string
	Service            factorysessionexecution.Service
	Stdin              *os.File
	Stdout             *os.File
}

// RunServe starts the Factory Session MCP stdio server until stdin closes or the
// process receives SIGINT/SIGTERM.
func RunServe(ctx context.Context, cfg ServeConfig) error {
	transport, err := composeMCPTransport(ctx, cfg)
	if err != nil {
		return err
	}
	client := transport.SessionClient()
	server, err := mcpserver.New(mcpserver.Options{Client: client})
	if err != nil {
		return err
	}

	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.ServeStdio(ctx, stdin, stdout)
}

func composeMCPTransport(ctx context.Context, cfg ServeConfig) (*initializer.MCPTransport, error) {
	if cfg.Service != nil {
		return &initializer.MCPTransport{SessionExecution: cfg.Service}, nil
	}
	mcpCfg := &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			FixtureCatalogPath: cfg.FixtureCatalogPath,
			RuntimeBacked:      cfg.RuntimeBacked,
			ProjectRoot:        cfg.ProjectRoot,
		},
	}
	if trimmed := strings.TrimSpace(cfg.FactoryDir); trimmed != "" {
		mcpCfg.Factory = &initializer.Config{Dir: trimmed}
	}
	return initializer.InitializeMCPTransport(ctx, mcpCfg)
}

// resolveServeService remains for focused unit tests that assert provider selection.
func resolveServeService(cfg ServeConfig) (factorysessionexecution.Service, error) {
	transport, err := composeMCPTransport(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return transport.SessionExecution, nil
}

// NewServeCommand constructs `you mcp serve`.
func NewServeCommand() *cobra.Command {
	var fixtureCatalogPath string
	var runtimeBacked bool
	var projectRoot string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Factory Session MCP stdio server",
		Long: "Start the dynamic workflow Factory Session MCP server over stdio.\n\n" +
			"Hosts launch this command as a child process and communicate through newline-delimited " +
			"JSON-RPC on stdin and stdout. The default backing service is the deterministic durable " +
			"session fixture catalog used by workflow CLI commands.\n\n" +
			"Use --runtime to select the shared durable JavaScript runtime execution service instead " +
			"of the fixture catalog. Runtime mode requires workflow sources to resolve from the MCP " +
			"host working directory or an explicit --project-root.\n\n" +
			"Set the MCP host working directory to the project root where workflow sources and the " +
			"fixture catalog resolve. See `you docs mcp-hosts` for host-specific configuration examples.",
		Example: "  # Typical MCP host child-process launch.\n" +
			"  you mcp serve\n\n" +
			"  # Explicit fixture catalog for offline smoke outside the repository root.\n" +
			"  you mcp serve --fixture-catalog ./pkg/factorysessionexecution/fixtures/durable-session-contract-fixtures.json\n\n" +
			"  # Runtime-backed serve against live durable JavaScript execution.\n" +
			"  you mcp serve --runtime",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtimeBacked && strings.TrimSpace(fixtureCatalogPath) != "" {
				return fmt.Errorf("cannot combine --runtime with --fixture-catalog")
			}
			return RunServe(cmd.Context(), ServeConfig{
				FixtureCatalogPath: fixtureCatalogPath,
				RuntimeBacked:      runtimeBacked,
				ProjectRoot:        projectRoot,
			})
		},
	}
	cmd.Flags().StringVar(
		&fixtureCatalogPath,
		"fixture-catalog",
		"",
		"optional path to durable-session contract fixtures; defaults to the catalog discovered from the current working directory",
	)
	cmd.Flags().BoolVar(
		&runtimeBacked,
		"runtime",
		false,
		"select the shared durable JavaScript runtime execution service instead of the fixture catalog",
	)
	cmd.Flags().StringVar(
		&projectRoot,
		"project-root",
		"",
		"project root for workflow source resolution in --runtime mode; defaults to the current working directory",
	)
	return cmd
}

// NewCommand constructs the `you mcp` command group.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol servers for Factory Session tools",
	}
	cmd.AddCommand(NewServeCommand())
	return cmd
}
