package help_test

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
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

	fixture := helpPackageFixtureForTest(t)
	result := fixture.execute(t,
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	)
	if result.err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			result.err,
			result.inputs.Stdout(),
			result.inputs.Stderr(),
		)
	}

	got := result.inputs.Stdout()
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

	duplicateResult := fixture.execute(t,
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help", "--help",
	)
	if duplicateResult.err != nil {
		t.Fatalf(
			"Process.Execute(duplicate --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			duplicateResult.err,
			duplicateResult.inputs.Stdout(),
			duplicateResult.inputs.Stderr(),
		)
	}
	if duplicateResult.inputs.Stdout() != got {
		t.Fatalf("duplicate --help output = %q, want idempotent output %q", duplicateResult.inputs.Stdout(), got)
	}
	if duplicateResult.inputs.Stderr() != result.inputs.Stderr() {
		t.Fatalf("duplicate --help stderr = %q, want %q", duplicateResult.inputs.Stderr(), result.inputs.Stderr())
	}
}

// TestCLIRunHelpDistinguishesRequiredAndOptionalParameters proves you run --named
// <factory> --help visibly marks required and optional invocationSignature
// parameters so operators know which arguments they must supply before a run.
func TestCLIRunHelpDistinguishesRequiredAndOptionalParameters(t *testing.T) {
	t.Parallel()

	fixture := helpPackageFixtureForTest(t)
	result := fixture.execute(t,
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	)
	if result.err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			result.err,
			result.inputs.Stdout(),
			result.inputs.Stderr(),
		)
	}

	got := result.inputs.Stdout()
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

	fixture := helpPackageFixtureForTest(t)
	result := fixture.execute(t,
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--help",
	)
	if result.err != nil {
		t.Fatalf(
			"Process.Execute(run --named %s --help) error = %v\nstdout:\n%s\nstderr:\n%s",
			invocationHelpNamedFactoryName,
			result.err,
			result.inputs.Stdout(),
			result.inputs.Stderr(),
		)
	}
	if fixture.providerRunner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner call count = %d, want 0 for read-only run help",
			fixture.providerRunner.CallCount(),
		)
	}

	got := result.inputs.Stdout()
	if !strings.Contains(got, "Factory invocation help") {
		t.Fatalf("run --named %s --help missing Factory invocation help:\n%s", invocationHelpNamedFactoryName, got)
	}
	if !strings.Contains(got, "Selected factory: "+invocationHelpFactoryConfigName+" (named factory "+invocationHelpNamedFactoryName+")") {
		t.Fatalf("run --named %s --help missing selected factory line:\n%s", invocationHelpNamedFactoryName, got)
	}

	conflictResult := fixture.execute(t,
		"you", "run",
		"--named", invocationHelpNamedFactoryName,
		"--factory", fixture.fullFactoryPath,
		"--help",
	)
	conflictErr := conflictResult.err
	if conflictErr == nil || !strings.Contains(conflictErr.Error(), "--named cannot be used with --factory") {
		t.Fatalf("conflicting --named/--factory error = %v, want stable selection conflict", conflictErr)
	}
	if conflictResult.inputs.Stdout() != "" {
		t.Fatalf("conflicting --named/--factory stdout = %q, want empty", conflictResult.inputs.Stdout())
	}
	if diagnostic := support.RequireSafeCLIDiagnostic(t, conflictResult.inputs.Stderr()); diagnostic.Code != "CLI_COMMAND_FAILED" {
		t.Fatalf("conflicting --named/--factory diagnostic code = %q, want CLI_COMMAND_FAILED", diagnostic.Code)
	}
	if fixture.providerRunner.CallCount() != 0 {
		t.Fatalf("provider command runner call count after help conflict = %d, want 0", fixture.providerRunner.CallCount())
	}
}

