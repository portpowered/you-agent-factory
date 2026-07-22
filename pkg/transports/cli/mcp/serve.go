package mcpcli

import (
	"fmt"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/spf13/cobra"
)

// ServeBinding supplies the mutable flag state and lifecycle delegate used by
// the handwritten MCP serve handler.
type ServeBinding struct {
	FixtureCatalogPath *string
	RuntimeBacked      *bool
	ProjectRoot        *string
	HomeDir            func() (string, error)
	InitializeStdio    startupcli.StdioHandler
}

// ServeRunE returns the handwritten MCP serve handler used by legacy and
// generated metadata construction.
func ServeRunE(binding ServeBinding) func(*cobra.Command, []string) error {
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
		if binding.InitializeStdio == nil {
			return fmt.Errorf("MCP stdio initializer is required")
		}
		homeDir := ""
		if runtimeBacked {
			if binding.HomeDir == nil {
				return fmt.Errorf("process home directory resolver is required")
			}
			var err error
			homeDir, err = binding.HomeDir()
			if err != nil {
				return fmt.Errorf("resolve process home directory: %w", err)
			}
		}
		return binding.InitializeStdio(cmd.Context(), startupcli.MCPIntent{
			FixtureCatalogPath: fixtureCatalogPath,
			RuntimeBacked:      runtimeBacked,
			ProjectRoot:        projectRoot,
			HomeDir:            homeDir,
			Stdin:              cmd.InOrStdin(),
			Stdout:             cmd.OutOrStdout(),
		})
	}
}

// NewServeCommand constructs `you mcp serve`.
func NewServeCommand() *cobra.Command {
	return newServeCommand(nil, nil)
}

func newServeCommand(initializeStdio startupcli.StdioHandler, homeDir func() (string, error)) *cobra.Command {
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
			HomeDir:            homeDir,
			InitializeStdio:    initializeStdio,
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
	return NewCommandWithStdioInitializer(nil, nil)
}

// NewCommandWithStdioInitializer constructs the MCP command group with stdio
// initialization delegated to the supplied bundle entrypoint.
func NewCommandWithStdioInitializer(
	initializeStdio startupcli.StdioHandler,
	homeDir func() (string, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol servers for Factory Session tools",
	}
	cmd.AddCommand(newServeCommand(initializeStdio, homeDir))
	return cmd
}
