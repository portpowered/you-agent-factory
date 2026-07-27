package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewWorkFamilyCommandBuildsContractedPaths(t *testing.T) {
	work, registry := mustWorkFamilyTree(t)

	if work.Name() != "work" {
		t.Fatalf("work name = %q, want work", work.Name())
	}
	if len(work.Commands()) != 4 {
		t.Fatalf("work child count = %d, want 4 runnable leaves", len(work.Commands()))
	}
	if work.Runnable() {
		t.Fatal("you work must remain non-runnable")
	}
	if work.RunE != nil {
		t.Fatal("you work must not attach RunE")
	}

	for _, path := range []string{
		"work list",
		"work show",
		"work move",
		"work visualize",
	} {
		cmd, err := findCommandByPath(work, path)
		if err != nil {
			t.Fatalf("FindCommandByPath(%q) error = %v", path, err)
		}
		if !cmd.Runnable() {
			t.Fatalf("%q must be runnable", path)
		}
		if cmd.RunE == nil {
			t.Fatalf("%q must attach handwritten RunE", path)
		}
	}

	list, err := findCommandByPath(work, "work list")
	if err != nil {
		t.Fatalf("FindCommandByPath(work list) error = %v", err)
	}
	handler, err := registry.Lookup("you.work.list")
	if err != nil {
		t.Fatalf("Lookup(you.work.list) error = %v", err)
	}
	if list.RunE == nil || handler == nil {
		t.Fatal("work list handler must resolve through registry")
	}
}

func TestNewWorkFamilyCommandRejectsOutOfFamilyManifestCommand(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.work.submit"] = manifest.Commands["you.work.list"]
	delete(manifest.Commands, "you.work.list")

	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	_, err = climanifestcobra.NewWorkFamilyCommandFromManifest(manifest, registry, testWorkBindings())
	if err == nil {
		t.Fatal("NewWorkFamilyCommandFromManifest() error = nil, want out-of-family rejection")
	}
}

func TestNewWorkFamilyCommandRejectsMissingHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if _, err := climanifestcobra.NewWorkFamilyCommand(registry, testWorkBindings()); err == nil {
		t.Fatal("NewWorkFamilyCommand() missing work handlers = nil, want error")
	}
}

func TestNewWorkFamilyCommandExposesOnlyWorkFamily(t *testing.T) {
	work, _ := mustWorkFamilyTree(t)
	for _, id := range climanifestgen.WorkFamilyCommandIDs {
		if id == "you.work" {
			continue
		}
		if _, err := findCommandByPath(work, workPathForID(id)); err != nil {
			t.Fatalf("work path for %q missing: %v", id, err)
		}
	}
	if _, err := findCommandByPath(work, "work submit"); err == nil {
		t.Fatal("generated work constructor must not expose work submit")
	}
}

func TestNewWorkFamilyCommandRegistersContractedFlagsAndArgs(t *testing.T) {
	work, _ := mustWorkFamilyTree(t)
	assertWorkShowContractedFlags(t, work)
	assertWorkListContractedFlags(t, work)
	assertWorkVisualizeContractedFlags(t, work)
}

func TestNewWorkFamilyCommandAppliesManifestFlagUsages(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	manifest := mustWorkFamilyManifest(t)
	listRecord := manifest.Commands["you.work.list"]
	listFlag := listRecord.Flags["you.work.list.flag.session"]
	listFlag.Usage = "custom session help from manifest"
	listRecord.Flags[listFlag.ID] = listFlag
	manifest.Commands[listRecord.ID] = listRecord
	visualizeRecord := manifest.Commands["you.work.visualize"]
	formatFlag := visualizeRecord.Flags["you.work.visualize.flag.format"]
	formatFlag.Usage = "custom visualize format help from manifest"
	visualizeRecord.Flags[formatFlag.ID] = formatFlag
	manifest.Commands[visualizeRecord.ID] = visualizeRecord

	work, err := climanifestcobra.NewWorkFamilyCommandFromManifest(
		manifest,
		registry,
		testWorkBindings(),
	)
	if err != nil {
		t.Fatalf("NewWorkFamilyCommand() error = %v", err)
	}
	list, err := findCommandByPath(work, "work list")
	if err != nil {
		t.Fatalf("FindCommandByPath(work list) error = %v", err)
	}
	if got := list.Flags().Lookup("session").Usage; got != "custom session help from manifest" {
		t.Fatalf("session flag usage = %q, want custom manifest usage", got)
	}
	visualize, err := findCommandByPath(work, "work visualize")
	if err != nil {
		t.Fatalf("FindCommandByPath(work visualize) error = %v", err)
	}
	if got := visualize.Flags().Lookup("format").Usage; got != "custom visualize format help from manifest" {
		t.Fatalf("format flag usage = %q, want custom manifest usage", got)
	}
}

