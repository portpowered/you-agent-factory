package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCoverageBuildTraceCountsCompilerAndLinkerActions(t *testing.T) {
	t.Parallel()

	trace := strings.Join([]string{
		"WORK=/tmp/go-build",
		"mkdir -p $WORK/b001/",
		`"/go/pkg/tool/linux_amd64/compile" -o "$WORK/b001/_pkg_.a" -p example.test`,
		`"/go/pkg/tool/linux_amd64/link" -o "$WORK/b001/example.test"`,
	}, "\n")

	got, err := parseCoverageBuildTrace(trace, 4)
	if err != nil {
		t.Fatalf("parseCoverageBuildTrace() error = %v", err)
	}
	if got.CompilerCommands != 1 || got.LinkerCommands != 1 {
		t.Fatalf("action counts = %+v, want one compiler and linker action", got)
	}
	if got.Misses != 2 || got.InferredHits != 2 || got.ExpectedPackages != 4 {
		t.Fatalf("cache inference = %+v, want misses=2 inferredHits=2 expectedPackages=4", got)
	}
}

func TestParseCoverageBuildTraceAcceptsAllHitTrace(t *testing.T) {
	t.Parallel()

	got, err := parseCoverageBuildTrace("WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", 3)
	if err != nil {
		t.Fatalf("parseCoverageBuildTrace() error = %v, want cached trace to be classifiable", err)
	}
	if got.CompilerCommands != 0 || got.LinkerCommands != 0 || got.Misses != 0 || got.InferredHits != 3 {
		t.Fatalf("cached trace summary = %+v, want all three actions inferred as hits", got)
	}
}

func TestParseCoverageBuildTraceRejectsUnclassifiableTrace(t *testing.T) {
	t.Parallel()

	_, err := parseCoverageBuildTrace("compiler output without go test -x markers", 1)
	if err == nil || !strings.Contains(err.Error(), "unclassifiable go test -x trace") {
		t.Fatalf("parseCoverageBuildTrace() error = %v, want unclassifiable trace diagnostic", err)
	}
}

func TestBuildCoverageCompileProbeInvocationPreservesCoverageShape(t *testing.T) {
	t.Parallel()

	invocation, err := buildCoverageCompileProbeInvocation(commandInvocation{
		name: "go",
		args: []string{
			"test",
			"-coverpkg=github.com/example/project/pkg/...",
			"-p=8",
			"-short",
			"-covermode=count",
			"-timeout=10m",
			"-json",
			"-coverprofile=/tmp/coverage.out",
			"-run=TestSubmit",
			"./tests/functional/work",
		},
	}, filepath.Join(t.TempDir(), "compile-probe-bin"))
	if err != nil {
		t.Fatalf("buildCoverageCompileProbeInvocation() error = %v", err)
	}

	if got, want := invocation.args[:5], []string{"test", "-c", "-o", invocation.args[3], "-x"}; !slicesEqual(got, want) {
		t.Fatalf("probe prefix = %v, want %v", got, want)
	}
	for _, forbidden := range []string{"-json", "-coverprofile=/tmp/coverage.out", "-count=1", "-parallel"} {
		if slicesContains(invocation.args, forbidden) {
			t.Fatalf("probe args = %v, must not contain %q", invocation.args, forbidden)
		}
	}
	for _, required := range []string{"-coverpkg=github.com/example/project/pkg/...", "-p=8", "-covermode=count", "-timeout=10m", "-run=TestSubmit", "./tests/functional/work"} {
		if !slicesContains(invocation.args, required) {
			t.Fatalf("probe args = %v, missing preserved argument %q", invocation.args, required)
		}
	}
}

func TestBuildCoverageCompileProbeInvocationsSplitDuplicatePackageNames(t *testing.T) {
	t.Parallel()

	probes, err := buildCoverageCompileProbeInvocations(commandInvocation{
		name: "go",
		args: []string{
			"test",
			"-run=TestSubmit",
			"-coverprofile=coverage.out",
			"-json",
			"github.com/example/recordings/process",
			"github.com/example/transport/process",
			"github.com/example/transport/stdio",
		},
	}, filepath.Join(t.TempDir(), "compile-probe-bin"))
	if err != nil {
		t.Fatalf("buildCoverageCompileProbeInvocations() error = %v", err)
	}
	if len(probes) != 2 {
		t.Fatalf("probe count = %d, want two unique-basename groups", len(probes))
	}

	for _, probe := range probes {
		processCount := 0
		for _, packagePath := range []string{
			"github.com/example/recordings/process",
			"github.com/example/transport/process",
		} {
			if slicesContains(probe.args, packagePath) {
				processCount++
			}
		}
		if processCount > 1 {
			t.Fatalf("probe args = %v, duplicate process basenames remain in one command", probe.args)
		}
	}
	if !slicesContains(probes[0].args, "github.com/example/transport/stdio") {
		t.Fatalf("first probe args = %v, distinct basename was not packed with the first group", probes[0].args)
	}
}

