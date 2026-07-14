package climanifestparity

import (
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/spf13/cobra"
)

const (
	legacyConstructorLabel    = "legacy"
	generatedConstructorLabel = "generated"
)

// CompareConstructorIdentityParity compares representative-family command identity
// metadata between legacy and generated constructor trees.
func CompareConstructorIdentityParity(commandID string, legacy, generated *cobra.Command) []Mismatch {
	var mismatches []Mismatch
	appendField := func(field, want, got string) {
		if want == got {
			return
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     field,
			Want:      want,
			Got:       got,
		})
	}

	appendField("path", legacy.CommandPath(), generated.CommandPath())
	appendField("name", legacy.Name(), generated.Name())
	appendField("aliases", formatStringList(legacy.Aliases), formatStringList(generated.Aliases))
	appendField("visibility", commandVisibility(legacy), commandVisibility(generated))
	appendField("runnable", fmt.Sprintf("%t", legacy.Runnable()), fmt.Sprintf("%t", generated.Runnable()))
	appendField("shortDescription", legacy.Short, generated.Short)
	appendField("longDescription", legacy.Long, generated.Long)
	appendField("usage.line", legacy.Use, generated.Use)
	appendField("usage.example", baseline.NormalizeFixtureText(legacy.Example), baseline.NormalizeFixtureText(generated.Example))
	appendField("validArgs", formatStringList(legacy.ValidArgs), formatStringList(generated.ValidArgs))

	return mismatches
}

// CompareConstructorHelpParity compares normalized help output for one command path.
func CompareConstructorHelpParity(commandID string, legacyRoot, generatedRoot *cobra.Command, path string) ([]Mismatch, error) {
	legacyHelp, err := baseline.CaptureHelpOutput(legacyRoot, HelpArgsForPath(path))
	if err != nil {
		return nil, fmt.Errorf("capture legacy help for %q: %w", path, err)
	}
	generatedHelp, err := baseline.CaptureHelpOutput(generatedRoot, HelpArgsForPath(path))
	if err != nil {
		return nil, fmt.Errorf("capture generated help for %q: %w", path, err)
	}
	if legacyHelp == generatedHelp {
		return nil, nil
	}
	return []Mismatch{{
		CommandID: commandID,
		Field:     "normalizedHelpUsageText",
		Want:      legacyHelp,
		Got:       generatedHelp,
	}}, nil
}

// ParseArgvOnRoot parses argv on one root through flag and positional validators
// without invoking RunE. Callers should pass a fresh root per parse because Cobra
// flag state is mutated in place.
func ParseArgvOnRoot(root *cobra.Command, argv []string) (*cobra.Command, []string, error) {
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
		remainder := flagArgs
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

// CompareConstructorParseParity compares parsing outcomes between legacy and generated trees.
func CompareConstructorParseParity(
	commandID string,
	legacyRoot, generatedRoot *cobra.Command,
	argv []string,
	wantParseErr bool,
	errContains string,
) []Mismatch {
	legacyLeaf, legacyPositionals, legacyErr := ParseArgvOnRoot(legacyRoot, argv)
	generatedLeaf, generatedPositionals, generatedErr := ParseArgvOnRoot(generatedRoot, argv)

	var mismatches []Mismatch
	mismatches = append(mismatches, compareParseErrorParity(commandID, legacyErr, generatedErr, wantParseErr, errContains)...)
	if wantParseErr {
		return mismatches
	}
	if legacyErr != nil || generatedErr != nil {
		return mismatches
	}

	if mismatch := AssertLeafCommandPath(commandID, legacyLeaf.CommandPath(), legacyLeaf); mismatch != nil {
		mismatch.Field = legacyConstructorLabel + "." + mismatch.Field
		mismatches = append(mismatches, *mismatch)
	}
	if mismatch := AssertLeafCommandPath(commandID, generatedLeaf.CommandPath(), generatedLeaf); mismatch != nil {
		mismatch.Field = generatedConstructorLabel + "." + mismatch.Field
		mismatches = append(mismatches, *mismatch)
	}
	if legacyLeaf.CommandPath() != generatedLeaf.CommandPath() {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "leafCommand",
			Want:      legacyLeaf.CommandPath(),
			Got:       generatedLeaf.CommandPath(),
		})
	}
	if fmt.Sprintf("%v", legacyPositionals) != fmt.Sprintf("%v", generatedPositionals) {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "positionals",
			Want:      fmt.Sprintf("%v", legacyPositionals),
			Got:       fmt.Sprintf("%v", generatedPositionals),
		})
	}
	return mismatches
}

