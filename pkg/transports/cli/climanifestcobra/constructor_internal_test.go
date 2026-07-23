package climanifestcobra

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestGenericConstructorRejectsSiblingAliasCollisionBeforeDispatch(t *testing.T) {
	manifest := feedbackManifest()
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Aliases = []string{"zeta"}
	manifest.Commands[alpha.ID] = alpha
	calls := 0
	bindings := feedbackBindings(manifest, func(context.Context, map[string]any) error {
		calls++
		return nil
	})

	root, err := NewCommandTree(manifest, bindings)
	if root != nil || err == nil || !strings.Contains(err.Error(), `name or alias "zeta" conflicts with sibling`) {
		t.Fatalf("NewCommandTree() = (%v, %v), want nil sibling collision error", root, err)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}
}

func TestGenericCompletionReservesLaterRequiredArgumentLikeParser(t *testing.T) {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Arguments = mixedCompletionArguments("static", "dynamic")
	manifest.Commands[alpha.ID] = alpha
	var received map[string]any
	dynamicCalls := 0
	bindings := feedbackBindings(manifest, func(_ context.Context, values map[string]any) error {
		received = values
		return nil
	})
	bindings.Completions = CompletionRegistry{
		"feedback.alpha.arg.required": func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			dynamicCalls++
			return []cobra.Completion{"required-dynamic"}, cobra.ShellCompDirectiveNoFileComp
		},
	}
	root, err := NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	alphaCommand := root.Commands()[0]
	values, _ := alphaCommand.ValidArgsFunction(alphaCommand, nil, "")
	if len(values) != 1 || values[0] != "required-dynamic" || dynamicCalls != 1 {
		t.Fatalf("first completion = %v calls=%d, want required dynamic input", values, dynamicCalls)
	}
	root.SetArgs([]string{"alpha", "required-static"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received["feedback.alpha.arg.optional"] != nil ||
		received["feedback.alpha.arg.required"] != "required-static" {
		t.Fatalf("parsed values = %#v, want one token assigned to required input", received)
	}

	alpha.Arguments = mixedCompletionArguments("dynamic", "static")
	manifest.Commands[alpha.ID] = alpha
	bindings = feedbackBindings(manifest, func(context.Context, map[string]any) error { return nil })
	bindings.Completions = CompletionRegistry{
		"feedback.alpha.arg.optional": func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return []cobra.Completion{"optional-dynamic"}, cobra.ShellCompDirectiveNoFileComp
		},
	}
	root, err = NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree(static required) error = %v", err)
	}
	alphaCommand = root.Commands()[0]
	values, _ = alphaCommand.ValidArgsFunction(alphaCommand, nil, "")
	if len(values) != 1 || values[0] != "required-static" {
		t.Fatalf("first completion = %v, want required static input", values)
	}
}

func TestGenericHelpDerivesCardinalityAndExposesFlagAliases(t *testing.T) {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Usage.Line = "alpha"
	alpha.Arguments = map[string]climanifest.Argument{
		"feedback.alpha.arg.target": {
			ID: "feedback.alpha.arg.target", Name: "target", Position: 0, Kind: "positional",
			ValueType: "string", Required: true, MinCardinality: 1, MaxCardinality: 1, Completion: "none",
		},
		"feedback.alpha.arg.pair": {
			ID: "feedback.alpha.arg.pair", Name: "pair", Position: 1, Kind: "positional",
			ValueType: "stringArray", Required: true, MinCardinality: 2, MaxCardinality: 2, Completion: "none",
		},
		"feedback.alpha.arg.extra": {
			ID: "feedback.alpha.arg.extra", Name: "extra", Position: 2, Kind: "positional",
			ValueType: "stringArray", MaxCardinality: -1, Variadic: true, Completion: "none",
		},
	}
	completeFeedbackArguments(alpha.Arguments)
	alpha.Flags = map[string]climanifest.Flag{
		"feedback.alpha.flag.cluster": {
			ID: "feedback.alpha.flag.cluster", Long: "cluster", Aliases: []string{"compute-cluster"},
			Scope: "local", ValueType: "string", Completion: "none", Visibility: "visible",
			Lifecycle: feedbackLifecycle("feedback.alpha.flag.cluster"),
		},
	}
	manifest.Commands[alpha.ID] = alpha
	root, err := NewCommandTree(manifest, feedbackBindings(
		manifest,
		func(context.Context, map[string]any) error { return nil },
	))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	alphaCommand := root.Commands()[0]
	const wantUse = "alpha <target> <pair>{2} [extra...]"
	if alphaCommand.Use != wantUse {
		t.Fatalf("command use = %q, want %q", alphaCommand.Use, wantUse)
	}
	var output bytes.Buffer
	alphaCommand.SetOut(&output)
	if err := alphaCommand.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}
	help := output.String()
	if !strings.Contains(help, wantUse) || !strings.Contains(help, "aliases: --compute-cluster") {
		t.Fatalf("help does not project cardinality and flag alias:\n%s", help)
	}
}

