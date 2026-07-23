package climanifestcobra

import (
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
		genericHandler := bindings.Handlers[handler.ID]
		cobraHandler := bindings.CobraHandlers[handler.ID]
		if genericHandler != nil && cobraHandler != nil {
			return fmt.Errorf(
				"command %q handler ID %q has multiple executable bindings",
				item.record.ID,
				handler.ID,
			)
		}
		if genericHandler == nil && cobraHandler == nil {
			return fmt.Errorf(
				"command %q handler ID %q has no registered executable binding",
				item.record.ID,
				handler.ID,
			)
		}
	}
	return nil
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

func validateArgumentRecordShape(commandID string, argument climanifest.Argument) error {
	compatibility := len(argument.Channels) > 0
	canonical := argument.Scope != "" || len(argument.AcceptedSources) > 0 ||
		argument.HandlerBindingID != "" || argument.Visibility != "" ||
		argument.Lifecycle != (climanifest.Lifecycle{})
	if compatibility == canonical {
		return genericArgumentError(
			commandID,
			argument.ID,
			"must declare exactly one complete compatibility channels record or canonical metadata record",
		)
	}
	if compatibility {
		if argument.DefaultValue != nil {
			return genericArgumentError(commandID, argument.ID, "compatibility record cannot declare canonical defaultValue")
		}
		return validateInputSources(commandID, argument.ID, argument.Channels, isCompatibilityArgumentSource)
	}
	if argument.Scope != "local" {
		return genericArgumentError(commandID, argument.ID, "canonical record requires local scope")
	}
	if strings.TrimSpace(argument.HandlerBindingID) == "" {
		return genericArgumentError(commandID, argument.ID, "canonical record requires handlerBindingId")
	}
	switch argument.Visibility {
	case "visible", "hidden":
	default:
		return genericArgumentError(commandID, argument.ID, "unsupported visibility %q", argument.Visibility)
	}
	if err := validateInputSources(commandID, argument.ID, argument.AcceptedSources, isCanonicalInputSource); err != nil {
		return err
	}
	return validateManifestDefaultSource(commandID, argument.ID, argument.DefaultValue != nil, argument.AcceptedSources)
}

func validateGenericFlagRecordShape(commandID string, flag climanifest.Flag) error {
	if !isCanonicalFlagRecord(flag) {
		return nil
	}
	if flag.Kind != "named" {
		return genericFlagError(commandID, flag.ID, "canonical record requires kind %q", "named")
	}
	if err := validateCanonicalFlagCardinality(commandID, flag); err != nil {
		return err
	}
	if strings.TrimSpace(flag.HandlerBindingID) == "" {
		return genericFlagError(commandID, flag.ID, "canonical record requires handlerBindingId")
	}
	if flag.Default != "" || flag.ChangedDefault || flag.NoOptionDefault != "" || flag.Binding != "" {
		return genericFlagError(commandID, flag.ID, "canonical record cannot mix compatibility default or binding fields")
	}
	if err := validateCanonicalFlagSources(commandID, flag); err != nil {
		return err
	}
	return validateCanonicalFlagDefaultSource(commandID, flag)
}

func isCanonicalFlagRecord(flag climanifest.Flag) bool {
	return flag.Kind != "" || flag.MinCardinality != 0 || flag.MaxCardinality != 0 ||
		flag.DefaultValue != nil || flag.NoOptionValue != nil || len(flag.AcceptedSources) > 0 ||
		flag.HandlerBindingID != ""
}

func validateCanonicalFlagCardinality(commandID string, flag climanifest.Flag) error {
	if flag.MinCardinality < 0 || flag.MaxCardinality < -1 ||
		(flag.MaxCardinality != -1 && flag.MinCardinality > flag.MaxCardinality) {
		return genericFlagError(
			commandID,
			flag.ID,
			"invalid cardinality %d..%d",
			flag.MinCardinality,
			flag.MaxCardinality,
		)
	}
	if flag.Required != (flag.MinCardinality > 0) {
		return genericFlagError(
			commandID,
			flag.ID,
			"required=%t is inconsistent with minimum cardinality %d",
			flag.Required,
			flag.MinCardinality,
		)
	}
	wantMaximum := 1
	if flag.Repeatable {
		wantMaximum = -1
	}
	if flag.MaxCardinality != wantMaximum {
		return genericFlagError(
			commandID,
			flag.ID,
			"maximum cardinality %d is incompatible with repeatable=%t",
			flag.MaxCardinality,
			flag.Repeatable,
		)
	}
	if (flag.Repeatable || flag.MaxCardinality == -1 || flag.MaxCardinality > 1) &&
		flag.ValueType != "stringArray" {
		return genericFlagError(commandID, flag.ID, "repeated or unbounded input must use stringArray value type")
	}
	return nil
}

