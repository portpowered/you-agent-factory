package mcpcli

import (
	"fmt"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const (
	projectRootInputID = "you.server.mcp.flag.project-root"
)

// MCPIntent is the stdio serve intent delivered to the injected initializer.
type MCPIntent = startupcli.MCPIntent

// StdioHandler initializes MCP stdio serving for a resolved intent.
type StdioHandler = startupcli.StdioHandler

// ServeBinding supplies the injected lifecycle operations used by the MCP
// resolved-input adapter.
type ServeBinding struct {
	InitializeStdio StdioHandler
}

// ResolvedServeHandler maps canonical stable-ID inputs into the already
// injected MCP stdio initializer.
func ResolvedServeHandler(
	binding ServeBinding,
) func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return func(cmd *cobra.Command, inputs, _ resolvedinput.Inputs) error {
		projectRoot, err := inputs.String(projectRootInputID)
		if err != nil {
			return fmt.Errorf("read MCP project root input: %w", err)
		}
		if binding.InitializeStdio == nil {
			return fmt.Errorf("MCP stdio initializer is required")
		}
		return binding.InitializeStdio(cmd.Context(), MCPIntent{
			ProjectRoot: projectRoot,
			Stdin:       cmd.InOrStdin(),
			Stdout:      cmd.OutOrStdout(),
		})
	}
}