// CompareConstructorFlagParity compares one parsed flag between legacy and generated leaves.
func CompareConstructorFlagParity(commandID, flagLong string, legacyLeaf, generatedLeaf *cobra.Command) []Mismatch {
	legacyFlag := LiveFlag(legacyLeaf, flagLong)
	generatedFlag := LiveFlag(generatedLeaf, flagLong)
	var mismatches []Mismatch

	if legacyFlag == nil && generatedFlag == nil {
		return nil
	}
	if legacyFlag == nil || generatedFlag == nil {
		present := legacyConstructorLabel
		missing := generatedConstructorLabel
		if generatedFlag == nil {
			present = generatedConstructorLabel
			missing = legacyConstructorLabel
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.present", flagLong),
			Want:      fmt.Sprintf("%s present", present),
			Got:       fmt.Sprintf("%s missing", missing),
		})
		return mismatches
	}

	appendField := func(field, want, got string) {
		if want == got {
			return
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.%s", flagLong, field),
			Want:      want,
			Got:       got,
		})
	}

	appendField("changed", fmt.Sprintf("%t", legacyFlag.Changed), fmt.Sprintf("%t", generatedFlag.Changed))
	appendField("value", liveFlagValue(legacyFlag), liveFlagValue(generatedFlag))
	appendField("default", legacyFlag.DefValue, generatedFlag.DefValue)
	appendField("noOptionDefault", legacyFlag.NoOptDefVal, generatedFlag.NoOptDefVal)

	wantHidden := legacyFlag.Hidden
	gotHidden := generatedFlag.Hidden
	appendField("hidden", fmt.Sprintf("%t", wantHidden), fmt.Sprintf("%t", gotHidden))

	return mismatches
}

// CompareConstructorPreRunParity compares PreRunE rejection between legacy and generated leaves.
func CompareConstructorPreRunParity(commandID string, legacyLeaf, generatedLeaf *cobra.Command, errContains string) []Mismatch {
	var mismatches []Mismatch
	legacyErr := runPreRunE(legacyLeaf)
	generatedErr := runPreRunE(generatedLeaf)

	if (legacyErr == nil) != (generatedErr == nil) {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "preRunE.rejected",
			Want:      fmt.Sprintf("legacy=%v generated=%v", legacyErr != nil, generatedErr != nil),
			Got:       fmt.Sprintf("legacy=%v generated=%v", legacyErr == nil, generatedErr == nil),
		})
	}
	if legacyErr != nil && generatedErr != nil && errContains != "" {
		if !strings.Contains(legacyErr.Error(), errContains) {
			mismatches = append(mismatches, Mismatch{
				CommandID: commandID,
				Field:     "preRunE.legacyError",
				Want:      errContains,
				Got:       legacyErr.Error(),
			})
		}
		if !strings.Contains(generatedErr.Error(), errContains) {
			mismatches = append(mismatches, Mismatch{
				CommandID: commandID,
				Field:     "preRunE.generatedError",
				Want:      errContains,
				Got:       generatedErr.Error(),
			})
		}
	}
	return mismatches
}