func TestGenericArgumentsSupportCobraDoubleDashTermination(t *testing.T) {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Arguments = map[string]climanifest.Argument{
		"feedback.alpha.arg.value": compatibilityFeedbackArgument(
			"feedback.alpha.arg.value",
			"value",
			0,
		),
	}
	manifest.Commands[alpha.ID] = alpha
	var received map[string]any
	root, err := NewCommandTree(manifest, feedbackBindings(
		manifest,
		func(_ context.Context, values map[string]any) error {
			received = values
			return nil
		},
	))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{"alpha", "--", "--literal"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received["feedback.alpha.arg.value"] != "--literal" {
		t.Fatalf("handler values = %#v, want bare -- to terminate flags", received)
	}
}

func TestGenericArgumentsRejectInvalidDoubleDashModesBeforeDispatch(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "missing"},
		{name: "known but unsupported", mode: "none"},
		{name: "unknown", mode: "passthrough"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := feedbackManifestWithArgument(
				compatibilityFeedbackArgument("feedback.alpha.arg.value", "value", 0),
			)
			updateFeedbackArgument(&manifest, func(argument *climanifest.Argument) {
				argument.DoubleDash = test.mode
			})
			assertFeedbackConstructionFailure(t, manifest, "doubleDash")
		})
	}
}

func TestGenericHiddenArgumentParsesWithoutAppearingInHelp(t *testing.T) {
	visible := canonicalFeedbackArgument("feedback.alpha.arg.visible", "visible", 0, "visible")
	hidden := canonicalFeedbackArgument("feedback.alpha.arg.secret", "secret", 1, "hidden")
	hidden.Required = true
	hidden.MinCardinality = 1
	manifest := feedbackManifestWithArguments(visible, hidden)
	var received map[string]any
	root, err := NewCommandTree(manifest, feedbackBindings(
		manifest,
		func(_ context.Context, values map[string]any) error {
			received = values
			return nil
		},
	))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	alpha := root.Commands()[0]
	if alpha.Use != "alpha [visible]" {
		t.Fatalf("command use = %q, want hidden argument omitted", alpha.Use)
	}
	var output bytes.Buffer
	alpha.SetOut(&output)
	if err := alpha.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("help exposes hidden positional input:\n%s", output.String())
	}
	root.SetArgs([]string{"alpha", "classified"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received["feedback.alpha.arg.secret"] != "classified" {
		t.Fatalf("handler values = %#v, want hidden argument parsed", received)
	}
}

func TestGenericArgumentsRejectIncompleteCanonicalRecordsBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Argument)
	}{
		{name: "incomplete shape", mutate: func(argument *climanifest.Argument) {
			argument.Channels = nil
		}},
		{name: "invalid visibility", mutate: func(argument *climanifest.Argument) {
			*argument = canonicalFeedbackArgument(argument.ID, argument.Name, argument.Position, "internal")
		}},
		{name: "default without source", mutate: func(argument *climanifest.Argument) {
			*argument = canonicalFeedbackArgument(argument.ID, argument.Name, argument.Position, "visible")
			value := "fallback"
			argument.DefaultValue = &climanifest.InputValue{String: &value}
		}},
		{name: "source without default", mutate: func(argument *climanifest.Argument) {
			*argument = canonicalFeedbackArgument(argument.ID, argument.Name, argument.Position, "visible")
			argument.AcceptedSources = append(argument.AcceptedSources, "manifest-default")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := feedbackManifestWithArgument(
				compatibilityFeedbackArgument("feedback.alpha.arg.value", "value", 0),
			)
			updateFeedbackArgument(&manifest, test.mutate)
			assertFeedbackConstructionFailure(t, manifest, "argument")
		})
	}
}