func validateGenericFlagShape(commandID string, flag climanifest.Flag) error {
	if err := validateGenericFlagRecordShape(commandID, flag); err != nil {
		return err
	}
	switch flag.ValueType {
	case "bool", "string", "int", "int64", "stringArray":
	default:
		return genericFlagError(commandID, flag.ID, "unsupported value type %q", flag.ValueType)
	}
	if flag.Repeatable != (flag.ValueType == "stringArray") {
		return genericFlagError(
			commandID,
			flag.ID,
			"repeatable=%t is incompatible with value type %q",
			flag.Repeatable,
			flag.ValueType,
		)
	}
	if err := validateGenericFlagNormalization(commandID, flag); err != nil {
		return err
	}
	switch flag.Visibility {
	case "", "visible", "hidden":
	default:
		return genericFlagError(commandID, flag.ID, "unsupported visibility %q", flag.Visibility)
	}
	if len(flag.Enum) > 0 && flag.ValueType != "string" && flag.ValueType != "stringArray" {
		return genericFlagError(commandID, flag.ID, "enumerated choices are incompatible with value type %q", flag.ValueType)
	}
	return nil
}

func validateGenericFlagNormalization(commandID string, flag climanifest.Flag) error {
	switch flag.Normalization {
	case "":
	case "lowercase", "trim", "lowercase-trim":
		if flag.ValueType != "string" && flag.ValueType != "stringArray" {
			return genericFlagError(commandID, flag.ID, "normalization %q is incompatible with value type %q", flag.Normalization, flag.ValueType)
		}
	default:
		return genericFlagError(commandID, flag.ID, "unsupported normalization %q", flag.Normalization)
	}
	for _, choice := range flag.Enum {
		if normalizeGenericInput(choice, flag.Normalization) != choice {
			return genericFlagError(commandID, flag.ID, "enumerated choice %q is not in declared normalized form", choice)
		}
	}
	return nil
}

func validateCanonicalFlagSources(commandID string, flag climanifest.Flag) error {
	if len(flag.AcceptedSources) == 0 {
		return genericFlagError(commandID, flag.ID, "canonical record requires acceptedSources")
	}
	seen := make(map[string]bool, len(flag.AcceptedSources))
	for _, source := range flag.AcceptedSources {
		if !isCanonicalInputSource(source) {
			return genericFlagError(commandID, flag.ID, "unsupported input source %q", source)
		}
		if seen[source] {
			return genericFlagError(commandID, flag.ID, "duplicate input source %q", source)
		}
		seen[source] = true
	}
	return nil
}

func validateCanonicalFlagDefaultSource(commandID string, flag climanifest.Flag) error {
	hasSource := false
	for _, source := range flag.AcceptedSources {
		hasSource = hasSource || source == "manifest-default"
	}
	if (flag.DefaultValue != nil) != hasSource {
		return genericFlagError(
			commandID,
			flag.ID,
			"defaultValue presence must exactly match accepted source %q",
			"manifest-default",
		)
	}
	return nil
}

func validateInputSources(
	commandID, inputID string,
	sources []string,
	supported func(string) bool,
) error {
	if len(sources) == 0 {
		return genericArgumentError(commandID, inputID, "input sources are required")
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if !supported(source) {
			return genericArgumentError(commandID, inputID, "unsupported input source %q", source)
		}
		if seen[source] {
			return genericArgumentError(commandID, inputID, "duplicate input source %q", source)
		}
		seen[source] = true
	}
	return nil
}

func isCompatibilityArgumentSource(source string) bool {
	switch source {
	case "cli", "env", "config", "stdin":
		return true
	default:
		return false
	}
}

func isCanonicalInputSource(source string) bool {
	switch source {
	case "cli", "stdin", "environment", "operator-config",
		"manifest-default", "factory-signature-default":
		return true
	default:
		return false
	}
}

func validateManifestDefaultSource(
	commandID, inputID string,
	hasDefault bool,
	sources []string,
) error {
	hasSource := false
	for _, source := range sources {
		hasSource = hasSource || source == "manifest-default"
	}
	if hasDefault != hasSource {
		return genericArgumentError(
			commandID,
			inputID,
			"defaultValue presence must exactly match accepted source %q",
			"manifest-default",
		)
	}
	return nil
}

type canonicalCommandInput struct {
	id        string
	bindingID string
	valueType string
	maximum   int
	accepted  map[string]bool
}

var canonicalSourcePrecedence = []string{
	"cli",
	"stdin",
	"environment",
	"operator-config",
	"manifest-default",
	"factory-signature-default",
}

