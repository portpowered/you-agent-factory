package climanifestcobra_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
)

func TestNewCommandTreeProjectsSchemaHelpLifecycleAndCompletion(t *testing.T) {
	manifest := syntheticPresentationManifest()
	dynamicCalls := 0
	bindings := genericBindingsForManifest(manifest)
	bindings.Completions = climanifestcobra.CompletionRegistry{
		"stable.alpha.flag.cluster": func(
			*cobra.Command,
			[]string,
			string,
		) ([]cobra.Completion, cobra.ShellCompDirective) {
			dynamicCalls++
			return []string{"cluster-b", "cluster-a"}, cobra.ShellCompDirectiveNoFileComp
		},
		"stable.alpha.arg.worker": func(
			*cobra.Command,
			[]string,
			string,
		) ([]cobra.Completion, cobra.ShellCompDirective) {
			dynamicCalls++
			return []string{"worker-2", "worker-1"}, cobra.ShellCompDirectiveNoFileComp
		},
	}
	root, err := climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	alpha, err := findCommandByPath(root, "forge alpha")
	if err != nil {
		t.Fatal(err)
	}
	assertProjectedHelpAndLifecycle(t, alpha)
	assertProjectedFlagCompletion(t, alpha, &dynamicCalls)
	assertProjectedArgumentCompletion(t, alpha, &dynamicCalls)
}

func TestNewCommandTreeProjectsRootNoArgumentHelpWithoutDispatch(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	bindings := genericBindingsForManifest(manifest)
	handlerCalls := 0
	for handlerID := range bindings.Handlers {
		bindings.Handlers[handlerID] = func(context.Context, map[string]any) error {
			handlerCalls++
			return nil
		}
	}
	root, err := climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	if handlerCalls != 0 {
		t.Fatalf("root handler calls = %d, want 0", handlerCalls)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, expected := range []string{
		"Run and manage CPN-based workflow factories",
		"Available Commands:",
		"session",
		"--server",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout omitted %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "How to use:") {
		t.Fatalf("stdout included long-form help instead of concise discovery help:\n%s", output)
	}
}

func TestNewCommandTreeProjectsMatchingInheritedPresentation(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*climanifest.Flag)
		wantHidden bool
		wantNotice string
	}{
		{name: "active"},
		{
			name: "deprecated",
			mutate: func(flag *climanifest.Flag) {
				flag.Lifecycle = deprecatedLifecycle(flag.ID, "stable.alpha.flag.cluster", "use --cluster instead")
			},
			wantNotice: "use --cluster instead",
		},
		{
			name: "hidden",
			mutate: func(flag *climanifest.Flag) {
				flag.Visibility = "hidden"
			},
			wantHidden: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticPresentationManifest()
			if test.mutate != nil {
				updatePresentationFlag(&manifest, "stable.root.flag.region", test.mutate)
				updatePresentationFlag(&manifest, "stable.alpha.flag.region", test.mutate)
			}
			root, err := climanifestcobra.NewCommandTree(manifest, presentationBindings(manifest))
			if err != nil {
				t.Fatalf("NewCommandTree() error = %v", err)
			}
			alpha, err := findCommandByPath(root, "forge alpha")
			if err != nil {
				t.Fatal(err)
			}
			region := alpha.Flag("region")
			if region == nil || region.Hidden != test.wantHidden || region.Deprecated != test.wantNotice {
				t.Fatalf("inherited region = %#v, want hidden=%t deprecated=%q", region, test.wantHidden, test.wantNotice)
			}
			var output bytes.Buffer
			alpha.SetOut(&output)
			if err := alpha.Help(); err != nil {
				t.Fatalf("Help() error = %v", err)
			}
			hasRegion := strings.Contains(output.String(), "--region")
			if hasRegion == test.wantHidden {
				t.Fatalf("help contains --region = %t, want %t:\n%s", hasRegion, !test.wantHidden, output.String())
			}
		})
	}
}

