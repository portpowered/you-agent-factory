package climanifestcobra_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

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