func validateCanonicalCommandInputs(command climanifest.Command) error {
	all := make(map[string]canonicalCommandInput, len(command.Arguments)+len(command.Flags))
	var canonical []canonicalCommandInput
	add := func(input canonicalCommandInput, isCanonical bool) {
		all[input.id] = input
		if isCanonical {
			canonical = append(canonical, input)
		}
	}
	for _, argument := range command.Arguments {
		add(canonicalInput(
			argument.ID,
			argument.HandlerBindingID,
			argument.ValueType,
			argument.MaxCardinality,
			argument.AcceptedSources,
		), argument.HandlerBindingID != "")
	}
	for _, flag := range command.Flags {
		add(canonicalInput(
			flag.ID,
			flag.HandlerBindingID,
			flag.ValueType,
			flag.MaxCardinality,
			flag.AcceptedSources,
		), isCanonicalFlagRecord(flag))
	}
	if len(canonical) == 0 {
		return nil
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].id < canonical[j].id })
	if err := validateCanonicalPrecedence(command); err != nil {
		return err
	}
	if err := validateCanonicalHandlerBindings(command, all, canonical); err != nil {
		return err
	}
	return validateCanonicalSourceBindings(command, all, canonical)
}

func canonicalInput(id, bindingID, valueType string, maximum int, sources []string) canonicalCommandInput {
	accepted := make(map[string]bool, len(sources))
	for _, source := range sources {
		accepted[source] = true
	}
	return canonicalCommandInput{
		id: id, bindingID: bindingID, valueType: valueType, maximum: maximum, accepted: accepted,
	}
}

func validateCanonicalPrecedence(command climanifest.Command) error {
	if len(command.Precedence.Order) != len(canonicalSourcePrecedence) {
		return fmt.Errorf("command %q canonical input precedence is missing or incomplete", command.ID)
	}
	for index, source := range canonicalSourcePrecedence {
		if command.Precedence.Order[index] != source {
			return fmt.Errorf(
				"command %q canonical input precedence tier %d is %q, want %q",
				command.ID,
				index,
				command.Precedence.Order[index],
				source,
			)
		}
	}
	if command.Precedence.WithinTier.Scalar != "last" ||
		command.Precedence.WithinTier.Repeated != "append" ||
		command.Precedence.AcrossTiers != "replace" ||
		command.Precedence.MultipleBindings != "reject" {
		return fmt.Errorf("command %q canonical input precedence policy is incomplete or unsupported", command.ID)
	}
	return nil
}

func validateCanonicalHandlerBindings(
	command climanifest.Command,
	all map[string]canonicalCommandInput,
	canonical []canonicalCommandInput,
) error {
	for _, key := range sortedKeys(command.HandlerBindings) {
		binding := command.HandlerBindings[key]
		if key != binding.ID || strings.TrimSpace(binding.ID) == "" {
			return fmt.Errorf("command %q handler binding map key %q does not match id %q", command.ID, key, binding.ID)
		}
		if _, exists := all[binding.InputID]; !exists {
			return fmt.Errorf("command %q handler binding %q references unknown input %q", command.ID, key, binding.InputID)
		}
	}
	owners := make(map[string]string, len(canonical))
	for _, input := range canonical {
		if owner, exists := owners[input.bindingID]; exists {
			return fmt.Errorf(
				"command %q handler binding %q is claimed by inputs %q and %q",
				command.ID,
				input.bindingID,
				owner,
				input.id,
			)
		}
		owners[input.bindingID] = input.id
	}
	for _, input := range canonical {
		binding, exists := command.HandlerBindings[input.bindingID]
		if !exists {
			return fmt.Errorf(
				"command %q input %q references unknown handler binding %q",
				command.ID,
				input.id,
				input.bindingID,
			)
		}
		if binding.InputID != input.id {
			return fmt.Errorf(
				"command %q input %q handler binding %q targets input %q",
				command.ID,
				input.id,
				input.bindingID,
				binding.InputID,
			)
		}
	}
	return nil
}

func validateCanonicalSourceBindings(
	command climanifest.Command,
	all map[string]canonicalCommandInput,
	canonical []canonicalCommandInput,
) error {
	bound := make(map[string]map[string]bool)
	routes := make(map[string]string)
	for _, key := range sortedKeys(command.SourceBindings) {
		binding := command.SourceBindings[key]
		input, err := validateCanonicalSourceBinding(command.ID, key, binding, all)
		if err != nil {
			return err
		}
		route := binding.Source + ":" + binding.ExternalKey
		if owner, exists := routes[route]; exists {
			return fmt.Errorf("command %q source route %q targets bindings %q and %q", command.ID, route, owner, key)
		}
		routes[route] = key
		if bound[input.id] == nil {
			bound[input.id] = make(map[string]bool)
		}
		if bound[input.id][binding.Source] {
			return fmt.Errorf(
				"command %q input %q has multiple bindings for source %q",
				command.ID,
				input.id,
				binding.Source,
			)
		}
		bound[input.id][binding.Source] = true
	}
	for _, input := range canonical {
		for _, source := range []string{"stdin", "environment", "operator-config"} {
			if input.accepted[source] && !bound[input.id][source] {
				return fmt.Errorf(
					"command %q input %q accepts source %q without a source binding",
					command.ID,
					input.id,
					source,
				)
			}
		}
	}
	return nil
}

