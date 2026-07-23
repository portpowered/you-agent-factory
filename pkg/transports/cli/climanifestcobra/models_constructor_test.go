package climanifestcobra_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
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
func (modelsHandlerStub) Invoke(*cobra.Command, []string) error { return nil }
func (modelsHandlerStub) Pull(*cobra.Command, []string) error   { return nil }

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
}
