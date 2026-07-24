package mcpcli

import (
	"fmt"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const (
	fixtureCatalogInputID = "you.mcp.serve.flag.fixture-catalog"
	runtimeInputID        = "you.mcp.serve.flag.runtime"
	projectRootInputID    = "you.mcp.serve.flag.project-root"
)

// ServeBinding supplies the injected lifecycle operations used by the MCP
// resolved-input adapter.
type ServeBinding struct {
	HomeDir         func() (string, error)
	InitializeStdio startupcli.StdioHandler
}

// ResolvedServeHandler maps canonical stable-ID inputs into the already
// injected MCP stdio initializer.
func ResolvedServeHandler(
	binding ServeBinding,
) func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return func(cmd *cobra.Command, inputs, _ resolvedinput.Inputs) error {
		fixtureCatalogPath, err := inputs.String(fixtureCatalogInputID)
		if err != nil {
			return fmt.Errorf("read MCP fixture catalog input: %w", err)
		}
		runtimeBacked, err := inputs.Bool(runtimeInputID)
		if err != nil {
			return fmt.Errorf("read MCP runtime input: %w", err)
		}
		projectRoot, err := inputs.String(projectRootInputID)
		if err != nil {
			return fmt.Errorf("read MCP project root input: %w", err)
		}
		if binding.InitializeStdio == nil {
			return fmt.Errorf("MCP stdio initializer is required")
		}
		homeDir := ""
		if runtimeBacked {
			if binding.HomeDir == nil {
				return fmt.Errorf("process home directory resolver is required")
			}
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
