package docs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestInstalledDocumentationBehaviorThroughPublicProcess proves the shipped
// docs/help path and the copied Factory examples through the customer process.
// The mock-worker run keeps this documentation proof local and avoids model or
// paid-provider effects while still crossing validation and runtime loading.
func TestInstalledDocumentationBehaviorThroughPublicProcess(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := process.Close(ctx); err != nil {
			t.Errorf("close documentation process: %v", err)
		}
	})

	t.Run("Factory examples validate and run", func(t *testing.T) {
		testFactoryDocumentationExamples(t, process)
	})
	t.Run("Models topic documents names costs and safety", func(t *testing.T) {
		testModelsDocumentation(t, process)
	})
	t.Run("Models invoke help documents prerequisites and names", func(t *testing.T) {
		testModelsInvokeHelp(t, process)
	})
}

func testFactoryDocumentationExamples(t *testing.T, process support.Process) {
	t.Helper()
	for _, example := range []struct {
		name    string
		topic   string
		heading string
	}{
		{name: "config", topic: "config", heading: "## Minimum Factory Authoring Contract"},
		{name: "authoring-factories", topic: "authoring-factories", heading: "## Minimal Workflow"},
	} {
		example := example
		t.Run(example.name, func(t *testing.T) {
			testFactoryDocumentationExample(t, process, example.topic, example.heading)
		})
	}
}

func testFactoryDocumentationExample(t *testing.T, process support.Process, topic, heading string) {
	t.Helper()
	env := isolatedDocumentationEnvironment(t)
	markdown := executeDocumentationCommand(t, process, env, t.TempDir(), "docs", topic)
	factoryDir := t.TempDir()
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, extractJSONExample(t, markdown, heading), 0o600); err != nil {
		t.Fatalf("write copied %s Factory example: %v", topic, err)
	}

	validate := executeDocumentationCommandResult(t, process, env, factoryDir, "factory", "config", "validate", factoryPath)
	if validate.err != nil {
		t.Fatalf("validate copied %s Factory example: %v\nstdout:\n%s\nstderr:\n%s", topic, validate.err, validate.stdout, validate.stderr)
	}
	if !strings.Contains(validate.stdout, "Factory validation passed.") {
		t.Fatalf("validate copied %s Factory example omitted success:\n%s", topic, validate.stdout)
	}

	workPath := filepath.Join(factoryDir, "work.json")
	if err := os.WriteFile(workPath, []byte(documentationFactoryWork), 0o600); err != nil {
		t.Fatalf("write %s Factory work: %v", topic, err)
	}
	run := executeDocumentationCommandResult(t, process, env, factoryDir,
		"run", "--dir", factoryDir, "--with-mock-workers", "--no-record", "--quiet", "--work", workPath,
	)
	if run.err != nil {
		t.Fatalf("run copied %s Factory example: %v\nstdout:\n%s\nstderr:\n%s", topic, run.err, run.stdout, run.stderr)
	}
	if !strings.Contains(run.stdout, "Batch completed successfully.") {
		t.Fatalf("run copied %s Factory example omitted completion:\n%s", topic, run.stdout)
	}
}

