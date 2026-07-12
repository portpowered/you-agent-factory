package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func rejectUnknownSubcommandArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cmd.Help()
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}

func configureGroupCommandUnknownSubcommandGuard(cmd *cobra.Command) {
	cmd.DisableFlagParsing = true
	cmd.Args = rejectUnknownSubcommandArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rejectUnknownSubcommandArgs(cmd, args)
	}
}