func TestNewCommandTreeDispatchesByStableHandlerIDWithNormalizedInputs(t *testing.T) {
	manifest := syntheticFlagManifest()
	alpha := manifest.Commands["stable.alpha"]
	alpha.Name = "nova"
	alpha.Path = "forge nova"
	alpha.Usage.Line = "nova"
	alpha.Aliases = []string{"alpha"}
	manifest.Commands[alpha.ID] = alpha

	var received map[string]any
	bindings := genericBindingsForManifest(manifest)
	bindings.Handlers[alpha.Handler.ID] = func(_ context.Context, values map[string]any) error {
		received = values
		return nil
	}
	root, err := climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{"nova", "--nebula-label", "  normalized  "})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received["stable.alpha.flag.label"] != "normalized" {
		t.Fatalf("handler inputs = %#v, want normalized stable-ID value", received)
	}

	root, err = climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() after rename error = %v", err)
	}
	root.SetArgs([]string{"alpha", "--nebula-label", "alias"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(alias) error = %v", err)
	}
	if received["stable.alpha.flag.label"] != "alias" {
		t.Fatalf("alias handler inputs = %#v, want same stable handler dispatch", received)
	}
}

func TestNewCommandTreeKeepsNonRunnableCommandsOnCobraHelpPath(t *testing.T) {
	manifest := syntheticTreeManifest()
	calls := 0
	bindings := genericBindingsForManifest(manifest)
	for handlerID := range bindings.Handlers {
		bindings.Handlers[handlerID] = func(context.Context, map[string]any) error {
			calls++
			return nil
		}
	}
	root, err := climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetErr(output)
	root.SetArgs([]string{"zeta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(non-runnable) error = %v", err)
	}
	zeta, _, err := root.Find([]string{"zeta"})
	if err != nil {
		t.Fatalf("Find(zeta) error = %v", err)
	}
	if zeta.RunE != nil || calls != 0 {
		t.Fatalf("non-runnable command has RunE = %t, handler calls = %d", zeta.RunE != nil, calls)
	}
	if !strings.Contains(output.String(), "Zeta description") {
		t.Fatalf("non-runnable output = %q, want Cobra help", output.String())
	}
}

func TestNewCommandTreeRejectsInvalidHandlerBindingsBeforeExecution(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*climanifest.Manifest, *climanifestcobra.GenericBindings)
		wantErr string
	}{
		{
			name: "missing handler record",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				command := manifest.Commands["stable.alpha"]
				command.Handler = nil
				manifest.Commands[command.ID] = command
			},
			wantErr: `command "stable.alpha" runnable handler ID is required`,
		},
		{
			name: "empty handler ID",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				command := manifest.Commands["stable.alpha"]
				command.Handler.ID = ""
				manifest.Commands[command.ID] = command
			},
			wantErr: `command "stable.alpha" runnable handler ID is required`,
		},
		{
			name: "unknown handler ID",
			mutate: func(manifest *climanifest.Manifest, bindings *climanifestcobra.GenericBindings) {
				delete(bindings.Handlers, manifest.Commands["stable.alpha"].Handler.ID)
			},
			wantErr: `command "stable.alpha" handler ID "stable.alpha.handler" has no registered executable binding`,
		},
		{
			name: "duplicate handler ID",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				command := manifest.Commands["stable.leaf"]
				command.Handler.ID = "stable.alpha.handler"
				manifest.Commands[command.ID] = command
			},
			wantErr: `handler ID "stable.alpha.handler" duplicates runnable command`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticTreeManifest()
			bindings := genericBindingsForManifest(manifest)
			calls := 0
			for handlerID := range bindings.Handlers {
				bindings.Handlers[handlerID] = func(context.Context, map[string]any) error {
					calls++
					return nil
				}
			}
			test.mutate(&manifest, &bindings)
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if root != nil || err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewCommandTree() = (%v, %v), want nil and error containing %q", root, err, test.wantErr)
			}
			if calls != 0 {
				t.Fatalf("construction failure invoked %d handlers, want zero", calls)
			}
		})
	}
}

