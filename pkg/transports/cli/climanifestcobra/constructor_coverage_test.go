package climanifestcobra_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
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
		Create: &sessioncli.CreateConfig{}, List: &sessioncli.ListConfig{}, Delete: &sessioncli.DeleteConfig{},
		Dispatches: &sessioncli.DispatchesConfig{}, Pause: &sessioncli.LifecycleControlConfig{}, Resume: &sessioncli.LifecycleControlConfig{},
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
	show, err := climanifestparity.FindCommandByPath(root, "you session show")
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
	if _, err := climanifestparity.FindCommandByPath(root, "you session show"); err != nil {
		t.Fatalf("generated from-manifest tree missing session show: %v", err)
	}
}
