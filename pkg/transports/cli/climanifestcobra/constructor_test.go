package climanifestcobra_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewRepresentativeFamilyCommandBuildsContractedPaths(t *testing.T) {
	root, registry := mustRepresentativeFamilyTree(t)

	if root.Name() != "you" {
		t.Fatalf("root name = %q, want you", root.Name())
	}
	if len(root.Commands()) != 1 {
		t.Fatalf("root child count = %d, want 1 representative session command", len(root.Commands()))
	}

	session, err := findCommandByPath(root, "you session")
	if err != nil {
		t.Fatalf("FindCommandByPath(you session) error = %v", err)
	}
	if session.Runnable() {
		t.Fatal("you session must remain non-runnable")
	}
	if session.RunE != nil {
		t.Fatal("you session must not attach RunE")
	}
	if len(session.Commands()) != 1 {
		t.Fatalf("session child count = %d, want only show", len(session.Commands()))
	}

	show, err := findCommandByPath(root, "you session show")
	if err != nil {
		t.Fatalf("FindCommandByPath(you session show) error = %v", err)
	}
	if !show.Runnable() {
		t.Fatal("you session show must be runnable")
	}
	if show.RunE == nil {
		t.Fatal("you session show must attach handwritten RunE")
	}
	handler, err := registry.Lookup("you.session.show")
	if err != nil {
		t.Fatalf("Lookup(you.session.show) error = %v", err)
	}
	if show.RunE == nil || handler == nil {
		t.Fatal("session show handler must resolve through registry")
	}
}

func TestNewRepresentativeFamilyCommandRejectsOutOfFamilyManifestCommand(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.session.list"] = manifest.Commands["you.session.show"]
	delete(manifest.Commands, "you.session.show")

	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}

	_, err = climanifestcobra.NewRepresentativeFamilyCommandFromManifest(manifest, registry, testBindings())
	if err == nil {
		t.Fatal("NewRepresentativeFamilyCommandFromManifest() error = nil, want out-of-family rejection")
	}
}

func TestNewRepresentativeFamilyCommandRejectsMissingHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you", noopRunE); err != nil {
		t.Fatalf("Register(you) error = %v", err)
	}
	if _, err := climanifestcobra.NewRepresentativeFamilyCommand(registry, testBindings()); err == nil {
		t.Fatal("NewRepresentativeFamilyCommand() missing session show handler = nil, want error")
	}
}

func TestNewRepresentativeFamilyCommandExposesOnlyRepresentativeFamily(t *testing.T) {
	root, _ := mustRepresentativeFamilyTree(t)
	for _, child := range root.Commands() {
		if child.Name() != "session" {
			t.Fatalf("root child = %q, want only session in representative cutover surface", child.Name())
		}
	}
	for _, id := range climanifestgen.RepresentativeFamilyCommandIDs {
		if id == "you" {
			continue
		}
		if _, err := findCommandByPath(root, commandPathForID(id)); err != nil {
			t.Fatalf("representative path for %q missing: %v", id, err)
		}
	}
	if _, err := findCommandByPath(root, "you run"); err == nil {
		t.Fatal("generated representative constructor must not expose you run")
	}
}

func TestNewRepresentativeFamilyCommandRegistersContractedFlagsAndArgs(t *testing.T) {
	root, _ := mustRepresentativeFamilyTree(t)
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	showRecord, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}
	show, err := findCommandByPath(root, showRecord.Path)
	if err != nil {
		t.Fatalf("FindCommandByPath(%q) error = %v", showRecord.Path, err)
	}

	portFlag := show.Flags().Lookup("port")
	if portFlag == nil || !portFlag.Hidden {
		t.Fatalf("port flag = %#v, want hidden local deprecated flag", portFlag)
	}
	if show.Args == nil {
		t.Fatal("session show must wire positional args from generated metadata")
	}
	if err := show.Args(show, []string{"one", "two"}); err == nil {
		t.Fatal("session show args = nil error, want excess positional rejection")
	}

}

