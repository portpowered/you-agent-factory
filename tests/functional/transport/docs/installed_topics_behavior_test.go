package docs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestInstalledDocumentationBehaviorThroughPublicProcess proves the shipped
// docs/help path and the copied Factory examples through the customer process.
// The provider command edge keeps this documentation proof local and avoids
// model or paid-provider effects while still crossing validation, runtime
// loading, and provider-backed worker execution.
func TestInstalledDocumentationBehaviorThroughPublicProcess(t *testing.T) {
	t.Run("Factory examples validate and run", func(t *testing.T) {
		testFactoryDocumentationExamples(t, documentationProcess(t).process, documentationProcess(t).providerRunner)
	})
	t.Run("Docs index is complete and singular", func(t *testing.T) {
		index := executeDocumentationCommand(
			t,
			documentationProcess(t).process,
			isolatedDocumentationEnvironment(t),
			documentationProcess(t).tempDir(t),
			"docs",
		)
		for _, marker := range []string{
			"# Docs",
			"Packaged reference topics:",
			"Run `you docs <topic>` below",
		} {
			if count := strings.Count(index, marker); count != 1 {
				t.Fatalf("docs index marker %q count = %d, want 1", marker, count)
			}
		}
		if strings.TrimSpace(index) == "" {
			t.Fatal("docs index is empty")
		}
	})
	t.Run("Models topic documents names costs and safety", func(t *testing.T) {
		testModelsDocumentation(t, documentationProcess(t).process)
	})
	t.Run("Models invoke help documents prerequisites and names", func(t *testing.T) {
		testModelsInvokeHelp(t, documentationProcess(t).process)
	})
}

func testFactoryDocumentationExamples(
	t *testing.T,
	process support.Process,
	providerRunner *support.ShapedProviderCommandRunner,
) {
	t.Helper()
	for _, example := range []struct {
		name            string
		topic           string
		heading         string
		workerName      string
		workstationName string
	}{
		{
			name:            "config",
			topic:           "config",
			heading:         "## Minimum Factory Authoring Contract",
			workerName:      "reviewer",
			workstationName: "review",
		},
		{
			name:            "authoring-factories",
			topic:           "authoring-factories",
			heading:         "## Minimal Workflow",
			workerName:      "processor",
			workstationName: "process-task",
		},
	} {
		example := example
		t.Run(example.name, func(t *testing.T) {
			testFactoryDocumentationExample(
				t,
				process,
				providerRunner,
				example.topic,
				example.heading,
				example.workerName,
				example.workstationName,
			)
		})
	}
}

func testFactoryDocumentationExample(
	t *testing.T,
	process support.Process,
	providerRunner *support.ShapedProviderCommandRunner,
	topic, heading, workerName, workstationName string,
) {
	t.Helper()
	env := isolatedDocumentationEnvironment(t)
	markdown := executeDocumentationCommand(t, process, env, documentationProcess(t).tempDir(t), "docs", topic)
	factoryDir := documentationProcess(t).tempDir(t)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, extractJSONExample(t, markdown, heading), 0o600); err != nil {
		t.Fatalf("write copied %s Factory example: %v", topic, err)
	}
	support.WriteAgentConfig(
		t,
		factoryDir,
		workerName,
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"),
	)
	support.WriteWorkstationConfig(
		t,
		factoryDir,
		workstationName,
		"---\ntype: MODEL_WORKSTATION\n---\nProcess the documentation example.\n",
	)

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
	providerCallsBefore := providerRunner.CallCount()
	run := executeDocumentationCommandResult(t, process, env, factoryDir,
		"run", "--dir", factoryDir, "--no-record", "--quiet", "--work", workPath,
	)
	if run.err != nil {
		t.Fatalf("run copied %s Factory example: %v\nstdout:\n%s\nstderr:\n%s", topic, run.err, run.stdout, run.stderr)
	}
	if !strings.Contains(run.stdout, "Batch completed successfully.") {
		t.Fatalf("run copied %s Factory example omitted completion:\n%s", topic, run.stdout)
	}
	if got := providerRunner.CallCount() - providerCallsBefore; got != 1 {
		t.Fatalf("provider command runner calls for copied %s Factory example = %d, want 1", topic, got)
	}
}

func testModelsDocumentation(t *testing.T, process support.Process) {
	t.Helper()
	env := isolatedDocumentationEnvironment(t)
	markdown := executeDocumentationCommand(t, process, env, documentationProcess(t).tempDir(t), "docs", "models")
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
	help := executeDocumentationCommand(t, process, env, documentationProcess(t).tempDir(t), "models", "invoke", "--help")
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
	var stdout, stderr bytes.Buffer
	err := executeDocumentationCommandInto(t, process, env, workingDirectory, &stdout, &stderr, args...)
	return documentationCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func isolatedDocumentationEnvironment(t *testing.T) []string {
	t.Helper()
	return append([]string(nil), os.Environ()...)
}

func executeDocumentationCommandInto(
	t *testing.T,
	process support.Process,
	env []string,
	workingDirectory string,
	stdout, stderr io.Writer,
	args ...string,
) error {
	t.Helper()
	fixture := documentationProcess(t)
	finishInvocation := fixture.ledger.beginInvocation()
	defer finishInvocation()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = fixture.freshEnvironment(t, env)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = stderr
	return process.Execute(inputs.Input)
}

var errDocumentationOutput = errors.New("docs output writer failed")

type boundedDocumentationWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (writer *boundedDocumentationWriter) Write(p []byte) (int, error) {
	remaining := writer.limit - writer.buffer.Len()
	if remaining <= 0 {
		return 0, errDocumentationOutput
	}
	if remaining < len(p) {
		_, _ = writer.buffer.Write(p[:remaining])
		return remaining, errDocumentationOutput
	}
	return writer.buffer.Write(p)
}

func (writer *boundedDocumentationWriter) String() string {
	return writer.buffer.String()
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
