package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
)

func TestModelsListCommand_DefaultServerAndJSONFlagMapToConfig(t *testing.T) {
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
	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
	if !got.JSON {
		t.Fatal("expected --json to map to ListConfig.JSON")
	}
}

func TestModelsListCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
	originalListModels := listModels
	defer func() {
		listModels = originalListModels
	}()

	listModels = func(cfg modelscli.ListConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: models list"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, `{"results":[]}`)
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"models", "list", "--json", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := payload["results"]; !ok {
		t.Fatalf("stdout JSON = %#v, want results key", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: models list") {
		t.Fatalf("stderr = %q, want diagnostics", got)
	}
}

func TestModelsInspectCommand_MapsModelArgumentAndServer(t *testing.T) {
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
	root.SetArgs([]string{"models", "inspect", "OMNIVOICE_Q4_K_M", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", got.ModelName)
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
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
	root.SetArgs([]string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models invoke: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "hello" || got.OutputPath != "speech.wav" || got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("invoke config = %#v, want mapped invoke args and flags", got)
	}
}

func TestModelsPullCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalPullModel := pullModel
	defer func() {
		pullModel = originalPullModel
	}()

	var got modelscli.PullConfig
	pullModel = func(cfg modelscli.PullConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "pull", "OMNIVOICE_Q4_K_M", "--server", "http://127.0.0.1:9090", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models pull: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Server != "http://127.0.0.1:9090" || !got.JSON {
		t.Fatalf("pull config = %#v, want mapped pull args and flags", got)
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
	for _, want := range []string{"Inspect discovered models", "list", "inspect", "invoke", "pull", "/models"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("models help missing %q:\n%s", want, help)
		}
	}
}