func testModelsDocumentation(t *testing.T, process support.Process) {
	t.Helper()
	env := isolatedDocumentationEnvironment(t)
	markdown := executeDocumentationCommand(t, process, env, t.TempDir(), "docs", "models")
	for _, want := range []string{
		"local Models composition",
		"| `llm` | `OMNI` | 5.0 GB |",
		"| `asr` | `ASR` | 148 MB |",
		"| `tts` | `TTS` | 18.7 GB |",
		"| `embed` | `EMBED` | 1.21 GB |",
		"additional platform-specific backend and runtime files",
		"`cacheBytes` reports the exact managed cache size",
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR",
		"Direct invocation requires a valid Current Factory",
		"./factory/factory.json",
		"CURRENT_FACTORY_NOT_FOUND",
		"An explicit `--server` must identify a reachable service",
		`you models invoke embed --operation EMBED --input text="Find similar work"`,
		`you models invoke llm --operation OMNI --input prompt="Write a haiku"`,
		"does not provide an `--offline` flag",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("installed Models topic missing %q", want)
		}
	}

	warning := strings.Index(markdown, "Warning: Pulling `tts`")
	pullSection := strings.Index(markdown, "## Pull A Managed Local Model")
	invokeSection := strings.Index(markdown, "## Invoke A Model Directly")
	if warning < 0 || pullSection < 0 || invokeSection < 0 || warning > pullSection || warning > invokeSection {
		t.Fatalf("TTS warning must precede pull and invocation guidance: warning=%d pull=%d invoke=%d", warning, pullSection, invokeSection)
	}
	for _, stale := range []string{
		"Managed Model Storage Finding",
		"issue #2201",
		"The placement is therefore not proven intentional",
		"OMNIVOICE_Q4_K_M",
		"MODEL_OFFLINE_CACHE_UNAVAILABLE",
		"Run this zero-configuration command",
		"shared in-process bootstrap",
	} {
		if strings.Contains(markdown, stale) {
			t.Fatalf("installed Models topic contains stale text %q", stale)
		}
	}
}

func testModelsInvokeHelp(t *testing.T, process support.Process) {
	t.Helper()
	env := isolatedDocumentationEnvironment(t)
	help := executeDocumentationCommand(t, process, env, t.TempDir(), "models", "invoke", "--help")
	for _, want := range []string{
		"Invoke one local model",
		"Current Factory at ./factory/factory.json",
		"llm for OMNI",
		"asr for speech recognition",
		"tts for voice synthesis",
		"embed for embeddings",
		"An explicit --server must identify a reachable service",
		"you models invoke embed --operation EMBED",
		"you models invoke llm --operation OMNI",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("Models invoke help missing %q", want)
		}
	}
	for _, stale := range []string{"OMNIVOICE_Q4_K_M", "--offline", "zero-configuration", "shared in-process bootstrap"} {
		if strings.Contains(help, stale) {
			t.Fatalf("Models invoke help contains stale text %q", stale)
		}
	}
}

type documentationCommandResult struct {
	stdout string
	stderr string
	err    error
}

func executeDocumentationCommand(t *testing.T, process support.Process, env []string, workingDirectory string, args ...string) string {
	t.Helper()
	result := executeDocumentationCommandResult(t, process, env, workingDirectory, args...)
	if result.err != nil {
		t.Fatalf("Process.Execute(%v): %v\nstdout:\n%s\nstderr:\n%s", args, result.err, result.stdout, result.stderr)
	}
	return result.stdout
}

func executeDocumentationCommandResult(t *testing.T, process support.Process, env []string, workingDirectory string, args ...string) documentationCommandResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	return documentationCommandResult{stdout: inputs.Stdout(), stderr: inputs.Stderr(), err: err}
}

func isolatedDocumentationEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR="+filepath.Join(home, "omnivoice-cache"),
	)
}

func extractJSONExample(t *testing.T, markdown, heading string) []byte {
	t.Helper()
	section := strings.Index(markdown, heading)
	if section < 0 {
		t.Fatalf("installed topic missing heading %q", heading)
	}
	remaining := markdown[section+len(heading):]
	const fence = "```json"
	open := strings.Index(remaining, fence)
	if open < 0 {
		t.Fatalf("installed topic heading %q has no JSON example", heading)
	}
	contentStart := open + len(fence)
	close := strings.Index(remaining[contentStart:], "```")
	if close < 0 {
		t.Fatalf("installed topic heading %q has an unterminated JSON example", heading)
	}
	return []byte(strings.TrimSpace(remaining[contentStart : contentStart+close]))
}

const documentationFactoryWork = `{
  "requestId": "docs-factory-example",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "docs-factory-example",
      "workTypeName": "task",
      "state": "init",
      "payload": {"title": "Verify copied Factory example"}
    }
  ]
}`
