package climanifestcobra

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// NewResolvedWorkCommandTree builds an independently executable `you work`
// tree through the generic manifest constructor. The tree contains no command
// families besides Work.
func NewResolvedWorkCommandTree(
	handlers commandregistry.ResolvedWorkHandlers,
) (*cobra.Command, error) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	return NewResolvedWorkCommandTreeFromManifest(manifest, handlers)
}

// NewResolvedWorkCommandTreeFromManifest builds the standalone Work tree from
// one supplied family snapshot. Only the generated root record is added.
func NewResolvedWorkCommandTreeFromManifest(
	manifest climanifest.Manifest,
	handlers commandregistry.ResolvedWorkHandlers,
) (*cobra.Command, error) {
	if err := validateResolvedWorkFamily(manifest); err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	handlerBindings, err := resolvedWorkHandlerBindings(manifest, handlers)
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	commands := make(map[string]climanifest.Command, len(manifest.Commands)+1)
	for commandID, record := range manifest.Commands {
		commands[commandID] = record
	}
	manifest.Commands = commands
	manifest.Commands[rootRecord.ID] = rootRecord
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error {
				return nil
			},
		},
		CobraHandlers:           handlerBindings,
		GuardUnknownSubcommands: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: %w", err)
	}
	for _, commandID := range resolvedWorkRunnableCommandIDs {
		record := manifest.Commands[commandID]
		command, _, findErr := root.Find(recordPathBelowRoot(record))
		if findErr != nil {
			return nil, fmt.Errorf("build resolved work command: find %q: %w", commandID, findErr)
		}
		command.PreRunE = rejectDeprecatedPortFlag
	}
	return root, nil
}

// NewResolvedWorkCommand builds the detached Work subtree intended for the
// post-lease root composition swap.
func NewResolvedWorkCommand(
	handlers commandregistry.ResolvedWorkHandlers,
) (*cobra.Command, error) {
	root, err := NewResolvedWorkCommandTree(handlers)
	if err != nil {
		return nil, err
	}
	work, _, err := root.Find([]string{"work"})
	if err != nil {
		return nil, fmt.Errorf("build resolved work command: find projected work command: %w", err)
	}
	root.RemoveCommand(work)
	return work, nil
}

var resolvedWorkRunnableCommandIDs = [...]string{
	"you.work.list",
	"you.work.show",
	"you.work.move",
	"you.work.visualize",
}

func validateResolvedWorkFamily(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.WorkFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d work-family commands",
			len(manifest.Commands),
			len(climanifestgen.WorkFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		record, ok := manifest.Commands[commandID]
		if !ok {
			return fmt.Errorf("manifest missing work-family command %q", commandID)
		}
		if commandID == "you.work" {
			if record.Runnable {
				return fmt.Errorf("%q must remain non-runnable", commandID)
			}
			continue
		}
		if !record.Runnable || record.Handler == nil || record.Handler.ID == "" {
			return fmt.Errorf("%q must declare a runnable stable handler", commandID)
		}
	}
	return nil
}

func resolvedWorkHandlerBindings(
	manifest climanifest.Manifest,
	handlers commandregistry.ResolvedWorkHandlers,
) (CobraHandlerRegistry, error) {
	supplied := map[string]commandregistry.ResolvedWorkRunE{
		"you.work.list":      handlers.List,
		"you.work.show":      handlers.Show,
		"you.work.move":      handlers.Move,
		"you.work.visualize": handlers.Visualize,
	}
	bindings := make(CobraHandlerRegistry, len(supplied))
	for _, commandID := range resolvedWorkRunnableCommandIDs {
		handler := supplied[commandID]
		if handler == nil {
			return nil, fmt.Errorf("handler for %q is required", commandID)
		}
		record := manifest.Commands[commandID]
		recordCopy := record
		if _, duplicate := bindings[record.Handler.ID]; duplicate {
			return nil, fmt.Errorf("stable handler %q is duplicated", record.Handler.ID)
		}
		bindings[record.Handler.ID] = func(
			cmd *cobra.Command,
			_ []string,
			values map[string]any,
			inherited resolvedinput.Inputs,
		) error {
			local, err := resolveCompatibilityWorkInputs(cmd, recordCopy, values)
			if err != nil {
				return fmt.Errorf("resolve %q inputs: %w", recordCopy.ID, err)
			}
			return handler(cmd, local, inherited)
		}
	}
	return bindings, nil
}

