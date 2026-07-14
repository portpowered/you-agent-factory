package climanifestparity

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CompareFlagDefault asserts one live flag's unset default matches the contract.
func CompareFlagDefault(commandID string, contract climanifest.Flag, live *pflag.Flag) *Mismatch {
	if live == nil {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.present", contract.Long),
			Want:      "present",
			Got:       "missing",
		}
	}

	wantVisibility := contract.Visibility
	gotVisibility := "visible"
	if live.Hidden {
		gotVisibility = "hidden"
	}
	if wantVisibility != gotVisibility {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.visibility", contract.Long),
			Want:      wantVisibility,
			Got:       gotVisibility,
		}
	}

	if live.Changed {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.default", contract.Long),
			Want:      fmt.Sprintf("unchanged default %q", contract.Default),
			Got:       fmt.Sprintf("changed value %q", liveFlagValue(live)),
		}
	}

	if live.DefValue != contract.Default {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.default", contract.Long),
			Want:      contract.Default,
			Got:       live.DefValue,
		}
	}

	if live.NoOptDefVal != contract.NoOptionDefault {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.noOptionDefault", contract.Long),
			Want:      contract.NoOptionDefault,
			Got:       live.NoOptDefVal,
		}
	}

	return nil
}

// CompareFlagParsed asserts one live flag's parsed value and changed state.
func CompareFlagParsed(commandID string, contract climanifest.Flag, live *pflag.Flag, wantChanged bool, wantValue string) *Mismatch {
	if live == nil {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.present", contract.Long),
			Want:      "present",
			Got:       "missing",
		}
	}

	if live.Changed != wantChanged {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.changed", contract.Long),
			Want:      fmt.Sprintf("%t", wantChanged),
			Got:       fmt.Sprintf("%t", live.Changed),
		}
	}

	gotValue := liveFlagValue(live)
	if gotValue != wantValue {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.value", contract.Long),
			Want:      wantValue,
			Got:       gotValue,
		}
	}

	return nil
}

// CompareInheritedFlagDefaultsAgainstRoot asserts inherited session-show defaults
// still match the root persistent flag contract.
func CompareInheritedFlagDefaultsAgainstRoot(root climanifest.Command, sessionShow climanifest.Command, leaf *cobra.Command) []Mismatch {
	var mismatches []Mismatch
	for _, contract := range sessionShow.Flags {
		if contract.Scope != "inherited" {
			continue
		}
		rootFlag, ok := root.FlagByLong(contract.Long)
		if !ok {
			mismatches = append(mismatches, Mismatch{
				CommandID: sessionShow.ID,
				Field:     fmt.Sprintf("flag.%s.rootBinding", contract.Long),
				Want:      fmt.Sprintf("root flag %q", contract.Long),
				Got:       "missing on root manifest",
			})
			continue
		}
		if rootFlag.Default != contract.Default {
			mismatches = append(mismatches, Mismatch{
				CommandID: sessionShow.ID,
				Field:     fmt.Sprintf("flag.%s.default", contract.Long),
				Want:      rootFlag.Default,
				Got:       contract.Default,
			})
		}
		if rootFlag.NoOptionDefault != contract.NoOptionDefault {
			mismatches = append(mismatches, Mismatch{
				CommandID: sessionShow.ID,
				Field:     fmt.Sprintf("flag.%s.noOptionDefault", contract.Long),
				Want:      rootFlag.NoOptionDefault,
				Got:       contract.NoOptionDefault,
			})
		}
		if mismatch := CompareFlagDefault(sessionShow.ID, contract, leaf.Flag(contract.Long)); mismatch != nil {
			mismatches = append(mismatches, *mismatch)
		}
	}
	return mismatches
}

// CompareArgumentCardinality asserts contracted positional cardinality matches live parsing.
func CompareArgumentCardinality(commandID string, contract climanifest.Argument, positionals []string) *Mismatch {
	if contract.Required && len(positionals) == 0 {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("arg.%d.required", contract.Position),
			Want:      "at least one positional",
			Got:       "none",
		}
	}
	if len(positionals) > contract.MaxCardinality {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("arg.%d.maxCardinality", contract.Position),
			Want:      fmt.Sprintf("at most %d", contract.MaxCardinality),
			Got:       fmt.Sprintf("%d positionals: %v", len(positionals), positionals),
		}
	}
	if !contract.Variadic && len(positionals) < contract.MinCardinality {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("arg.%d.minCardinality", contract.Position),
			Want:      fmt.Sprintf("at least %d", contract.MinCardinality),
			Got:       fmt.Sprintf("%d positionals: %v", len(positionals), positionals),
		}
	}
	return nil
}

// LiveFlag returns the effective flag on a parsed leaf command.
func LiveFlag(leaf *cobra.Command, longName string) *pflag.Flag {
	if leaf == nil {
		return nil
	}
	return leaf.Flag(longName)
}

// AssertLeafCommandPath fails parity when the parsed leaf path drifts.
func AssertLeafCommandPath(commandID, wantPath string, leaf *cobra.Command) *Mismatch {
	if leaf == nil {
		return &Mismatch{
			CommandID: commandID,
			Field:     "leafCommand",
			Want:      wantPath,
			Got:       "<nil>",
		}
	}
	if leaf.CommandPath() != wantPath {
		return &Mismatch{
			CommandID: commandID,
			Field:     "leafCommand",
			Want:      wantPath,
			Got:       leaf.CommandPath(),
		}
	}
	return nil
}

func liveFlagValue(flag *pflag.Flag) string {
	if flag == nil || flag.Value == nil {
		return ""
	}
	return flag.Value.String()
}