func TestCoverageBuildDiagnosticRunWritesCompleteSummaryAndCleansBinaries(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutWriter = &stdout
	stderrWriter = &stderr
	var invocations []commandInvocation
	var probeDir string
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		if slicesContains(invocation.args, "-c") {
			probeDir = argumentAfter(invocation.args, "-o")
			if err := os.WriteFile(filepath.Join(probeDir, "fake.test"), []byte("binary"), 0o600); err != nil {
				return "", "", err
			}
			return strings.Join([]string{
				"WORK=/tmp/go-build",
				"mkdir -p $WORK/b001/",
				`"/go/pkg/tool/linux_amd64/compile" -o "$WORK/b001/_pkg_.a" -p example.test`,
				`"/go/pkg/tool/linux_amd64/link" -o "$WORK/b001/example.test"`,
			}, "\n"), "", nil
		}
		if !helperHasArgPrefix(invocation.args, "-coverprofile=") {
			return "", "", errors.New("unexpected coverage test invocation")
		}
		return "coverage test ran once\n", "", nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	plan := coverageInvocationPlan{
		invocations: []commandInvocation{{
			name: "go",
			args: []string{"test", "-coverpkg=example/pkg/...", "-coverprofile=coverage.out", "example/tests/functional/work"},
		}},
		cleanup: func() error { return nil },
	}
	if err := executeCoverageInvocationPlan(
		config{coverageBuildDiagnosticsOutput: diagnosticPath},
		plan,
		[]string{"example/tests/functional/work"},
		filepath.Join(t.TempDir(), "coverage.out"),
		".",
		[]string{"example/pkg/config"},
		"run coverage",
		nil,
	); err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v", err)
	}

	if len(invocations) != 2 {
		t.Fatalf("go invocations = %d, want one compile probe and one test run", len(invocations))
	}
	if _, err := os.Stat(probeDir); !os.IsNotExist(err) {
		t.Fatalf("compile probe directory still exists, stat error = %v", err)
	}
	diagnostics := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusComplete {
		t.Fatalf("diagnostic status = %q, want complete", diagnostics.Status)
	}
	if diagnostics.SchemaVersion != coverageBuildDiagnosticsSchemaVersion || diagnostics.GoVersion == "" || !strings.HasPrefix(diagnostics.CoverageInvocationHash, "sha256:") {
		t.Fatalf("diagnostic identity = %+v, want version, Go version, and sha256 hash", diagnostics)
	}
	if diagnostics.CompileProbe.ExpectedPackages != 1 || diagnostics.CompileProbe.CompilerCommands != 1 || diagnostics.CompileProbe.LinkerCommands != 1 || diagnostics.CompileProbe.Misses != 2 {
		t.Fatalf("compile probe summary = %+v, want observed command counts", diagnostics.CompileProbe)
	}
	if diagnostics.CompileProbe.WallSeconds < 0 || diagnostics.TestRunWallSeconds < 0 || diagnostics.TotalMeasuredSeconds < diagnostics.TestRunWallSeconds {
		t.Fatalf("diagnostic timings = %+v, want non-negative sequential measurements", diagnostics)
	}
	if !strings.Contains(stdout.String(), "Coverage build diagnostic:") || stderr.Len() != 0 {
		t.Fatalf("diagnostic streams = stdout %q stderr %q, want concise stdout evidence only", stdout.String(), stderr.String())
	}
}

