package climanifestcobra

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestNewWorkerSessionsFamilyCommandBuildsDetachedRunnableLeaves(t *testing.T) {
	registry := commandregistry.NewRegistry()
	calls := make(map[string]int)
	for _, command := range []struct {
		id        string
		handlerID string
	}{
		{id: "you.worker-sessions.list", handlerID: "you.worker-sessions.list.handler"},
		{id: "you.worker-sessions.show", handlerID: "you.worker-sessions.show.handler"},
		{id: "you.worker-sessions.read", handlerID: "you.worker-sessions.read.handler"},
		{id: "you.worker-sessions.stream", handlerID: "you.worker-sessions.stream.handler"},
		{id: "you.worker-sessions.invoke", handlerID: "you.worker-sessions.invoke.handler"},
		{id: "you.worker-sessions.continue", handlerID: "you.worker-sessions.continue.handler"},
		{id: "you.worker-sessions.interrupt", handlerID: "you.worker-sessions.interrupt.handler"},
		{id: "you.worker-sessions.pause", handlerID: "you.worker-sessions.pause.handler"},
		{id: "you.worker-sessions.resume", handlerID: "you.worker-sessions.resume.handler"},
		{id: "you.worker-sessions.cancel", handlerID: "you.worker-sessions.cancel.handler"},
		{id: "you.worker-sessions.terminate", handlerID: "you.worker-sessions.terminate.handler"},
	} {
		command := command
		if err := registry.RegisterHandlers(command.handlerID, commandregistry.CommandHandlers{
			PreRunE: func(_ *cobra.Command, _ []string) error {
				calls[command.id+".pre"]++
				return nil
			},
			RunE: func(_ *cobra.Command, _ []string) error {
				calls[command.id]++
				return nil
			},
		}); err != nil {
			t.Fatalf("RegisterHandlers(%q) error = %v", command.handlerID, err)
		}
	}

	workerSessions, err := NewWorkerSessionsFamilyCommand(registry)
	if err != nil {
		t.Fatalf("NewWorkerSessionsFamilyCommand() error = %v", err)
	}
	if workerSessions.Name() != "worker-sessions" || workerSessions.Parent() != nil {
		t.Fatalf("worker-sessions command = name %q parent %v, want detached root", workerSessions.Name(), workerSessions.Parent())
	}
	if len(workerSessions.Commands()) != 11 {
		t.Fatalf("worker-sessions child count = %d, want 11", len(workerSessions.Commands()))
	}

	for _, command := range []string{"invoke", "continue", "interrupt", "pause", "resume", "cancel", "terminate", "list", "show", "read", "stream"} {
		leaf, err := findCommandByPath(workerSessions, "worker-sessions "+command)
		if err != nil {
			t.Fatalf("FindCommandByPath(%s) error = %v", command, err)
		}
		if !leaf.Runnable() || leaf.RunE == nil {
			t.Fatalf("worker-sessions %s must expose a runnable RunE leaf", command)
		}
	}
	_ = calls
}

func TestNewWorkerSessionsFamilyCommandRejectsInvalidManifestsAndHandlers(t *testing.T) {
	valid := workerSessionsManifestWithRoot(t)
	validRegistry := workerSessionsRegistry(t)
	cases := []struct {
		name   string
		mutate func(climanifest.Manifest) climanifest.Manifest
		reg    *commandregistry.Registry
	}{
		{name: "nil registry", mutate: func(manifest climanifest.Manifest) climanifest.Manifest { return manifest }, reg: nil},
		{name: "wrong root", mutate: func(manifest climanifest.Manifest) climanifest.Manifest { manifest.RootPath = "root"; return manifest }, reg: validRegistry},
		{name: "wrong command count", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			delete(manifest.Commands, "you.worker-sessions.list")
			return manifest
		}, reg: validRegistry},
		{name: "foreign command", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you.worker-sessions.list"]
			delete(manifest.Commands, "you.worker-sessions.list")
			record.ID, record.Name, record.Path = "you.work.list", "list", "you work list"
			manifest.Commands[record.ID] = record
			return manifest
		}, reg: validRegistry},
		{name: "key and record mismatch", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you.worker-sessions.list"]
			record.ID = "you.worker-sessions.show"
			manifest.Commands["you.worker-sessions.list"] = record
			return manifest
		}, reg: validRegistry},
		{name: "parent runnable", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you.worker-sessions"]
			record.Runnable = true
			manifest.Commands[record.ID] = record
			return manifest
		}, reg: validRegistry},
		{name: "missing runnable handler", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you.worker-sessions.show"]
			record.Handler = nil
			manifest.Commands[record.ID] = record
			return manifest
		}, reg: validRegistry},
		{name: "non-runnable leaf", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you.worker-sessions.show"]
			record.Runnable = false
			manifest.Commands[record.ID] = record
			return manifest
		}, reg: validRegistry},
		{name: "missing root handler", mutate: func(manifest climanifest.Manifest) climanifest.Manifest {
			record := manifest.Commands["you"]
			record.Handler = nil
			manifest.Commands[record.ID] = record
			return manifest
		}, reg: validRegistry},
		{name: "handler not registered", mutate: func(manifest climanifest.Manifest) climanifest.Manifest { return manifest }, reg: commandregistry.NewRegistry()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewWorkerSessionsFamilyCommandFromManifest(test.mutate(cloneWorkerSessionsManifest(valid)), test.reg); err == nil {
				t.Fatal("NewWorkerSessionsFamilyCommandFromManifest() error = nil, want rejection")
			}
		})
	}
}

