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
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

// WorkFamilyComponents holds detached work-family commands before production
// wiring attaches the generated work parent to the root.
type WorkFamilyComponents struct {
	Work      *cobra.Command
	List      *cobra.Command
	Show      *cobra.Command
	Move      *cobra.Command
	Visualize *cobra.Command
}

// WorkFamilyBindings supplies live variables for work-family local flags declared
// in generated metadata.
type WorkFamilyBindings struct {
	ListConfig      *workcli.ListConfig
	ShowConfig      *workcli.ShowConfig
	MoveConfig      *workcli.MoveConfig
	VisualizeFormat *string
	// FlagUsages supplies Cobra help text for local flags when the manifest does
	// not carry per-flag usage strings.
	FlagUsages map[string]string
}

// NewWorkFamilyCommand builds the work you.work → list/show/move/visualize tree
// from generated metadata and attaches handwritten handlers by stable command ID.
// Only contracted work-family commands are constructed.
func NewWorkFamilyCommand(registry *commandregistry.Registry, bindings WorkFamilyBindings) (*cobra.Command, error) {
	components, err := NewWorkFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Work.AddCommand(components.List, components.Show, components.Move, components.Visualize)
	return components.Work, nil
}

// NewWorkFamilyComponents builds detached work-family commands so production
// wiring can attach the generated work parent without rewriting unrelated roots.
func NewWorkFamilyComponents(registry *commandregistry.Registry, bindings WorkFamilyBindings) (WorkFamilyComponents, error) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	return NewWorkFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewWorkFamilyCommandFromManifest builds the work tree from one generated
// manifest snapshot. Manifest command IDs must stay within the work family.
func NewWorkFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (*cobra.Command, error) {
	components, err := NewWorkFamilyComponentsFromManifest(manifest, registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Work.AddCommand(components.List, components.Show, components.Move, components.Visualize)
	return components.Work, nil
}

// NewWorkFamilyComponentsFromManifest builds detached work-family commands from
// one generated manifest snapshot.
func NewWorkFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (WorkFamilyComponents, error) {
	if registry == nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: registry is required")
	}
	if err := validateWorkBindings(bindings); err != nil {
		return WorkFamilyComponents{}, err
	}
	if err := validateWorkManifest(manifest); err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}

	workRecord, listRecord, showRecord, moveRecord, visualizeRecord, err := workManifestRecords(manifest)
	if err != nil {
		return WorkFamilyComponents{}, err
	}

	work, err := buildWorkCommandFromRecord(workRecord)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	if workRecord.Runnable {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %q must remain non-runnable", workRecord.ID)
	}

	list, err := buildRunnableWorkLeaf(listRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	show, err := buildRunnableWorkLeaf(showRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	move, err := buildRunnableWorkLeaf(moveRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}
	visualize, err := buildRunnableWorkLeaf(visualizeRecord, registry, bindings)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}

	return WorkFamilyComponents{
		Work:      work,
		List:      list,
		Show:      show,
		Move:      move,
		Visualize: visualize,
	}, nil
}

func buildRunnableWorkLeaf(
	record climanifest.Command,
	registry *commandregistry.Registry,
	bindings WorkFamilyBindings,
) (*cobra.Command, error) {
	cmd, err := buildWorkCommandFromRecord(record)
	if err != nil {
		return nil, err
	}
	cmd.Args = positionalArgsFromManifest(record)
	cmd.PreRunE = rejectDeprecatedPortFlag
	if err := registerWorkLocalFlags(cmd, record, bindings); err != nil {
		return nil, err
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, err
	}
	return cmd, nil
}

func workManifestRecords(manifest climanifest.Manifest) (
	work, list, show, move, visualize climanifest.Command,
	err error,
) {
	work, err = manifest.CommandByID("you.work")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	list, err = manifest.CommandByID("you.work.list")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	show, err = manifest.CommandByID("you.work.show")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	move, err = manifest.CommandByID("you.work.move")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	visualize, err = manifest.CommandByID("you.work.visualize")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
	}
	return work, list, show, move, visualize, nil
}

