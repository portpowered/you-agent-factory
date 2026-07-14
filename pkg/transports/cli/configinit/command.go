package configinitcmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// CommandGlobals carries top-level CLI flags used by you config init.
type CommandGlobals struct {
	JSON    func() bool
	HomeDir func() (string, error)
}

// CommandDiagnostics carries diagnostic output hooks for you config init.
type CommandDiagnostics struct {
	Writer  func(cmd *cobra.Command) io.Writer
	Verbose func() bool
}

// RunInit is the init implementation invoked by the Cobra command. Tests may
// replace it to observe mapped InitConfig without running the full initializer.
var RunInit = Init

// NewSystemConfigCommand wires the top-level you config command tree.
func NewSystemConfigCommand(binaryName string, globals CommandGlobals, diagnostics CommandDiagnostics) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Initialize operator and system configuration",
		Long: "Initialize operator and system configuration under the shared home directory.\n\n" +
			"Subcommands:\n" +
			"  init  create operator/system config on a fresh home without overwriting existing files\n\n" +
			"Use `you factory config` to inspect or transform factory.json configuration.",
		Example: "  # Bootstrap operator/system config on a fresh home.\n" +
			"  " + binaryName + " config init",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	configCmd.AddCommand(newSystemConfigInitCommand(binaryName, globals, diagnostics))
	return configCmd
}

func newSystemConfigInitCommand(binaryName string, globals CommandGlobals, diagnostics CommandDiagnostics) *cobra.Command {
	cfg := InitConfig{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create operator/system config on a fresh home",
		Long: "Create operator/system config at ~/.you-agent-factory/config.json on a fresh home.\n\n" +
			"Re-running against an existing config file succeeds without rewriting user-edited contents.",
		Example: "  # Bootstrap operator/system config on a fresh home.\n" +
			"  " + binaryName + " config init",
		RunE: func(cmd *cobra.Command, args []string) error {
			if globals.HomeDir != nil {
				homeDir, err := globals.HomeDir()
				if err != nil {
					return fmt.Errorf("resolve config init home directory: %w", err)
				}
				cfg.HomeDir = homeDir
			}
			cfg.JSON = globals.JSON()
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.Writer(cmd)
			cfg.Verbose = diagnostics.Verbose()
			return RunInit(cfg)
		},
	}
	return cmd
}