func TestNewWorkerSessionsFamilyCommandAcceptsResolvedHandlers(t *testing.T) {
	command, err := NewWorkerSessionsFamilyCommand(workerSessionsResolvedOnlyRegistry(t))
	if err != nil {
		t.Fatalf("NewWorkerSessionsFamilyCommand() with resolved handler: %v", err)
	}
	list, err := findCommandByPath(command, "worker-sessions list")
	if err != nil {
		t.Fatalf("find resolved worker-sessions list: %v", err)
	}
	if list.RunE == nil {
		t.Fatal("resolved worker-sessions list must expose a runnable Cobra handler")
	}
}

func workerSessionsManifestWithRoot(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.WorkerSessionsFamilyManifest()
	if err != nil {
		t.Fatalf("WorkerSessionsFamilyManifest() error = %v", err)
	}
	root, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	rootRecord, err := root.CommandByID("you")
	if err != nil {
		t.Fatalf("representative root lookup error = %v", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return manifest
}

func cloneWorkerSessionsManifest(manifest climanifest.Manifest) climanifest.Manifest {
	clone := manifest
	clone.Commands = make(map[string]climanifest.Command, len(manifest.Commands))
	for id, record := range manifest.Commands {
		clone.Commands[id] = record
	}
	return clone
}

func workerSessionsRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	for _, handlerID := range []string{
		"you.worker-sessions.invoke.handler",
		"you.worker-sessions.continue.handler",
		"you.worker-sessions.list.handler",
		"you.worker-sessions.show.handler",
		"you.worker-sessions.read.handler",
		"you.worker-sessions.stream.handler",
		"you.worker-sessions.interrupt.handler",
		"you.worker-sessions.pause.handler",
		"you.worker-sessions.resume.handler",
		"you.worker-sessions.cancel.handler",
		"you.worker-sessions.terminate.handler",
	} {
		if err := registry.RegisterHandlers(handlerID, commandregistry.CommandHandlers{RunE: noopRunE}); err != nil {
			t.Fatalf("RegisterHandlers(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func workerSessionsResolvedOnlyRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry := commandregistry.NewRegistry()
	if err := registry.RegisterResolved("you.worker-sessions.list.handler", func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil }); err != nil {
		t.Fatalf("RegisterResolved() error = %v", err)
	}
	for _, handlerID := range []string{
		"you.worker-sessions.invoke.handler",
		"you.worker-sessions.continue.handler",
		"you.worker-sessions.interrupt.handler",
		"you.worker-sessions.pause.handler",
		"you.worker-sessions.resume.handler",
		"you.worker-sessions.cancel.handler",
		"you.worker-sessions.terminate.handler",
		"you.worker-sessions.show.handler",
		"you.worker-sessions.read.handler",
		"you.worker-sessions.stream.handler",
	} {
		if err := registry.RegisterHandlers(handlerID, commandregistry.CommandHandlers{RunE: noopRunE}); err != nil {
			t.Fatalf("RegisterHandlers(%q) error = %v", handlerID, err)
		}
	}
	return registry
}

func TestNewResolvedWorkCommandTreeBuildsOnlyGeneratedWorkFamily(t *testing.T) {
	root, err := NewResolvedWorkCommandTree(noopResolvedWorkHandlers())
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
	if len(work.Commands()) != 6 {
		t.Fatalf("work children=%d, want approval plus 5 leaves", len(work.Commands()))
	}
	for _, path := range [][]string{
		{"work", "approval", "list"},
		{"work", "approval", "show"},
		{"work", "list"},
		{"work", "watch"},
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
	root, err := NewResolvedWorkCommandTreeFromManifest(manifest, handlers)
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
	root, err := NewResolvedWorkCommandTree(handlers)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"work", "show", "work-123"})
	if err := root.Execute(); err != nil {
		t.Fatalf("show Execute() error = %v", err)
	}
	assertResolvedString(t, showInputs, "you.work.show.arg.0", "work-123")

	root, err = NewResolvedWorkCommandTree(handlers)
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
		root, err := NewResolvedWorkCommandTree(handlers)
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
	if _, err := NewResolvedWorkCommandTree(handlers); err == nil {
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
	if _, err := NewResolvedWorkCommandTreeFromManifest(
		manifest,
		noopResolvedWorkHandlers(),
	); err == nil {
		t.Fatal("foreign command error = nil")
	}
}

func TestNewResolvedWorkCommandReturnsDetachedSubtree(t *testing.T) {
	work, err := NewResolvedWorkCommand(noopResolvedWorkHandlers())
	if err != nil {
		t.Fatal(err)
	}
	if work.Name() != "work" || work.Parent() != nil || len(work.Commands()) != 6 {
		t.Fatalf(
			"detached work = name %q parent %v children %d, want approval plus five leaves",
			work.Name(),
			work.Parent(),
			len(work.Commands()),
		)
	}
	if work.RunE != nil || work.DisableFlagParsing {
		t.Fatal("detached work group must preserve non-runnable compatibility behavior")
	}
	approval, _, err := work.Find([]string{"approval"})
	if err != nil {
		t.Fatalf("Find(approval) error = %v", err)
	}
	if approval.RunE != nil || approval.DisableFlagParsing {
		t.Fatal("detached approval group must preserve non-runnable compatibility behavior")
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
	root, err := NewResolvedWorkCommandTree(handlers)
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
		List: noop, Watch: noop, Show: noop, Move: noop, Visualize: noop,
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

func testScalarTarget[T bool | string | int](value T) *T { return &value }
