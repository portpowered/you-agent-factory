package climanifestcobra_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
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
	list, err := findCommandByPath(work, "work list")
	if err != nil {
		t.Fatalf("FindCommandByPath(work list) error = %v", err)
	}
	if got := list.Flags().Lookup("session").Usage; got != "custom session help for parity" {
		t.Fatalf("session flag usage = %q, want custom binding usage", got)
	}
	visualize, err := findCommandByPath(work, "work visualize")
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
	listCfg := workcli.ListConfig{Context: context.Background()}
	showCfg := workcli.ShowConfig{Context: context.Background()}
	moveCfg := workcli.MoveConfig{Context: context.Background()}
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

func TestNewResolvedWorkCommandTreeBuildsOnlyGeneratedWorkFamily(t *testing.T) {
	root, err := climanifestcobra.NewResolvedWorkCommandTree(noopResolvedWorkHandlers())
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	if root.Name() != "you" || len(root.Commands()) != 1 {
		t.Fatalf("root = %q with %d children, want you with only work", root.Name(), len(root.Commands()))
	}
	work, _, err := root.Find([]string{"work"})
	if err != nil {
		t.Fatalf("Find(work) error = %v", err)
	}
	if len(work.Commands()) != 4 {
		t.Fatalf("work children=%d, want 4", len(work.Commands()))
	}
	for _, path := range [][]string{
		{"work", "list"},
		{"work", "show"},
		{"work", "move"},
		{"work", "visualize"},
	} {
		command, remaining, findErr := root.Find(path)
		if findErr != nil || len(remaining) != 0 || !command.Runnable() {
			t.Fatalf("Find(%v) = %v, %v, runnable=%t", path, remaining, findErr, command.Runnable())
		}
	}
	if _, remaining, _ := root.Find([]string{"work", "submit"}); len(remaining) == 0 {
		t.Fatal("resolved Work tree unexpectedly exposes work submit")
	}
}

func TestNewResolvedWorkCommandTreeUsesManifestInputsAndStableHandlerID(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	listRecord := manifest.Commands["you.work.list"]
	listRecord.Handler.ID = "stable.work.list.test-handler"
	sessionFlag := listRecord.Flags["you.work.list.flag.session"]
	sessionFlag.Aliases = []string{"factory-session"}
	listRecord.Flags[sessionFlag.ID] = sessionFlag
	manifest.Commands[listRecord.ID] = listRecord

	var local resolvedinput.Inputs
	var inherited resolvedinput.Inputs
	handlers := noopResolvedWorkHandlers()
	handlers.List = func(
		_ *cobra.Command,
		gotLocal resolvedinput.Inputs,
		gotInherited resolvedinput.Inputs,
	) error {
		local = gotLocal
		inherited = gotInherited
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTreeFromManifest(manifest, handlers)
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTreeFromManifest() error = %v", err)
	}
	root.SetArgs([]string{
		"--server", "https://factory.example",
		"work", "list",
		"--factory-session", "session-alpha",
		"--name", "review",
		"--max-results", "7",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertResolvedString(t, local, "you.work.list.flag.session", "session-alpha")
	assertResolvedString(t, local, "you.work.list.flag.name", "review")
	maximum, err := local.Int("you.work.list.flag.max-results")
	if err != nil || maximum != 7 {
		t.Fatalf("resolved max-results = %d, %v; want 7", maximum, err)
	}
	assertResolvedString(t, inherited, "you.flag.server", "https://factory.example")
	assertWorkResolvedState(t, local, "you.work.list.flag.session", resolvedinput.State{
		Provenance: resolvedinput.SourceCLIFlag,
		Changed:    true,
	})
}

func TestNewResolvedWorkCommandTreeSuppliesFreshTypedSnapshots(t *testing.T) {
	first := executeResolvedWorkList(t, []string{"work", "list", "--name", "first"})
	second := executeResolvedWorkList(t, []string{"work", "list"})

	assertResolvedString(t, first, "you.work.list.flag.name", "first")
	assertResolvedString(t, second, "you.work.list.flag.name", "")
	assertWorkResolvedState(t, second, "you.work.list.flag.name", resolvedinput.State{
		Provenance: resolvedinput.SourceManifestDefault,
		Default:    true,
	})
}

func TestNewResolvedWorkCommandTreeResolvesGeneratedArgumentsAndDefaults(t *testing.T) {
	var showInputs resolvedinput.Inputs
	var visualizeInputs resolvedinput.Inputs
	handlers := noopResolvedWorkHandlers()
	handlers.Show = func(_ *cobra.Command, local, _ resolvedinput.Inputs) error {
		showInputs = local
		return nil
	}
	handlers.Visualize = func(_ *cobra.Command, local, _ resolvedinput.Inputs) error {
		visualizeInputs = local
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(handlers)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"work", "show", "work-123"})
	if err := root.Execute(); err != nil {
		t.Fatalf("show Execute() error = %v", err)
	}
	assertResolvedString(t, showInputs, "you.work.show.arg.0", "work-123")

	root, err = climanifestcobra.NewResolvedWorkCommandTree(handlers)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"work", "visualize", "batch.json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("visualize Execute() error = %v", err)
	}
	assertResolvedString(t, visualizeInputs, "you.work.visualize.arg.0", "batch.json")
	assertResolvedString(t, visualizeInputs, "you.work.visualize.flag.format", "mermaid")
}

