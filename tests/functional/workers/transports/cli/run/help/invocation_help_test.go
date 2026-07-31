package help_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	invocationHelpNamedFactoryName      = "invocation-help-alpha"
	invocationHelpFactoryConfigName     = "invocation-help-portable"
	invocationHelpWorkTypeName          = "help-task"
	invocationHelpRequiredParameter     = "input"
	invocationHelpOptionalParameter     = "mode"
	invocationHelpOptionalPathParameter = "artifact"
)

// TestCLIRunHelpShowsInvocationSignatureForNamedFactory proves you run --named
// <factory> --help prints Factory invocation help for the selected named Factory,
// including signature usage and the factory-defined argument surface customers
// use to compose a run command without starting Work.
func TestCLIRunHelpShowsInvocationSignatureForNamedFactory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	sourceDir := support.ScaffoldFactory(t, invocationHelpFactoryConfig())
	support.CreateNamedFactory(
		t,
		homeDir,
		workingDirectory,
		invocationHelpNamedFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	got := inputs.Stdout()
	for _, want := range []string{
		"Factory invocation help",
		"Selected factory: " + invocationHelpFactoryConfigName + " (named factory " + invocationHelpNamedFactoryName + ")",
		"Usage:\n  you run --named " + invocationHelpNamedFactoryName + " <" + invocationHelpRequiredParameter + "> [--" + invocationHelpOptionalPathParameter + " <file-path>] [--" + invocationHelpOptionalParameter + " <value>]",
		"Factory-defined arguments:",
		invocationHelpRequiredParameter,
		invocationHelpOptionalParameter,
		invocationHelpOptionalPathParameter,
		"you run --named " + invocationHelpNamedFactoryName + " 'Fix the lint issues' --" + invocationHelpOptionalPathParameter + " report.md --" + invocationHelpOptionalParameter + " safe",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run --named %s --help missing %q:\n%s", invocationHelpNamedFactoryName, want, got)
		}
	}
	if strings.Contains(got, "Load workflow and run the factory engine") {
		t.Fatalf("expected signature-aware help instead of generic Cobra help:\n%s", got)
	}
}

// TestCLIRunHelpDistinguishesRequiredAndOptionalParameters proves you run --named
// <factory> --help visibly marks required and optional invocationSignature
// parameters so operators know which arguments they must supply before a run.
func TestCLIRunHelpDistinguishesRequiredAndOptionalParameters(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	sourceDir := support.ScaffoldFactory(t, invocationHelpFactoryConfig())
	support.CreateNamedFactory(
		t,
		homeDir,
		workingDirectory,
		invocationHelpNamedFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	got := inputs.Stdout()
	usageLine := invocationHelpUsageLine(t, got)
	if !strings.Contains(usageLine, "<"+invocationHelpRequiredParameter+">") {
		t.Fatalf("usage line missing required token %q:\n%s", invocationHelpRequiredParameter, usageLine)
	}
	if strings.Contains(usageLine, "[<"+invocationHelpRequiredParameter+">]") {
		t.Fatalf("required parameter %q must not be bracketed in usage:\n%s", invocationHelpRequiredParameter, usageLine)
	}
	for _, optionalParameter := range []string{
		invocationHelpOptionalParameter,
		invocationHelpOptionalPathParameter,
	} {
		if !strings.Contains(usageLine, "[--"+optionalParameter) {
			t.Fatalf("usage line missing bracketed optional token for %q:\n%s", optionalParameter, usageLine)
		}
	}

	assertInvocationHelpParameterRequirement(t, got, invocationHelpRequiredParameter, true)
	assertInvocationHelpParameterRequirement(t, got, invocationHelpOptionalParameter, false)
	assertInvocationHelpParameterRequirement(t, got, invocationHelpOptionalPathParameter, false)
}

// TestCLIRunHelpDoesNotDispatchExternalWork proves you run --named <factory>
// --help completes as read-only Factory invocation discovery without invoking
// external provider command execution or worker dispatch.
func TestCLIRunHelpDoesNotDispatchExternalWork(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	sourceDir := support.ScaffoldFactory(t, invocationHelpFactoryConfig())
	support.CreateNamedFactory(
		t,
		homeDir,
		workingDirectory,
		invocationHelpNamedFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	runner := testutil.NewProviderCommandRunner()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if runner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner call count = %d, want 0 for read-only run help",
			runner.CallCount(),
		)
	}

	got := inputs.Stdout()
	if !strings.Contains(got, "Factory invocation help") {
		t.Fatalf("run --named %s --help missing Factory invocation help:\n%s", invocationHelpNamedFactoryName, got)
	}
	if !strings.Contains(got, "Selected factory: "+invocationHelpFactoryConfigName+" (named factory "+invocationHelpNamedFactoryName+")") {
		t.Fatalf("run --named %s --help missing selected factory line:\n%s", invocationHelpNamedFactoryName, got)
	}
}