func resolveCompatibilityWorkInputs(
	cmd *cobra.Command,
	record climanifest.Command,
	values map[string]any,
) (resolvedinput.Inputs, error) {
	definitions := make([]resolvedinput.Definition, 0, len(record.Arguments)+len(record.Flags))
	candidates := make([]resolvedinput.Candidate, 0, len(record.Arguments)+len(record.Flags))

	argumentIDs := sortedWorkArgumentIDs(record.Arguments)
	for _, inputID := range argumentIDs {
		argument := record.Arguments[inputID]
		kind, err := resolvedValueKind(argument.ValueType)
		if err != nil {
			return resolvedinput.Inputs{}, fmt.Errorf("argument %q: %w", inputID, err)
		}
		definitions = append(definitions, resolvedinput.Definition{
			ID: inputID, Kind: kind,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		})
		value, present := values[inputID]
		if !present || value == nil {
			continue
		}
		candidate, err := resolvedWorkCandidate(inputID, resolvedinput.SourcePositionalArgument, value)
		if err != nil {
			return resolvedinput.Inputs{}, err
		}
		candidates = append(candidates, candidate)
	}

	for _, inputID := range sortedKeys(record.Flags) {
		flag := record.Flags[inputID]
		if flag.Scope != "local" {
			continue
		}
		kind, err := resolvedValueKind(flag.ValueType)
		if err != nil {
			return resolvedinput.Inputs{}, fmt.Errorf("flag %q: %w", inputID, err)
		}
		definitions = append(definitions, resolvedinput.Definition{
			ID: inputID, Kind: kind,
			Precedence: []resolvedinput.Source{
				resolvedinput.SourceCLIFlag,
				resolvedinput.SourceManifestDefault,
			},
		})
		value, present := values[inputID]
		if !present {
			continue
		}
		source := resolvedinput.SourceManifestDefault
		if workFlagChanged(cmd, flag) {
			source = resolvedinput.SourceCLIFlag
		}
		candidate, err := resolvedWorkCandidate(inputID, source, value)
		if err != nil {
			return resolvedinput.Inputs{}, err
		}
		candidates = append(candidates, candidate)
	}
	input, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		return resolvedinput.Inputs{}, err
	}
	return input, nil
}

func sortedWorkArgumentIDs(arguments map[string]climanifest.Argument) []string {
	ids := make([]string, 0, len(arguments))
	for inputID := range arguments {
		ids = append(ids, inputID)
	}
	sort.Slice(ids, func(i, j int) bool {
		left := arguments[ids[i]]
		right := arguments[ids[j]]
		if left.Position != right.Position {
			return left.Position < right.Position
		}
		return ids[i] < ids[j]
	})
	return ids
}

func resolvedWorkCandidate(
	inputID string,
	source resolvedinput.Source,
	value any,
) (resolvedinput.Candidate, error) {
	resolved, err := resolvedValue(value)
	if err != nil {
		return resolvedinput.Candidate{}, fmt.Errorf("input %q: %w", inputID, err)
	}
	return resolvedinput.Candidate{InputID: inputID, Source: source, Value: resolved}, nil
}

func workFlagChanged(cmd *cobra.Command, flag climanifest.Flag) bool {
	for _, name := range append([]string{flag.Long}, flag.Aliases...) {
		if parsed := lookupCommandFlag(cmd, name); parsed != nil && parsed.Changed {
			return true
		}
	}
	return false
}

func recordPathBelowRoot(record climanifest.Command) []string {
	path := strings.Fields(record.Path)
	if len(path) == 0 {
		return nil
	}
	return path[1:]
}