func TestNewResolvedWorkCommandTreeEnforcesManifestArgumentCardinality(t *testing.T) {
	calls := 0
	handlers := noopResolvedWorkHandlers()
	handlers.Show = func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		calls++
		return nil
	}
	for _, args := range [][]string{
		{"work", "show"},
		{"work", "show", "work-123", "extra"},
	} {
		root, err := climanifestcobra.NewResolvedWorkCommandTree(handlers)
		if err != nil {
			t.Fatal(err)
		}
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("Execute(%v) error = nil", args)
		}
	}
	if calls != 0 {
		t.Fatalf("show handler calls = %d, want zero", calls)
	}
}

func TestNewResolvedWorkCommandTreeRejectsMissingAndForeignContracts(t *testing.T) {
	handlers := noopResolvedWorkHandlers()
	handlers.Move = nil
	if _, err := climanifestcobra.NewResolvedWorkCommandTree(handlers); err == nil {
		t.Fatal("missing move handler error = nil")
	}

	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	foreign := manifest.Commands["you.work.list"]
	foreign.ID = "you.work.submit"
	foreign.Name = "submit"
	foreign.Path = "you work submit"
	delete(manifest.Commands, "you.work.list")
	manifest.Commands[foreign.ID] = foreign
	if _, err := climanifestcobra.NewResolvedWorkCommandTreeFromManifest(
		manifest,
		noopResolvedWorkHandlers(),
	); err == nil {
		t.Fatal("foreign command error = nil")
	}
}

func TestNewResolvedWorkCommandReturnsDetachedSubtree(t *testing.T) {
	work, err := climanifestcobra.NewResolvedWorkCommand(noopResolvedWorkHandlers())
	if err != nil {
		t.Fatal(err)
	}
	if work.Name() != "work" || work.Parent() != nil || len(work.Commands()) != 4 {
		t.Fatalf(
			"detached work = name %q parent %v children %d",
			work.Name(),
			work.Parent(),
			len(work.Commands()),
		)
	}
}

func executeResolvedWorkList(t *testing.T, args []string) resolvedinput.Inputs {
	t.Helper()
	var got resolvedinput.Inputs
	handlers := noopResolvedWorkHandlers()
	handlers.List = func(_ *cobra.Command, local, _ resolvedinput.Inputs) error {
		got = local
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(handlers)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
	return got
}

func noopResolvedWorkHandlers() commandregistry.ResolvedWorkHandlers {
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	return commandregistry.ResolvedWorkHandlers{
		List: noop, Show: noop, Move: noop, Visualize: noop,
	}
}

func assertResolvedString(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	want string,
) {
	t.Helper()
	got, err := inputs.String(inputID)
	if err != nil || got != want {
		t.Fatalf("resolved %s = %q, %v; want %q", inputID, got, err, want)
	}
}

func assertWorkResolvedState(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	want resolvedinput.State,
) {
	t.Helper()
	got, ok := inputs.State(inputID)
	if !ok || got != want {
		t.Fatalf("resolved %s state = %#v, %t; want %#v", inputID, got, ok, want)
	}
}