func TestGenericFlagsRejectIncompleteCanonicalRecordsBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Flag)
	}{
		{name: "unsupported kind", mutate: func(flag *climanifest.Flag) {
			flag.Kind = "unsupported"
		}},
		{name: "missing handler binding", mutate: func(flag *climanifest.Flag) {
			flag.HandlerBindingID = ""
		}},
		{name: "default without source", mutate: func(flag *climanifest.Flag) {
			value := "fallback"
			flag.DefaultValue = &climanifest.InputValue{String: &value}
		}},
		{name: "source without default", mutate: func(flag *climanifest.Flag) {
			flag.AcceptedSources = append(flag.AcceptedSources, "manifest-default")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := feedbackManifest()
			delete(manifest.Commands, "feedback.zeta")
			alpha := manifest.Commands["feedback.alpha"]
			flag := canonicalFeedbackFlag()
			alpha.Flags = map[string]climanifest.Flag{flag.ID: flag}
			completeFeedbackCanonicalCommandContract(&alpha)
			test.mutate(&flag)
			alpha.Flags[flag.ID] = flag
			manifest.Commands[alpha.ID] = alpha
			assertFeedbackConstructionFailure(t, manifest, "input")
		})
	}
}

func TestCanonicalInputTablesRejectIncompleteBindingsAndPrecedenceBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Command)
		want   string
	}{
		{name: "unknown handler binding", mutate: func(command *climanifest.Command) {
			delete(command.HandlerBindings, "feedback.alpha.arg.value.binding")
		}, want: "unknown handler binding"},
		{name: "mismatched handler target", mutate: func(command *climanifest.Command) {
			other := compatibilityFeedbackArgument("feedback.alpha.arg.other", "other", 1)
			command.Arguments[other.ID] = other
			binding := command.HandlerBindings["feedback.alpha.arg.value.binding"]
			binding.InputID = other.ID
			command.HandlerBindings[binding.ID] = binding
		}, want: "targets input"},
		{name: "duplicate handler binding", mutate: func(command *climanifest.Command) {
			other := canonicalFeedbackArgument("feedback.alpha.arg.other", "other", 1, "visible")
			other.HandlerBindingID = "feedback.alpha.arg.value.binding"
			command.Arguments[other.ID] = other
		}, want: "claimed by inputs"},
		{name: "missing source binding", mutate: func(command *climanifest.Command) {
			delete(command.SourceBindings, "feedback.alpha.arg.value.source.environment")
		}, want: "without a source binding"},
		{name: "mismatched source binding", mutate: func(command *climanifest.Command) {
			binding := command.SourceBindings["feedback.alpha.arg.value.source.environment"]
			binding.Source = "operator-config"
			command.SourceBindings[binding.ID] = binding
		}, want: "is not accepted"},
		{name: "missing precedence", mutate: func(command *climanifest.Command) {
			command.Precedence = climanifest.Precedence{}
		}, want: "precedence is missing or incomplete"},
		{name: "incomplete precedence", mutate: func(command *climanifest.Command) {
			command.Precedence.WithinTier.Repeated = ""
		}, want: "precedence policy is incomplete"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			argument := canonicalFeedbackArgument("feedback.alpha.arg.value", "value", 0, "visible")
			argument.AcceptedSources = []string{"cli", "environment"}
			manifest := feedbackManifestWithArgument(argument)
			alpha := manifest.Commands["feedback.alpha"]
			test.mutate(&alpha)
			manifest.Commands[alpha.ID] = alpha
			assertFeedbackConstructionFailure(t, manifest, test.want)
		})
	}
}

