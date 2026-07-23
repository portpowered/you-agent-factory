package climanifestcobra_test

import (
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

	root, err := climanifestcobra.NewCommandTree(manifest)
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
	if alpha.Short != "Alpha title" || alpha.Long != "Alpha description" {
		t.Fatalf("alpha documentation = (%q, %q)", alpha.Short, alpha.Long)
	}
	if len(alpha.Aliases) != 1 || alpha.Aliases[0] != "a" {
		t.Fatalf("alpha aliases = %v, want [a]", alpha.Aliases)
	}
	if !alpha.Runnable() {
		t.Fatal("schema-runnable alpha command is not runnable")
	}
	if err := alpha.RunE(alpha, nil); err == nil || !strings.Contains(err.Error(), `command "stable.alpha"`) {
		t.Fatalf("unbound runnable error = %v", err)
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
	return climanifest.Command{
		ID:         id,
		Name:       name,
		Path:       path,
		Visibility: "visible",
		Runnable:   runnable,
		Usage:      climanifest.Usage{Line: name},
		Documentation: climanifest.Documentation{
			Documentation: climanifest.DocumentationCopy{
				Title:       climanifest.DocumentationField{CanonicalEnglish: titleName + " title"},
				Description: climanifest.DocumentationField{CanonicalEnglish: titleName + " description"},
			},
		},
	}
}

func commandNames(commands []*cobra.Command) []string {
	names := make([]string, len(commands))
	for index, command := range commands {
		names[index] = command.Name()
	}
	return names
}
