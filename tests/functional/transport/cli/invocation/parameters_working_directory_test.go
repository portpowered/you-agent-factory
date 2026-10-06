package invocation_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestCLIRelativeFactoryPathResolvesFromInvocationDirectory proves the public CLI
// selects the Current Factory rooted at ./factory/factory.json relative to the
// invocation working directory, reporting the resolved Factory directory through
// customer-visible startup output.
func TestCLIRelativeFactoryPathResolvesFromInvocationDirectory(t *testing.T) {
	t.Parallel()
	invocationDirectory := t.TempDir()
	factoryDirectory := filepath.Join(invocationDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDirectory, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}

	inputs := parameterInputs(t, []string{
		"you", "run", "--no-record",
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcessesForTest(t).process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(Current Factory from invocation directory) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	wantInitiated := "Factory initiated: " + factoryDirectory
	if !strings.Contains(inputs.Stdout(), wantInitiated) {
		t.Fatalf(
			"stdout omitted resolved Factory root %q:\n%s",
			wantInitiated,
			inputs.Stdout(),
		)
	}
}

// TestCLIWorkingDirectoryDoesNotLeakIntoOutput proves customer-visible portable
// Factory config flatten output omits the absolute host invocation working
// directory while still returning canonical portable JSON from the public CLI.
func TestCLIWorkingDirectoryDoesNotLeakIntoOutput(t *testing.T) {
	t.Parallel()
	invocationDirectory := t.TempDir()
	factoryDirectory := seedFlattenableFactoryUnderInvocation(t, invocationDirectory)

	inputs := parameterInputs(t, []string{
		"you", "factory", "config", "flatten", factoryDirectory,
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcessesForTest(t).process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory config flatten) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	portableOutput := inputs.Stdout() + inputs.Stderr()
	if strings.TrimSpace(portableOutput) == "" {
		t.Fatal("portable flatten output is empty")
	}
	if strings.Contains(portableOutput, invocationDirectory) {
		t.Fatalf(
			"portable output leaked invocation working directory %q:\n%s",
			invocationDirectory,
			portableOutput,
		)
	}
}

// TestCLIMissingWorkingDirectoryAssetFailsActionably proves a missing
// invocation-local Current Factory asset fails with a stable diagnostic before
// provider dispatch or other lifecycle activation side effects can start.
func TestCLIMissingWorkingDirectoryAssetFailsActionably(t *testing.T) {
	t.Parallel()
	invocationDirectory := t.TempDir()
	missingFactoryJSON := filepath.Join(invocationDirectory, "factory", "factory.json")

	beforeLifecycleEffects := parameterProcessesForTest(t).lifecycleEffects.Load()
	beforeProviderCalls := parameterProcessesForTest(t).missingProvider.CallCount()
	inputs := parameterInputs(t, []string{
		"you", "run", "--no-record",
	})
	inputs.Input.WorkingDirectory = invocationDirectory

	if err := parameterProcessesForTest(t).missingAssetProcess.Execute(inputs.Input); err == nil {
		t.Fatalf(
			"missing Current Factory succeeded; stdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	response := requireStartupCLIDiagnostic(t, inputs.Stderr())
	if response.Code != factoryapi.ErrorResponseCode("CURRENT_FACTORY_NOT_FOUND") {
		t.Fatalf("ErrorResponse = %#v, want code CURRENT_FACTORY_NOT_FOUND", response)
	}
	diagnostic := response.Message + inputs.Stderr()
	if !strings.Contains(diagnostic, "factory.json") {
		t.Fatalf(
			"diagnostic omitted missing factory.json asset %q:\n%s",
			missingFactoryJSON,
			diagnostic,
		)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("missing Current Factory stdout = %q, want empty", inputs.Stdout())
	}
	if got := parameterProcessesForTest(t).missingProvider.CallCount() - beforeProviderCalls; got != 0 {
		t.Fatalf("provider dispatch call delta = %d, want 0", got)
	}
	if got := parameterProcessesForTest(t).lifecycleEffects.Load() - beforeLifecycleEffects; got != 0 {
		t.Fatalf("lifecycle activation effect delta = %d, want 0", got)
	}
}

func seedFlattenableFactoryUnderInvocation(t *testing.T, invocationDirectory string) string {
	t.Helper()

	repositoryRoot := testutil.MustRepoRoot(t)
	sourceFactoryDirectory := filepath.Join(repositoryRoot, "examples", "basic", "factory")
	factoryDirectory := filepath.Join(invocationDirectory, "factory")
	if err := copyDirectoryTree(sourceFactoryDirectory, factoryDirectory); err != nil {
		t.Fatalf("copy flattenable factory fixture: %v", err)
	}
	return factoryDirectory
}

func copyDirectoryTree(sourceDirectory, destinationDirectory string) error {
	return filepath.WalkDir(sourceDirectory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDirectory, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destinationDirectory, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, payload, 0o644)
	})
}

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [{"name": "processor"}],
  "workstations": [
    {
      "name": "process",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "worker": "processor"
    }
  ]
}`

// requireStartupCLIDiagnostic accepts one envelope followed only by the bounded
// local startup cause contract. Callers retain scenario-specific privacy checks.
// Ordinary and remote diagnostics should continue using strict JSON decoding.
func requireStartupCLIDiagnostic(t testing.TB, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
		t.Fatalf("decode startup envelope: %v; stderr=%q", err, stderr)
	}
	if response.Code == "" || response.Family == "" || response.Message == "" {
		t.Fatalf("incomplete startup envelope: %#v", response)
	}
	if len(lines) > 18 {
		t.Fatalf("startup causes exceed 16 nodes plus truncation: %q", stderr)
	}
	for index, line := range lines[1:] {
		prefix := fmt.Sprintf("cause[%d]=", index)
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("unexpected trailing startup diagnostic: %q", line)
		}
		cause := strings.TrimPrefix(line, prefix)
		if cause == "" || len(cause) > 515 || unsafeStartupCause.MatchString(cause) {
			t.Fatalf("unbounded or unsafe startup cause: %q", line)
		}
	}
	return response
}

var unsafeStartupCause = regexp.MustCompile(`(?i)(?:^|[\s=("'])(?:[A-Za-z]:[\\/]|\\\\|\.\.?[\\/]|~/|/)[^\s]+|https?://[^\s]*[?@#]|\b(?:password|secret|token|prompt|payload|body|authorization)\s*[:=]\s*(?:[^<\s]|<(?:[^r]|r[^e]))`)