// CompareConstructorCompletionInventoryParity compares completion wiring inventories.
func CompareConstructorCompletionInventoryParity(commandID, path string, legacyRoot, generatedRoot *cobra.Command) ([]Mismatch, error) {
	legacyInventory, err := cliinputs.Walk(legacyRoot)
	if err != nil {
		return nil, fmt.Errorf("walk legacy tree: %w", err)
	}
	generatedInventory, err := cliinputs.Walk(generatedRoot)
	if err != nil {
		return nil, fmt.Errorf("walk generated tree: %w", err)
	}

	legacyArgs, legacyFlags := InputsForCommandPath(legacyInventory, path)
	generatedArgs, generatedFlags := InputsForCommandPath(generatedInventory, path)

	var mismatches []Mismatch
	mismatches = append(mismatches, compareCompletionRecordsParity(commandID, "arg", legacyArgs, generatedArgs, func(record cliinputs.ArgumentRecord) string {
		return fmt.Sprintf("%d:%s", record.Position, record.CompletionKind)
	})...)
	mismatches = append(mismatches, compareCompletionRecordsParity(commandID, "flag", legacyFlags, generatedFlags, func(record cliinputs.FlagRecord) string {
		return fmt.Sprintf("%s:%s", record.Long, record.CompletionKind)
	})...)
	return mismatches, nil
}

func compareParseErrorParity(commandID string, legacyErr, generatedErr error, wantParseErr bool, errContains string) []Mismatch {
	var mismatches []Mismatch

	legacyFailed := legacyErr != nil
	generatedFailed := generatedErr != nil
	if legacyFailed != wantParseErr {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "parse.legacyError",
			Want:      fmt.Sprintf("parseErr=%t", wantParseErr),
			Got:       fmt.Sprintf("%v", legacyErr),
		})
	}
	if generatedFailed != wantParseErr {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "parse.generatedError",
			Want:      fmt.Sprintf("parseErr=%t", wantParseErr),
			Got:       fmt.Sprintf("%v", generatedErr),
		})
	}
	if wantParseErr && legacyErr != nil && generatedErr != nil && errContains != "" {
		if !strings.Contains(legacyErr.Error(), errContains) {
			mismatches = append(mismatches, Mismatch{
				CommandID: commandID,
				Field:     "parse.legacyError",
				Want:      errContains,
				Got:       legacyErr.Error(),
			})
		}
		if !strings.Contains(generatedErr.Error(), errContains) {
			mismatches = append(mismatches, Mismatch{
				CommandID: commandID,
				Field:     "parse.generatedError",
				Want:      errContains,
				Got:       generatedErr.Error(),
			})
		}
	}
	return mismatches
}

func compareCompletionRecordsParity[T cliinputs.ArgumentRecord | cliinputs.FlagRecord](
	commandID, kind string,
	legacy, generated []T,
	format func(T) string,
) []Mismatch {
	if len(legacy) != len(generated) {
		return []Mismatch{{
			CommandID: commandID,
			Field:     fmt.Sprintf("completion.%s.count", kind),
			Want:      fmt.Sprintf("%d", len(legacy)),
			Got:       fmt.Sprintf("%d", len(generated)),
		}}
	}

	legacySummary := make([]string, 0, len(legacy))
	for _, record := range legacy {
		legacySummary = append(legacySummary, format(record))
	}
	generatedSummary := make([]string, 0, len(generated))
	for _, record := range generated {
		generatedSummary = append(generatedSummary, format(record))
	}
	if fmt.Sprintf("%v", legacySummary) != fmt.Sprintf("%v", generatedSummary) {
		return []Mismatch{{
			CommandID: commandID,
			Field:     fmt.Sprintf("completion.%s.inventory", kind),
			Want:      fmt.Sprintf("%v", legacySummary),
			Got:       fmt.Sprintf("%v", generatedSummary),
		}}
	}
	return nil
}

func runPreRunE(leaf *cobra.Command) error {
	if leaf == nil || leaf.PreRunE == nil {
		return nil
	}
	return leaf.PreRunE(leaf, leaf.Flags().Args())
}
