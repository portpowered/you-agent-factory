package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func parseRunCommandArgs(cmd *cobra.Command, args []string) ([]string, error) {
	remainder := make([]string, 0, len(args))
	flagsByToken := indexRunCommandFlags(cmd)

	for index := 0; index < len(args); index++ {
		token := args[index]
		if token == "--" {
			remainder = append(remainder, args[index+1:]...)
			break
		}
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

		value, consumedNext, err := resolveRunFlagValue(flag, args, index, hasInlineValue, inlineValue)
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