func TestCanonicalFlagNormalizationDispatchesDefaultsAndExplicitValues(t *testing.T) {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	worker, stable, base := "worker", "stable", []string{"base"}
	alpha.Flags = map[string]climanifest.Flag{
		"feedback.alpha.flag.name": canonicalNormalizedFlag(
			"feedback.alpha.flag.name", "name", "string", "lowercase-trim",
			&climanifest.InputValue{String: &worker}, []string{"worker"},
		),
		"feedback.alpha.flag.code": canonicalNormalizedFlag(
			"feedback.alpha.flag.code", "code", "string", "lowercase",
			&climanifest.InputValue{String: &stable}, []string{"stable", "mixed"},
		),
		"feedback.alpha.flag.tag": canonicalNormalizedFlag(
			"feedback.alpha.flag.tag", "tag", "stringArray", "lowercase-trim",
			&climanifest.InputValue{StringArray: &base}, []string{"base", "alpha", "beta"},
		),
	}
	completeFeedbackCanonicalCommandContract(&alpha)
	manifest.Commands[alpha.ID] = alpha

	var received map[string]any
	build := func() *cobra.Command {
		root, err := NewCommandTree(manifest, feedbackBindings(
			manifest,
			func(_ context.Context, values map[string]any) error {
				received = values
				return nil
			},
		))
		if err != nil {
			t.Fatalf("NewCommandTree() error = %v", err)
		}
		return root
	}
	root := build()
	root.SetArgs([]string{"alpha"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(defaults) error = %v", err)
	}
	if received["feedback.alpha.flag.name"] != "worker" ||
		received["feedback.alpha.flag.code"] != "stable" ||
		!reflect.DeepEqual(received["feedback.alpha.flag.tag"], []string{"base"}) {
		t.Fatalf("default values = %#v, want normalized typed defaults", received)
	}

	root = build()
	root.SetArgs([]string{"alpha", "--name", " WoRkEr ", "--code", "MiXeD", "--tag", " ALPHA ", "--tag", " BeTa "})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(explicit) error = %v", err)
	}
	if received["feedback.alpha.flag.name"] != "worker" ||
		received["feedback.alpha.flag.code"] != "mixed" ||
		!reflect.DeepEqual(received["feedback.alpha.flag.tag"], []string{"alpha", "beta"}) {
		t.Fatalf("explicit values = %#v, want lowercase/trim normalization", received)
	}
}

func TestRelationshipSetsRejectDuplicatesContradictionsAndCyclesBeforeDispatch(t *testing.T) {
	tests := []struct {
		name          string
		relationships map[string]climanifest.Relationship
		want          string
	}{
		{name: "duplicate", relationships: map[string]climanifest.Relationship{
			"rel.one": relationship("rel.one", "mutually-exclusive", "flag.alpha", "flag.beta"),
			"rel.two": relationship("rel.two", "mutually-exclusive", "flag.beta", "flag.alpha"),
		}, want: "duplicates equivalent relationship"},
		{name: "contradictory", relationships: map[string]climanifest.Relationship{
			"rel.one": relationship("rel.one", "required-together", "flag.alpha", "flag.beta"),
			"rel.two": relationship("rel.two", "conflict", "flag.alpha", "flag.beta"),
		}, want: "contradicts relationship"},
		{name: "cycle", relationships: map[string]climanifest.Relationship{
			"rel.one": directedFeedbackRelationship("rel.one", "flag.alpha", "flag.beta"),
			"rel.two": directedFeedbackRelationship("rel.two", "flag.beta", "flag.alpha"),
		}, want: "contains a cycle"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := feedbackRelationshipManifest(test.relationships)
			assertFeedbackConstructionFailure(t, manifest, test.want)
		})
	}
}

func mixedCompletionArguments(optionalMode, requiredMode string) map[string]climanifest.Argument {
	arguments := map[string]climanifest.Argument{
		"feedback.alpha.arg.optional": {
			ID: "feedback.alpha.arg.optional", Name: "optional", Position: 0, Kind: "positional",
			ValueType: "string", MaxCardinality: 1, Enum: []string{"optional-static"}, Completion: optionalMode,
		},
		"feedback.alpha.arg.required": {
			ID: "feedback.alpha.arg.required", Name: "required", Position: 1, Kind: "positional",
			ValueType: "string", Required: true, MinCardinality: 1, MaxCardinality: 1,
			Enum: []string{"required-static"}, Completion: requiredMode,
		},
	}
	completeFeedbackArguments(arguments)
	return arguments
}

func compatibilityFeedbackArgument(id, name string, position int) climanifest.Argument {
	return climanifest.Argument{
		ID: id, Name: name, Position: position, Kind: "positional",
		ValueType: "string", MaxCardinality: 1, Completion: "none",
		DoubleDash: "terminates-flags", Channels: []string{"cli"},
	}
}

func canonicalFeedbackArgument(id, name string, position int, visibility string) climanifest.Argument {
	argument := compatibilityFeedbackArgument(id, name, position)
	argument.Channels = nil
	argument.Scope = "local"
	argument.AcceptedSources = []string{"cli"}
	argument.HandlerBindingID = id + ".binding"
	argument.Visibility = visibility
	argument.Lifecycle = feedbackLifecycle(id)
	return argument
}

func canonicalFeedbackFlag() climanifest.Flag {
	const id = "feedback.alpha.flag.value"
	return climanifest.Flag{
		ID: id, Kind: "named", Long: "value", Scope: "local", ValueType: "string",
		MaxCardinality: 1, Completion: "none", AcceptedSources: []string{"cli"},
		HandlerBindingID: id + ".binding", Visibility: "visible", Lifecycle: feedbackLifecycle(id),
	}
}

