package cli

import (
	"bytes"
	"io"
	"testing"

	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
)

func TestModelsListCommand_DefaultPortAndJSONFlagMapToConfig(t *testing.T) {
	originalListModels := listModels
	defer func() {
		listModels = originalListModels
	}()

	var got modelscli.ListConfig
	listModels = func(cfg modelscli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list: %v", err)
	}
	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
	}
	if !got.JSON {
		t.Fatal("expected --json to map to ListConfig.JSON")
	}
}

func TestModelsInspectCommand_MapsModelArgumentAndPort(t *testing.T) {
	originalInspectModel := inspectModel
	defer func() {
		inspectModel = originalInspectModel
	}()

	var got modelscli.InspectConfig
	inspectModel = func(cfg modelscli.InspectConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "inspect", "OMNIVOICE_Q4_K_M", "--port", "9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", got.ModelName)
	}
	if got.Port != 9090 {
		t.Fatalf("port = %d, want 9090", got.Port)
	}
}

func TestModelsInvokeCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalInvokeModel := invokeModel
	defer func() {
		invokeModel = originalInvokeModel
	}()

	var got modelscli.InvokeConfig
	invokeModel = func(cfg modelscli.InvokeConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav", "--port", "9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models invoke: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "hello" || got.OutputPath != "speech.wav" || got.Port != 9090 {
		t.Fatalf("invoke config = %#v, want mapped invoke args and flags", got)
	}
}

func TestModelsCommand_HelpMentionsDiscoverySurface(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Inspect discovered models", "list", "inspect", "invoke", "/models"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("models help missing %q:\n%s", want, help)
		}
	}
}
