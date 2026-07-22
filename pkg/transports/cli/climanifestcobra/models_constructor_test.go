package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
)

type modelsHandlerStub struct{}

func (modelsHandlerStub) List(*cobra.Command, []string) error    { return nil }
func (modelsHandlerStub) Inspect(*cobra.Command, []string) error { return nil }
func (modelsHandlerStub) Invoke(*cobra.Command, []string) error  { return nil }
func (modelsHandlerStub) Pull(*cobra.Command, []string) error    { return nil }

func TestDocsAndModelsCommandsAreConstructedIndependently(t *testing.T) {
	docsRegistry, err := commandregistry.NewDocsRegistry(commandregistry.DocsHandlers{DocsRunE: noopRunE})
	if err != nil {
		t.Fatal(err)
	}
	modelsRegistry, err := commandregistry.NewModelsRegistry(modelsHandlerStub{})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := climanifestcobra.NewDocsCommand(docsRegistry)
	if err != nil {
		t.Fatal(err)
	}
	models, err := climanifestcobra.NewModelsCommand(modelsRegistry)
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

func TestModelsCommandRegistersPositionalsAndFlagsFromManifest(t *testing.T) {
	registry, err := commandregistry.NewModelsRegistry(modelsHandlerStub{})
	if err != nil {
		t.Fatal(err)
	}
	models, err := climanifestcobra.NewModelsCommand(registry)
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