func canonicalNormalizedFlag(
	id, long, valueType, normalization string,
	defaultValue *climanifest.InputValue,
	enum []string,
) climanifest.Flag {
	repeatable := valueType == "stringArray"
	maximum := 1
	if repeatable {
		maximum = -1
	}
	return climanifest.Flag{
		ID: id, Kind: "named", Long: long, Scope: "local", ValueType: valueType,
		MaxCardinality: maximum, DefaultValue: defaultValue, Repeatable: repeatable,
		Normalization: normalization, Completion: "none", Enum: enum,
		AcceptedSources:  []string{"cli", "manifest-default"},
		HandlerBindingID: id + ".binding", Visibility: "visible", Lifecycle: feedbackLifecycle(id),
	}
}

func feedbackRelationshipManifest(relationships map[string]climanifest.Relationship) climanifest.Manifest {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Flags = map[string]climanifest.Flag{
		"flag.alpha": {
			ID: "flag.alpha", Long: "alpha", Scope: "local", ValueType: "bool",
			NoOptionDefault: "true", Completion: "none", Visibility: "visible",
			Lifecycle: feedbackLifecycle("flag.alpha"),
		},
		"flag.beta": {
			ID: "flag.beta", Long: "beta", Scope: "local", ValueType: "bool",
			NoOptionDefault: "true", Completion: "none", Visibility: "visible",
			Lifecycle: feedbackLifecycle("flag.beta"),
		},
	}
	alpha.Relationships = relationships
	manifest.Commands[alpha.ID] = alpha
	return manifest
}

func relationship(id, kind string, inputIDs ...string) climanifest.Relationship {
	participants := make([]climanifest.ParticipantRef, len(inputIDs))
	for index, inputID := range inputIDs {
		participants[index] = climanifest.ParticipantRef{Type: "flag", ID: inputID}
	}
	return climanifest.Relationship{ID: id, Kind: kind, Participants: participants}
}

func directedFeedbackRelationship(id, trigger, target string) climanifest.Relationship {
	relationship := relationship(id, "dependency", target)
	relationship.When = &climanifest.ParticipantRef{Type: "flag", ID: trigger}
	return relationship
}

func feedbackManifestWithArgument(argument climanifest.Argument) climanifest.Manifest {
	return feedbackManifestWithArguments(argument)
}

func feedbackManifestWithArguments(arguments ...climanifest.Argument) climanifest.Manifest {
	manifest := feedbackManifest()
	delete(manifest.Commands, "feedback.zeta")
	alpha := manifest.Commands["feedback.alpha"]
	alpha.Arguments = make(map[string]climanifest.Argument, len(arguments))
	for _, argument := range arguments {
		alpha.Arguments[argument.ID] = argument
	}
	completeFeedbackCanonicalCommandContract(&alpha)
	manifest.Commands[alpha.ID] = alpha
	return manifest
}

func completeFeedbackCanonicalCommandContract(command *climanifest.Command) {
	command.HandlerBindings = make(map[string]climanifest.HandlerBinding)
	command.SourceBindings = make(map[string]climanifest.SourceBinding)
	add := func(id, bindingID string, sources []string) {
		if bindingID == "" {
			return
		}
		command.HandlerBindings[bindingID] = climanifest.HandlerBinding{ID: bindingID, InputID: id}
		for _, source := range sources {
			if source != "environment" {
				continue
			}
			sourceID := id + ".source.environment"
			command.SourceBindings[sourceID] = climanifest.SourceBinding{
				ID: sourceID, Source: source, ExternalKey: strings.ToUpper(id), InputID: id,
			}
		}
	}
	for _, argument := range command.Arguments {
		add(argument.ID, argument.HandlerBindingID, argument.AcceptedSources)
	}
	for _, flag := range command.Flags {
		add(flag.ID, flag.HandlerBindingID, flag.AcceptedSources)
	}
	command.Precedence = climanifest.Precedence{
		Order: []string{
			"cli", "stdin", "environment", "operator-config",
			"manifest-default", "factory-signature-default",
		},
		WithinTier:       climanifest.WithinTierRule{Scalar: "last", Repeated: "append"},
		AcrossTiers:      "replace",
		MultipleBindings: "reject",
	}
}

