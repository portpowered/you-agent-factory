package climanifestparity

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
)

// CompareArgumentCompletion asserts one contracted positional completion kind matches live wiring.
func CompareArgumentCompletion(commandID string, contract climanifest.Argument, live cliinputs.ArgumentRecord) *Mismatch {
	if contract.Completion != live.CompletionKind {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("arg.%d.completion", contract.Position),
			Want:      contract.Completion,
			Got:       live.CompletionKind,
		}
	}
	return nil
}

// CompareFlagCompletion asserts one contracted flag completion kind matches live wiring.
func CompareFlagCompletion(commandID string, contract climanifest.Flag, live cliinputs.FlagRecord) *Mismatch {
	if contract.Completion != live.CompletionKind {
		return &Mismatch{
			CommandID: commandID,
			Field:     fmt.Sprintf("flag.%s.completion", contract.Long),
			Want:      contract.Completion,
			Got:       live.CompletionKind,
		}
	}
	return nil
}

// CompareCompletionParity compares contracted argument and flag completion kinds against live inputs inventory.
func CompareCompletionParity(record climanifest.Command, liveArgs []cliinputs.ArgumentRecord, liveFlags []cliinputs.FlagRecord) []Mismatch {
	var mismatches []Mismatch

	liveArgsByPosition := map[int]cliinputs.ArgumentRecord{}
	for _, live := range liveArgs {
		liveArgsByPosition[live.Position] = live
	}
	for _, contract := range record.Arguments {
		live, ok := liveArgsByPosition[contract.Position]
		if !ok {
			mismatches = append(mismatches, Mismatch{
				CommandID: record.ID,
				Field:     fmt.Sprintf("arg.%d.present", contract.Position),
				Want:      "present in live inputs inventory",
				Got:       "missing",
			})
			continue
		}
		if mismatch := CompareArgumentCompletion(record.ID, contract, live); mismatch != nil {
			mismatches = append(mismatches, *mismatch)
		}
	}

	liveFlagsByLong := map[string]cliinputs.FlagRecord{}
	for _, live := range liveFlags {
		liveFlagsByLong[live.Long] = live
	}
	for _, contract := range record.Flags {
		live, ok := liveFlagsByLong[contract.Long]
		if !ok {
			mismatches = append(mismatches, Mismatch{
				CommandID: record.ID,
				Field:     fmt.Sprintf("flag.%s.present", contract.Long),
				Want:      "present in live inputs inventory",
				Got:       "missing",
			})
			continue
		}
		if mismatch := CompareFlagCompletion(record.ID, contract, live); mismatch != nil {
			mismatches = append(mismatches, *mismatch)
		}
	}

	return mismatches
}

// CompareDeclaredOutputs asserts contracted stdout output modes include human text and JSON.
func CompareDeclaredOutputs(record climanifest.Command) []Mismatch {
	var mismatches []Mismatch
	formats := map[string]climanifest.Output{}
	for _, output := range record.Outputs {
		if output.Channel == "stdout" {
			formats[output.Format] = output
		}
	}
	if _, ok := formats["text"]; !ok {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "outputs.stdout.text",
			Want:      "declared human text output on stdout",
			Got:       formatOutputSummary(record.Outputs),
		})
	}
	if _, ok := formats["json"]; !ok {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "outputs.stdout.json",
			Want:      "declared JSON output on stdout",
			Got:       formatOutputSummary(record.Outputs),
		})
	}
	return mismatches
}

// CompareDeclaredExits asserts contracted success, failure, and usage exits are present with expected codes.
func CompareDeclaredExits(record climanifest.Command) []Mismatch {
	want := map[string]int{
		"success": 0,
		"failure": 1,
		"usage":   2,
	}
	got := map[string]int{}
	for _, exit := range record.Exits {
		got[exit.Kind] = exit.Code
	}

	var mismatches []Mismatch
	for kind, wantCode := range want {
		gotCode, ok := got[kind]
		if !ok {
			mismatches = append(mismatches, Mismatch{
				CommandID: record.ID,
				Field:     fmt.Sprintf("exits.%s", kind),
				Want:      fmt.Sprintf("declared exit kind %q", kind),
				Got:       formatExitSummary(record.Exits),
			})
			continue
		}
		if gotCode != wantCode {
			mismatches = append(mismatches, Mismatch{
				CommandID: record.ID,
				Field:     fmt.Sprintf("exits.%s.code", kind),
				Want:      fmt.Sprintf("%d", wantCode),
				Got:       fmt.Sprintf("%d", gotCode),
			})
		}
	}
	return mismatches
}