// TestCLISessionHelpPublishesRunnablePlacementExamples proves the packaged
// session guidance is executable CLI behavior: placement examples use the
// accepted run grammar and do not advertise flags that belong to another
// command family.
func TestCLISessionHelpPublishesRunnablePlacementExamples(t *testing.T) {
	t.Parallel()

	fixture := helpPackageFixtureForTest(t)
	for _, command := range [][]string{
		{"you", "session", "--help"},
		{"you", "session", "pause", "--help"},
		{"you", "session", "resume", "--help"},
	} {
		result := fixture.execute(t, command...)
		if result.err != nil {
			t.Fatalf("Process.Execute(%q) error = %v\nstdout:\n%s\nstderr:\n%s", command, result.err, result.inputs.Stdout(), result.inputs.Stderr())
		}
		got := result.inputs.Stdout()
		if !strings.Contains(got, "--remote --server http://factory.example:7437") {
			t.Fatalf("help %q missing explicit remote placement example:\n%s", command, got)
		}
		if !strings.Contains(got, "run --named @you/research --output primary") {
			t.Fatalf("help %q missing runnable run example:\n%s", command, got)
		}
		for _, invalid := range []string{"you run --wait", "run --follow"} {
			if strings.Contains(got, invalid) {
				t.Fatalf("help %q advertises invalid example %q:\n%s", command, invalid, got)
			}
		}
	}
}

// TestCLIRunHelpCoversGenericAndExplicitFactorySelections proves the generic
// run help remains byte-for-byte stable and an explicit Factory path exposes
// the same authored invocation signature as the named selection.
func TestCLIRunHelpCoversGenericAndExplicitFactorySelections(t *testing.T) {
	t.Parallel()

	fixture := helpPackageFixtureForTest(t)
	generic := fixture.execute(t, "you", "run", "--help")
	if generic.err != nil {
		t.Fatalf("generic run help error = %v\nstdout:\n%s\nstderr:\n%s", generic.err, generic.inputs.Stdout(), generic.inputs.Stderr())
	}
	wantGeneric, err := os.ReadFile(testutil.MustRepoPath(t, "pkg/transports/cli/baseline/testdata/run_help.txt"))
	if err != nil {
		t.Fatalf("read canonical run help baseline: %v", err)
	}
	if generic.inputs.Stdout() != string(wantGeneric) {
		t.Fatalf("generic run help drifted from the canonical public baseline")
	}
	genericRepeat := fixture.execute(t, "you", "run", "--help")
	if genericRepeat.err != nil {
		t.Fatalf("repeated generic run help error = %v", genericRepeat.err)
	}
	if genericRepeat.inputs.Stdout() != generic.inputs.Stdout() || genericRepeat.inputs.Stderr() != generic.inputs.Stderr() {
		t.Fatalf("repeated generic run help changed output or diagnostics")
	}

	named := fixture.execute(t, "you", "run", "--named", invocationHelpNamedFactoryName, "--help")
	explicit := fixture.execute(t, "you", "run", "--factory", fixture.fullFactoryPath, "--help")
	for name, result := range map[string]helpInvocationResult{
		"named":    named,
		"explicit": explicit,
	} {
		if result.err != nil {
			t.Fatalf("%s Factory help error = %v\nstdout:\n%s\nstderr:\n%s", name, result.err, result.inputs.Stdout(), result.inputs.Stderr())
		}
		assertFullInvocationHelp(t, result.inputs.Stdout())
	}
	if !strings.Contains(explicit.inputs.Stdout(), "Selected factory: "+invocationHelpFactoryConfigName+" (factory config "+fixture.fullFactoryPath+")") {
		t.Fatalf("explicit Factory help missing selected path identity:\n%s", explicit.inputs.Stdout())
	}
	if !strings.Contains(named.inputs.Stdout(), "Usage:\n  you run --named "+invocationHelpNamedFactoryName) ||
		!strings.Contains(explicit.inputs.Stdout(), "Usage:\n  you run --factory "+fixture.fullFactoryPath) {
		t.Fatalf("named and explicit help did not preserve their selection grammar")
	}
}