func updateFeedbackArgument(manifest *climanifest.Manifest, update func(*climanifest.Argument)) {
	alpha := manifest.Commands["feedback.alpha"]
	for id, argument := range alpha.Arguments {
		update(&argument)
		alpha.Arguments[id] = argument
		break
	}
	manifest.Commands[alpha.ID] = alpha
}

func assertFeedbackConstructionFailure(t *testing.T, manifest climanifest.Manifest, want string) {
	t.Helper()
	calls := 0
	root, err := NewCommandTree(manifest, feedbackBindings(
		manifest,
		func(context.Context, map[string]any) error {
			calls++
			return nil
		},
	))
	if root != nil || err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("NewCommandTree() = (%v, %v), want nil error containing %q", root, err, want)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}
}

func completeFeedbackArguments(arguments map[string]climanifest.Argument) {
	for id, argument := range arguments {
		argument.DoubleDash = "terminates-flags"
		argument.Channels = []string{"cli"}
		arguments[id] = argument
	}
}

func feedbackManifest() climanifest.Manifest {
	return climanifest.Manifest{
		FormatVersion: "1.0.0",
		RootPath:      "feedback",
		Commands: map[string]climanifest.Command{
			"feedback.root":  feedbackCommand("feedback.root", "feedback", "feedback", false),
			"feedback.alpha": feedbackCommand("feedback.alpha", "alpha", "feedback alpha", true),
			"feedback.zeta":  feedbackCommand("feedback.zeta", "zeta", "feedback zeta", true),
		},
	}
}

func feedbackCommand(id, name, path string, runnable bool) climanifest.Command {
	command := climanifest.Command{
		ID: id, Name: name, Path: path, Visibility: "visible", Runnable: runnable,
		Usage: climanifest.Usage{Line: name},
		Documentation: climanifest.Documentation{Documentation: climanifest.DocumentationCopy{
			Title:       climanifest.DocumentationField{CanonicalEnglish: name + " title"},
			Description: climanifest.DocumentationField{CanonicalEnglish: name + " description"},
		}},
		Lifecycle: feedbackLifecycle(id),
	}
	if runnable {
		command.Handler = &climanifest.Handler{ID: id + ".handler"}
	}
	return command
}

func feedbackLifecycle(id string) climanifest.Lifecycle {
	return climanifest.Lifecycle{
		FormatVersion: "1.0.0",
		ItemID:        id,
		State:         "active",
		Since:         "1.0.0",
	}
}

func feedbackBindings(manifest climanifest.Manifest, handler GenericHandler) GenericBindings {
	bindings := GenericBindings{Handlers: HandlerRegistry{}}
	for _, command := range manifest.Commands {
		if command.Handler != nil {
			bindings.Handlers[command.Handler.ID] = handler
		}
	}
	return bindings
}

func TestLocalBindingTargetParsesIntDefaults(t *testing.T) {
	target, err := localBindingTarget(climanifest.Flag{
		Long:      "count",
		ValueType: "int",
		Default:   "3",
	})
	if err != nil {
		t.Fatalf("localBindingTarget() error = %v", err)
	}
	if target.intValue == nil || *target.intValue != 3 {
		t.Fatalf("int binding = %#v, want 3", target.intValue)
	}
}

func TestSessionLocalBindingTargetsRejectInheritedJSON(t *testing.T) {
	flag := climanifest.Flag{Long: "json"}
	tests := []struct {
		name   string
		target func() error
	}{
		{
			name: "create",
			target: func() error {
				_, err := sessionCreateFlagTarget(flag, &sessioncli.CreateConfig{})
				return err
			},
		},
		{
			name: "list",
			target: func() error {
				_, err := sessionListFlagTarget(flag, &sessioncli.ListConfig{})
				return err
			},
		},
		{
			name: "delete",
			target: func() error {
				_, err := sessionDeleteFlagTarget(flag, &sessioncli.DeleteConfig{})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.target(); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("local JSON target error = %v, want unsupported inherited flag", err)
			}
		})
	}
}

