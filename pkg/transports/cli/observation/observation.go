// Package observation defines the detached, read-only CLI observation emitted
// at the outer process edge. It deliberately exposes no Cobra command tree.
package observation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Snapshot is a detached description of one freshly constructed production
// CLI tree. Its collections do not alias the private Cobra representation.
type Snapshot struct {
	Commands    commandidentity.Inventory
	Inputs      cliinputs.Inventory
	CommandTree string
	RunFlags    string
}

// Flag returns a detached parsed flag by its long name.
func Flag(result platformprocess.CLIParseResult, name string) (platformprocess.CLIParsedFlag, bool) {
	for _, flag := range result.Flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return platformprocess.CLIParsedFlag{}, false
}

// Result is the complete observation emitted for one Process.Execute call.
type Result struct {
	Snapshot Snapshot
	Parse    platformprocess.CLIParseResult
}

// Encode converts the typed transport projection into its neutral process-edge
// contract.
func Encode(result Result) (platformprocess.CLIObservation, error) {
	commands, err := json.Marshal(result.Snapshot.Commands)
	if err != nil {
		return platformprocess.CLIObservation{}, fmt.Errorf("encode command identity observation: %w", err)
	}
	inputs, err := json.Marshal(result.Snapshot.Inputs)
	if err != nil {
		return platformprocess.CLIObservation{}, fmt.Errorf("encode command inputs observation: %w", err)
	}
	return platformprocess.CLIObservation{
		CommandIdentityJSON: string(commands), CommandInputsJSON: string(inputs),
		CommandTree: result.Snapshot.CommandTree, RunFlags: result.Snapshot.RunFlags,
		Parse: result.Parse,
	}, nil
}

// Decode restores typed transport inventories from a neutral process-edge
// observation.
func Decode(observed platformprocess.CLIObservation) (Result, error) {
	var result Result
	if err := json.Unmarshal([]byte(observed.CommandIdentityJSON), &result.Snapshot.Commands); err != nil {
		return Result{}, fmt.Errorf("decode command identity observation: %w", err)
	}
	if err := json.Unmarshal([]byte(observed.CommandInputsJSON), &result.Snapshot.Inputs); err != nil {
		return Result{}, fmt.Errorf("decode command inputs observation: %w", err)
	}
	result.Snapshot.CommandTree = observed.CommandTree
	result.Snapshot.RunFlags = observed.RunFlags
	result.Parse = observed.Parse
	return result, nil
}

// Capture returns an exact process-edge observer that decodes into target.
func Capture(target *Result) platformprocess.CLIObserver {
	return func(observed platformprocess.CLIObservation) error {
		result, err := Decode(observed)
		if err != nil {
			return err
		}
		if target != nil {
			*target = result
		}
		return nil
	}
}

// CaptureAppend returns an observer that appends each detached invocation.
func CaptureAppend(target *[]Result) platformprocess.CLIObserver {
	return func(observed platformprocess.CLIObservation) error {
		result, err := Decode(observed)
		if err != nil {
			return err
		}
		if target != nil {
			*target = append(*target, result)
		}
		return nil
	}
}

// CaptureSnapshot projects the private command tree into detached contracts.
func CaptureSnapshot(root *cobra.Command) (Snapshot, error) {
	commands, err := commandidentity.Walk(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe command identity: %w", err)
	}
	inputs, err := cliinputs.Walk(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe command inputs: %w", err)
	}
	run, remaining, err := root.Find([]string{"run"})
	if err != nil {
		return Snapshot{}, fmt.Errorf("observe run command: %w", err)
	}
	if len(remaining) != 0 || run.CommandPath() != "you run" {
		return Snapshot{}, fmt.Errorf("observe run command: resolved %q with remaining arguments %v", run.CommandPath(), remaining)
	}
	return Snapshot{
		Commands:    commands,
		Inputs:      inputs,
		CommandTree: SerializeCommandTree(root),
		RunFlags:    SerializeRunFlags(run),
	}, nil
}

// SerializeCommandTree records the private tree in deterministic baseline form.
func SerializeCommandTree(root *cobra.Command) string {
	lines := collectCommandTreeLines(root)
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func collectCommandTreeLines(command *cobra.Command) []string {
	parent := ""
	if command.Parent() != nil {
		parent = command.Parent().CommandPath()
	}
	lines := []string{command.CommandPath() + "\t" + command.Use + "\t" + parent}
	children := append([]*cobra.Command(nil), command.Commands()...)
	sort.Slice(children, func(i, j int) bool { return children[i].CommandPath() < children[j].CommandPath() })
	for _, child := range children {
		lines = append(lines, collectCommandTreeLines(child)...)
	}
	return lines
}

// SerializeRunFlags records local and inherited run flags deterministically.
func SerializeRunFlags(run *cobra.Command) string {
	byName := map[string]*pflag.Flag{}
	visit := func(set *pflag.FlagSet) {
		if set != nil {
			set.VisitAll(func(flag *pflag.Flag) { byName[flag.Name] = flag })
		}
	}
	visit(run.InheritedFlags())
	visit(run.Flags())
	lines := make([]string, 0, len(byName))
	for _, flag := range byName {
		usage := strings.NewReplacer("\t", " ", "\n", " ").Replace(flag.Usage)
		lines = append(lines, flag.Name+"\t"+flag.Shorthand+"\t"+flag.DefValue+"\t"+usage)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// CaptureParseResult projects parser state without retaining Cobra values.
func CaptureParseResult(command *cobra.Command, positionals []string) platformprocess.CLIParseResult {
	result := platformprocess.CLIParseResult{Positionals: append([]string(nil), positionals...)}
	if command == nil {
		return result
	}
	result.CommandPath = command.CommandPath()
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		value := ""
		if flag.Value != nil {
			value = flag.Value.String()
		}
		result.Flags = append(result.Flags, platformprocess.CLIParsedFlag{Name: flag.Name, Changed: flag.Changed, Value: value})
	})
	return result
}