// TestCLIRunHelpResetsEmptyAndInvalidSelections proves an empty signature can
// be followed by a full signature on the same process, while missing and
// malformed selections fail before help text or external work are produced.
func TestCLIRunHelpResetsEmptyAndInvalidSelections(t *testing.T) {
	t.Parallel()

	fixture := helpPackageFixtureForTest(t)
	empty := fixture.execute(t, "you", "run", "--factory", fixture.emptyFactoryPath, "--help")
	if empty.err != nil {
		t.Fatalf("empty signature help error = %v\nstdout:\n%s\nstderr:\n%s", empty.err, empty.inputs.Stdout(), empty.inputs.Stderr())
	}
	emptyOutput := empty.inputs.Stdout()
	if !strings.Contains(emptyOutput, "Factory invocation help") ||
		strings.Contains(emptyOutput, invocationHelpRequiredParameter) ||
		strings.Contains(emptyOutput, invocationHelpOptionalParameter) ||
		strings.Contains(emptyOutput, invocationHelpOptionalPathParameter) ||
		strings.Contains(emptyOutput, "Examples:") {
		t.Fatalf("empty signature help retained stale full-signature text:\n%s", emptyOutput)
	}

	full := fixture.execute(t, "you", "run", "--factory", fixture.fullFactoryPath, "--help")
	if full.err != nil {
		t.Fatalf("full signature after empty help error = %v\nstdout:\n%s\nstderr:\n%s", full.err, full.inputs.Stdout(), full.inputs.Stderr())
	}
	assertFullInvocationHelp(t, full.inputs.Stdout())

	missing := fixture.execute(t, "you", "run", "--named", "invocation-help-missing", "--help")
	if missing.err == nil || !strings.Contains(missing.err.Error(), "invocation-help-missing") {
		t.Fatalf("missing named help error = %v, want stable missing-name error", missing.err)
	}
	if missing.inputs.Stdout() != "" {
		t.Fatalf("missing named help stdout = %q, want empty", missing.inputs.Stdout())
	}
	support.RequireSafeCLIDiagnostic(t, missing.inputs.Stderr())

	malformed := fixture.execute(t, "you", "run", "--factory", fixture.malformedFactoryPath, "--help")
	if malformed.err == nil {
		t.Fatal("malformed explicit Factory help error = nil, want validation failure")
	}
	if malformed.inputs.Stdout() != "" || strings.Contains(malformed.inputs.Stdout(), "Factory invocation help") {
		t.Fatalf("malformed explicit Factory help stdout = %q, want no fabricated help", malformed.inputs.Stdout())
	}
	support.RequireSafeCLIDiagnostic(t, malformed.inputs.Stderr())
	if fixture.providerRunner.CallCount() != 0 {
		t.Fatalf("provider command runner calls after help matrix = %d, want 0", fixture.providerRunner.CallCount())
	}
}

func assertFullInvocationHelp(t *testing.T, output string) {
	t.Helper()
	for _, want := range []string{
		"Factory invocation help",
		"Factory-defined arguments:",
		invocationHelpRequiredParameter,
		invocationHelpOptionalParameter,
		invocationHelpOptionalPathParameter,
		"Examples:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("full invocation help missing %q:\n%s", want, output)
		}
	}
	usage := invocationHelpUsageLine(t, output)
	for _, want := range []string{
		"<" + invocationHelpRequiredParameter + ">",
		"[--" + invocationHelpOptionalPathParameter + " <file-path>]",
		"[--" + invocationHelpOptionalParameter + " <value>]",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("full invocation help usage missing %q:\n%s", want, usage)
		}
	}
	assertInvocationHelpParameterRequirement(t, output, invocationHelpRequiredParameter, true)
	assertInvocationHelpParameterRequirement(t, output, invocationHelpOptionalParameter, false)
	assertInvocationHelpParameterRequirement(t, output, invocationHelpOptionalPathParameter, false)
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