func TestNewCommandTreeRejectsRepeatedScalarArgumentValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
	}{
		{name: "booleans", valueType: "bool"},
		{name: "strings", valueType: "string"},
		{name: "integers", valueType: "int"},
		{name: "64-bit integers", valueType: "int64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticArgumentManifest()
			command := manifest.Commands["stable.shape"]
			command.Arguments = map[string]climanifest.Argument{
				"stable.shape.arg.values": {
					ID: "stable.shape.arg.values", Name: "values", Position: 0, Kind: "positional",
					ValueType: test.valueType, Variadic: true, MinCardinality: 0, MaxCardinality: -1,
					Completion: "none",
				},
			}
			withNoneArgumentCompletion(command.Arguments)
			manifest.Commands[command.ID] = command
			bindings := genericBindingsForManifest(manifest)
			calls := 0
			bindings.Handlers[command.Handler.ID] = func(_ context.Context, values map[string]any) error {
				calls++
				return nil
			}
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if root != nil || err == nil || !strings.Contains(err.Error(), "must use stringArray") {
				t.Fatalf("NewCommandTree() = (%v, %v), want repeated scalar rejection", root, err)
			}
			if calls != 0 {
				t.Fatalf("handler calls = %d, want 0", calls)
			}
		})
	}
}

func TestNewCommandTreeDispatchesScalarBooleanAndInt64Arguments(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
		arg       string
		want      any
	}{
		{name: "boolean", valueType: "bool", arg: "true", want: true},
		{name: "64-bit integer", valueType: "int64", arg: "9223372036854775000", want: int64(9223372036854775000)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticArgumentManifest()
			command := manifest.Commands["stable.shape"]
			command.Arguments = map[string]climanifest.Argument{
				"stable.shape.arg.value": {
					ID: "stable.shape.arg.value", Name: "value", Position: 0, Kind: "positional",
					ValueType: test.valueType, Required: true, MinCardinality: 1, MaxCardinality: 1,
					Completion: "none",
				},
			}
			withNoneArgumentCompletion(command.Arguments)
			manifest.Commands[command.ID] = command
			var received map[string]any
			bindings := genericBindingsForManifest(manifest)
			bindings.Handlers[command.Handler.ID] = func(_ context.Context, values map[string]any) error {
				received = values
				return nil
			}
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if err != nil {
				t.Fatalf("NewCommandTree() error = %v", err)
			}
			root.SetArgs([]string{"shape", test.arg})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !reflect.DeepEqual(received["stable.shape.arg.value"], test.want) {
				t.Fatalf("handler values = %#v, want %#v", received, test.want)
			}
		})
	}
}

func assertProjectedHelpAndLifecycle(t *testing.T, alpha *cobra.Command) {
	t.Helper()
	var output bytes.Buffer
	alpha.SetOut(&output)
	alpha.SetErr(&output)
	if err := alpha.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}
	help := output.String()
	for _, fragment := range []string{
		"Alpha title",
		"Alpha description",
		"alpha <target> [worker] [note]",
		"Aliases:",
		"alpha, a",
		"forge alpha red worker-1",
		"--cluster string",
		"--region string",
		"(default \"west\")",
		"DEPRECATED: use stable.beta",
		"--legacy-mode string",
		"(DEPRECATED: use --cluster instead)",
	} {
		if !strings.Contains(help, fragment) {
			t.Fatalf("help missing %q:\n%s", fragment, help)
		}
	}
	for _, hidden := range []string{"secret-code"} {
		if strings.Contains(help, hidden) {
			t.Fatalf("help unexpectedly contains hidden input %q:\n%s", hidden, help)
		}
	}
	if alpha.Deprecated != "use stable.beta" {
		t.Fatalf("command deprecation = %q, want successor guidance", alpha.Deprecated)
	}
	legacy := alpha.Flag("legacy-mode")
	if legacy == nil || legacy.Deprecated != "use --cluster instead" {
		t.Fatalf("legacy flag = %#v, want projected deprecation guidance", legacy)
	}
}