func invocationHelpUsageLine(t *testing.T, help string) string {
	t.Helper()

	const usagePrefix = "Usage:\n  "
	start := strings.Index(help, usagePrefix)
	if start < 0 {
		t.Fatalf("help missing usage section:\n%s", help)
	}
	rest := help[start+len(usagePrefix):]
	end := strings.Index(rest, "\n")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func assertInvocationHelpParameterRequirement(t *testing.T, help, parameter string, required bool) {
	t.Helper()

	marker := "Factory-defined arguments:"
	start := strings.Index(help, marker)
	if start < 0 {
		t.Fatalf("help missing %q section:\n%s", marker, help)
	}
	argumentsSection := help[start+len(marker):]
	if end := strings.Index(argumentsSection, "\n\n"); end >= 0 {
		argumentsSection = argumentsSection[:end]
	}

	parameterSection, ok := invocationHelpParameterSection(argumentsSection, parameter)
	if !ok {
		t.Fatalf("help missing parameter %q:\n%s", parameter, help)
	}

	if required {
		if !strings.Contains(parameterSection, "Required.") {
			t.Fatalf("parameter %q missing Required. label:\n%s", parameter, parameterSection)
		}
		if strings.Contains(parameterSection, "\n    Optional.") {
			t.Fatalf("parameter %q incorrectly marked optional:\n%s", parameter, parameterSection)
		}
		return
	}
	if !strings.Contains(parameterSection, "Optional.") {
		t.Fatalf("parameter %q missing Optional. label:\n%s", parameter, parameterSection)
	}
}

func invocationHelpParameterSection(argumentsSection, parameter string) (string, bool) {
	lines := strings.Split(argumentsSection, "\n")
	for index, line := range lines {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") {
			continue
		}
		if !invocationHelpParameterHeaderMatches(line, parameter) {
			continue
		}
		block := []string{line}
		for _, detail := range lines[index+1:] {
			if strings.HasPrefix(detail, "  ") && !strings.HasPrefix(detail, "    ") {
				break
			}
			block = append(block, detail)
		}
		return strings.Join(block, "\n"), true
	}
	return "", false
}

func invocationHelpParameterHeaderMatches(headerLine, parameter string) bool {
	return strings.Contains(headerLine, "<"+parameter+">") ||
		strings.Contains(headerLine, "--"+parameter+" ") ||
		strings.Contains(headerLine, "--"+parameter+"|")
}

func invocationHelpFactoryConfig() map[string]any {
	return map[string]any{
		"name": invocationHelpFactoryConfigName,
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":        invocationHelpRequiredParameter,
					"description": "Primary text input for the portable factory.",
					"required":    true,
					"bindings": []any{
						map[string]any{"kind": "POSITIONAL", "position": 1},
						map[string]any{"kind": "STDIN"},
					},
				},
				map[string]any{
					"name":         invocationHelpOptionalParameter,
					"description":  "Execution mode for the portable factory.",
					"choices":      []any{"fast", "safe"},
					"defaultValue": "safe",
					"bindings":     []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":        invocationHelpOptionalPathParameter,
					"description": "Optional output file path.",
					"aliases":     []any{"out"},
					"typeHint":    "FILE_PATH",
					"bindings":    []any{map[string]any{"kind": "NAMED"}},
				},
			},
			"outputContract": map[string]any{
				"mode":          "FILE",
				"pathParameter": invocationHelpOptionalPathParameter,
				"contentType":   "text/plain",
				"fileExtension": ".txt",
			},
			"examples": []any{
				map[string]any{
					"name": "positional-input",
					"argv": []any{
						"Fix the lint issues",
						"--" + invocationHelpOptionalParameter,
						"safe",
						"--" + invocationHelpOptionalPathParameter,
						"report.md",
					},
				},
			},
		},
		"workTypes": []any{
			map[string]any{
				"name":             invocationHelpWorkTypeName,
				"handlingBehavior": []any{"DEFAULT"},
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "complete", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []any{
			map[string]any{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "init"}},
				"outputs":   []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": invocationHelpWorkTypeName, "state": "failed"}},
			},
		},
	}
}
