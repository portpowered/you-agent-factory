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

// WorkFamilyComponents holds detached work-family commands before production
// wiring attaches the generated work parent to the root.
type WorkFamilyComponents struct {
	Work         *cobra.Command
	Approval     *cobra.Command
	ApprovalList *cobra.Command
	ApprovalShow *cobra.Command
	List         *cobra.Command
	Watch        *cobra.Command
	Show         *cobra.Command
	Move         *cobra.Command
	Visualize    *cobra.Command
}

// WorkFamilyBindings supplies parser-only scalar storage keyed by stable
// manifest input ID. Handlers build typed Work requests per invocation.
type WorkFamilyBindings struct {
	LocalTargets map[string]any
}

// NewWorkFamilyCommand builds the work you.work → approval/list/watch/show/move/visualize tree
// from generated metadata and attaches handwritten handlers by stable command ID.
// Only contracted work-family commands are constructed.
func NewWorkFamilyCommand(registry *commandregistry.Registry, bindings WorkFamilyBindings) (*cobra.Command, error) {
	components, err := NewWorkFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Work.AddCommand(components.Approval, components.List, components.Watch, components.Show, components.Move, components.Visualize)
	components.Approval.AddCommand(components.ApprovalList, components.ApprovalShow)
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
	components.Work.AddCommand(components.Approval, components.List, components.Watch, components.Show, components.Move, components.Visualize)
	components.Approval.AddCommand(components.ApprovalList, components.ApprovalShow)
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

	workRecord, approvalRecord, approvalListRecord, approvalShowRecord, listRecord, watchRecord, showRecord, moveRecord, visualizeRecord, err := workManifestRecordsWithApprovals(manifest)
	if err != nil {
		return WorkFamilyComponents{}, err
	}

	work, err := buildNonRunnableWorkCommand(workRecord)
	if err != nil {
		return WorkFamilyComponents{}, err
	}
	approval, err := buildNonRunnableWorkCommand(approvalRecord)
	if err != nil {
		return WorkFamilyComponents{}, err
	}
	leaves, err := buildRunnableWorkLeaves(registry, bindings, approvalListRecord, approvalShowRecord, listRecord, watchRecord, showRecord, moveRecord, visualizeRecord)
	if err != nil {
		return WorkFamilyComponents{}, fmt.Errorf("build work family command: %w", err)
	}

	return WorkFamilyComponents{
		Work:         work,
		Approval:     approval,
		ApprovalList: leaves[0], ApprovalShow: leaves[1], List: leaves[2],
		Watch: leaves[3], Show: leaves[4], Move: leaves[5], Visualize: leaves[6],
	}, nil
}

func buildNonRunnableWorkCommand(record climanifest.Command) (*cobra.Command, error) {
	cmd, err := buildWorkCommandFromRecord(record)
	if err != nil {
		return nil, fmt.Errorf("build work family command: %w", err)
	}
	if record.Runnable {
		return nil, fmt.Errorf("build work family command: %q must remain non-runnable", record.ID)
	}
	return cmd, nil
}

func buildRunnableWorkLeaves(registry *commandregistry.Registry, bindings WorkFamilyBindings, records ...climanifest.Command) ([]*cobra.Command, error) {
	leaves := make([]*cobra.Command, 0, len(records))
	for _, record := range records {
		leaf, err := buildRunnableWorkLeaf(record, registry, bindings)
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, leaf)
	}
	return leaves, nil
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
	if cmd.Args == nil && record.ID == "you.work.watch" {
		cmd.Args = cobra.NoArgs
	}
	cmd.PreRunE = rejectDeprecatedPortFlag
	if err := registerWorkLocalFlags(cmd, record, bindings); err != nil {
		return nil, err
	}
	relationships, err := planStandaloneCommandRelationships(record)
	if err != nil {
		return nil, err
	}
	if err := projectCobraFlagGroupAnnotations(cmd, record.ID, relationships); err != nil {
		return nil, err
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, err
	}
	return cmd, nil
}