// CompareDeclaredSideEffects asserts contracted side-effect kinds include every required kind.
func CompareDeclaredSideEffects(record climanifest.Command, requiredKinds []string) []Mismatch {
	present := map[string]bool{}
	for _, effect := range record.SideEffects {
		present[effect.Kind] = true
	}

	var mismatches []Mismatch
	for _, kind := range requiredKinds {
		if present[kind] {
			continue
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     fmt.Sprintf("sideEffects.%s", kind),
			Want:      fmt.Sprintf("declared side effect kind %q", kind),
			Got:       formatSideEffectSummary(record.SideEffects),
		})
	}
	return mismatches
}

// CompareDeclaredConstraints asserts runtime and platform declarations match approved baseline evidence.
func CompareDeclaredConstraints(record climanifest.Command) []Mismatch {
	var mismatches []Mismatch

	if !stringListContains(record.Constraints.Runtime, "local") {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "constraints.runtime",
			Want:      `["local"]`,
			Got:       formatStringList(record.Constraints.Runtime),
		})
	}

	wantPlatforms := []string{"darwin", "linux", "windows"}
	missing := make([]string, 0)
	for _, platform := range wantPlatforms {
		if !stringListContains(record.Constraints.Platforms, platform) {
			missing = append(missing, platform)
		}
	}
	if len(missing) > 0 {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "constraints.platforms",
			Want:      formatStringList(wantPlatforms),
			Got:       formatStringList(record.Constraints.Platforms),
		})
	}

	if !stringListContains(record.Constraints.Platforms, runtime.GOOS) {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "constraints.platforms.current",
			Want:      fmt.Sprintf("include current platform %q", runtime.GOOS),
			Got:       formatStringList(record.Constraints.Platforms),
		})
	}

	return mismatches
}

// CompareDeclaredChannels asserts stdout and stderr output channels are declared.
func CompareDeclaredChannels(record climanifest.Command) []Mismatch {
	var mismatches []Mismatch
	if !stringListContains(record.Channels.Output, "stdout") {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "channels.output.stdout",
			Want:      "stdout output channel",
			Got:       formatStringList(record.Channels.Output),
		})
	}
	if !stringListContains(record.Channels.Output, "stderr") {
		mismatches = append(mismatches, Mismatch{
			CommandID: record.ID,
			Field:     "channels.output.stderr",
			Want:      "stderr output channel",
			Got:       formatStringList(record.Channels.Output),
		})
	}
	return mismatches
}

func formatOutputSummary(outputs map[string]climanifest.Output) string {
	if len(outputs) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(outputs))
	for _, output := range outputs {
		parts = append(parts, fmt.Sprintf("%s:%s", output.Channel, output.Format))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%q", parts)
}

func formatExitSummary(exits map[string]climanifest.Exit) string {
	if len(exits) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(exits))
	for _, exit := range exits {
		parts = append(parts, fmt.Sprintf("%s=%d", exit.Kind, exit.Code))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%q", parts)
}

func formatSideEffectSummary(effects map[string]climanifest.SideEffect) string {
	if len(effects) == 0 {
		return "[]"
	}
	kinds := make([]string, 0, len(effects))
	for _, effect := range effects {
		kinds = append(kinds, effect.Kind)
	}
	sort.Strings(kinds)
	return fmt.Sprintf("%q", kinds)
}

func stringListContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// InputsForCommandPath returns live argument and flag records for one command path.
func InputsForCommandPath(inventory cliinputs.Inventory, commandPath string) ([]cliinputs.ArgumentRecord, []cliinputs.FlagRecord) {
	args := make([]cliinputs.ArgumentRecord, 0)
	flags := make([]cliinputs.FlagRecord, 0)
	for _, record := range inventory.Arguments {
		if record.CommandPath == commandPath {
			args = append(args, record)
		}
	}
	for _, record := range inventory.Flags {
		if record.CommandPath == commandPath {
			flags = append(flags, record)
		}
	}
	sort.Slice(args, func(i, j int) bool { return args[i].Position < args[j].Position })
	sort.Slice(flags, func(i, j int) bool { return flags[i].Long < flags[j].Long })
	return args, flags
}

// NormalizeJSONOutput trims trailing whitespace for JSON parity checks.
func NormalizeJSONOutput(output string) string {
	return strings.TrimSpace(output)
}
