package climanifestcobra_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

type modelsHandlerStub struct{}

func (modelsHandlerStub) List(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (modelsHandlerStub) Inspect(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (modelsHandlerStub) Invoke(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (modelsHandlerStub) Pull(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func TestDocsAndModelsCommandsAreConstructedIndependently(t *testing.T) {
	docs, err := climanifestcobra.NewDocsCommand(
		func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	models, err := climanifestcobra.NewModelsCommand(modelsHandlerStub{})
	if err != nil {
		t.Fatal(err)
	}
	if docs.Name() != "docs" || models.Name() != "models" {
		t.Fatalf("commands = %q/%q, want docs/models", docs.Name(), models.Name())
	}
	if docs.Parent() != nil || models.Parent() != nil {
		t.Fatal("independent commands must remain detached before root composition")
	}
}

func TestGenericDocsDispatchResolvesTopicByStableManifestID(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatal(err)
	}
	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID: rootRecord,
		docsRecord.ID: docsRecord,
	}

	var got resolvedinput.Inputs
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: climanifestcobra.ResolvedCobraHandlerRegistry{
			docsRecord.Handler.ID: func(
				_ *cobra.Command,
				inputs resolvedinput.Inputs,
				_ resolvedinput.Inputs,
			) error {
				got = inputs
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{"docs", "models"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(docs models) error = %v", err)
	}
	topic, err := got.String("you.docs.arg.0")
	if err != nil || topic != "models" {
		t.Fatalf("resolved topic = %q, %v; want models", topic, err)
	}
	state, found := got.State("you.docs.arg.0")
	wantState := resolvedinput.State{
		Provenance: resolvedinput.SourcePositionalArgument,
		Changed:    true,
	}
	if !found || !reflect.DeepEqual(state, wantState) {
		t.Fatalf("resolved topic state = %#v, %t; want %#v", state, found, wantState)
	}
}

func TestGenericModelsInspectDispatchResolvesLocalAndInheritedInputs(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatal(err)
	}
	modelsRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		t.Fatal(err)
	}
	inspectRecord, err := manifest.CommandByID("you.models.inspect")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:    rootRecord,
		modelsRecord.ID:  modelsRecord,
		inspectRecord.ID: inspectRecord,
	}

	var local, inherited resolvedinput.Inputs
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: climanifestcobra.ResolvedCobraHandlerRegistry{
			inspectRecord.Handler.ID: func(
				_ *cobra.Command,
				gotLocal resolvedinput.Inputs,
				gotInherited resolvedinput.Inputs,
			) error {
				local, inherited = gotLocal, gotInherited
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{
		"--server", "http://127.0.0.1:9090",
		"models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(models inspect) error = %v", err)
	}
	modelName, err := local.String("you.models.inspect.arg.0")
	if err != nil || modelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("resolved model name = %q, %v", modelName, err)
	}
	assertResolvedState(t, local, "you.models.inspect.arg.0", resolvedinput.State{
		Provenance: resolvedinput.SourcePositionalArgument, Changed: true,
	})
	server, err := inherited.String("you.flag.server")
	if err != nil || server != "http://127.0.0.1:9090" {
		t.Fatalf("resolved server = %q, %v", server, err)
	}
	assertResolvedState(t, inherited, "you.flag.server", resolvedinput.State{
		Provenance: resolvedinput.SourceCLIFlag, Changed: true,
	})
	assertResolvedState(t, inherited, "you.flag.json", resolvedinput.State{
		Provenance: resolvedinput.SourceManifestDefault, Default: true,
	})
}

func TestGenericModelsPullDispatchResolvesLocalAndInheritedInputs(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatal(err)
	}
	modelsRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		t.Fatal(err)
	}
	pullRecord, err := manifest.CommandByID("you.models.pull")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		modelsRecord.ID: modelsRecord,
		pullRecord.ID:   pullRecord,
	}

	var local, inherited resolvedinput.Inputs
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: climanifestcobra.ResolvedCobraHandlerRegistry{
			pullRecord.Handler.ID: func(
				_ *cobra.Command,
				gotLocal resolvedinput.Inputs,
				gotInherited resolvedinput.Inputs,
			) error {
				local, inherited = gotLocal, gotInherited
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{"--json", "models", "pull", "OMNIVOICE_Q4_K_M"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(models pull) error = %v", err)
	}
	modelName, err := local.String("you.models.pull.arg.0")
	if err != nil || modelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("resolved model name = %q, %v", modelName, err)
	}
	assertResolvedState(t, local, "you.models.pull.arg.0", resolvedinput.State{
		Provenance: resolvedinput.SourcePositionalArgument, Changed: true,
	})
	jsonOutput, err := inherited.Bool("you.flag.json")
	if err != nil || !jsonOutput {
		t.Fatalf("resolved json = %t, %v; want true", jsonOutput, err)
	}
	assertResolvedState(t, inherited, "you.flag.json", resolvedinput.State{
		Provenance: resolvedinput.SourceCLIFlag, Changed: true,
	})
	assertResolvedState(t, inherited, "you.flag.server", resolvedinput.State{
		Provenance: resolvedinput.SourceManifestDefault, Default: true,
	})
}