func workManifestRecordsWithApprovals(manifest climanifest.Manifest) (
	work, approval, approvalList, approvalShow, list, watch, show, move, visualize climanifest.Command,
	err error,
) {
	ids := []string{
		"you.work",
		"you.work.approval",
		"you.work.approval.list",
		"you.work.approval.show",
		"you.work.list",
		"you.work.watch",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	}
	records := make([]climanifest.Command, len(ids))
	for i, id := range ids {
		records[i], err = manifest.CommandByID(id)
		if err != nil {
			return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build work family command: %w", err)
		}
	}
	return records[0], records[1], records[2], records[3], records[4], records[5], records[6], records[7], records[8], nil
}

// workManifestRecords preserves the legacy internal test/helper shape while
// the production constructor also consumes the approval subfamily.
func workManifestRecords(manifest climanifest.Manifest) (
	work, list, watch, show, move, visualize climanifest.Command,
	err error,
) {
	work, _, _, _, list, watch, show, move, visualize, err = workManifestRecordsWithApprovals(manifest)
	return work, list, watch, show, move, visualize, err
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
	if len(bindings.LocalTargets) == 0 {
		return fmt.Errorf("build work family command: bindings.LocalTargets is required")
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
			annotateStableInput(cmd, flag)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
			}
			continue
		}
		target, err := flagBindingTarget(flag.ID, bindings.LocalTargets)
		if err != nil {
			return err
		}
		if err := registerFlag(cmd.Flags(), flag, target, flag.Usage); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		annotateStableInput(cmd, flag)
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

// NewResolvedWorkCommandTree is the canonical independently executable
// `you work` constructor. It uses the generic manifest constructor and contains
// no command families besides Work.
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
	// The generic constructor guards non-runnable groups with a help-producing
	// RunE. This detached family is attached beneath the existing root, whose
	// compatibility contract keeps `you work` and `you work approval`
	// non-runnable; preserve that shape while retaining the generated leaves.
	if err := clearWorkGroupExecution(work); err != nil {
		return nil, fmt.Errorf("build resolved work command: preserve group behavior: %w", err)
	}
	return work, nil
}

func clearWorkGroupExecution(work *cobra.Command) error {
	groups := []*cobra.Command{work}
	approval, _, err := work.Find([]string{"approval"})
	if err != nil {
		return err
	}
	groups = append(groups, approval)
	for _, group := range groups {
		group.RunE = nil
		group.Args = nil
		group.DisableFlagParsing = false
	}
	return nil
}

