package mcpcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	"github.com/spf13/cobra"
)

// ServeConfig holds CLI inputs for the stdio MCP server.
type ServeConfig struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	Service            factorysessionexecution.Service
	Stdin              io.Reader
	Stdout             io.Writer
}

// ServeBinding supplies the mutable flag state and lifecycle delegate used by
// the handwritten MCP serve handler.
type ServeBinding struct {
	FixtureCatalogPath *string
	RuntimeBacked      *bool
	ProjectRoot        *string
	Startup            startupcli.Handler
	RunServe           func(context.Context, ServeConfig) error
}

// ServeRunE returns the handwritten MCP serve handler used by legacy and
// generated metadata construction.
func ServeRunE(binding ServeBinding) func(*cobra.Command, []string) error {
	runServe := binding.RunServe
	if runServe == nil {
		runServe = RunServe
	}
	return func(cmd *cobra.Command, _ []string) error {
		fixtureCatalogPath := ""
		if binding.FixtureCatalogPath != nil {
			fixtureCatalogPath = *binding.FixtureCatalogPath
		}
		runtimeBacked := false
		if binding.RuntimeBacked != nil {
			runtimeBacked = *binding.RuntimeBacked
		}
		if runtimeBacked && strings.TrimSpace(fixtureCatalogPath) != "" {
			return fmt.Errorf("cannot combine --runtime with --fixture-catalog")
		}
		projectRoot := ""
		if binding.ProjectRoot != nil {
			projectRoot = *binding.ProjectRoot
		}
		cfg := ServeConfig{
			FixtureCatalogPath: fixtureCatalogPath,
			RuntimeBacked:      runtimeBacked,
			ProjectRoot:        projectRoot,
			Stdin:              cmd.InOrStdin(),
			Stdout:             cmd.OutOrStdout(),
		}
		if binding.Startup == nil {
			return runServe(cmd.Context(), cfg)
		}
		return binding.Startup(cmd.Context(), startupcli.Request{
			Kind: startupcli.KindMCPServe,
			MCP: startupcli.MCPIntent{
				FixtureCatalogPath: cfg.FixtureCatalogPath,
				RuntimeBacked:      cfg.RuntimeBacked,
				ProjectRoot:        cfg.ProjectRoot,
				Stdin:              cfg.Stdin,
				Stdout:             cfg.Stdout,
			},
		})
	}
}

// RunServe starts the Factory Session MCP stdio server until stdin closes or the
// process receives SIGINT/SIGTERM.
func RunServe(ctx context.Context, cfg ServeConfig) error {
	application, err := BuildServeApplication(cfg)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

// ServeApplication is the already-constructed MCP transport graph consumed by
// the initializer lifecycle boundary.
type ServeApplication struct {
	server *mcpserver.Server
	stdin  io.Reader
	stdout io.Writer
}

// BuildServeApplication constructs the MCP service, client, and server without
// starting stdio processing.
func BuildServeApplication(cfg ServeConfig) (*ServeApplication, error) {
	if cfg.Service == nil {
		return nil, fmt.Errorf("build MCP serve application: durable execution service is required")
	}
	client := mcpfactorysession.NewClientWithService(cfg.Service)
	server, err := mcpserver.New(mcpserver.Options{Client: client})
	if err != nil {
		return nil, err
	}

	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	return &ServeApplication{server: server, stdin: stdin, stdout: stdout}, nil
}

// Run starts stdio handling for an MCP graph that has already been built.
func (application *ServeApplication) Run(ctx context.Context) error {
	if application == nil || application.server == nil {
		return fmt.Errorf("run MCP application: graph is required")
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return application.server.ServeStdio(ctx, application.stdin, application.stdout)
}

// NewServeCommand constructs `you mcp serve`.
func NewServeCommand() *cobra.Command {
	return newServeCommand(nil)
}

func newServeCommand(startup startupcli.Handler) *cobra.Command {
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
			"fixture catalog resolve. See `you docs mcp` for host configuration, serve modes, smoke, and troubleshooting.",
		Example: "  # Typical MCP host child-process launch.\n" +
			"  you mcp serve\n\n" +
			"  # Explicit fixture catalog for offline smoke outside the repository root.\n" +
			"  you mcp serve --fixture-catalog ./pkg/transports/http/testdata/durable-session-contract-fixtures.json\n\n" +
			"  # Runtime-backed serve against live durable JavaScript execution.\n" +
			"  you mcp serve --runtime",
		RunE: ServeRunE(ServeBinding{
			FixtureCatalogPath: &fixtureCatalogPath,
			RuntimeBacked:      &runtimeBacked,
			ProjectRoot:        &projectRoot,
			Startup:            startup,
		}),
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
	return NewCommandWithStartup(nil)
}

// NewCommandWithStartup constructs the MCP command group with process startup
// delegated to the supplied root handler.
func NewCommandWithStartup(startup startupcli.Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol servers for Factory Session tools",
	}
	cmd.AddCommand(newServeCommand(startup))
	return cmd
}