func TestGenericModelsInvokeDispatchResolvesDefaultsAndExplicitInputs(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatal(err)
	}
	modelsRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		t.Fatal(err)
	}
	invokeRecord, err := manifest.CommandByID("you.models.invoke")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		modelsRecord.ID: modelsRecord,
		invokeRecord.ID: invokeRecord,
	}

	var local, inherited resolvedinput.Inputs
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: climanifestcobra.ResolvedCobraHandlerRegistry{
			invokeRecord.Handler.ID: func(
				_ *cobra.Command,
				gotLocal resolvedinput.Inputs,
				gotInherited resolvedinput.Inputs,
			) error {
				local, inherited = gotLocal, gotInherited
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCommandTree() error = %v", err)
	}
	root.SetArgs([]string{
		"--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--text", "  hello  ", "--output", "  speech.wav  ",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(models invoke) error = %v", err)
	}
	for inputID, want := range map[string]string{
		"you.models.invoke.arg.0":          "OMNIVOICE_Q4_K_M",
		"you.models.invoke.flag.operation": "TTS",
		"you.models.invoke.flag.text":      "hello",
		"you.models.invoke.flag.output":    "speech.wav",
	} {
		got, valueErr := local.String(inputID)
		if valueErr != nil || got != want {
			t.Fatalf("resolved %s = %q, %v; want %q", inputID, got, valueErr, want)
		}
	}
	assertResolvedState(t, local, "you.models.invoke.arg.0", resolvedinput.State{
		Provenance: resolvedinput.SourcePositionalArgument, Changed: true,
	})
	assertResolvedState(t, local, "you.models.invoke.flag.operation", resolvedinput.State{
		Provenance: resolvedinput.SourceManifestDefault, Default: true,
	})
	assertResolvedState(t, local, "you.models.invoke.flag.text", resolvedinput.State{
		Provenance: resolvedinput.SourceCLIFlag, Changed: true,
	})
	textObservation, found := local.Observe("you.models.invoke.flag.text")
	if !found || textObservation.Value != resolvedinput.RedactedValue {
		t.Fatalf("text observation = %#v, %t; want redacted value", textObservation, found)
	}
	assertResolvedState(t, inherited, "you.flag.json", resolvedinput.State{
		Provenance: resolvedinput.SourceCLIFlag, Changed: true,
	})
}

func assertResolvedState(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	want resolvedinput.State,
) {
	t.Helper()
	got, found := inputs.State(inputID)
	if !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved %s state = %#v, %t; want %#v", inputID, got, found, want)
	}
}

