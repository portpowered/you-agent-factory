package climanifestcobra_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
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
	bindings := climanifestcobra.GenericBindings{
		Completions: climanifestcobra.CompletionRegistry{
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
			},
			wantErr: `input "stable.root.flag.region": static completion requires declared choices`,
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
			bindings := presentationBindings()
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
	return map[string]climanifest.Argument{
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
}

func presentationBindings() climanifestcobra.GenericBindings {
	callback := func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return climanifestcobra.GenericBindings{
		Completions: climanifestcobra.CompletionRegistry{
			"stable.alpha.flag.cluster": callback,
			"stable.alpha.arg.worker":   callback,
		},
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

func TestNewSessionFamilyCommandBuildsCanonicalRunnableLeaves(t *testing.T) {
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: noopRunE, ListRunE: noopRunE, ShowRunE: noopRunE,
		DeleteRunE: noopRunE, PauseRunE: noopRunE, ResumeRunE: noopRunE, DispatchesRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewSessionRegistry() error = %v", err)
	}
	session, err := climanifestcobra.NewSessionFamilyCommand(registry, testSessionBindings())
	if err != nil {
		t.Fatalf("NewSessionFamilyCommand() error = %v", err)
	}
	want := []string{"create", "delete", "dispatches", "list", "pause", "resume", "show"}
	children := session.Commands()
	if len(children) != len(want) {
		t.Fatalf("session children = %d, want %d", len(children), len(want))
	}
	for i, name := range want {
		if children[i].Name() != name || children[i].RunE == nil {
			t.Fatalf("session child[%d] = %q runnable=%t, want %q runnable", i, children[i].Name(), children[i].RunE != nil, name)
		}
	}
}

func TestNewSessionFamilyCommandAppliesCreateFlagContracts(t *testing.T) {
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: noopRunE, ListRunE: noopRunE, ShowRunE: noopRunE,
		DeleteRunE: noopRunE, PauseRunE: noopRunE, ResumeRunE: noopRunE, DispatchesRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewSessionRegistry() error = %v", err)
	}
	session, err := climanifestcobra.NewSessionFamilyCommand(registry, testSessionBindings())
	if err != nil {
		t.Fatalf("NewSessionFamilyCommand() error = %v", err)
	}
	create, _, err := session.Find([]string{"create"})
	if err != nil {
		t.Fatalf("Find(create) error = %v", err)
	}
	if err := create.ParseFlags([]string{"--init-new-factory", "--validate-only"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := create.ValidateRequiredFlags(); err == nil || !strings.Contains(err.Error(), "dir") {
		t.Fatalf("ValidateRequiredFlags() error = %v, want required dir", err)
	}
	if err := create.Flags().Set("dir", "."); err != nil {
		t.Fatalf("Set(dir) error = %v", err)
	}
	if err := create.ValidateFlagGroups(); err == nil || !strings.Contains(err.Error(), "none of the others") {
		t.Fatalf("ValidateFlagGroups() error = %v, want mutex rejection", err)
	}
}

func testSessionBindings() climanifestcobra.SessionFamilyBindings {
	return climanifestcobra.SessionFamilyBindings{
		Create: &sessioncli.CreateConfig{}, List: &sessioncli.ListConfig{Context: context.Background()}, Delete: &sessioncli.DeleteConfig{},
		Dispatches: &sessioncli.DispatchesConfig{Context: context.Background()}, Pause: &sessioncli.LifecycleControlConfig{Context: context.Background()}, Resume: &sessioncli.LifecycleControlConfig{Context: context.Background()},
	}
}

func TestNewRepresentativeFamilyComponentsReturnsDetachedCommands(t *testing.T) {
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}

	components, err := climanifestcobra.NewRepresentativeFamilyComponents(registry, testBindings())
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyComponents() error = %v", err)
	}
	if components.Root == nil || components.Session == nil || components.Show == nil {
		t.Fatalf("components = %#v, want detached root/session/show commands", components)
	}
	if components.Session.Parent() != nil || components.Show.Parent() != nil {
		t.Fatal("detached components must not be attached before production wiring")
	}
}

func TestRejectDeprecatedPortFlagRejectsChangedPort(t *testing.T) {
	root, _ := mustRepresentativeFamilyTree(t)
	show, err := findCommandByPath(root, "you session show")
	if err != nil {
		t.Fatalf("FindCommandByPath(you session show) error = %v", err)
	}
	if show.PreRunE == nil {
		t.Fatal("session show must wire deprecated --port PreRunE")
	}
	if err := show.ParseFlags([]string{"--port", "7437"}); err != nil {
		t.Fatalf("ParseFlags(--port) error = %v", err)
	}
	if err := show.PreRunE(show, nil); err == nil {
		t.Fatal("PreRunE(--port) error = nil, want deprecated flag rejection")
	} else if !strings.Contains(err.Error(), "--server") {
		t.Fatalf("PreRunE error = %v, want deprecated --port guidance", err)
	}
}

func TestNewRepresentativeFamilyCommandRejectsIncompleteBindings(t *testing.T) {
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewRepresentativeFamilyCommand(registry, climanifestcobra.PersistentFlagBindings{}); err == nil {
		t.Fatal("NewRepresentativeFamilyCommand() incomplete bindings = nil, want error")
	}
}

func TestNewRepresentativeFamilyCommandUsesFlagUsagesBridge(t *testing.T) {
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	bindings := testBindings()
	bindings.FlagUsages = map[string]string{
		"verbose": "emit concise command diagnostics to stderr",
	}
	root, err := climanifestcobra.NewRepresentativeFamilyCommand(registry, bindings)
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyCommand() error = %v", err)
	}
	verbose := root.PersistentFlags().Lookup("verbose")
	if verbose == nil || verbose.Usage != bindings.FlagUsages["verbose"] {
		t.Fatalf("verbose usage = %q, want flag usages bridge", verbose.Usage)
	}
}

func TestNewRepresentativeFamilyCommandFromManifestRejectsRunnableSessionParent(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	session := manifest.Commands["you.session"]
	session.Runnable = true
	manifest.Commands["you.session"] = session

	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewRepresentativeFamilyCommandFromManifest(manifest, registry, testBindings()); err == nil {
		t.Fatal("NewRepresentativeFamilyCommandFromManifest() runnable session = nil, want error")
	}
}

func TestNewRepresentativeFamilyCommandFromManifestRejectsWrongFamilyCardinality(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.session.list"] = climanifest.Command{ID: "you.session.list", Path: "you session list"}

	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewRepresentativeFamilyCommandFromManifest(manifest, registry, testBindings()); err == nil {
		t.Fatal("NewRepresentativeFamilyCommandFromManifest() extra command = nil, want error")
	}
}

func TestNewRepresentativeFamilyCommandRejectsNilRegistry(t *testing.T) {
	if _, err := climanifestcobra.NewRepresentativeFamilyCommand(nil, testBindings()); err == nil {
		t.Fatal("NewRepresentativeFamilyCommand() nil registry = nil, want error")
	}
}

func TestNewRepresentativeFamilyCommandFromManifestBuildsDetachedTree(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        noopRunE,
		SessionShowRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	root, err := climanifestcobra.NewRepresentativeFamilyCommandFromManifest(manifest, registry, testBindings())
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyCommandFromManifest() error = %v", err)
	}
	if _, err := findCommandByPath(root, "you session show"); err != nil {
		t.Fatalf("generated from-manifest tree missing session show: %v", err)
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
