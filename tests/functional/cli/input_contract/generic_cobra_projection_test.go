package inputcontract

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestGenericCobraProjectionConstructsEveryGeneratedFamily(t *testing.T) {
	manifest := climanifest.Manifest{
		FormatVersion: "1.0.0",
		RootPath:      "you",
		Commands:      make(map[string]climanifest.Command),
	}
	loaders := []func() (climanifest.Manifest, error){
		generated.RepresentativeFamilyManifest,
		generated.SessionFamilyManifest,
		generated.WorkFamilyManifest,
		generated.FactoryConfigInitFamilyManifest,
		generated.ModelsDocsFamilyManifest,
		generated.RunSubmitFamilyManifest,
		generated.MCPFamilyManifest,
	}
	for _, load := range loaders {
		family, err := load()
		if err != nil {
			t.Fatalf("load generated family manifest: %v", err)
		}
		for commandID, command := range family.Commands {
			manifest.Commands[commandID] = command
		}
	}
	root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, genericProjectionBindings(manifest))
	if err != nil {
		t.Fatalf("Construct(all generated families) error = %v", err)
	}
	for _, args := range [][]string{{"work", "list"}, {"factory", "query"}, {"models", "list"}, {"submit"}, {"mcp", "serve"}} {
		command, _, err := root.Find(args)
		if err != nil || command == root {
			t.Fatalf("Find(%v) = (%v, %v), want generated command", args, command, err)
		}
	}
}

func TestGenericCobraProjectionDispatchesTypedValuesAndCompletion(t *testing.T) {
	manifest := genericProjectionManifest()
	var received map[string]any
	bindings := genericProjectionBindings(manifest)
	bindings.Handlers["handler.typed"] = func(_ context.Context, values map[string]any) error {
		received = values
		return nil
	}
	root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, bindings)
	if err != nil {
		t.Fatalf("Construct() error = %v", err)
	}
	root.SetArgs([]string{
		"typed", "--alpha", "--label", "  safe  ", "--attempts", "7",
		"--epoch", "9223372036854775000", "--tag", " first ", "--tag", "second",
		"destination", "11", "12",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received["flag.alpha"] != true || received["flag.label"] != "safe" ||
		received["flag.attempts"] != 7 || received["flag.epoch"] != int64(9223372036854775000) {
		t.Fatalf("scalar handler inputs = %#v", received)
	}
	if values, ok := received["flag.tags"].([]string); !ok || strings.Join(values, ",") != "first,second" {
		t.Fatalf("repeated handler input = %#v", received["flag.tags"])
	}
	if values, ok := received["arg.ids"].([]int64); !ok || len(values) != 2 || values[0] != 11 || values[1] != 12 {
		t.Fatalf("variadic handler input = %#v", received["arg.ids"])
	}
	typed, _, err := root.Find([]string{"typed"})
	if err != nil || typed.ValidArgsFunction == nil {
		t.Fatalf("Find(typed) = (%v, %v), want positional completion", typed, err)
	}
	completeLabel, ok := typed.GetFlagCompletionFunc("label")
	if !ok {
		t.Fatal("static label completion is missing")
	}
	choices, directive := completeLabel(typed, nil, "s")
	if len(choices) != 2 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("label completion = (%v, %v)", choices, directive)
	}
	choices, directive = typed.ValidArgsFunction(typed, []string{"destination"}, "")
	if choices != nil || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("dynamic argument completion = (%v, %v)", choices, directive)
	}
}

