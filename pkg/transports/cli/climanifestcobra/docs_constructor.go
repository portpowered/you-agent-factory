package climanifestcobra

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

const supportedLifecycleFormatVersion = "1.0.0"

var lifecycleVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$`)

// CompletionRegistry maps stable manifest input IDs to transport-edge dynamic
// completion callbacks.
type CompletionRegistry map[string]cobra.CompletionFunc

// GenericHandler receives one invocation's normalized inputs keyed by stable
// manifest input ID. It remains independent of Cobra and public command names.
type GenericHandler func(context.Context, map[string]any) error

// HandlerRegistry maps stable manifest handler IDs to transport-edge behavior.
type HandlerRegistry map[string]GenericHandler

// GenericBindings supplies executable transport bindings used while projecting
// a generic manifest. Additional stable-ID registries can be added here without
// coupling manifest records to public command or input spellings.
type GenericBindings struct {
	Completions CompletionRegistry
	Handlers    HandlerRegistry
}

func resolveGenericBindings(bindingSets []GenericBindings) (GenericBindings, error) {
	if len(bindingSets) > 1 {
		return GenericBindings{}, fmt.Errorf("at most one generic binding set is supported")
	}
	if len(bindingSets) == 0 {
		return GenericBindings{}, nil
	}
	return bindingSets[0], nil
}

func validateGenericHandlers(plan []plannedCommand, bindings GenericBindings) error {
	owners := make(map[string]string)
	for _, item := range plan {
		handler := item.record.Handler
		if !item.record.Runnable {
			if handler != nil {
				return fmt.Errorf(
					"command %q is non-runnable but declares handler ID %q",
					item.record.ID,
					handler.ID,
				)
			}
			continue
		}
		if handler == nil || strings.TrimSpace(handler.ID) == "" {
			return fmt.Errorf("command %q runnable handler ID is required", item.record.ID)
		}
		if owner, exists := owners[handler.ID]; exists {
			return fmt.Errorf(
				"command %q handler ID %q duplicates runnable command %q",
				item.record.ID,
				handler.ID,
				owner,
			)
		}
		owners[handler.ID] = item.record.ID
		if bindings.Handlers == nil || bindings.Handlers[handler.ID] == nil {
			return fmt.Errorf(
				"command %q handler ID %q has no registered executable binding",
				item.record.ID,
				handler.ID,
			)
		}
	}
	return nil
}

func projectGenericHandler(cmd *cobra.Command, record climanifest.Command, bindings GenericBindings) {
	if !record.Runnable {
		return
	}
	handler := bindings.Handlers[record.Handler.ID]
	cmd.RunE = func(command *cobra.Command, _ []string) error {
		values, err := InputValues(command)
		if err != nil {
			return fmt.Errorf("dispatch command %q handler %q: %w", record.ID, record.Handler.ID, err)
		}
		return handler(command.Context(), values)
	}
}

func validateGenericPresentation(plan []plannedCommand, bindings GenericBindings) error {
	if err := validateSiblingCommandIdentities(plan); err != nil {
		return err
	}
	flags := make(map[string]climanifest.Flag)
	for _, item := range plan {
		if err := validateLifecycle(item.record.ID, item.record.Lifecycle, false); err != nil {
			return fmt.Errorf("command %q lifecycle: %w", item.record.ID, err)
		}
		for _, flag := range item.flags {
			if err := validateLifecycle(flag.record.ID, flag.record.Lifecycle, false); err != nil {
				return genericFlagError(item.record.ID, flag.record.ID, "lifecycle: %v", err)
			}
			flags[flag.record.ID] = flag.record
			if err := validateInputCompletion(item.record.ID, flag.record.ID, flag.record.Completion, flag.record.Enum, bindings); err != nil {
				return err
			}
		}
		for _, argument := range item.arguments {
			if err := validateLifecycle(argument.ID, argument.Lifecycle, true); err != nil {
				return genericArgumentError(item.record.ID, argument.ID, "lifecycle: %v", err)
			}
			if err := validateInputCompletion(item.record.ID, argument.ID, argument.Completion, argument.Enum, bindings); err != nil {
				return err
			}
		}
	}
	return validateInheritedPresentation(plan, flags)
}

func validateSiblingCommandIdentities(plan []plannedCommand) error {
	namesByParent := make(map[string]map[string]string)
	for _, item := range plan {
		names := namesByParent[item.parentPath]
		if names == nil {
			names = make(map[string]string)
			namesByParent[item.parentPath] = names
		}
		for _, name := range append([]string{item.record.Name}, item.record.Aliases...) {
			if owner, exists := names[name]; exists {
				return fmt.Errorf(
					"command %q name or alias %q conflicts with sibling command %q under %q",
					item.record.ID,
					name,
					owner,
					item.parentPath,
				)
			}
			names[name] = item.record.ID
		}
	}
	return nil
}

func validateInputCompletion(
	commandID, inputID, mode string,
	choices []string,
	bindings GenericBindings,
) error {
	switch mode {
	case "none":
		return nil
	case "":
		return fmt.Errorf("command %q input %q: completion mode is required", commandID, inputID)
	case "static":
		if len(choices) == 0 {
			return fmt.Errorf("command %q input %q: static completion requires declared choices", commandID, inputID)
		}
		return nil
	case "dynamic":
		if bindings.Completions == nil || bindings.Completions[inputID] == nil {
			return fmt.Errorf("command %q input %q: missing dynamic completion binding", commandID, inputID)
		}
		return nil
	default:
		return fmt.Errorf("command %q input %q: unsupported completion mode %q", commandID, inputID, mode)
	}
}

func validateInheritedPresentation(plan []plannedCommand, flags map[string]climanifest.Flag) error {
	for _, item := range plan {
		for _, flag := range item.flags {
			if flag.record.Scope != "inherited" {
				continue
			}
			canonical := flags[flag.canonicalID]
			if flag.record.Completion != canonical.Completion ||
				!equalStrings(flag.record.Enum, canonical.Enum) {
				return genericFlagError(
					item.record.ID,
					flag.record.ID,
					"inheritance target %q has incompatible completion metadata",
					flag.canonicalID,
				)
			}
		}
	}
	return nil
}

func validateLifecycle(itemID string, lifecycle climanifest.Lifecycle, optional bool) error {
	if optional && lifecycle == (climanifest.Lifecycle{}) {
		return nil
	}
	if lifecycle.FormatVersion != supportedLifecycleFormatVersion {
		return fmt.Errorf("unsupported or missing formatVersion %q", lifecycle.FormatVersion)
	}
	if lifecycle.ItemID != itemID {
		return fmt.Errorf("itemId %q does not match stable item ID %q", lifecycle.ItemID, itemID)
	}
	if !lifecycleVersionPattern.MatchString(lifecycle.Since) {
		return fmt.Errorf("since %q must be a stable semantic version", lifecycle.Since)
	}
	switch lifecycle.State {
	case "active":
		if lifecycle.Deprecated != "" || lifecycle.Removed != "" || lifecycle.Successor != nil {
			return fmt.Errorf("active state cannot declare deprecation or removal metadata")
		}
		return nil
	case "deprecated":
		return validateDeprecatedLifecycle(lifecycle)
	default:
		return fmt.Errorf("unsupported lifecycle state %q", lifecycle.State)
	}
}

func validateDeprecatedLifecycle(lifecycle climanifest.Lifecycle) error {
	if !lifecycleVersionPattern.MatchString(lifecycle.Deprecated) {
		return fmt.Errorf("deprecated state requires a stable deprecated version")
	}
	if lifecycle.Removed != "" {
		return fmt.Errorf("deprecated state cannot declare a removed version")
	}
	if lifecycle.Successor == nil ||
		strings.TrimSpace(lifecycle.Successor.TargetItemID) == "" ||
		strings.TrimSpace(lifecycle.Successor.CanonicalEnglish) == "" {
		return fmt.Errorf("deprecated state requires complete successor guidance")
	}
	return nil
}

func projectGenericPresentation(cmd *cobra.Command, plan plannedCommand, bindings GenericBindings) error {
	applyLifecycleToCommand(cmd, plan.record.Lifecycle)
	for _, item := range plan.flags {
		if item.record.Scope == "inherited" {
			continue
		}
		flag := cmd.Flag(item.record.Long)
		if flag != nil && item.record.Lifecycle.State == "deprecated" {
			flag.Deprecated = item.record.Lifecycle.Successor.CanonicalEnglish
		}
		if err := projectFlagCompletion(cmd, item.record, bindings); err != nil {
			return genericFlagError(plan.record.ID, item.record.ID, "project completion: %v", err)
		}
	}
	applyArgumentLifecycleHelp(cmd, plan.arguments)
	projectArgumentCompletion(cmd, plan.arguments, bindings)
	return nil
}

func applyLifecycleToCommand(cmd *cobra.Command, lifecycle climanifest.Lifecycle) {
	if lifecycle.State == "deprecated" {
		cmd.Deprecated = lifecycle.Successor.CanonicalEnglish
		cmd.Long += "\n\nDEPRECATED: " + lifecycle.Successor.CanonicalEnglish
	}
}

func applyArgumentLifecycleHelp(cmd *cobra.Command, arguments []climanifest.Argument) {
	for _, argument := range arguments {
		if argument.Lifecycle.State != "deprecated" {
			continue
		}
		cmd.Long += fmt.Sprintf(
			"\n\nDeprecated argument <%s>: %s",
			argument.Name,
			argument.Lifecycle.Successor.CanonicalEnglish,
		)
	}
}

func projectFlagCompletion(cmd *cobra.Command, flag climanifest.Flag, bindings GenericBindings) error {
	completion := completionForInput(flag.ID, flag.Completion, flag.Enum, bindings)
	if completion == nil {
		return nil
	}
	if err := cmd.RegisterFlagCompletionFunc(flag.Long, completion); err != nil {
		return err
	}
	for _, alias := range flag.Aliases {
		if err := cmd.RegisterFlagCompletionFunc(alias, completion); err != nil {
			return err
		}
	}
	return nil
}

func projectArgumentCompletion(cmd *cobra.Command, arguments []climanifest.Argument, bindings GenericBindings) {
	for _, argument := range arguments {
		if argument.Completion != "" && argument.Completion != "none" {
			cmd.ValidArgsFunction = func(
				command *cobra.Command,
				args []string,
				toComplete string,
			) ([]cobra.Completion, cobra.ShellCompDirective) {
				current := completionArgument(arguments, len(args))
				if current == nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				fn := completionForInput(current.ID, current.Completion, current.Enum, bindings)
				if fn == nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				return fn(command, args, toComplete)
			}
			return
		}
	}
}

func completionArgument(arguments []climanifest.Argument, supplied int) *climanifest.Argument {
	counts := argumentValueCounts(arguments, supplied+1)
	offset := 0
	for index, count := range counts {
		if supplied < offset+count {
			return &arguments[index]
		}
		offset += count
	}
	return nil
}

func projectedCommandUsage(record climanifest.Command, arguments []climanifest.Argument) string {
	if len(arguments) == 0 {
		return record.Usage.Line
	}
	fields := []string{record.Name}
	for _, argument := range arguments {
		fields = append(fields, argumentUsageToken(argument))
	}
	return strings.Join(fields, " ")
}

func argumentUsageToken(argument climanifest.Argument) string {
	if argument.MaxCardinality < 0 {
		if argument.MinCardinality > 0 {
			return "<" + argument.Name + "...>"
		}
		return "[" + argument.Name + "...]"
	}
	token := "[" + argument.Name + "]"
	if argument.MinCardinality > 0 {
		token = "<" + argument.Name + ">"
	}
	switch {
	case argument.MaxCardinality > 1 && argument.MinCardinality == argument.MaxCardinality:
		return fmt.Sprintf("%s{%d}", token, argument.MaxCardinality)
	case argument.MaxCardinality > 1:
		return fmt.Sprintf("%s{%d,%d}", token, argument.MinCardinality, argument.MaxCardinality)
	default:
		return token
	}
}

func projectedFlagUsage(flag climanifest.Flag) string {
	if len(flag.Aliases) == 0 {
		return ""
	}
	aliases := make([]string, len(flag.Aliases))
	for index, alias := range flag.Aliases {
		aliases[index] = "--" + alias
	}
	return "aliases: " + strings.Join(aliases, ", ")
}

func completionForInput(
	inputID, mode string,
	choices []string,
	bindings GenericBindings,
) cobra.CompletionFunc {
	switch mode {
	case "static":
		values := append([]string(nil), choices...)
		sort.Strings(values)
		return func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		}
	case "dynamic":
		return bindings.Completions[inputID]
	default:
		return nil
	}
}

func commandExamples(record climanifest.Command) string {
	if strings.TrimSpace(record.Usage.Example) != "" {
		return record.Usage.Example
	}
	return strings.Join(record.Documentation.Examples, "\n")
}

func commandLong(record climanifest.Command) string {
	return record.Documentation.Documentation.Title.CanonicalEnglish + "\n\n" +
		record.Documentation.Documentation.Description.CanonicalEnglish
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// NewDocsCommand builds the independently injected `you docs` command.
func NewDocsCommand(registry *commandregistry.Registry) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	return NewDocsCommandFromManifest(manifest, registry)
}

// NewDocsCommandFromManifest builds `you docs` from authored manifest data.
func NewDocsCommandFromManifest(manifest climanifest.Manifest, registry *commandregistry.Registry) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build docs command: registry is required")
	}
	record, err := manifest.CommandByID("you.docs")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	cmd := commandFromManifest(record, true)
	cmd.SilenceUsage = true
	cmd.Args = positionalArgsFromManifest(record)
	cmd.ValidArgs = docscli.SupportedTopicCommands()
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	return cmd, nil
}