func assertProjectedFlagCompletion(t *testing.T, alpha *cobra.Command, dynamicCalls *int) {
	t.Helper()
	static, ok := alpha.GetFlagCompletionFunc("region")
	if !ok {
		t.Fatal("static inherited flag completion is not registered")
	}
	values, directive := static(alpha, nil, "")
	if !reflect.DeepEqual(values, []cobra.Completion{"east", "west"}) ||
		directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("static completion = (%v, %v), want sorted choices and no-file directive", values, directive)
	}
	dynamic, ok := alpha.GetFlagCompletionFunc("cluster")
	if !ok {
		t.Fatal("dynamic flag completion is not registered")
	}
	values, _ = dynamic(alpha, nil, "cluster")
	if !reflect.DeepEqual(values, []cobra.Completion{"cluster-b", "cluster-a"}) || *dynamicCalls != 1 {
		t.Fatalf("dynamic completion = %v calls=%d", values, *dynamicCalls)
	}
	if _, ok := alpha.GetFlagCompletionFunc("secret-code"); ok {
		t.Fatal("none completion unexpectedly registered a schema callback")
	}
}

func assertProjectedArgumentCompletion(t *testing.T, alpha *cobra.Command, dynamicCalls *int) {
	t.Helper()
	if alpha.ValidArgsFunction == nil {
		t.Fatal("argument completion function is not projected")
	}
	values, _ := alpha.ValidArgsFunction(alpha, nil, "")
	if !reflect.DeepEqual(values, []cobra.Completion{"blue", "red"}) {
		t.Fatalf("static argument completion = %v, want [blue red]", values)
	}
	values, _ = alpha.ValidArgsFunction(alpha, []string{"blue"}, "")
	if !reflect.DeepEqual(values, []cobra.Completion{"worker-2", "worker-1"}) || *dynamicCalls != 2 {
		t.Fatalf("dynamic argument completion = %v calls=%d", values, *dynamicCalls)
	}
	values, directive := alpha.ValidArgsFunction(alpha, []string{"blue", "worker-1"}, "")
	if len(values) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("none argument completion = (%v, %v), want no schema candidates", values, directive)
	}
}