func TestGenericCobraProjectionEnforcesRelationshipsBeforeDispatch(t *testing.T) {
	tests := []struct {
		relationship climanifest.Relationship
		args         []string
		wantErr      string
	}{
		{projectionGroupRelationship("rel.mutex", "mutually-exclusive", projectionFlagRef("flag.alpha"), projectionFlagRef("flag.beta")), []string{"typed", "--alpha", "--beta", "destination"}, "cannot be used together"},
		{projectionGroupRelationship("rel.together", "required-together", projectionFlagRef("flag.alpha"), projectionArgumentRef("arg.target")), []string{"typed", "--alpha"}, "must be provided together"},
		{projectionGroupRelationship("rel.one", "at-least-one", projectionFlagRef("flag.alpha"), projectionArgumentRef("arg.target")), []string{"typed"}, "requires at least one"},
		{projectionDirectedRelationship("rel.dependency", "dependency", projectionFlagRef("flag.alpha"), projectionArgumentRef("arg.target")), []string{"typed", "--alpha"}, "requires target"},
		{projectionDirectedRelationship("rel.conditional", "conditional", projectionArgumentRef("arg.target"), projectionFlagRef("flag.alpha")), []string{"typed", "destination"}, "requires --alpha"},
		{projectionGroupRelationship("rel.conflict", "conflict", projectionFlagRef("flag.alpha"), projectionArgumentRef("arg.target")), []string{"typed", "--alpha", "destination"}, "cannot be used together"},
	}
	for _, test := range tests {
		t.Run(test.relationship.ID, func(t *testing.T) {
			manifest := genericProjectionManifest()
			command := manifest.Commands["typed"]
			command.Relationships = map[string]climanifest.Relationship{test.relationship.ID: test.relationship}
			manifest.Commands[command.ID] = command
			calls := 0
			bindings := genericProjectionBindings(manifest)
			bindings.Handlers["handler.typed"] = func(context.Context, map[string]any) error {
				calls++
				return nil
			}
			root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, bindings)
			if err != nil {
				t.Fatalf("Construct() error = %v", err)
			}
			root.SetArgs(test.args)
			err = root.Execute()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) || calls != 0 {
				t.Fatalf("Execute(%v) error = %v calls=%d", test.args, err, calls)
			}
		})
	}
}

type projectionInvalidCase struct {
	name   string
	mutate func(*climanifest.Manifest, *climanifestcobra.GenericBindings)
}

func TestGenericCobraProjectionRejectsInvalidContracts(t *testing.T) {
	tests := []projectionInvalidCase{
		{"format", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			manifest.FormatVersion = "2.0.0"
		}},
		{"missing parent", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			delete(manifest.Commands, "root")
		}},
		{"map identity", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.ID = "different"
			manifest.Commands["typed"] = command
		}},
		{"visibility", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Visibility = "internal"
			manifest.Commands[command.ID] = command
		}},
		{"completeness", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Completeness = "partial"
			manifest.Commands[command.ID] = command
		}},
		{"duplicate alias", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Aliases = []string{"typed"}
			manifest.Commands[command.ID] = command
		}},
		{"missing name", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Name = ""
			manifest.Commands[command.ID] = command
		}},
		{"path spacing", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Path = "forge  typed"
			manifest.Commands[command.ID] = command
		}},
		{"missing usage", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Usage.Line = ""
			manifest.Commands[command.ID] = command
		}},
		{"missing documentation", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Documentation.Documentation.Description.CanonicalEnglish = ""
			manifest.Commands[command.ID] = command
		}},
		{"missing handler", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Handler = nil
			manifest.Commands[command.ID] = command
		}},
		{"non-runnable handler", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["root"]
			command.Handler = &climanifest.Handler{ID: "handler.root"}
			manifest.Commands[command.ID] = command
		}},
		{"unknown handler", func(manifest *climanifest.Manifest, bindings *climanifestcobra.GenericBindings) {
			delete(bindings.Handlers, manifest.Commands["typed"].Handler.ID)
		}},
		{"unsupported flag", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			flag := command.Flags["flag.label"]
			flag.ValueType = "duration"
			command.Flags[flag.ID] = flag
			manifest.Commands[command.ID] = command
		}},
		{"invalid default", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			flag := command.Flags["flag.attempts"]
			flag.Default = "many"
			command.Flags[flag.ID] = flag
			manifest.Commands[command.ID] = command
		}},
		{"missing completion", func(_ *climanifest.Manifest, bindings *climanifestcobra.GenericBindings) {
			delete(bindings.Completions, "flag.tags")
		}},
		{"unsupported argument", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			argument := command.Arguments["arg.target"]
			argument.ValueType = "object"
			command.Arguments[argument.ID] = argument
			manifest.Commands[command.ID] = command
		}},
		{"unsupported lifecycle", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			command.Lifecycle.State = "removed"
			manifest.Commands[command.ID] = command
		}},
		{"invalid relationship", func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
			command := manifest.Commands["typed"]
			relationship := projectionGroupRelationship("rel.invalid", "sometimes", projectionFlagRef("flag.alpha"), projectionFlagRef("flag.beta"))
			command.Relationships = map[string]climanifest.Relationship{relationship.ID: relationship}
			manifest.Commands[command.ID] = command
		}},
	}
	assertInvalidProjectionContracts(t, tests)
}

