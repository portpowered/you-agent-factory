package cli

import (
	"fmt"

	configinitcmd "github.com/portpowered/infinite-you/pkg/cli/configinit"
	"github.com/spf13/cobra"
)

var configInit = configinitcmd.Init

func newSystemConfigCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Initialize operator and system configuration",
		Long: "Initialize operator and system configuration under the shared home directory.\n\n" +
			"Subcommands:\n" +
			"  init  create operator/system config on a fresh home without overwriting existing files\n\n" +
			"Use `you factory config` to inspect or transform factory.json configuration.",
		Example: "  # Bootstrap operator/system config on a fresh home.\n" +
			"  " + cliBinaryName + " config init",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	configCmd.AddCommand(newSystemConfigInitCommand(globals, diagnostics))
	return configCmd
}

func newSystemConfigInitCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := configinitcmd.InitConfig{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create operator/system config on a fresh home",
		Long: "Create operator/system config at ~/.you-agent-factory/config.json on a fresh home.\n\n" +
			"Re-running against an existing config file succeeds without rewriting user-edited contents.",
		Example: "  # Bootstrap operator/system config on a fresh home.\n" +
			"  " + cliBinaryName + " config init",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			return configInit(cfg)
		},
	}
	return cmd
}