func TestNewCommandTreeRejectsInvalidLifecycleAndCompletionBeforeProjection(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*climanifest.Manifest, *climanifestcobra.GenericBindings)
		wantErr string
	}{
		{
			name: "missing command lifecycle",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				command := manifest.Commands["stable.alpha"]
				command.Lifecycle = climanifest.Lifecycle{}
				manifest.Commands[command.ID] = command
			},
			wantErr: `command "stable.alpha" lifecycle`,
		},
		{
			name: "unsupported removed command",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				command := manifest.Commands["stable.alpha"]
				command.Lifecycle.State = "removed"
				manifest.Commands[command.ID] = command
			},
			wantErr: `unsupported lifecycle state "removed"`,
		},
		{
			name: "incomplete flag lifecycle",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				updatePresentationFlag(manifest, "stable.alpha.flag.cluster", func(flag *climanifest.Flag) {
					flag.Lifecycle = climanifest.Lifecycle{}
				})
			},
			wantErr: `input "stable.alpha.flag.cluster": lifecycle`,
		},
		{
			name: "static completion without choices",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				updatePresentationFlag(manifest, "stable.root.flag.region", func(flag *climanifest.Flag) {
					flag.Enum = nil
				})
				updatePresentationFlag(manifest, "stable.alpha.flag.region", func(flag *climanifest.Flag) {
					flag.Enum = nil
				})
			},
			wantErr: `input "stable.root.flag.region": static completion requires declared choices`,
		},
		{
			name: "inherited lifecycle differs from declaration",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				updatePresentationFlag(manifest, "stable.alpha.flag.region", func(flag *climanifest.Flag) {
					flag.Lifecycle = deprecatedLifecycle(flag.ID, "stable.alpha.flag.cluster", "use --cluster instead")
				})
			},
			wantErr: `input "stable.alpha.flag.region": inheritance target "stable.root.flag.region" has incompatible flag metadata`,
		},
		{
			name: "inherited visibility differs from declaration",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				updatePresentationFlag(manifest, "stable.alpha.flag.region", func(flag *climanifest.Flag) {
					flag.Visibility = "hidden"
				})
			},
			wantErr: `input "stable.alpha.flag.region": inheritance target "stable.root.flag.region" has incompatible flag metadata`,
		},
		{
			name: "missing dynamic binding",
			mutate: func(_ *climanifest.Manifest, bindings *climanifestcobra.GenericBindings) {
				delete(bindings.Completions, "stable.alpha.flag.cluster")
			},
			wantErr: `input "stable.alpha.flag.cluster": missing dynamic completion binding`,
		},
		{
			name: "unsupported completion mode",
			mutate: func(manifest *climanifest.Manifest, _ *climanifestcobra.GenericBindings) {
				updatePresentationFlag(manifest, "stable.alpha.flag.cluster", func(flag *climanifest.Flag) {
					flag.Completion = "filesystem"
				})
			},
			wantErr: `input "stable.alpha.flag.cluster": unsupported completion mode "filesystem"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticPresentationManifest()
			bindings := presentationBindings(manifest)
			test.mutate(&manifest, &bindings)
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if root != nil || err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewCommandTree() = (%v, %v), want nil and error containing %q", root, err, test.wantErr)
			}
		})
	}
}

func syntheticPresentationManifest() climanifest.Manifest {
	manifest := syntheticTreeManifest()
	delete(manifest.Commands, "stable.zeta")
	delete(manifest.Commands, "stable.leaf")
	root := manifest.Commands["stable.root"]
	root.Flags = map[string]climanifest.Flag{
		"stable.root.flag.region": {
			ID: "stable.root.flag.region", Long: "region", Scope: "persistent", ValueType: "string",
			Enum: []string{"west", "east"}, Default: "west", Completion: "static", Visibility: "visible",
		},
	}
	withActiveFlagLifecycle(root.Flags)
	manifest.Commands[root.ID] = root

	alpha := manifest.Commands["stable.alpha"]
	alpha.Usage.Line = "alpha <target> [worker] [note]"
	alpha.Usage.Example = ""
	alpha.Documentation.Examples = []string{"forge alpha red worker-1"}
	alpha.Lifecycle = deprecatedLifecycle(alpha.ID, "stable.beta", "use stable.beta")
	alpha.Flags = presentationFlags()
	alpha.Arguments = presentationArguments()
	manifest.Commands[alpha.ID] = alpha
	return manifest
}

func presentationFlags() map[string]climanifest.Flag {
	flags := map[string]climanifest.Flag{
		"stable.alpha.flag.region": {
			ID: "stable.alpha.flag.region", Long: "region", Scope: "inherited",
			InheritedFromID: "stable.root.flag.region", ValueType: "string",
			Enum: []string{"west", "east"}, Default: "west", Completion: "static", Visibility: "visible",
		},
		"stable.alpha.flag.cluster": {
			ID: "stable.alpha.flag.cluster", Long: "cluster", Scope: "local",
			ValueType: "string", Completion: "dynamic", Visibility: "visible",
		},
		"stable.alpha.flag.secret": {
			ID: "stable.alpha.flag.secret", Long: "secret-code", Scope: "local",
			ValueType: "string", Completion: "none", Visibility: "hidden",
		},
		"stable.alpha.flag.legacy": {
			ID: "stable.alpha.flag.legacy", Long: "legacy-mode", Scope: "local",
			ValueType: "string", Completion: "none", Visibility: "visible",
		},
	}
	withActiveFlagLifecycle(flags)
	legacy := flags["stable.alpha.flag.legacy"]
	legacy.Lifecycle = deprecatedLifecycle(legacy.ID, "stable.alpha.flag.cluster", "use --cluster instead")
	flags[legacy.ID] = legacy
	return flags
}

func presentationArguments() map[string]climanifest.Argument {
	arguments := map[string]climanifest.Argument{
		"stable.alpha.arg.target": {
			ID: "stable.alpha.arg.target", Name: "target", Position: 0, Kind: "positional",
			ValueType: "string", Required: true, MinCardinality: 1, MaxCardinality: 1,
			Enum: []string{"red", "blue"}, Completion: "static",
		},
		"stable.alpha.arg.worker": {
			ID: "stable.alpha.arg.worker", Name: "worker", Position: 1, Kind: "positional",
			ValueType: "string", MinCardinality: 0, MaxCardinality: 1, Completion: "dynamic",
		},
		"stable.alpha.arg.note": {
			ID: "stable.alpha.arg.note", Name: "note", Position: 2, Kind: "positional",
			ValueType: "string", MinCardinality: 0, MaxCardinality: 1, Completion: "none",
		},
	}
	withNoneArgumentCompletion(arguments)
	arguments["stable.alpha.arg.target"] = withArgumentCompletion(arguments["stable.alpha.arg.target"], "static")
	arguments["stable.alpha.arg.worker"] = withArgumentCompletion(arguments["stable.alpha.arg.worker"], "dynamic")
	return arguments
}

func withArgumentCompletion(argument climanifest.Argument, completion string) climanifest.Argument {
	argument.Completion = completion
	return argument
}

func presentationBindings(manifest climanifest.Manifest) climanifestcobra.GenericBindings {
	callback := func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	bindings := genericBindingsForManifest(manifest)
	bindings.Completions = climanifestcobra.CompletionRegistry{
		"stable.alpha.flag.cluster": callback,
		"stable.alpha.arg.worker":   callback,
	}
	return bindings
}

func genericBindingsForManifest(manifest climanifest.Manifest) climanifestcobra.GenericBindings {
	handlers := make(climanifestcobra.HandlerRegistry)
	for _, command := range manifest.Commands {
		if command.Handler != nil && command.Handler.ID != "" {
			handlers[command.Handler.ID] = func(context.Context, map[string]any) error { return nil }
		}
	}
	return climanifestcobra.GenericBindings{Handlers: handlers}
}

func TestInheritedFlagGroupsStayCommandLocalAcrossRootAndSiblings(t *testing.T) {
	tests := []struct {
		kind           string
		unrelatedArgs  []string
		completionArgs []string
	}{
		{kind: "mutually-exclusive", unrelatedArgs: []string{"--mode", "--other"}, completionArgs: []string{"--mode", "--"}},
		{kind: "conflict", unrelatedArgs: []string{"--mode", "--other"}, completionArgs: []string{"--mode", "--"}},
		{kind: "required-together", unrelatedArgs: []string{"--mode"}, completionArgs: []string{"--mode", "--"}},
		{kind: "at-least-one", completionArgs: []string{"--"}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			assertUnrelatedRelationshipInvocation(t, test.kind, "", test.unrelatedArgs)
			assertUnrelatedRelationshipInvocation(t, test.kind, "zeta", test.unrelatedArgs)
			assertUnrelatedRelationshipCompletion(t, test.kind, "zeta", test.completionArgs)
		})
	}
}

func assertUnrelatedRelationshipInvocation(t *testing.T, kind, command string, args []string) {
	t.Helper()
	manifest := inheritedRelationshipIsolationManifest(kind)
	calls := 0
	bindings := genericBindingsForManifest(manifest)
	for id := range bindings.Handlers {
		bindings.Handlers[id] = func(context.Context, map[string]any) error {
			calls++
			return nil
		}
	}
	root, err := climanifestcobra.NewCommandTree(manifest, bindings)
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	invocation := append(make([]string, 0, len(args)+1), args...)
	if command != "" {
		invocation = append([]string{command}, invocation...)
	}
	root.SetArgs(invocation)
	if err := root.Execute(); err != nil || calls != 1 {
		t.Fatalf("unrelated %q invocation = (error %v, calls %d), want nil and one handler call", command, err, calls)
	}
}

func assertUnrelatedRelationshipCompletion(t *testing.T, kind, command string, args []string) {
	t.Helper()
	manifest := inheritedRelationshipIsolationManifest(kind)
	control := inheritedRelationshipIsolationManifest(kind)
	alpha := control.Commands["stable.alpha"]
	alpha.Relationships = nil
	control.Commands[alpha.ID] = alpha
	got := commandCompletionOutput(t, manifest, command, args)
	want := commandCompletionOutput(t, control, command, args)
	if got != want {
		t.Fatalf("unrelated %q completion with %s relationship = %q, want control %q", command, kind, got, want)
	}
}

func commandCompletionOutput(t *testing.T, manifest climanifest.Manifest, command string, args []string) string {
	t.Helper()
	root, err := climanifestcobra.NewCommandTree(manifest, genericBindingsForManifest(manifest))
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	completionArgs := append([]string{"__complete", command}, args...)
	root.SetArgs(completionArgs)
	if err := root.Execute(); err != nil {
		t.Fatalf("completion %v error = %v", completionArgs, err)
	}
	return output.String()
}

func inheritedRelationshipIsolationManifest(kind string) climanifest.Manifest {
	manifest := syntheticTreeManifest()
	delete(manifest.Commands, "stable.leaf")
	root := syntheticCommand("stable.root", "forge", "forge", true)
	root.Flags = map[string]climanifest.Flag{
		"stable.root.flag.mode":  relationshipIsolationFlag("stable.root.flag.mode", "persistent", "", "mode"),
		"stable.root.flag.other": relationshipIsolationFlag("stable.root.flag.other", "local", "", "other"),
	}
	manifest.Commands[root.ID] = root

	alpha := manifest.Commands["stable.alpha"]
	alpha.Flags = map[string]climanifest.Flag{
		"stable.alpha.flag.mode":  relationshipIsolationFlag("stable.alpha.flag.mode", "inherited", "stable.root.flag.mode", "mode"),
		"stable.alpha.flag.other": relationshipIsolationFlag("stable.alpha.flag.other", "local", "", "other"),
	}
	relationship := groupRelationship(
		"stable.alpha.relationship.inherited",
		kind,
		flagRef("stable.alpha.flag.mode"),
		flagRef("stable.alpha.flag.other"),
	)
	alpha.Relationships = map[string]climanifest.Relationship{relationship.ID: relationship}
	manifest.Commands[alpha.ID] = alpha

	zeta := syntheticCommand("stable.zeta", "zeta", "forge zeta", true)
	zeta.Flags = map[string]climanifest.Flag{
		"stable.zeta.flag.other": relationshipIsolationFlag("stable.zeta.flag.other", "local", "", "other"),
	}
	manifest.Commands[zeta.ID] = zeta
	return manifest
}

func relationshipIsolationFlag(id, scope, inheritedFromID, long string) climanifest.Flag {
	return climanifest.Flag{
		ID: id, Long: long, Scope: scope, InheritedFromID: inheritedFromID,
		ValueType: "bool", NoOptionDefault: "true", Completion: "none",
		Visibility: "visible", Lifecycle: activeLifecycle(id),
	}
}

func deprecatedLifecycle(id, target, guidance string) climanifest.Lifecycle {
	return climanifest.Lifecycle{
		FormatVersion: "1.0.0",
		ItemID:        id,
		State:         "deprecated",
		Since:         "1.0.0",
		Deprecated:    "2.0.0",
		Successor: &climanifest.LifecycleSuccessor{
			TargetItemID:     target,
			CanonicalEnglish: guidance,
		},
	}
}

func updatePresentationFlag(
	manifest *climanifest.Manifest,
	inputID string,
	update func(*climanifest.Flag),
) {
	for commandID, command := range manifest.Commands {
		flag, ok := command.Flags[inputID]
		if !ok {
			continue
		}
		update(&flag)
		command.Flags[inputID] = flag
		manifest.Commands[commandID] = command
		return
	}
}

func TestNewRunSubmitFamilyComponentsBuildsDetachedContractedTree(t *testing.T) {
	components := mustRunSubmitFamilyComponents(t)
	if components.Run.Parent() != nil || components.Submit.Parent() != nil {
		t.Fatal("run and submit components must remain detached from the shared root")
	}
	if components.SubmitBatch.Parent() != components.Submit {
		t.Fatal("submit batch must be attached only beneath submit")
	}
	if !components.Run.DisableFlagParsing || !components.Run.SilenceErrors {
		t.Fatal("generated run must preserve custom parser and silence-errors metadata")
	}
	if !strings.Contains(components.Run.Example, "you run --work") || strings.Contains(components.Run.Example, "session pause") {
		t.Fatalf("generated run examples do not describe run behavior:\n%s", components.Run.Example)
	}
	for _, cmd := range []*cobra.Command{components.Run, components.Submit, components.SubmitBatch} {
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("%s missing handwritten lifecycle", cmd.CommandPath())
		}
	}
}

func TestNewRunSubmitFamilyComponentsRegistersLocalFlagsWithoutChangingHandlerValidation(t *testing.T) {
	components := mustRunSubmitFamilyComponents(t)
	for _, flagName := range []string{
		"continuously", "work", "dir", "named", "factory", "record", "no-record",
		"replay", "runtime-log-dir", "runtime-log-max-size-mb", "runtime-log-max-backups",
		"runtime-log-max-age-days", "runtime-log-compress", "runtime-metrics-dir",
		"runtime-metrics-max-size-mb", "runtime-metrics-max-backups",
		"runtime-metrics-max-age-days", "runtime-metrics-compress", "with-mock-workers",
		"quiet", "output", "skip-permissions", "port",
	} {
		if components.Run.Flags().Lookup(flagName) == nil {
			t.Fatalf("generated run missing local flag %q", flagName)
		}
	}
	if flag := components.Run.Flags().Lookup("with-mock-workers"); flag == nil || flag.NoOptDefVal == "" {
		t.Fatalf("with-mock-workers no-option contract = %#v", flag)
	}
	for _, flagName := range []string{"name", "work-type-name", "payload", "session", "port"} {
		if components.Submit.Flags().Lookup(flagName) == nil {
			t.Fatalf("generated submit missing local flag %q", flagName)
		}
	}
	if err := components.Submit.ValidateRequiredFlags(); err != nil {
		t.Fatalf("submit Cobra validation = %v, want handwritten handler to retain required-input validation", err)
	}
	for _, flagName := range []string{"file", "dry-run", "session", "port"} {
		if components.SubmitBatch.Flags().Lookup(flagName) == nil {
			t.Fatalf("generated submit batch missing local flag %q", flagName)
		}
	}
	if components.SubmitBatch.Args != nil {
		t.Fatal("submit batch Cobra Args validation should remain in the handwritten input resolver")
	}
}

func TestNewRunSubmitFamilyComponentsRejectsMissingAndOutOfFamilyBindings(t *testing.T) {
	bindings := testRunSubmitBindings()
	if _, err := climanifestcobra.NewRunSubmitFamilyComponents(nil, bindings); err == nil {
		t.Fatal("nil registry = nil, want error")
	}
	registry := mustRunSubmitRegistry(t)
	bindings.SubmitBatch = nil
	if _, err := climanifestcobra.NewRunSubmitFamilyComponents(registry, bindings); err == nil {
		t.Fatal("missing submit batch binding = nil, want error")
	}

	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.work.list"] = manifest.Commands["you.run"]
	delete(manifest.Commands, "you.run")
	if _, err := climanifestcobra.NewRunSubmitFamilyComponentsFromManifest(
		manifest,
		registry,
		testRunSubmitBindings(),
	); err == nil {
		t.Fatal("out-of-family manifest command = nil, want error")
	}
}

func mustRunSubmitFamilyComponents(t *testing.T) climanifestcobra.RunSubmitFamilyComponents {
	t.Helper()
	components, err := climanifestcobra.NewRunSubmitFamilyComponents(
		mustRunSubmitRegistry(t),
		testRunSubmitBindings(),
	)
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyComponents() error = %v", err)
	}
	return components
}

func mustRunSubmitRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	preRun := func(*cobra.Command, []string) error { return nil }
	registry, err := commandregistry.NewRunSubmitRegistry(commandregistry.RunSubmitHandlers{
		Run:         commandregistry.CommandHandlers{PreRunE: preRun, RunE: noopRunE},
		Submit:      commandregistry.CommandHandlers{PreRunE: preRun, RunE: noopRunE},
		SubmitBatch: commandregistry.CommandHandlers{PreRunE: preRun, RunE: noopRunE},
	})
	if err != nil {
		t.Fatalf("NewRunSubmitRegistry() error = %v", err)
	}
	return registry
}

func testRunSubmitBindings() climanifestcobra.RunSubmitFlagBindings {
	runConfig := &runcli.RunConfig{}
	output := ""
	return climanifestcobra.RunSubmitFlagBindings{
		Run:                 runConfig,
		RunInvocationOutput: &output,
		Submit:              &submitcli.SubmitConfig{Context: context.Background()},
		SubmitBatch:         &submitcli.BatchConfig{Context: context.Background()},
	}
}

func replaceFlagSpelling(args []string, spelling string) []string {
	replaced := append([]string(nil), args...)
	for index, arg := range replaced {
		if arg == "--alpha" {
			replaced[index] = spelling
		}
	}
	return replaced
}