func TestRegisterFlagSupportsBoolStringAndInt(t *testing.T) {
	t.Run("bool shorthand", func(t *testing.T) {
		var value bool
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "verbose",
			Shorthand: "v",
			ValueType: "bool",
			Default:   "true",
		}, flagTarget{boolValue: &value}, "verbose help"); err != nil {
			t.Fatalf("registerFlag(bool) error = %v", err)
		}
		if err := flags.Set("verbose", "true"); err != nil {
			t.Fatalf("Set(verbose) error = %v", err)
		}
		if !value {
			t.Fatal("bool flag did not bind")
		}
	})

	t.Run("string", func(t *testing.T) {
		var value string
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "server",
			ValueType: "string",
			Default:   "http://localhost:7437",
		}, flagTarget{stringValue: &value}, "server help"); err != nil {
			t.Fatalf("registerFlag(string) error = %v", err)
		}
		if value != "http://localhost:7437" {
			t.Fatalf("string default = %q", value)
		}
	})

	t.Run("int", func(t *testing.T) {
		var value int
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if err := registerFlag(flags, climanifest.Flag{
			Long:      "retries",
			ValueType: "int",
			Default:   "2",
		}, flagTarget{intValue: &value}, "retries help"); err != nil {
			t.Fatalf("registerFlag(int) error = %v", err)
		}
		if value != 2 {
			t.Fatalf("int default = %d, want 2", value)
		}
	})
}

func TestPositionalArgsFromManifestCoversCardinalityModes(t *testing.T) {
	exact := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, MaxCardinality: 1},
		},
	})
	cmd := &cobra.Command{Use: "exact", Args: exact}
	if err := cmd.Args(cmd, []string{"one"}); err != nil {
		t.Fatalf("exact args error = %v", err)
	}

	variadic := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, Variadic: true},
		},
	})
	cmd = &cobra.Command{Use: "variadic", Args: variadic}
	if err := cmd.Args(cmd, []string{"one", "two"}); err != nil {
		t.Fatalf("variadic args error = %v", err)
	}

	maxOnly := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MaxCardinality: 2},
		},
	})
	cmd = &cobra.Command{Use: "max", Args: maxOnly}
	if err := cmd.Args(cmd, []string{"one", "two"}); err != nil {
		t.Fatalf("max args error = %v", err)
	}

	twoRequired := positionalArgsFromManifest(climanifest.Command{
		Arguments: map[string]climanifest.Argument{
			"arg0": {Position: 0, MinCardinality: 1, MaxCardinality: 1},
			"arg1": {Position: 1, MinCardinality: 1, MaxCardinality: 1},
		},
	})
	cmd = &cobra.Command{Use: "move", Args: twoRequired}
	if err := cmd.Args(cmd, []string{"work-1", "ready"}); err != nil {
		t.Fatalf("two required args error = %v", err)
	}
	if err := cmd.Args(cmd, []string{"work-1"}); err == nil {
		t.Fatal("two required args accepted one positional, want rejection")
	}
}

func TestRegisterManifestLocalFlagsRejectsUnsupportedValueType(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	err := registerManifestLocalFlags(cmd, climanifest.Command{
		ID: "you.models.list",
		Flags: map[string]climanifest.Flag{
			"you.models.list.flag.foo": {
				Long:      "foo",
				Scope:     "local",
				ValueType: "duration",
			},
		},
	})
	if err == nil {
		t.Fatal("registerManifestLocalFlags() unsupported type = nil, want error")
	}
}

