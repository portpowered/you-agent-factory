package climanifestcobra_test

import (
	"context"
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