var resolvedWorkRunnableCommandIDs = [...]string{
	"you.work.approval.list",
	"you.work.approval.show",
	"you.work.list",
	"you.work.watch",
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
		if commandID == "you.work" || commandID == "you.work.approval" {
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
		"you.work.approval.list": handlers.ApprovalList,
		"you.work.approval.show": handlers.ApprovalShow,
		"you.work.list":          handlers.List,
		"you.work.watch":         handlers.Watch,
		"you.work.show":          handlers.Show,
		"you.work.move":          handlers.Move,
		"you.work.visualize":     handlers.Visualize,
	}
	bindings := make(CobraHandlerRegistry, len(supplied))
	for _, commandID := range resolvedWorkRunnableCommandIDs {
		handler := supplied[commandID]
		if handler == nil {
			if commandID != "you.work.watch" &&
				commandID != "you.work.approval.list" &&
				commandID != "you.work.approval.show" {
				return nil, fmt.Errorf("handler for %q is required", commandID)
			}
			message := "work watch service is required"
			if commandID == "you.work.approval.list" {
				message = "human approval list service is required"
			} else if commandID == "you.work.approval.show" {
				message = "human approval show service is required"
			}
			handler = func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
				return fmt.Errorf("%s", message)
			}
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

const workerSessionsListHandlerID = "you.worker-sessions.list.handler"
const workerSessionsShowHandlerID = "you.worker-sessions.show.handler"
const workerSessionsReadHandlerID = "you.worker-sessions.read.handler"
const workerSessionsStreamHandlerID = "you.worker-sessions.stream.handler"
const workerSessionsInvokeHandlerID = "you.worker-sessions.invoke.handler"
const workerSessionsContinueHandlerID = "you.worker-sessions.continue.handler"
const workerSessionsInterruptHandlerID = "you.worker-sessions.interrupt.handler"
const workerSessionsPauseHandlerID = "you.worker-sessions.pause.handler"
const workerSessionsResumeHandlerID = "you.worker-sessions.resume.handler"
const workerSessionsCancelHandlerID = "you.worker-sessions.cancel.handler"
const workerSessionsTerminateHandlerID = "you.worker-sessions.terminate.handler"

var workerSessionsRunnableCommands = []struct {
	id        string
	handlerID string
}{
	{id: "you.worker-sessions.invoke", handlerID: workerSessionsInvokeHandlerID},
	{id: "you.worker-sessions.continue", handlerID: workerSessionsContinueHandlerID},
	{id: "you.worker-sessions.interrupt", handlerID: workerSessionsInterruptHandlerID},
	{id: "you.worker-sessions.pause", handlerID: workerSessionsPauseHandlerID},
	{id: "you.worker-sessions.resume", handlerID: workerSessionsResumeHandlerID},
	{id: "you.worker-sessions.cancel", handlerID: workerSessionsCancelHandlerID},
	{id: "you.worker-sessions.terminate", handlerID: workerSessionsTerminateHandlerID},
	{id: "you.worker-sessions.list", handlerID: workerSessionsListHandlerID},
	{id: "you.worker-sessions.show", handlerID: workerSessionsShowHandlerID},
	{id: "you.worker-sessions.read", handlerID: workerSessionsReadHandlerID},
	{id: "you.worker-sessions.stream", handlerID: workerSessionsStreamHandlerID},
}

// NewWorkerSessionsFamilyCommand builds the detached `you worker-sessions`
// observation family from generated metadata and a stable handler registry.
func NewWorkerSessionsFamilyCommand(registry *commandregistry.Registry) (*cobra.Command, error) {
	manifest, err := generated.WorkerSessionsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewWorkerSessionsFamilyCommandFromManifest(manifest, registry)
}

// NewWorkerSessionsFamilyCommandFromManifest projects one worker-session
// family snapshot. The supplied manifest is validated against the stable
// family IDs before any Cobra command is returned.
func NewWorkerSessionsFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build worker sessions family command: registry is required")
	}
	if err := validateWorkerSessionsManifest(manifest); err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	registered, err := lookupWorkerSessionsHandlers(registry)
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	cobraHandlers := CobraHandlerRegistry{}
	resolvedHandlers := ResolvedCobraHandlerRegistry{}
	for _, binding := range []struct {
		handlerID string
		handlers  commandregistry.CommandHandlers
	}{
		{workerSessionsInvokeHandlerID, registered.invoke},
		{workerSessionsContinueHandlerID, registered.continueOperation},
		{workerSessionsInterruptHandlerID, registered.interrupt},
		{workerSessionsPauseHandlerID, registered.pause},
		{workerSessionsResumeHandlerID, registered.resume},
		{workerSessionsCancelHandlerID, registered.cancel},
		{workerSessionsTerminateHandlerID, registered.terminate},
		{workerSessionsListHandlerID, registered.list},
		{workerSessionsShowHandlerID, registered.show},
		{workerSessionsReadHandlerID, registered.read},
		{workerSessionsStreamHandlerID, registered.stream},
	} {
		cobraHandler, resolvedHandler, bindingErr := workerSessionsHandlerBindings(binding.handlers)
		if bindingErr != nil {
			return nil, fmt.Errorf("build worker sessions family command: %w", bindingErr)
		}
		if resolvedHandler != nil {
			resolvedHandlers[binding.handlerID] = resolvedHandler
		} else {
			cobraHandlers[binding.handlerID] = cobraHandler
		}
	}

	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		CobraHandlers:         cobraHandlers,
		ResolvedCobraHandlers: resolvedHandlers,
		DeferRequiredValidation: map[string]bool{
			workerSessionsInvokeHandlerID:    true,
			workerSessionsContinueHandlerID:  true,
			workerSessionsInterruptHandlerID: true,
			workerSessionsPauseHandlerID:     true,
			workerSessionsResumeHandlerID:    true,
			workerSessionsCancelHandlerID:    true,
			workerSessionsTerminateHandlerID: true,
			workerSessionsListHandlerID:      true,
			workerSessionsShowHandlerID:      true,
			workerSessionsReadHandlerID:      true,
			workerSessionsStreamHandlerID:    true,
		},
		GuardUnknownSubcommands: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	workerSessions, _, err := root.Find([]string{"worker-sessions"})
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: find worker-sessions: %w", err)
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("build worker sessions family command: worker-sessions command is unavailable")
	}
	root.RemoveCommand(workerSessions)
	workerSessions.SilenceUsage = true
	return workerSessions, nil
}