func TestNewWorkFamilyCommandRegistersEveryManifestLocalFlag(t *testing.T) {
	manifest := mustWorkFamilyManifest(t)
	work, _ := mustWorkFamilyTree(t)
	for _, tc := range []struct {
		commandID string
		path      string
	}{
		{commandID: "you.work.list", path: "work list"},
		{commandID: "you.work.show", path: "work show"},
		{commandID: "you.work.move", path: "work move"},
		{commandID: "you.work.visualize", path: "work visualize"},
	} {
		t.Run(tc.commandID, func(t *testing.T) {
			record, err := manifest.CommandByID(tc.commandID)
			if err != nil {
				t.Fatalf("CommandByID(%q) error = %v", tc.commandID, err)
			}
			cmd, err := findCommandByPath(work, tc.path)
			if err != nil {
				t.Fatalf("FindCommandByPath(%q) error = %v", tc.path, err)
			}
			for _, flag := range record.Flags {
				if flag.Scope != "local" {
					continue
				}
				if got := cmd.Flags().Lookup(flag.Long); got == nil {
					t.Fatalf("%s missing local flag %q", tc.path, flag.Long)
				}
			}
		})
	}
}

func mustWorkFamilyManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	return manifest
}

func assertWorkShowContractedFlags(t *testing.T, work *cobra.Command) {
	t.Helper()
	show, err := findCommandByPath(work, "work show")
	if err != nil {
		t.Fatalf("FindCommandByPath(work show) error = %v", err)
	}
	portFlag := show.Flags().Lookup("port")
	if portFlag == nil || !portFlag.Hidden {
		t.Fatalf("port flag = %#v, want hidden local deprecated flag", portFlag)
	}
	if show.Args == nil {
		t.Fatal("work show must wire positional args from generated metadata")
	}
	if err := show.Args(show, []string{}); err == nil {
		t.Fatal("work show args = nil error, want required positional rejection")
	}
}

func assertWorkListContractedFlags(t *testing.T, work *cobra.Command) {
	t.Helper()
	list, err := findCommandByPath(work, "work list")
	if err != nil {
		t.Fatalf("FindCommandByPath(work list) error = %v", err)
	}
	for _, flagName := range []string{"session", "max-results", "name"} {
		if list.Flags().Lookup(flagName) == nil {
			t.Fatalf("work list missing local flag %q", flagName)
		}
	}
}

func assertWorkVisualizeContractedFlags(t *testing.T, work *cobra.Command) {
	t.Helper()
	visualize, err := findCommandByPath(work, "work visualize")
	if err != nil {
		t.Fatalf("FindCommandByPath(work visualize) error = %v", err)
	}
	formatFlag := visualize.Flags().Lookup("format")
	if formatFlag == nil || formatFlag.DefValue != "mermaid" {
		t.Fatalf("format flag = %#v, want default mermaid", formatFlag)
	}
}

func TestNewWorkFamilyComponentsReturnsDetachedCommands(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	components, err := climanifestcobra.NewWorkFamilyComponents(registry, testWorkBindings())
	if err != nil {
		t.Fatalf("NewWorkFamilyComponents() error = %v", err)
	}
	if components.Work == nil || components.List == nil || components.Show == nil || components.Move == nil || components.Visualize == nil {
		t.Fatalf("components = %#v, want detached work/list/show/move/visualize commands", components)
	}
	if components.List.Parent() != nil || components.Show.Parent() != nil || components.Move.Parent() != nil || components.Visualize.Parent() != nil {
		t.Fatal("detached leaf components must not be attached before production wiring")
	}
}