func assertInvalidProjectionContracts(t *testing.T, tests []projectionInvalidCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := genericProjectionManifest()
			bindings := genericProjectionBindings(manifest)
			test.mutate(&manifest, &bindings)
			root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, bindings)
			if root != nil || err == nil {
				t.Fatalf("Construct() = (%v, %v), want construction error", root, err)
			}
		})
	}
}

func TestGenericCobraProjectionRejectsInvalidRelationshipShapes(t *testing.T) {
	relationships := []climanifest.Relationship{
		projectionGroupRelationship("rel.unknown", "mutually-exclusive", projectionFlagRef("flag.missing"), projectionFlagRef("flag.beta")),
		projectionGroupRelationship("rel.kind", "sometimes", projectionFlagRef("flag.alpha"), projectionFlagRef("flag.beta")),
		projectionGroupRelationship("rel.short", "at-least-one", projectionFlagRef("flag.alpha")),
		{ID: "rel.no-trigger", Kind: "dependency", Participants: []climanifest.ParticipantRef{projectionFlagRef("flag.beta")}},
		projectionDirectedRelationship("rel.trigger", "conflict", projectionFlagRef("flag.alpha"), projectionFlagRef("flag.beta")),
	}
	for _, relationship := range relationships {
		t.Run(relationship.ID, func(t *testing.T) {
			manifest := genericProjectionManifest()
			command := manifest.Commands["typed"]
			command.Relationships = map[string]climanifest.Relationship{relationship.ID: relationship}
			manifest.Commands[command.ID] = command
			root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, genericProjectionBindings(manifest))
			if root != nil || err == nil || !strings.Contains(err.Error(), relationship.ID) {
				t.Fatalf("Construct() = (%v, %v), want relationship error", root, err)
			}
		})
	}
}

func TestGenericCobraProjectionRejectsInvalidTypedInvocations(t *testing.T) {
	for _, args := range [][]string{
		{"typed", "--label", "dangerous", "destination"},
		{"typed", "--attempts", "many", "destination"},
		{"typed", "--epoch", "later", "destination"},
		{"typed", "destination", "later"},
	} {
		manifest := genericProjectionManifest()
		calls := 0
		bindings := genericProjectionBindings(manifest)
		bindings.Handlers["handler.typed"] = func(context.Context, map[string]any) error {
			calls++
			return nil
		}
		root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, bindings)
		if err != nil {
			t.Fatalf("Construct() error = %v", err)
		}
		root.SetArgs(args)
		if err := root.Execute(); err == nil || calls != 0 {
			t.Fatalf("Execute(%v) error = %v calls=%d", args, err, calls)
		}
	}
}

func TestGenericCobraProjectionProjectsDeprecatedLifecycle(t *testing.T) {
	manifest := genericProjectionManifest()
	command := manifest.Commands["typed"]
	command.Lifecycle = climanifest.Lifecycle{
		FormatVersion: "1.0.0", ItemID: command.ID, State: "deprecated",
		Since: "1.0.0", Deprecated: "2.0.0",
		Successor: &climanifest.LifecycleSuccessor{
			TargetItemID: "replacement", CanonicalEnglish: "Use replacement instead.",
		},
	}
	manifest.Commands[command.ID] = command
	root, err := (climanifestcobra.GenericConstructor{}).Construct(manifest, genericProjectionBindings(manifest))
	if err != nil {
		t.Fatalf("Construct() error = %v", err)
	}
	typed, _, err := root.Find([]string{"typed"})
	if err != nil || typed.Deprecated != "Use replacement instead." ||
		!strings.Contains(typed.Long, "DEPRECATED: Use replacement instead.") {
		t.Fatalf("deprecated command = (%v, %v)", typed, err)
	}
}