type workerSessionsHandlers struct {
	invoke, continueOperation, interrupt, pause, resume, cancel, terminate, list, show, read, stream commandregistry.CommandHandlers
}

func lookupWorkerSessionsHandlers(registry *commandregistry.Registry) (workerSessionsHandlers, error) {
	found := make([]commandregistry.CommandHandlers, len(workerSessionsRunnableCommands))
	for index, command := range workerSessionsRunnableCommands {
		handlers, err := registry.LookupHandlers(command.handlerID)
		if err != nil {
			return workerSessionsHandlers{}, err
		}
		if (handlers.RunE == nil) == (handlers.ResolvedRunE == nil) {
			return workerSessionsHandlers{}, fmt.Errorf(
				"handler %q must provide exactly one of RunE or ResolvedRunE",
				command.handlerID,
			)
		}
		found[index] = handlers
	}
	return workerSessionsHandlers{
		invoke: found[0], continueOperation: found[1], interrupt: found[2],
		pause: found[3], resume: found[4], cancel: found[5], terminate: found[6],
		list: found[7], show: found[8], read: found[9], stream: found[10],
	}, nil
}

func workerSessionsHandlerBindings(
	handlers commandregistry.CommandHandlers,
) (CobraHandler, ResolvedCobraHandler, error) {
	if handlers.ResolvedRunE != nil {
		return nil, handlers.ResolvedRunE, nil
	}
	if handlers.RunE == nil {
		return nil, nil, fmt.Errorf("worker session handler is required")
	}
	return func(
		cmd *cobra.Command,
		args []string,
		_ map[string]any,
		_ resolvedinput.Inputs,
	) error {
		if handlers.PreRunE != nil {
			if err := handlers.PreRunE(cmd, args); err != nil {
				return err
			}
		}
		return handlers.RunE(cmd, args)
	}, nil, nil
}

func validateWorkerSessionsManifest(manifest climanifest.Manifest) error {
	if manifest.RootPath != "you" {
		return fmt.Errorf("manifest root path = %q, want %q", manifest.RootPath, "you")
	}
	if len(manifest.Commands) != len(climanifestgen.WorkerSessionsFamilyCommandIDs)+1 {
		return fmt.Errorf("manifest command count = %d, want %d", len(manifest.Commands), len(climanifestgen.WorkerSessionsFamilyCommandIDs)+1)
	}
	for commandID, record := range manifest.Commands {
		if commandID != "you" {
			if err := climanifestgen.AssertWorkerSessionsFamilyCommandID(commandID); err != nil {
				return err
			}
		}
		if record.ID != commandID {
			return fmt.Errorf("manifest command key %q has record ID %q", commandID, record.ID)
		}
	}
	parent, err := manifest.CommandByID("you.worker-sessions")
	if err != nil {
		return err
	}
	if parent.Runnable {
		return fmt.Errorf("command %q must remain non-runnable", parent.ID)
	}
	for _, command := range workerSessionsRunnableCommands {
		if err := validateWorkerSessionsRunnableCommand(manifest, command.id, command.handlerID); err != nil {
			return err
		}
	}
	if root, err := manifest.CommandByID("you"); err != nil {
		return err
	} else if root.Handler == nil || root.Handler.ID == "" {
		return fmt.Errorf("root command %q must declare a handler", root.ID)
	}
	return nil
}

func validateWorkerSessionsRunnableCommand(manifest climanifest.Manifest, commandID, handlerID string) error {
	command, err := manifest.CommandByID(commandID)
	if err != nil {
		return err
	}
	if !command.Runnable || command.Handler == nil || command.Handler.ID != handlerID {
		return fmt.Errorf("command %q must declare runnable handler %q", command.ID, handlerID)
	}
	return nil
}

