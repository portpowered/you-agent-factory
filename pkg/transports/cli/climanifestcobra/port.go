package climanifestcobra

import (
	"fmt"

	"github.com/spf13/cobra"
)

const deprecatedPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"

func rejectDeprecatedPortFlag(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Lookup("port") != nil && cmd.Flags().Changed("port") {
		return fmt.Errorf("%s", deprecatedPortFlagMessage)
	}
	return nil
}

func registerDeprecatedPortFlag(cmd *cobra.Command, target *int) {
	cmd.Flags().IntVar(target, "port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
}