func genericProjectionManifest() climanifest.Manifest {
	root := projectionCommand("root", "forge", "forge", false)
	command := projectionCommand("typed", "typed", "forge typed", true)
	command.Handler = &climanifest.Handler{ID: "handler.typed"}
	command.Flags = map[string]climanifest.Flag{
		"flag.alpha": projectionBoolFlag("flag.alpha", "alpha"),
		"flag.beta":  projectionBoolFlag("flag.beta", "beta"),
		"flag.label": {
			ID: "flag.label", Long: "label", Scope: "local", ValueType: "string",
			Enum: []string{"safe", "fast"}, Normalization: "trim", Visibility: "visible",
			Completion: "static", Lifecycle: projectionLifecycle("flag.label"),
		},
		"flag.attempts": {
			ID: "flag.attempts", Long: "attempts", Scope: "local", ValueType: "int",
			Default: "3", Visibility: "visible", Completion: "none", Lifecycle: projectionLifecycle("flag.attempts"),
		},
		"flag.epoch": {
			ID: "flag.epoch", Long: "epoch", Scope: "local", ValueType: "int64",
			Default: "42", Visibility: "visible", Completion: "none", Lifecycle: projectionLifecycle("flag.epoch"),
		},
		"flag.tags": {
			ID: "flag.tags", Long: "tag", Scope: "local", ValueType: "stringArray",
			Repeatable: true, Normalization: "trim", Visibility: "visible",
			Completion: "dynamic", Lifecycle: projectionLifecycle("flag.tags"),
		},
	}
	command.Arguments = map[string]climanifest.Argument{
		"arg.target": {
			ID: "arg.target", Name: "target", Position: 0, Kind: "positional",
			ValueType: "string", Enum: []string{"destination", "alternate"},
			MinCardinality: 0, MaxCardinality: 1, Completion: "static",
		},
		"arg.ids": {
			ID: "arg.ids", Name: "ids", Position: 1, Kind: "positional",
			ValueType: "int64", Variadic: true, MinCardinality: 0, MaxCardinality: -1, Completion: "dynamic",
		},
	}
	return climanifest.Manifest{
		FormatVersion: "1.0.0",
		RootPath:      "forge",
		Commands:      map[string]climanifest.Command{root.ID: root, command.ID: command},
	}
}

func projectionCommand(id, name, path string, runnable bool) climanifest.Command {
	return climanifest.Command{
		ID: id, Name: name, Path: path, Runnable: runnable, Visibility: "visible",
		Usage:     climanifest.Usage{Line: name},
		Lifecycle: projectionLifecycle(id),
		Documentation: climanifest.Documentation{Documentation: climanifest.DocumentationCopy{
			Title:       climanifest.DocumentationField{CanonicalEnglish: name + " title"},
			Description: climanifest.DocumentationField{CanonicalEnglish: name + " description"},
		}},
	}
}

func projectionBoolFlag(id, name string) climanifest.Flag {
	return climanifest.Flag{
		ID: id, Long: name, Scope: "local", ValueType: "bool",
		NoOptionDefault: "true", Visibility: "visible", Completion: "none",
		Lifecycle: projectionLifecycle(id),
	}
}

func projectionLifecycle(id string) climanifest.Lifecycle {
	return climanifest.Lifecycle{
		FormatVersion: "1.0.0",
		ItemID:        id,
		State:         "active",
		Since:         "1.0.0",
	}
}

func genericProjectionBindings(manifest climanifest.Manifest) climanifestcobra.GenericBindings {
	handlers := make(climanifestcobra.HandlerRegistry)
	completions := make(climanifestcobra.CompletionRegistry)
	for _, command := range manifest.Commands {
		if command.Handler != nil {
			handlers[command.Handler.ID] = func(context.Context, map[string]any) error { return nil }
		}
		for _, flag := range command.Flags {
			if flag.Completion == "dynamic" {
				completions[flag.ID] = emptyProjectionCompletion
			}
		}
		for _, argument := range command.Arguments {
			if argument.Completion == "dynamic" {
				completions[argument.ID] = emptyProjectionCompletion
			}
		}
	}
	return climanifestcobra.GenericBindings{Handlers: handlers, Completions: completions}
}

func emptyProjectionCompletion(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func projectionFlagRef(id string) climanifest.ParticipantRef {
	return climanifest.ParticipantRef{Type: "flag", ID: id}
}

func projectionArgumentRef(id string) climanifest.ParticipantRef {
	return climanifest.ParticipantRef{Type: "argument", ID: id}
}

func projectionGroupRelationship(
	id, kind string,
	participants ...climanifest.ParticipantRef,
) climanifest.Relationship {
	return climanifest.Relationship{ID: id, Kind: kind, Participants: participants}
}

func projectionDirectedRelationship(
	id, kind string,
	when climanifest.ParticipantRef,
	participants ...climanifest.ParticipantRef,
) climanifest.Relationship {
	return climanifest.Relationship{ID: id, Kind: kind, When: &when, Participants: participants}
}