func TestBuildCommandFromRecordAppliesHiddenVisibility(t *testing.T) {
	cmd, err := buildCommandFromRecord(climanifest.Command{
		ID:         "you.session.show",
		Usage:      climanifest.Usage{Line: "show"},
		Visibility: "hidden",
		Documentation: climanifest.Documentation{
			Documentation: climanifest.DocumentationCopy{
				Title:       climanifest.DocumentationField{CanonicalEnglish: "title"},
				Description: climanifest.DocumentationField{CanonicalEnglish: "description"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildCommandFromRecord() error = %v", err)
	}
	if !cmd.Hidden {
		t.Fatal("hidden command must set cmd.Hidden")
	}
}

func TestApplyFlagContractSetsHiddenAndNoOptDefault(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "json")
	if err := applyFlagContract(flags.Lookup("json"), climanifest.Flag{
		Long:            "json",
		Visibility:      "hidden",
		NoOptionDefault: "true",
	}); err != nil {
		t.Fatalf("applyFlagContract() error = %v", err)
	}
	flag := flags.Lookup("json")
	if flag == nil || !flag.Hidden || flag.NoOptDefVal != "true" {
		t.Fatalf("flag = %#v, want hidden with no-opt default", flag)
	}
}

func TestRegisterLocalFlagsRegistersDeprecatedPort(t *testing.T) {
	cmd := &cobra.Command{Use: "show"}
	if err := registerLocalFlags(cmd, climanifest.Command{
		ID: "you.session.show",
		Flags: map[string]climanifest.Flag{
			"you.session.show.flag.port": {
				Long:       "port",
				Scope:      "local",
				ValueType:  "int",
				Default:    "0",
				Visibility: "hidden",
			},
		},
	}, PersistentFlagBindings{}); err != nil {
		t.Fatalf("registerLocalFlags() error = %v", err)
	}
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil || !portFlag.Hidden {
		t.Fatalf("port flag = %#v, want hidden deprecated local flag", portFlag)
	}
}

func TestRegisterPersistentFlagsRegistersRootBindings(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	bindings := PersistentFlagBindings{
		Verbose:                    boolPtr(false),
		Debug:                      boolPtr(false),
		Server:                     stringPtr("http://localhost:7437"),
		JSON:                       boolPtr(false),
		DefaultWorkerModelProvider: stringPtr(""),
		DefaultWorkerModel:         stringPtr(""),
	}
	if err := registerPersistentFlags(root, climanifest.Command{
		ID: "you",
		Flags: map[string]climanifest.Flag{
			"you.flag.verbose": {Long: "verbose", Shorthand: "v", Scope: "persistent", ValueType: "bool", Default: "false"},
			"you.flag.server":  {Long: "server", Scope: "persistent", ValueType: "string", Default: "http://localhost:7437"},
		},
	}, bindings); err != nil {
		t.Fatalf("registerPersistentFlags() error = %v", err)
	}
	if root.PersistentFlags().Lookup("verbose") == nil || root.PersistentFlags().Lookup("server") == nil {
		t.Fatal("expected root persistent flags to register")
	}
}

func TestRejectDeprecatedPortFlagAllowsUnsetPort(t *testing.T) {
	cmd := &cobra.Command{Use: "show"}
	var port int
	registerDeprecatedPortFlag(cmd, &port)
	if err := rejectDeprecatedPortFlag(cmd, nil); err != nil {
		t.Fatalf("rejectDeprecatedPortFlag() unset port error = %v", err)
	}
}

func TestRepresentativeManifestRecordsRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		Commands: map[string]climanifest.Command{
			"you": {ID: "you"},
		},
	}
	if _, _, _, err := representativeManifestRecords(manifest); err == nil {
		t.Fatal("representativeManifestRecords() missing commands = nil, want error")
	}
}

func boolPtr(value bool) *bool { return &value }

func stringPtr(value string) *string { return &value }

func TestPersistentBindingTargetRejectsUnknownFlag(t *testing.T) {
	if _, err := persistentBindingTarget("unknown-flag", PersistentFlagBindings{}); err == nil {
		t.Fatal("persistentBindingTarget() unknown flag = nil, want error")
	}
}

func TestRegisterFlagRejectsMissingBindings(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := registerFlag(flags, climanifest.Flag{
		Long:      "verbose",
		ValueType: "bool",
		Default:   "false",
	}, flagTarget{}, "help"); err == nil {
		t.Fatal("registerFlag() missing bool binding = nil, want error")
	}
}

func TestNewRepresentativeFamilyComponentsLoadsEmbeddedManifest(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you", func(cmd *cobra.Command, args []string) error { return nil }); err != nil {
		t.Fatalf("Register(you) error = %v", err)
	}
	if err := registry.Register("you.session.show", func(cmd *cobra.Command, args []string) error { return nil }); err != nil {
		t.Fatalf("Register(you.session.show) error = %v", err)
	}
	components, err := NewRepresentativeFamilyComponents(registry, PersistentFlagBindings{
		Verbose:                    boolPtr(false),
		Debug:                      boolPtr(false),
		Server:                     stringPtr("http://localhost:7437"),
		JSON:                       boolPtr(false),
		DefaultWorkerModelProvider: stringPtr(""),
		DefaultWorkerModel:         stringPtr(""),
	})
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyComponents() error = %v", err)
	}
	if components.Show == nil {
		t.Fatal("expected show component from generated manifest")
	}
}

func TestLocalBindingTargetRejectsUnsupportedValueType(t *testing.T) {
	if _, err := localBindingTarget(climanifest.Flag{
		Long:      "name",
		ValueType: "string",
	}); err == nil {
		t.Fatal("localBindingTarget(string) = nil, want error")
	}
}

func TestRegisterFlagRejectsUnsupportedValueType(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := registerFlag(flags, climanifest.Flag{
		Long:      "weird",
		ValueType: "float",
	}, flagTarget{}, "help"); err == nil {
		t.Fatal("registerFlag(float) = nil, want error")
	}
}