func mustRepresentativeFamilyTree(t *testing.T) (*cobra.Command, *commandregistry.Registry) {
	t.Helper()
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	root, err := climanifestcobra.NewRepresentativeFamilyCommand(registry, testBindings())
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyCommand() error = %v", err)
	}
	return root, registry
}

func testBindings() climanifestcobra.PersistentFlagBindings {
	var verbose bool
	var debug bool
	server := "http://localhost:7437"
	var json bool
	defaultWorkerModelProvider := ""
	defaultWorkerModel := ""
	return climanifestcobra.PersistentFlagBindings{
		Verbose:                    &verbose,
		Debug:                      &debug,
		Server:                     &server,
		JSON:                       &json,
		DefaultWorkerModelProvider: &defaultWorkerModelProvider,
		DefaultWorkerModel:         &defaultWorkerModel,
	}
}

func commandPathForID(commandID string) string {
	switch commandID {
	case "you.session":
		return "you session"
	case "you.session.show":
		return "you session show"
	default:
		return commandID
	}
}

func noopRunE(cmd *cobra.Command, args []string) error {
	return nil
}

func TestNewCommandTreeBuildsSyntheticHierarchyDeterministically(t *testing.T) {
	manifest := syntheticTreeManifest()

	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}

	assertSyntheticRoot(t, root)
	assertSyntheticChildren(t, root)
}

func assertSyntheticRoot(t *testing.T, root *cobra.Command) {
	t.Helper()
	if root.Name() != "forge" || root.Use != "forge [flags]" {
		t.Fatalf("root identity = (%q, %q), want (forge, forge [flags])", root.Name(), root.Use)
	}
	if root.Runnable() {
		t.Fatal("schema non-runnable root received an execution handler")
	}
}

func assertSyntheticChildren(t *testing.T, root *cobra.Command) {
	t.Helper()
	children := root.Commands()
	if len(children) != 2 || children[0].Name() != "alpha" || children[1].Name() != "zeta" {
		t.Fatalf("root children = %v, want [alpha zeta]", commandNames(children))
	}
	assertSyntheticAlpha(t, children[0])
	if children[1].Runnable() {
		t.Fatal("schema non-runnable zeta command received an execution handler")
	}
}

func assertSyntheticAlpha(t *testing.T, alpha *cobra.Command) {
	t.Helper()
	if alpha.Short != "Alpha title" || alpha.Long != "Alpha title\n\nAlpha description" {
		t.Fatalf("alpha documentation = (%q, %q)", alpha.Short, alpha.Long)
	}
	if len(alpha.Aliases) != 1 || alpha.Aliases[0] != "a" {
		t.Fatalf("alpha aliases = %v, want [a]", alpha.Aliases)
	}
	if !alpha.Runnable() {
		t.Fatal("schema-runnable alpha command is not runnable")
	}
	leaf := alpha.Commands()
	if len(leaf) != 1 || leaf[0].Name() != "leaf" || !leaf[0].Hidden {
		t.Fatalf("alpha leaf = names %v hidden %t, want [leaf] hidden", commandNames(leaf), len(leaf) == 1 && leaf[0].Hidden)
	}
}

