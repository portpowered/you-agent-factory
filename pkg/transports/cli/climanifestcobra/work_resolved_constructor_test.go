package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

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
