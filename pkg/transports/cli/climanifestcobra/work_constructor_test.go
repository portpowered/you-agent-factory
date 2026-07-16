package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
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
		cmd, err := climanifestparity.FindCommandByPath(work, path)
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

	list, err := climanifestparity.FindCommandByPath(work, "work list")
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
		if _, err := climanifestparity.FindCommandByPath(work, workPathForID(id)); err != nil {
			t.Fatalf("work path for %q missing: %v", id, err)
		}
	}
	if _, err := climanifestparity.FindCommandByPath(work, "work submit"); err == nil {
		t.Fatal("generated work constructor must not expose work submit")
	}
}

func TestNewWorkFamilyCommandRegistersContractedFlagsAndArgs(t *testing.T) {
	work, _ := mustWorkFamilyTree(t)
	manifest := mustWorkFamilyManifest(t)
	assertWorkShowContractedFlags(t, work)
	assertWorkListContractedFlags(t, work)
	assertWorkVisualizeContractedFlags(t, work)
	assertWorkFamilyCompletionParity(t, work, manifest)
}

func TestNewWorkFamilyCommandAppliesBindingFlagUsages(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	bindings := testWorkBindings()
	bindings.FlagUsages = map[string]string{
		"session": "custom session help for parity",
		"format":  "custom visualize format help",
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		t.Fatalf("NewWorkFamilyCommand() error = %v", err)
	}
	list, err := climanifestparity.FindCommandByPath(work, "work list")
	if err != nil {
		t.Fatalf("FindCommandByPath(work list) error = %v", err)
	}
	if got := list.Flags().Lookup("session").Usage; got != "custom session help for parity" {
		t.Fatalf("session flag usage = %q, want custom binding usage", got)
	}
	visualize, err := climanifestparity.FindCommandByPath(work, "work visualize")
	if err != nil {
		t.Fatalf("FindCommandByPath(work visualize) error = %v", err)
	}
	if got := visualize.Flags().Lookup("format").Usage; got != "custom visualize format help" {
		t.Fatalf("format flag usage = %q, want custom binding usage", got)
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
			cmd, err := climanifestparity.FindCommandByPath(work, tc.path)
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
	show, err := climanifestparity.FindCommandByPath(work, "work show")
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
	list, err := climanifestparity.FindCommandByPath(work, "work list")
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
	visualize, err := climanifestparity.FindCommandByPath(work, "work visualize")
	if err != nil {
		t.Fatalf("FindCommandByPath(work visualize) error = %v", err)
	}
	formatFlag := visualize.Flags().Lookup("format")
	if formatFlag == nil || formatFlag.DefValue != "mermaid" {
		t.Fatalf("format flag = %#v, want default mermaid", formatFlag)
	}
}

func assertWorkFamilyCompletionParity(t *testing.T, work *cobra.Command, manifest climanifest.Manifest) {
	t.Helper()
	showRecord, err := manifest.CommandByID("you.work.show")
	if err != nil {
		t.Fatalf("CommandByID(you.work.show) error = %v", err)
	}
	listRecord, err := manifest.CommandByID("you.work.list")
	if err != nil {
		t.Fatalf("CommandByID(you.work.list) error = %v", err)
	}

	root := workCommandWithInheritedFlags(t, work)
	inventory, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("cliinputs.Walk() error = %v", err)
	}
	liveArgs, liveFlags := climanifestparity.InputsForCommandPath(inventory, showRecord.Path)
	if len(liveArgs) != 1 {
		t.Fatalf("show inputs inventory args = %d, want 1 positional", len(liveArgs))
	}
	if mismatches := climanifestparity.CompareCompletionParity(showRecord, liveArgs, liveFlags); len(mismatches) != 0 {
		t.Fatalf("show completion wiring drift:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}

	liveListArgs, liveListFlags := climanifestparity.InputsForCommandPath(inventory, listRecord.Path)
	if mismatches := climanifestparity.CompareCompletionParity(listRecord, liveListArgs, liveListFlags); len(mismatches) != 0 {
		t.Fatalf("list completion wiring drift:\n%s", climanifestparity.FormatMismatchReport(mismatches))
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
	listCfg := workcli.ListConfig{}
	showCfg := workcli.ShowConfig{}
	moveCfg := workcli.MoveConfig{}
	format := "mermaid"
	return climanifestcobra.WorkFamilyBindings{
		ListConfig:      &listCfg,
		ShowConfig:      &showCfg,
		MoveConfig:      &moveCfg,
		VisualizeFormat: &format,
	}
}

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
