package cli

import (
	"fmt"
	"io"
	"strings"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func parseRunCommandArgs(cmd *cobra.Command, args []string) ([]string, error) {
	remainder := make([]string, 0, len(args))
	flagsByToken := indexRunCommandFlags(cmd)
	flagArgs, positional, _ := runcli.SplitFlagTerminator(args)

	for index := 0; index < len(flagArgs); index++ {
		token := flagArgs[index]
		if token == "-" || !strings.HasPrefix(token, "-") {
			remainder = append(remainder, token)
			continue
		}

		lookupToken := token
		inlineValue := ""
		hasInlineValue := false
		if strings.HasPrefix(token, "--") {
			if name, value, ok := strings.Cut(token, "="); ok {
				lookupToken = name
				inlineValue = value
				hasInlineValue = true
			}
		}

		flag, ok := flagsByToken[lookupToken]
		if !ok {
			remainder = append(remainder, token)
			continue
		}
		if flag == nil || flag.Value == nil {
			return nil, fmt.Errorf("flag %s is unavailable", lookupToken)
		}

		value, consumedNext, err := resolveRunFlagValue(flag, flagArgs, index, hasInlineValue, inlineValue)
		if err != nil {
			return nil, err
		}
		if err := flag.Value.Set(value); err != nil {
			return nil, err
		}
		flag.Changed = true
		if consumedNext {
			index++
		}
	}

	remainder = append(remainder, positional...)
	return remainder, nil
}

func indexRunCommandFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	indexed := map[string]*pflag.Flag{}
	addFlags := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			indexed["--"+flag.Name] = flag
			if flag.Shorthand != "" {
				indexed["-"+flag.Shorthand] = flag
			}
		})
	}
	addFlags(cmd.InheritedFlags())
	addFlags(cmd.Flags())
	return indexed
}

func resolveRunFlagValue(flag *pflag.Flag, args []string, index int, hasInlineValue bool, inlineValue string) (string, bool, error) {
	if hasInlineValue {
		return inlineValue, false, nil
	}
	if flag.Value.Type() == "bool" {
		if flag.NoOptDefVal != "" {
			return flag.NoOptDefVal, false, nil
		}
		return "true", false, nil
	}
	if flag.NoOptDefVal != "" {
		return flag.NoOptDefVal, false, nil
	}
	if index+1 >= len(args) {
		return "", false, fmt.Errorf("flag needs an argument: %s", "--"+flag.Name)
	}
	return args[index+1], true, nil
}

// ParseArgvForCLIInputsInventory parses argv on the caller-supplied canonical
// process command through the target command's flag and positional validators
// without invoking RunE.
// Run uses its custom tokenizer because the command sets DisableFlagParsing.
func ParseArgvForCLIInputsInventory(root *cobra.Command, argv []string) (*cobra.Command, []string, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("parse CLI inputs inventory: root command is required")
	}
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
