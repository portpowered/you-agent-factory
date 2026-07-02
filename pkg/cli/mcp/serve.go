package mcpcli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
	"github.com/spf13/cobra"
)

// ServeConfig holds CLI inputs for the stdio MCP server.
type ServeConfig struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	Service            factorysessionexecution.Service
	Stdin              *os.File
	Stdout             *os.File
}

// RunServe starts the Factory Session MCP stdio server until stdin closes or the
// process receives SIGINT/SIGTERM.
func RunServe(ctx context.Context, cfg ServeConfig) error {
	service, err := resolveServeService(cfg)
	if err != nil {
		return err
	}
	client := mcpfactorysession.NewClientWithService(service)
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

func resolveServeService(cfg ServeConfig) (factorysessionexecution.Service, error) {
	if cfg.Service != nil {
		return cfg.Service, nil
	}
	if cfg.RuntimeBacked {
		projectRoot, err := resolveProjectRoot(cfg.ProjectRoot)
		if err != nil {
			return nil, err
		}
		service, err := factorysessionexecution.NewExecutionService(
			factorysessionexecution.ExecutionProviderJavaScriptRuntime,
			factorysessionexecution.ServiceConfig{ProjectRoot: projectRoot},
		)
		if err != nil {
			return nil, fmt.Errorf("initialize runtime-backed execution service: %w", err)
		}
		return service, nil
	}
	catalogPath, err := resolveFixtureCatalogPath(cfg.FixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return service, nil
}

func resolveProjectRoot(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	return cwd, nil
}

func resolveFixtureCatalogPath(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	relative := filepath.FromSlash(fixtures.ContractFixtureCatalogRelativePath)
	dir := cwd
	for {
		candidate := filepath.Join(dir, relative)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf(
		"fixture catalog not found; run from the repository root or set --fixture-catalog to %s",
		fixtures.ContractFixtureCatalogRelativePath,
	)
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