func TestModelsCommandRegistersPositionalsAndFlagsFromManifest(t *testing.T) {
	models, err := climanifestcobra.NewModelsCommand(modelsHandlerStub{})
	if err != nil {
		t.Fatal(err)
	}
	invoke, _, err := models.Find([]string{"invoke"})
	if err != nil {
		t.Fatal(err)
	}
	if invoke.Use != "invoke <model-name>" {
		t.Fatalf("invoke use = %q", invoke.Use)
	}
	for _, name := range []string{"operation", "text", "output", "port"} {
		if invoke.Flags().Lookup(name) == nil {
			t.Fatalf("manifest flag %q was not registered", name)
		}
	}
	if err := invoke.ParseFlags([]string{"--operation", "TTS", "--text", "hello", "--output", "speech.wav"}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"operation": "TTS", "text": "hello", "output": "speech.wav"} {
		got, getErr := invoke.Flags().GetString(name)
		if getErr != nil || got != want {
			t.Fatalf("flag %s = %q, %v; want %q", name, got, getErr, want)
		}
	}
	if err := invoke.ParseFlags([]string{"--operation", "INVALID"}); err == nil {
		t.Fatal("invalid manifest operation choice was accepted")
	}
}

func TestMCPCommandIsDetachedAndManifestPresented(t *testing.T) {
	mcp, err := climanifestcobra.NewMCPCommand(
		func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if mcp.Name() != "mcp" || mcp.Parent() != nil {
		t.Fatalf("MCP command = name %q parent %v, want detached mcp", mcp.Name(), mcp.Parent())
	}
	serve, _, err := mcp.Find([]string{"serve"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fixture-catalog", "runtime", "project-root"} {
		if serve.Flags().Lookup(name) == nil {
			t.Fatalf("manifest flag %q was not registered", name)
		}
	}
	var output bytes.Buffer
	serve.SetOut(&output)
	if err := serve.Help(); err != nil {
		t.Fatalf("help: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "you docs mcp") ||
		!strings.Contains(got, "durable-session-contract-fixtures.json") {
		t.Fatalf("manifest help missing canonical details:\n%s", got)
	}
}

func TestMCPCommandResolvesNormalizedInputsAndProvenance(t *testing.T) {
	var local resolvedinput.Inputs
	root := newMCPRoot(t,
		func(_ *cobra.Command, inputs, _ resolvedinput.Inputs) error {
			local = inputs
			return nil
		},
	)
	root.SetArgs([]string{"mcp", "serve", "--fixture-catalog", "  fixtures.json  ", "--project-root", "  project  "})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for inputID, want := range map[string]string{
		"you.mcp.serve.flag.fixture-catalog": "fixtures.json",
		"you.mcp.serve.flag.project-root":    "project",
	} {
		got, valueErr := local.String(inputID)
		if valueErr != nil || got != want {
			t.Fatalf("resolved %s = %q, %v; want %q", inputID, got, valueErr, want)
		}
		assertMCPResolvedState(t, local, inputID, resolvedinput.State{
			Provenance: resolvedinput.SourceCLIFlag, Changed: true,
		})
	}
	runtimeBacked, err := local.Bool("you.mcp.serve.flag.runtime")
	if err != nil || runtimeBacked {
		t.Fatalf("resolved runtime = %t, %v; want false", runtimeBacked, err)
	}
	assertMCPResolvedState(t, local, "you.mcp.serve.flag.runtime", resolvedinput.State{
		Provenance: resolvedinput.SourceManifestDefault, Default: true,
	})
}

func newMCPRoot(
	t *testing.T,
	handler climanifestcobra.ResolvedCobraHandler,
) *cobra.Command {
	t.Helper()
	manifest, err := generated.MCPFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatal(err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	serveRecord, err := manifest.CommandByID("you.mcp.serve")
	if err != nil {
		t.Fatal(err)
	}
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: climanifestcobra.ResolvedCobraHandlerRegistry{
			serveRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMCPManifestRelationshipRejectsConflictingSourcesBeforeHandler(t *testing.T) {
	calls := 0
	mcp, err := climanifestcobra.NewMCPCommand(
		func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
			calls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	mcp.SetArgs([]string{"serve", "--runtime", "--fixture-catalog", "fixtures.json"})
	err = mcp.Execute()
	if err == nil || err.Error() != "cannot combine --runtime with --fixture-catalog" {
		t.Fatalf("execute error = %v, want manifest relationship rejection", err)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want zero", calls)
	}
}

func assertMCPResolvedState(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	want resolvedinput.State,
) {
	t.Helper()
	got, found := inputs.State(inputID)
	if !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved %s state = %#v, %t; want %#v", inputID, got, found, want)
	}
}

func TestNewSubmitFamilyCommandProjectsOnlyCanonicalSubmitFamily(t *testing.T) {
	command, _ := mustSubmitFamilyCommand(t, nil)
	if command.Use != "submit" {
		t.Fatalf("Use = %q, want submit", command.Use)
	}
	children := command.Commands()
	if len(children) != 1 || children[0].Use != "batch [path|-|<inline-json>]" {
		t.Fatalf("children = %#v, want only batch", submitChildUses(children))
	}
	if command.Flags().Lookup("name") == nil ||
		command.Flags().Lookup("work-type-name") == nil ||
		command.Flags().Lookup("payload") == nil {
		t.Fatal("unary local flags were not projected")
	}
	batch, _, err := command.Find([]string{"batch"})
	if err != nil {
		t.Fatalf("Find(batch) error = %v", err)
	}
	if batch.Flags().Lookup("file") == nil || batch.Flags().Lookup("dry-run") == nil {
		t.Fatal("batch local flags were not projected")
	}
}

func TestNewSubmitFamilyCommandUsesManifestPresentationAndStableInputs(t *testing.T) {
	manifest := submitConstructorManifest(t)
	submit := manifest.Commands["you.submit"]
	submit.Documentation.Documentation.Title.CanonicalEnglish = "Manifest unary title"
	nameFlag := submit.Flags["you.submit.flag.name"]
	nameFlag.Long = "work-name"
	submit.Flags[nameFlag.ID] = nameFlag
	manifest.Commands[submit.ID] = submit
	batch := manifest.Commands["you.submit.batch"]
	batch.Visibility = "hidden"
	manifest.Commands[batch.ID] = batch

	var unaryCalls, batchCalls int
	handlerRegistry := mustSubmitHandlerRegistry(t, commandregistry.SubmitHandlers{
		Submit: func(_ *cobra.Command, inputs, inherited resolvedinput.Inputs) error {
			unaryCalls++
			assertSubmitString(t, inputs, "you.submit.flag.name", "work-1")
			assertSubmitString(t, inputs, "you.submit.flag.work-type-name", "REVIEW")
			assertSubmitString(t, inputs, "you.submit.flag.payload", "payload.md")
			assertSubmitString(t, inputs, "you.submit.flag.session", "")
			if _, ok := inherited.Lookup("you.flag.server"); ok {
				t.Fatal("standalone inherited inputs unexpectedly populated")
			}
			return nil
		},
		SubmitBatch: func(_ *cobra.Command, inputs, _ resolvedinput.Inputs) error {
			batchCalls++
			assertSubmitString(t, inputs, "you.submit.batch.arg.0", "batch.json")
			dryRun, err := inputs.Bool("you.submit.batch.flag.dry-run")
			if err != nil || !dryRun {
				t.Fatalf("dry-run = %t, error = %v", dryRun, err)
			}
			return nil
		},
	})
	command, err := climanifestcobra.NewSubmitFamilyCommandFromManifest(manifest, handlerRegistry)
	if err != nil {
		t.Fatalf("NewSubmitFamilyCommandFromManifest() error = %v", err)
	}
	if command.Short != "Manifest unary title" {
		t.Fatalf("Short = %q, want manifest title", command.Short)
	}
	batchCommand, _, _ := command.Find([]string{"batch"})
	if !batchCommand.Hidden {
		t.Fatal("batch Hidden = false, want manifest visibility")
	}

	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{
		"--work-name", "work-1",
		"--work-type-name", "REVIEW",
		"--payload", "payload.md",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute(unary) error = %v", err)
	}
	command.SetArgs([]string{"batch", "--dry-run", "batch.json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute(batch) error = %v", err)
	}
	if unaryCalls != 1 || batchCalls != 1 {
		t.Fatalf("calls unary=%d batch=%d, want 1 each", unaryCalls, batchCalls)
	}
}

func TestNewSubmitFamilyCommandRejectsContradictionsBeforeHandlersRun(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Manifest)
	}{
		{
			name: "contradictory root",
			mutate: func(manifest *climanifest.Manifest) {
				manifest.RootPath = "submit"
			},
		},
		{
			name: "missing record",
			mutate: func(manifest *climanifest.Manifest) {
				delete(manifest.Commands, "you.submit.batch")
			},
		},
		{
			name: "extra record",
			mutate: func(manifest *climanifest.Manifest) {
				manifest.Commands["you.extra"] = climanifest.Command{ID: "you.extra"}
			},
		},
		{
			name: "non-runnable",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Runnable = false
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "incompatible type",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				flag := record.Flags["you.submit.flag.name"]
				flag.ValueType = "object"
				record.Flags[flag.ID] = flag
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "incompatible inherited binding",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				flag := record.Flags["you.submit.flag.server"]
				flag.InheritedFromID = "you.flag.missing"
				record.Flags[flag.ID] = flag
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "unknown handler",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler.ID = "you.submit.unknown.handler"
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "missing handler",
			mutate: func(manifest *climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler = nil
				manifest.Commands[record.ID] = record
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlerCalls := 0
			handler := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
				handlerCalls++
				return nil
			}
			manifest := submitConstructorManifest(t)
			test.mutate(&manifest)
			_, err := climanifestcobra.NewSubmitFamilyCommandFromManifest(
				manifest,
				mustSubmitHandlerRegistry(t, commandregistry.SubmitHandlers{
					Submit: handler, SubmitBatch: handler,
				}),
			)
			if err == nil {
				t.Fatal("NewSubmitFamilyCommandFromManifest() error = nil, want rejection")
			}
			if handlerCalls != 0 {
				t.Fatalf("handler calls = %d, want zero", handlerCalls)
			}
		})
	}
}

func TestNewSubmitFamilyCommandRejectsMissingRegistry(t *testing.T) {
	if _, err := climanifestcobra.NewSubmitFamilyCommandFromManifest(
		submitConstructorManifest(t),
		nil,
	); err == nil {
		t.Fatal("NewSubmitFamilyCommandFromManifest(nil registry) error = nil, want rejection")
	}
}

func mustSubmitFamilyCommand(
	t *testing.T,
	handlers *commandregistry.SubmitHandlers,
) (*cobra.Command, *commandregistry.SubmitRegistry) {
	t.Helper()
	selected := commandregistry.SubmitHandlers{
		Submit: func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil },
		SubmitBatch: func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
			return nil
		},
	}
	if handlers != nil {
		selected = *handlers
	}
	handlerRegistry := mustSubmitHandlerRegistry(t, selected)
	command, err := climanifestcobra.NewSubmitFamilyCommand(handlerRegistry)
	if err != nil {
		t.Fatalf("NewSubmitFamilyCommand() error = %v", err)
	}
	return command, handlerRegistry
}

func mustSubmitHandlerRegistry(
	t *testing.T,
	handlers commandregistry.SubmitHandlers,
) *commandregistry.SubmitRegistry {
	t.Helper()
	handlerRegistry, err := commandregistry.NewSubmitRegistry(handlers)
	if err != nil {
		t.Fatalf("NewSubmitRegistry() error = %v", err)
	}
	return handlerRegistry
}

func submitConstructorManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands: map[string]climanifest.Command{
			"you.submit":       manifest.Commands["you.submit"],
			"you.submit.batch": manifest.Commands["you.submit.batch"],
		},
	}
}

func assertSubmitString(t *testing.T, inputs resolvedinput.Inputs, inputID, want string) {
	t.Helper()
	got, err := inputs.String(inputID)
	if err != nil || got != want {
		t.Fatalf("String(%q) = %q, error = %v, want %q", inputID, got, err, want)
	}
}

func submitChildUses(commands []*cobra.Command) string {
	uses := make([]string, len(commands))
	for index, command := range commands {
		uses[index] = command.Use
	}
	return strings.Join(uses, ", ")
}