func projectCobraFlagGroupAnnotations(
	cmd *cobra.Command,
	commandID string,
	relationships []plannedRelationship,
) error {
	for _, relationship := range relationships {
		names := relationshipFlagNames(relationship)
		if len(names) == 0 {
			continue
		}
		if err := validateRelationshipFlags(cmd, commandID, relationship, names); err != nil {
			return err
		}
		if projected, err := projectLocalFlagGroupAnnotations(cmd, relationship.record.Kind, names); err != nil {
			return fmt.Errorf(
				"command %q relationship %q: %w",
				commandID,
				relationship.record.ID,
				err,
			)
		} else if projected {
			continue
		}
		markCobraFlagGroup(cmd, relationship.record.Kind, names)
	}
	return nil
}

func relationshipFlagNames(relationship plannedRelationship) []string {
	names := make([]string, 0, len(relationship.participants))
	for _, participant := range relationship.participants {
		if participant.kind != "flag" || !participant.cobraGroupAnnotationSafe {
			return nil
		}
		names = append(names, strings.TrimPrefix(participant.public, "--"))
	}
	return names
}

func validateRelationshipFlags(
	cmd *cobra.Command,
	commandID string,
	relationship plannedRelationship,
	names []string,
) error {
	for _, name := range names {
		if lookupCommandFlag(cmd, name) == nil {
			return fmt.Errorf(
				"command %q relationship %q cannot project unavailable flag %q",
				commandID, relationship.record.ID, "--"+name,
			)
		}
	}
	return nil
}

const (
	cobraRequiredAsGroupAnnotation   = "cobra_annotation_required_if_others_set"
	cobraOneRequiredAnnotation       = "cobra_annotation_one_required"
	cobraMutuallyExclusiveAnnotation = "cobra_annotation_mutually_exclusive"
)

// projectLocalFlagGroupAnnotations writes Cobra's standard relationship
// annotations directly when every participant is a flag declared by the
// command itself. Calling Cobra's MarkFlags* helpers in that case would first
// merge detached parent persistent flags into the command. Detached command
// families are later attached to the production root, so that eager merge can
// shadow the real root flag storage (for example, --server).
func projectLocalFlagGroupAnnotations(
	cmd *cobra.Command,
	kind string,
	names []string,
) (bool, error) {
	flags := cmd.Flags()
	for _, name := range names {
		if flags.Lookup(name) == nil {
			return false, nil
		}
	}
	annotation, ok := cobraFlagGroupAnnotation(kind)
	if !ok {
		return false, nil
	}
	group := strings.Join(names, " ")
	for _, name := range names {
		flag := flags.Lookup(name)
		values := append([]string(nil), flag.Annotations[annotation]...)
		values = append(values, group)
		if err := flags.SetAnnotation(name, annotation, values); err != nil {
			return true, err
		}
	}
	return true, nil
}

func cobraFlagGroupAnnotation(kind string) (string, bool) {
	switch kind {
	case "mutually-exclusive", "conflict":
		return cobraMutuallyExclusiveAnnotation, true
	case "required-together":
		return cobraRequiredAsGroupAnnotation, true
	case "at-least-one":
		return cobraOneRequiredAnnotation, true
	default:
		return "", false
	}
}

func markCobraFlagGroup(cmd *cobra.Command, kind string, names []string) {
	switch kind {
	case "mutually-exclusive", "conflict":
		cmd.MarkFlagsMutuallyExclusive(names...)
	case "required-together":
		cmd.MarkFlagsRequiredTogether(names...)
	case "at-least-one":
		cmd.MarkFlagsOneRequired(names...)
	}
}

func relationshipError(relationship plannedRelationship, message string) error {
	names := make([]string, len(relationship.participants))
	for index, participant := range relationship.participants {
		names[index] = participant.public
	}
	return fmt.Errorf(
		"input relationship %q: %s %s",
		relationship.record.ID,
		message,
		strings.Join(names, ", "),
	)
}