func TestNewCommandTreeRejectsInvalidManifestBeforeReturningTree(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*climanifest.Manifest)
		wantErr string
	}{
		{
			name: "missing parent",
			mutate: func(manifest *climanifest.Manifest) {
				delete(manifest.Commands, "stable.alpha")
			},
			wantErr: `command "stable.leaf" path "forge alpha leaf" has missing parent "forge alpha"`,
		},
		{
			name: "inconsistent map identity",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.alpha"]
				record.ID = "different.id"
				manifest.Commands["stable.alpha"] = record
			},
			wantErr: `command map key "stable.alpha" does not match record id "different.id"`,
		},
		{
			name: "duplicate path",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.zeta"]
				record.Name = "alpha"
				record.Path = "forge alpha"
				record.Usage.Line = "alpha"
				manifest.Commands["stable.zeta"] = record
			},
			wantErr: `declare duplicate path "forge alpha"`,
		},
		{
			name: "inconsistent public identity",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.alpha"]
				record.Name = "renamed"
				manifest.Commands["stable.alpha"] = record
			},
			wantErr: `name "renamed" does not match path "forge alpha"`,
		},
		{
			name: "missing metadata",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.alpha"]
				record.Documentation.Documentation.Description.CanonicalEnglish = ""
				manifest.Commands["stable.alpha"] = record
			},
			wantErr: `command "stable.alpha" is missing documentation description`,
		},
		{
			name: "unsupported visibility",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.alpha"]
				record.Visibility = "internal"
				manifest.Commands["stable.alpha"] = record
			},
			wantErr: `command "stable.alpha" has unsupported visibility "internal"`,
		},
		{
			name: "unsupported completeness mode",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["stable.alpha"]
				record.Completeness = "partial"
				manifest.Commands["stable.alpha"] = record
			},
			wantErr: `command "stable.alpha" has unsupported completeness mode "partial"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticTreeManifest()
			test.mutate(&manifest)

			root, err := climanifestcobra.NewCommandTree(manifest)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewCommandTree() = (%v, %v), want nil error containing %q", root, err, test.wantErr)
			}
			if root != nil {
				t.Fatalf("NewCommandTree() root = %v after validation failure, want nil", root)
			}
		})
	}
}

func TestNewCommandTreeParsesSchemaNeutralTypedFlagsByStableInputID(t *testing.T) {
	manifest := syntheticFlagManifest()
	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{
		"alpha",
		"--enable",
		"--nebula-label", "  unfamiliar value  ",
		"-c", "7",
		"--generation", "9223372036854775000",
		"--tag", " first ",
		"--tag", "second",
		"--mode",
	})
	err = root.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	alpha := root.Commands()[0]
	values, err := climanifestcobra.InputValues(alpha)
	if err != nil {
		t.Fatalf("InputValues(alpha) error = %v", err)
	}
	want := map[string]any{
		"stable.alpha.flag.activate": true,
		"stable.alpha.flag.label":    "unfamiliar value",
		"stable.alpha.flag.attempts": 7,
		"stable.alpha.flag.epoch":    int64(9223372036854775000),
		"stable.alpha.flag.tags":     []string{"first", "second"},
		"stable.alpha.flag.mode":     "safe",
		"stable.alpha.flag.secret":   "",
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("InputValues(alpha) = %#v, want %#v", values, want)
	}
	if flag := alpha.Flags().Lookup("secret-code"); flag == nil || !flag.Hidden {
		t.Fatalf("hidden flag = %#v, want registered and hidden", flag)
	}
}

func TestNewCommandTreeAcceptsRepresentativeGeneratedFlagRecords(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	if _, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest)); err != nil {
		t.Fatalf("NewCommandTree(RepresentativeFamilyManifest()) error = %v", err)
	}
}

func TestNewCommandTreeAppliesTypedDefaultsAndRejectsInvalidInvocations(t *testing.T) {
	t.Run("typed defaults", func(t *testing.T) {
		manifest := syntheticFlagManifest()
		root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
		if err != nil {
			t.Fatalf("NewCommandTree() error = %v", err)
		}
		root.SetArgs([]string{"alpha", "--label", "present"})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		values, err := climanifestcobra.InputValues(root.Commands()[0])
		if err != nil {
			t.Fatalf("InputValues(alpha) error = %v", err)
		}
		if values["stable.alpha.flag.attempts"] != 3 ||
			values["stable.alpha.flag.epoch"] != int64(42) ||
			!reflect.DeepEqual(values["stable.alpha.flag.tags"], []string{"base"}) {
			t.Fatalf("typed defaults = %#v", values)
		}
	})

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "required", args: []string{"alpha"}, wantErr: `required flag(s) "--nebula-label" not set`},
		{name: "enum", args: []string{"alpha", "--label", "present", "--mode=dangerous"}, wantErr: `value "dangerous" is not one of the declared choices`},
		{name: "typed parse", args: []string{"alpha", "--label", "present", "--attempts", "many"}, wantErr: `invalid syntax`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticFlagManifest()
			root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
			if err != nil {
				t.Fatalf("NewCommandTree() error = %v", err)
			}
			root.SetArgs(test.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestNewCommandTreeRejectsInvalidFlagRecordsBeforeReturningTree(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*climanifest.Manifest)
		wantErr string
	}{
		{
			name: "unsupported value type",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticFlag(manifest, "stable.alpha", "stable.alpha.flag.attempts", func(flag *climanifest.Flag) {
					flag.ValueType = "float"
				})
			},
			wantErr: `command "stable.alpha" input "stable.alpha.flag.attempts": unsupported value type "float"`,
		},
		{
			name: "invalid typed default",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticFlag(manifest, "stable.alpha", "stable.alpha.flag.attempts", func(flag *climanifest.Flag) {
					value := "three"
					flag.DefaultValue = &climanifest.InputValue{String: &value}
				})
			},
			wantErr: `command "stable.alpha" input "stable.alpha.flag.attempts": invalid typed default`,
		},
		{
			name: "missing inheritance target",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticFlag(manifest, "stable.alpha", "stable.alpha.flag.activate", func(flag *climanifest.Flag) {
					flag.InheritedFromID = "stable.root.flag.missing"
				})
			},
			wantErr: `command "stable.alpha" input "stable.alpha.flag.activate": inherited input "stable.root.flag.missing"`,
		},
		{
			name: "incompatible repeatability",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticFlag(manifest, "stable.alpha", "stable.alpha.flag.attempts", func(flag *climanifest.Flag) {
					flag.Repeatable = true
				})
			},
			wantErr: `command "stable.alpha" input "stable.alpha.flag.attempts": maximum cardinality 1 is incompatible with repeatable=true`,
		},
		{
			name: "incompatible inherited metadata",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticFlag(manifest, "stable.alpha", "stable.alpha.flag.activate", func(flag *climanifest.Flag) {
					flag.Long = "different"
				})
			},
			wantErr: `command "stable.alpha" input "stable.alpha.flag.activate": inheritance target "stable.root.flag.activate" has incompatible flag metadata`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticFlagManifest()
			test.mutate(&manifest)
			root, err := climanifestcobra.NewCommandTree(manifest)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewCommandTree() = (%v, %v), want nil error containing %q", root, err, test.wantErr)
			}
			if root != nil {
				t.Fatalf("NewCommandTree() root = %v after validation failure, want nil", root)
			}
		})
	}
}

func syntheticFlagManifest() climanifest.Manifest {
	manifest := syntheticTreeManifest()
	delete(manifest.Commands, "stable.zeta")
	delete(manifest.Commands, "stable.leaf")

	root := manifest.Commands["stable.root"]
	root.Flags = map[string]climanifest.Flag{
		"stable.root.flag.activate": {
			ID:            "stable.root.flag.activate",
			Long:          "activate",
			Shorthand:     "a",
			Aliases:       []string{"enable"},
			Scope:         "persistent",
			ValueType:     "bool",
			DefaultValue:  boolInputValue(false),
			NoOptionValue: boolInputValue(true),
			Visibility:    "visible",
		},
	}
	withActiveFlagLifecycle(root.Flags)
	completeCanonicalCommandContract(&root)
	manifest.Commands[root.ID] = root

	alpha := manifest.Commands["stable.alpha"]
	alpha.Flags = map[string]climanifest.Flag{
		"stable.alpha.flag.activate": {
			ID:              "stable.alpha.flag.activate",
			Long:            "activate",
			Shorthand:       "a",
			Aliases:         []string{"enable"},
			Scope:           "inherited",
			InheritedFromID: "stable.root.flag.activate",
			ValueType:       "bool",
			DefaultValue:    boolInputValue(false),
			NoOptionValue:   boolInputValue(true),
			Visibility:      "visible",
		},
		"stable.alpha.flag.label": {
			ID:            "stable.alpha.flag.label",
			Long:          "nebula-label",
			Aliases:       []string{"label"},
			Scope:         "local",
			ValueType:     "string",
			Default:       "",
			Required:      true,
			Normalization: "trim",
			Visibility:    "visible",
		},
		"stable.alpha.flag.attempts": {
			ID:           "stable.alpha.flag.attempts",
			Long:         "attempts",
			Shorthand:    "c",
			Scope:        "local",
			ValueType:    "int",
			DefaultValue: intInputValue(3),
			Visibility:   "visible",
		},
		"stable.alpha.flag.epoch": {
			ID:           "stable.alpha.flag.epoch",
			Long:         "generation",
			Scope:        "local",
			ValueType:    "int64",
			DefaultValue: int64InputValue(42),
			Visibility:   "visible",
		},
		"stable.alpha.flag.tags": {
			ID:            "stable.alpha.flag.tags",
			Long:          "tag",
			Scope:         "local",
			ValueType:     "stringArray",
			Repeatable:    true,
			DefaultValue:  stringArrayInputValue([]string{"base"}),
			Normalization: "trim",
			Visibility:    "visible",
		},
		"stable.alpha.flag.mode": {
			ID:              "stable.alpha.flag.mode",
			Long:            "mode",
			Scope:           "local",
			ValueType:       "string",
			Enum:            []string{"safe", "fast"},
			Default:         "fast",
			NoOptionDefault: "safe",
			Visibility:      "visible",
		},
		"stable.alpha.flag.secret": {
			ID:         "stable.alpha.flag.secret",
			Long:       "secret-code",
			Scope:      "local",
			ValueType:  "string",
			Default:    "",
			Visibility: "hidden",
		},
	}
	withActiveFlagLifecycle(alpha.Flags)
	completeCanonicalCommandContract(&alpha)
	manifest.Commands[alpha.ID] = alpha
	return manifest
}

func updateSyntheticFlag(
	manifest *climanifest.Manifest,
	commandID string,
	inputID string,
	update func(*climanifest.Flag),
) {
	command := manifest.Commands[commandID]
	flag := command.Flags[inputID]
	update(&flag)
	command.Flags[inputID] = flag
	manifest.Commands[commandID] = command
}

func intInputValue(value int) *climanifest.InputValue {
	return &climanifest.InputValue{Int: &value}
}

func boolInputValue(value bool) *climanifest.InputValue {
	return &climanifest.InputValue{Boolean: &value}
}

func int64InputValue(value int64) *climanifest.InputValue {
	return &climanifest.InputValue{Int64: &value}
}

func stringArrayInputValue(value []string) *climanifest.InputValue {
	return &climanifest.InputValue{StringArray: &value}
}

func syntheticTreeManifest() climanifest.Manifest {
	return climanifest.Manifest{
		FormatVersion: "1.0.0",
		RootPath:      "forge",
		Commands: map[string]climanifest.Command{
			"stable.zeta": syntheticCommand("stable.zeta", "zeta", "forge zeta", false),
			"stable.leaf": func() climanifest.Command {
				record := syntheticCommand("stable.leaf", "leaf", "forge alpha leaf", true)
				record.Visibility = "hidden"
				return record
			}(),
			"stable.root": func() climanifest.Command {
				record := syntheticCommand("stable.root", "forge", "forge", false)
				record.Usage.Line = "forge [flags]"
				return record
			}(),
			"stable.alpha": func() climanifest.Command {
				record := syntheticCommand("stable.alpha", "alpha", "forge alpha", true)
				record.Aliases = []string{"a"}
				return record
			}(),
		},
	}
}

func syntheticCommand(id, name, path string, runnable bool) climanifest.Command {
	titleName := strings.ToUpper(name[:1]) + name[1:]
	record := climanifest.Command{
		ID:         id,
		Name:       name,
		Path:       path,
		Visibility: "visible",
		Runnable:   runnable,
		Usage:      climanifest.Usage{Line: name},
		Lifecycle:  activeLifecycle(id),
		Documentation: climanifest.Documentation{
			Documentation: climanifest.DocumentationCopy{
				Title:       climanifest.DocumentationField{CanonicalEnglish: titleName + " title"},
				Description: climanifest.DocumentationField{CanonicalEnglish: titleName + " description"},
			},
		},
	}
	if runnable {
		record.Handler = &climanifest.Handler{ID: id + ".handler"}
	}
	return record
}

func commandNames(commands []*cobra.Command) []string {
	names := make([]string, len(commands))
	for index, command := range commands {
		names[index] = command.Name()
	}
	return names
}

func TestNewCommandTreeAssignsTypedPositionalArgumentsByStableInputID(t *testing.T) {
	manifest := syntheticArgumentManifest()
	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	command := root.Commands()[0]
	command.RunE = func(*cobra.Command, []string) error { return nil }
	root.SetArgs([]string{"shape", "7", "label", "11", "12"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	values, err := climanifestcobra.InputValues(command)
	if err != nil {
		t.Fatalf("InputValues() error = %v", err)
	}
	want := map[string]any{
		"stable.shape.arg.count": 7,
		"stable.shape.arg.label": "label",
		"stable.shape.arg.ids":   []string{"11", "12"},
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("InputValues() = %#v, want %#v", values, want)
	}
}

func TestNewCommandTreeAppliesOptionalDefaultsAndFixedCardinality(t *testing.T) {
	manifest := syntheticArgumentManifest()
	command := manifest.Commands["stable.shape"]
	label := command.Arguments["stable.shape.arg.label"]
	defaultLabel := "fallback"
	label.DefaultValue = &climanifest.InputValue{String: &defaultLabel}
	label = canonicalTestArgument(label)
	command.Arguments[label.ID] = label
	ids := command.Arguments["stable.shape.arg.ids"]
	ids.MinCardinality = 2
	ids.MaxCardinality = 2
	ids.Variadic = false
	ids.Required = true
	command.Arguments[ids.ID] = ids
	completeCanonicalCommandContract(&command)
	manifest.Commands[command.ID] = command

	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	shape := root.Commands()[0]
	shape.RunE = func(*cobra.Command, []string) error { return nil }
	root.SetArgs([]string{"shape", "7", "11", "12"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	values, err := climanifestcobra.InputValues(shape)
	if err != nil {
		t.Fatalf("InputValues() error = %v", err)
	}
	if values["stable.shape.arg.label"] != "fallback" ||
		!reflect.DeepEqual(values["stable.shape.arg.ids"], []string{"11", "12"}) {
		t.Fatalf("InputValues() = %#v, want typed default and fixed repeated values", values)
	}
}

func TestNewCommandTreeEnforcesEveryRelationshipKindBeforeHandler(t *testing.T) {
	tests := []struct {
		name         string
		relationship climanifest.Relationship
		accepted     []string
		rejected     []string
		wantInputs   string
	}{
		{
			name:         "mutually exclusive flags",
			relationship: groupRelationship("rel.mutex", "mutually-exclusive", flagRef("flag.alpha"), flagRef("flag.beta")),
			accepted:     []string{"--alpha"},
			rejected:     []string{"--alpha", "--beta"},
			wantInputs:   "--alpha, --beta",
		},
		{
			name:         "required together mixed inputs",
			relationship: groupRelationship("rel.together", "required-together", flagRef("flag.alpha"), argumentRef("arg.target")),
			accepted:     []string{"--alpha", "destination"},
			rejected:     []string{"--alpha"},
			wantInputs:   "--alpha, target",
		},
		{
			name:         "at least one mixed inputs",
			relationship: groupRelationship("rel.one", "at-least-one", flagRef("flag.alpha"), argumentRef("arg.target")),
			accepted:     []string{"destination"},
			rejected:     nil,
			wantInputs:   "--alpha, target",
		},
		{
			name:         "dependency",
			relationship: directedRelationship("rel.dependency", "dependency", flagRef("flag.alpha"), argumentRef("arg.target")),
			accepted:     []string{"--alpha", "destination"},
			rejected:     []string{"--alpha"},
			wantInputs:   "target",
		},
		{
			name:         "conditional",
			relationship: directedRelationship("rel.conditional", "conditional", argumentRef("arg.target"), flagRef("flag.alpha")),
			accepted:     []string{"--alpha", "destination"},
			rejected:     []string{"destination"},
			wantInputs:   "--alpha",
		},
		{
			name:         "conflict mixed inputs",
			relationship: groupRelationship("rel.conflict", "conflict", flagRef("flag.alpha"), argumentRef("arg.target")),
			accepted:     []string{"--alpha"},
			rejected:     []string{"--alpha", "destination"},
			wantInputs:   "--alpha, target",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, spelling := range []string{"--alpha", "--alternate-alpha", "-a"} {
				t.Run(spelling, func(t *testing.T) {
					assertRelationshipInvocation(t, test.relationship, replaceFlagSpelling(test.accepted, spelling), true, "")
					assertRelationshipInvocation(t, test.relationship, replaceFlagSpelling(test.rejected, spelling), false, test.wantInputs)
				})
			}
		})
	}
}
func assertRelationshipInvocation(
	t *testing.T,
	relationship climanifest.Relationship,
	args []string,
	accepted bool,
	wantInputs string,
) {
	t.Helper()
	manifest := syntheticRelationshipManifest(relationship)
	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	called := 0
	root.Commands()[0].RunE = func(*cobra.Command, []string) error {
		called++
		return nil
	}
	root.SetArgs(append([]string{"check"}, args...))
	err = root.Execute()
	if accepted {
		if err != nil || called != 1 {
			t.Fatalf("accepted invocation = (error %v, calls %d), want nil and one handler call", err, called)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantInputs) || called != 0 {
		t.Fatalf("rejected invocation = (error %v, calls %d), want inputs %q and zero handler calls", err, called, wantInputs)
	}
}

func TestNewCommandTreeRejectsInvalidArgumentAndRelationshipRecords(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*climanifest.Manifest)
		wantErr string
	}{
		{
			name: "impossible position",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticArgument(manifest, "arg.target", func(argument *climanifest.Argument) {
					argument.Position = 1
				})
			},
			wantErr: `argument "arg.target": position 1 leaves an impossible positional layout`,
		},
		{
			name: "incompatible typed default",
			mutate: func(manifest *climanifest.Manifest) {
				updateSyntheticArgument(manifest, "arg.target", func(argument *climanifest.Argument) {
					value := 9
					argument.DefaultValue = &climanifest.InputValue{Int: &value}
					*argument = canonicalTestArgument(*argument)
				})
			},
			wantErr: `argument "arg.target": invalid typed default`,
		},
		{
			name: "unknown relationship participant",
			mutate: func(manifest *climanifest.Manifest) {
				command := manifest.Commands["stable.check"]
				relationship := command.Relationships["rel.mutex"]
				relationship.Participants[0].ID = "flag.missing"
				command.Relationships[relationship.ID] = relationship
				manifest.Commands[command.ID] = command
			},
			wantErr: `relationship "rel.mutex" references unknown participant "flag.missing"`,
		},
		{
			name: "unsupported relationship shape",
			mutate: func(manifest *climanifest.Manifest) {
				command := manifest.Commands["stable.check"]
				relationship := command.Relationships["rel.mutex"]
				relationship.When = &climanifest.ParticipantRef{Type: "flag", ID: "flag.alpha"}
				command.Relationships[relationship.ID] = relationship
				manifest.Commands[command.ID] = command
			},
			wantErr: `relationship "rel.mutex" has incompatible when trigger`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticRelationshipManifest(
				groupRelationship("rel.mutex", "mutually-exclusive", flagRef("flag.alpha"), flagRef("flag.beta")),
			)
			test.mutate(&manifest)
			root, err := climanifestcobra.NewCommandTree(manifest)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) || root != nil {
				t.Fatalf("NewCommandTree() = (%v, %v), want nil and error containing %q", root, err, test.wantErr)
			}
		})
	}
}

func syntheticArgumentManifest() climanifest.Manifest {
	manifest := syntheticTreeManifest()
	delete(manifest.Commands, "stable.zeta")
	delete(manifest.Commands, "stable.alpha")
	delete(manifest.Commands, "stable.leaf")
	command := syntheticCommand("stable.shape", "shape", "forge shape", true)
	command.Arguments = map[string]climanifest.Argument{
		"stable.shape.arg.count": {
			ID: "stable.shape.arg.count", Name: "count", Position: 0, Kind: "positional",
			ValueType: "int", Required: true, MinCardinality: 1, MaxCardinality: 1,
		},
		"stable.shape.arg.label": {
			ID: "stable.shape.arg.label", Name: "label", Position: 1, Kind: "positional",
			ValueType: "string", MinCardinality: 0, MaxCardinality: 1,
		},
		"stable.shape.arg.ids": {
			ID: "stable.shape.arg.ids", Name: "ids", Position: 2, Kind: "positional",
			ValueType: "stringArray", MinCardinality: 0, MaxCardinality: -1, Variadic: true,
		},
	}
	withNoneArgumentCompletion(command.Arguments)
	manifest.Commands[command.ID] = command
	return manifest
}

func syntheticRelationshipManifest(relationship climanifest.Relationship) climanifest.Manifest {
	manifest := syntheticTreeManifest()
	delete(manifest.Commands, "stable.zeta")
	delete(manifest.Commands, "stable.alpha")
	delete(manifest.Commands, "stable.leaf")
	command := syntheticCommand("stable.check", "check", "forge check", true)
	command.Flags = map[string]climanifest.Flag{
		"flag.alpha": {
			ID: "flag.alpha", Long: "alpha", Shorthand: "a", Aliases: []string{"alternate-alpha"},
			Scope: "local", ValueType: "bool", NoOptionDefault: "true", Visibility: "visible",
		},
		"flag.beta": {ID: "flag.beta", Long: "beta", Scope: "local", ValueType: "bool", NoOptionDefault: "true", Visibility: "visible"},
	}
	withActiveFlagLifecycle(command.Flags)
	command.Arguments = map[string]climanifest.Argument{
		"arg.target": {
			ID: "arg.target", Name: "target", Position: 0, Kind: "positional",
			ValueType: "string", MinCardinality: 0, MaxCardinality: 1,
		},
	}
	withNoneArgumentCompletion(command.Arguments)
	command.Relationships = map[string]climanifest.Relationship{relationship.ID: relationship}
	manifest.Commands[command.ID] = command
	return manifest
}

func activeLifecycle(id string) climanifest.Lifecycle {
	return climanifest.Lifecycle{
		FormatVersion: "1.0.0",
		ItemID:        id,
		State:         "active",
		Since:         "1.0.0",
	}
}

func updateSyntheticArgument(
	manifest *climanifest.Manifest,
	inputID string,
	update func(*climanifest.Argument),
) {
	command := manifest.Commands["stable.check"]
	argument := command.Arguments[inputID]
	update(&argument)
	command.Arguments[inputID] = argument
	manifest.Commands[command.ID] = command
}

func flagRef(id string) climanifest.ParticipantRef {
	return climanifest.ParticipantRef{Type: "flag", ID: id}
}

func argumentRef(id string) climanifest.ParticipantRef {
	return climanifest.ParticipantRef{Type: "argument", ID: id}
}

func groupRelationship(id, kind string, participants ...climanifest.ParticipantRef) climanifest.Relationship {
	return climanifest.Relationship{ID: id, Kind: kind, Participants: participants}
}

func directedRelationship(
	id, kind string,
	when climanifest.ParticipantRef,
	participants ...climanifest.ParticipantRef,
) climanifest.Relationship {
	return climanifest.Relationship{ID: id, Kind: kind, When: &when, Participants: participants}
}
