package climanifestcobra_test

import (
	"testing"

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