func validateWorkManifest(manifest climanifest.Manifest) error {
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
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing work-family command %q", commandID)
		}
	}
	return nil
}

func validateWorkBindings(bindings WorkFamilyBindings) error {
	required := []struct {
		name string
		ok   bool
	}{
		{"ListConfig", bindings.ListConfig != nil},
		{"ShowConfig", bindings.ShowConfig != nil},
		{"MoveConfig", bindings.MoveConfig != nil},
		{"VisualizeFormat", bindings.VisualizeFormat != nil},
	}
	for _, field := range required {
		if !field.ok {
			return fmt.Errorf("build work family command: bindings.%s is required", field.name)
		}
	}
	return nil
}

func buildWorkCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertWorkFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if record.Visibility == "hidden" {
		cmd.Hidden = true
	}
	return cmd, nil
}

func registerWorkLocalFlags(cmd *cobra.Command, record climanifest.Command, bindings WorkFamilyBindings) error {
	var deprecatedPort int
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
			}
			continue
		}
		target, err := workLocalBindingTarget(record.ID, flag, bindings)
		if err != nil {
			return err
		}
		if err := registerFlag(cmd.Flags(), flag, target, workFlagUsage(bindings, flag.Long)); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func workLocalBindingTarget(commandID string, flag climanifest.Flag, bindings WorkFamilyBindings) (flagTarget, error) {
	switch commandID {
	case "you.work.list":
		return listLocalBindingTarget(flag, bindings.ListConfig)
	case "you.work.show":
		return showLocalBindingTarget(flag, bindings.ShowConfig)
	case "you.work.move":
		return moveLocalBindingTarget(flag, bindings.MoveConfig)
	case "you.work.visualize":
		return visualizeLocalBindingTarget(flag, bindings.VisualizeFormat)
	default:
		return flagTarget{}, fmt.Errorf("unsupported work command %q for local flag %q", commandID, flag.Long)
	}
}

func listLocalBindingTarget(flag climanifest.Flag, cfg *workcli.ListConfig) (flagTarget, error) {
	switch flag.Long {
	case "state-name":
		return flagTarget{stringValue: &cfg.StateName}, nil
	case "state-type":
		return flagTarget{stringValue: &cfg.StateType}, nil
	case "name":
		return flagTarget{stringValue: &cfg.Name}, nil
	case "work-type-name":
		return flagTarget{stringValue: &cfg.WorkTypeName}, nil
	case "trace-id":
		return flagTarget{stringValue: &cfg.TraceID}, nil
	case "sort-by":
		return flagTarget{stringValue: &cfg.SortBy}, nil
	case "max-results":
		return flagTarget{intValue: &cfg.MaxResults}, nil
	case "next-token":
		return flagTarget{stringValue: &cfg.NextToken}, nil
	case "session":
		return flagTarget{stringValue: &cfg.SessionID}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported list local flag %q", flag.Long)
	}
}

func showLocalBindingTarget(flag climanifest.Flag, cfg *workcli.ShowConfig) (flagTarget, error) {
	switch flag.Long {
	case "session":
		return flagTarget{stringValue: &cfg.SessionID}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported show local flag %q", flag.Long)
	}
}

func moveLocalBindingTarget(flag climanifest.Flag, cfg *workcli.MoveConfig) (flagTarget, error) {
	switch flag.Long {
	case "session":
		return flagTarget{stringValue: &cfg.SessionID}, nil
	case "request-id":
		return flagTarget{stringValue: &cfg.RequestID}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported move local flag %q", flag.Long)
	}
}

func visualizeLocalBindingTarget(flag climanifest.Flag, format *string) (flagTarget, error) {
	switch flag.Long {
	case "format":
		return flagTarget{stringValue: format}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported visualize local flag %q", flag.Long)
	}
}

func workFlagUsage(bindings WorkFamilyBindings, longName string) string {
	if bindings.FlagUsages == nil {
		return ""
	}
	return bindings.FlagUsages[longName]
}

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