func isExternalInputSource(source string) bool {
	return source == "stdin" || source == "environment" || source == "operator-config"
}

func validateRelationshipSet(commandID string, relationships []plannedRelationship) error {
	seen := make(map[string]string, len(relationships))
	adjacency := make(map[string][]string)
	for index, relationship := range relationships {
		signature := relationship.record.Kind + ":" + relationshipSignature(relationship)
		if owner, exists := seen[signature]; exists {
			return fmt.Errorf(
				"command %q relationship %q duplicates equivalent relationship %q",
				commandID,
				relationship.record.ID,
				owner,
			)
		}
		seen[signature] = relationship.record.ID
		for prior := 0; prior < index; prior++ {
			if relationshipsContradict(relationships[prior], relationship) {
				return fmt.Errorf(
					"command %q relationship %q contradicts relationship %q",
					commandID,
					relationship.record.ID,
					relationships[prior].record.ID,
				)
			}
		}
		if relationshipMode(relationship.record.Kind) == "directed" {
			for _, participant := range relationship.participants {
				adjacency[relationship.when.identity] = append(
					adjacency[relationship.when.identity],
					participant.identity,
				)
			}
		}
	}
	for from, targets := range adjacency {
		for _, target := range targets {
			if relationshipReachable(target, from, adjacency, make(map[string]bool)) {
				return fmt.Errorf("command %q relationship dependency graph contains a cycle", commandID)
			}
		}
	}
	return nil
}

func relationshipSignature(relationship plannedRelationship) string {
	participants := make([]string, len(relationship.participants))
	for index, participant := range relationship.participants {
		participants[index] = participant.kind + ":" + participant.identity
	}
	sort.Strings(participants)
	trigger := ""
	if relationship.when != nil {
		trigger = relationship.when.kind + ":" + relationship.when.identity + "->"
	}
	return trigger + strings.Join(participants, ",")
}

func relationshipsContradict(left, right plannedRelationship) bool {
	if relationshipGroupsContradict(left, right) {
		return true
	}
	for _, pair := range [][2]plannedRelationship{{left, right}, {right, left}} {
		directed, excluded := pair[0], pair[1]
		if relationshipMode(directed.record.Kind) != "directed" ||
			!relationshipIsExclusion(excluded.record.Kind) {
			continue
		}
		ids := make(map[string]bool, len(excluded.participants))
		for _, participant := range excluded.participants {
			ids[participant.identity] = true
		}
		for _, participant := range directed.participants {
			if ids[directed.when.identity] && ids[participant.identity] {
				return true
			}
		}
	}
	return false
}

func relationshipGroupsContradict(left, right plannedRelationship) bool {
	if relationshipSignature(left) != relationshipSignature(right) {
		return false
	}
	return left.record.Kind == "required-together" && relationshipIsExclusion(right.record.Kind) ||
		right.record.Kind == "required-together" && relationshipIsExclusion(left.record.Kind)
}

func relationshipIsExclusion(kind string) bool {
	return kind == "mutually-exclusive" || kind == "conflict"
}

func relationshipReachable(current, target string, adjacency map[string][]string, visited map[string]bool) bool {
	if current == target {
		return true
	}
	if visited[current] {
		return false
	}
	visited[current] = true
	for _, next := range adjacency[current] {
		if relationshipReachable(next, target, adjacency, visited) {
			return true
		}
	}
	return false
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
		if argument.Visibility == "hidden" {
			continue
		}
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
	usage := flag.Usage
	if len(flag.Aliases) == 0 {
		return usage
	}
	aliases := make([]string, len(flag.Aliases))
	for index, alias := range flag.Aliases {
		aliases[index] = "--" + alias
	}
	aliasUsage := "aliases: " + strings.Join(aliases, ", ")
	if usage == "" {
		return aliasUsage
	}
	return usage + " (" + aliasUsage + ")"
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
	title := record.Documentation.Documentation.Title.CanonicalEnglish
	description := record.Documentation.Documentation.Description.CanonicalEnglish
	if strings.HasPrefix(description, title) {
		return description
	}
	return title + "\n\n" + description
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