func TestCoverageBuildDiagnosticFailureStopsTestAndWritesFailedSummary(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	callCount := 0
	var probeDir string
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		callCount++
		if !slicesContains(invocation.args, "-c") {
			return "", "", errors.New("coverage test must not start after probe failure")
		}
		probeDir = argumentAfter(invocation.args, "-o")
		return "WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", "original compiler failure", errors.New("exit status 2")
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	plan := coverageInvocationPlan{
		invocations: []commandInvocation{{
			name: "go",
			args: []string{"test", "-coverpkg=example/pkg/...", "-coverprofile=coverage.out", "example/tests/functional/work"},
		}},
		cleanup: func() error { return nil },
	}
	err := executeCoverageInvocationPlan(
		config{coverageBuildDiagnosticsOutput: diagnosticPath},
		plan,
		[]string{"example/tests/functional/work"},
		"coverage.out",
		".",
		nil,
		"run coverage",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "original compiler failure") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want original compiler failure", err)
	}
	if callCount != 1 {
		t.Fatalf("go invocation count = %d, want probe only", callCount)
	}
	if _, err := os.Stat(probeDir); !os.IsNotExist(err) {
		t.Fatalf("failed compile probe directory still exists, stat error = %v", err)
	}
	diagnostics := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusFailed {
		t.Fatalf("diagnostic status = %q, want failed", diagnostics.Status)
	}
	if diagnostics.TestRunWallSeconds != 0 || diagnostics.TotalMeasuredSeconds != diagnostics.CompileProbe.WallSeconds {
		t.Fatalf("failed diagnostic timings = %+v, want no test timing and compile-only total", diagnostics)
	}
}

func TestCoverageBuildDiagnosticUnclassifiableTraceStopsTest(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	callCount := 0
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		callCount++
		if !slicesContains(invocation.args, "-c") {
			return "", "", errors.New("coverage test must not start after an unclassifiable probe")
		}
		return "WORK=/tmp/go-build\n", "", nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	plan := coverageInvocationPlan{
		invocations: []commandInvocation{{
			name: "go",
			args: []string{"test", "-coverpkg=example/pkg/...", "-coverprofile=coverage.out", "example/tests/functional/work"},
		}},
		cleanup: func() error { return nil },
	}
	err := executeCoverageInvocationPlan(
		config{coverageBuildDiagnosticsOutput: diagnosticPath},
		plan,
		[]string{"example/tests/functional/work"},
		"coverage.out",
		".",
		nil,
		"run coverage",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "unclassifiable go test -x trace") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want unclassifiable trace diagnostic", err)
	}
	if callCount != 1 {
		t.Fatalf("go invocation count = %d, want probe only", callCount)
	}
	diagnostics := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusFailed {
		t.Fatalf("diagnostic status = %q, want failed", diagnostics.Status)
	}
}

func TestCoverageBuildDiagnosticDisabledPreservesSingleTestInvocation(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		return "", "", nil
	}
	plan := coverageInvocationPlan{
		invocations: []commandInvocation{{
			name: "go",
			args: []string{"test", "-coverpkg=example/pkg/...", "-coverprofile=coverage.out", "example/tests/functional/work"},
		}},
		cleanup: func() error { return nil },
	}
	if err := executeCoverageInvocationPlan(config{}, plan, []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil); err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v", err)
	}
	if len(invocations) != 1 || slicesContains(invocations[0].args, "-c") {
		t.Fatalf("default diagnostic invocations = %+v, want one unchanged test invocation", invocations)
	}
}

func TestCoverageInvocationHashIgnoresProfilePath(t *testing.T) {
	t.Parallel()

	left := coverageInvocationPlan{invocations: []commandInvocation{{name: "go", args: []string{"test", "-coverprofile=/tmp/left.out", "./tests/functional"}}}}
	right := coverageInvocationPlan{invocations: []commandInvocation{{name: "go", args: []string{"test", "-coverprofile=/tmp/right.out", "./tests/functional"}}}}
	if coverageInvocationHash(left) != coverageInvocationHash(right) {
		t.Fatal("coverageInvocationHash() changed when only the profile path changed")
	}
}

func readCoverageBuildDiagnostics(t *testing.T, path string) coverageBuildDiagnosticsJSON {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage build diagnostics: %v", err)
	}
	var diagnostics coverageBuildDiagnosticsJSON
	if err := json.Unmarshal(data, &diagnostics); err != nil {
		t.Fatalf("decode coverage build diagnostics: %v", err)
	}
	return diagnostics
}

func argumentAfter(args []string, name string) string {
	for index, arg := range args[:len(args)-1] {
		if arg == name {
			return args[index+1]
		}
	}
	return ""
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
