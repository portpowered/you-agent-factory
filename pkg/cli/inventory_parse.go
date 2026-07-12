package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// ParseArgvForCLIInputsInventory parses argv on a fresh production root through
// the target command's flag and positional validators without invoking RunE.
// Run uses its custom tokenizer because the command sets DisableFlagParsing.
func ParseArgvForCLIInputsInventory(argv []string) (*cobra.Command, []string, error) {
	root := NewRootCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	cmd, flagArgs, err := root.Find(argv)
	if err != nil {
		return cmd, nil, err
	}

	cmd.InitDefaultHelpFlag()

	if cmd.DisableFlagParsing {
		remainder, err := parseRunCommandArgs(cmd, flagArgs)
		if err != nil {
			return cmd, nil, err
		}
		if err := cmd.ValidateArgs(remainder); err != nil {
			return cmd, remainder, err
		}
		if err := cmd.ValidateRequiredFlags(); err != nil {
			return cmd, remainder, err
		}
		if err := cmd.ValidateFlagGroups(); err != nil {
			return cmd, remainder, err
		}
		return cmd, remainder, nil
	}

	if err := cmd.ParseFlags(flagArgs); err != nil {
		return cmd, nil, err
	}

	positionals := cmd.Flags().Args()
	if err := cmd.ValidateArgs(positionals); err != nil {
		return cmd, positionals, err
	}
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return cmd, positionals, err
	}
	if err := cmd.ValidateFlagGroups(); err != nil {
		return cmd, positionals, err
	}
	return cmd, positionals, nil
}