func TestNewWorkFamilyCommandRejectsIncompleteBindings(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewWorkFamilyCommand(registry, climanifestcobra.WorkFamilyBindings{}); err == nil {
		t.Fatal("NewWorkFamilyCommand() incomplete bindings = nil, want error")
	}
}

func TestNewWorkFamilyCommandFromManifestRejectsRunnableWorkParent(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	work := manifest.Commands["you.work"]
	work.Runnable = true
	manifest.Commands["you.work"] = work

	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewWorkFamilyCommandFromManifest(manifest, registry, testWorkBindings()); err == nil {
		t.Fatal("NewWorkFamilyCommandFromManifest() runnable work parent = nil, want error")
	}
}

func TestNewWorkFamilyCommandFromManifestRejectsWrongFamilyCardinality(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.work.submit"] = climanifest.Command{ID: "you.work.submit", Path: "you work submit"}

	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	if _, err := climanifestcobra.NewWorkFamilyCommandFromManifest(manifest, registry, testWorkBindings()); err == nil {
		t.Fatal("NewWorkFamilyCommandFromManifest() extra command = nil, want error")
	}
}

func TestNewWorkFamilyCommandRejectsNilRegistry(t *testing.T) {
	if _, err := climanifestcobra.NewWorkFamilyCommand(nil, testWorkBindings()); err == nil {
		t.Fatal("NewWorkFamilyCommand() nil registry = nil, want error")
	}
}

func mustWorkFamilyTree(t *testing.T) (*cobra.Command, *commandregistry.Registry) {
	t.Helper()
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, testWorkBindings())
	if err != nil {
		t.Fatalf("NewWorkFamilyCommand() error = %v", err)
	}
	return work, registry
}

func testWorkBindings() climanifestcobra.WorkFamilyBindings {
	return climanifestcobra.WorkFamilyBindings{LocalTargets: map[string]any{
		"you.work.list.flag.state-name":     testScalarTarget(""),
		"you.work.list.flag.state-type":     testScalarTarget(""),
		"you.work.list.flag.name":           testScalarTarget(""),
		"you.work.list.flag.work-type-name": testScalarTarget(""),
		"you.work.list.flag.trace-id":       testScalarTarget(""),
		"you.work.list.flag.sort-by":        testScalarTarget(""),
		"you.work.list.flag.max-results":    testScalarTarget(0),
		"you.work.list.flag.next-token":     testScalarTarget(""),
		"you.work.list.flag.session":        testScalarTarget(""),
		"you.work.show.flag.session":        testScalarTarget(""),
		"you.work.move.flag.session":        testScalarTarget(""),
		"you.work.move.flag.request-id":     testScalarTarget(""),
		"you.work.visualize.flag.format":    testScalarTarget("mermaid"),
	}}
}

func testScalarTarget[T bool | string | int](value T) *T { return &value }

func workPathForID(commandID string) string {
	switch commandID {
	case "you.work.list":
		return "work list"
	case "you.work.show":
		return "work show"
	case "you.work.move":
		return "work move"
	case "you.work.visualize":
		return "work visualize"
	default:
		return commandID
	}
}

func workCommandWithInheritedFlags(t *testing.T, work *cobra.Command) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "you"}
	var verbose bool
	var debug bool
	server := "http://localhost:7437"
	var json bool
	defaultWorkerModelProvider := ""
	defaultWorkerModel := ""
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "")
	root.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "")
	root.PersistentFlags().StringVar(&server, "server", server, "")
	root.PersistentFlags().BoolVar(&json, "json", false, "")
	root.PersistentFlags().StringVar(&defaultWorkerModelProvider, "default-worker-model-provider", "", "")
	root.PersistentFlags().StringVar(&defaultWorkerModel, "default-worker-model", "", "")
	root.AddCommand(work)
	return root
}
