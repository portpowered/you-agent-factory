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
	trace := strings.Join([]string{
		"WORK=/tmp/go-build",
		"mkdir -p $WORK/b001/",
		`"/go/pkg/tool/linux_amd64/compile" -o "$WORK/b001/_pkg_.a" -p example.test`,
		`"/go/pkg/tool/linux_amd64/link" -o "$WORK/b001/example.test"`,
	}, "\n")

	got, err := parseCoverageBuildTrace(trace)
	if err != nil {
		t.Fatalf("parseCoverageBuildTrace() error = %v", err)
	}
	if !got.classifiable || got.compilerCommands != 1 || got.linkerCommands != 1 || got.buildActions != 2 {
		t.Fatalf("trace summary = %+v, want one compiler, one linker, and two build actions", got)
	}
}

func TestParseCoverageBuildTraceAcceptsZeroActionTrace(t *testing.T) {
	got, err := parseCoverageBuildTrace("WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n")
	if err != nil {
		t.Fatalf("parseCoverageBuildTrace() error = %v, want classifiable zero-action trace", err)
	}
	if !got.classifiable || got.compilerCommands != 0 || got.linkerCommands != 0 || got.buildActions != 0 {
		t.Fatalf("zero-action trace summary = %+v", got)
	}
}

func TestParseCoverageBuildTraceRejectsUnclassifiableTrace(t *testing.T) {
	_, err := parseCoverageBuildTrace("compiler output without go test -x markers")
	if err == nil || !strings.Contains(err.Error(), "unclassifiable go test -x trace") {
		t.Fatalf("parseCoverageBuildTrace() error = %v, want unclassifiable trace diagnostic", err)
	}
}

func TestAddCoverageBuildDiagnosticFlagsInstrumentsExactCoverageInvocation(t *testing.T) {
	plan := diagnosticCoveragePlan()
	if err := addCoverageBuildDiagnosticFlags(&plan); err != nil {
		t.Fatalf("addCoverageBuildDiagnosticFlags() error = %v", err)
	}

	if len(plan.invocations) != 1 {
		t.Fatalf("invocation count = %d, want one", len(plan.invocations))
	}
	args := plan.invocations[0].args
	if slicesContains(args, "-c") {
		t.Fatalf("diagnostic args = %v, must not compile a probe", args)
	}
	if countArgs(args, "-x") != 1 || countArgs(args, "-json") != 1 {
		t.Fatalf("diagnostic args = %v, want one -x and one -json", args)
	}
	if !helperHasArgPrefix(args, "-coverprofile=") || !slicesContains(args, "example/tests/functional/work") {
		t.Fatalf("diagnostic args = %v, lost coverage profile or package selection", args)
	}
	if indexOf(args, "-x") > indexOf(args, "example/tests/functional/work") || indexOf(args, "-json") > indexOf(args, "example/tests/functional/work") {
		t.Fatalf("diagnostic flags must precede package arguments: %v", args)
	}
}

func TestCoverageBuildDiagnosticRunWritesSchemaV2FromOneExactInvocation(t *testing.T) {
	setActionCacheIdentity(t, "build-key", "build-key", "true")

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
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		if slicesContains(invocation.args, "-c") {
			return "", "", errors.New("compile probe must not run")
		}
		if countArgs(invocation.args, "-x") != 1 || countArgs(invocation.args, "-json") != 1 {
			return "", "", errors.New("diagnostics must instrument the coverage command")
		}
		return "coverage test ran once\n", "WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	if err := executeCoverageInvocationPlan(
		config{coverageBuildDiagnosticsOutput: diagnosticPath},
		diagnosticCoveragePlan(),
		[]string{"example/tests/functional/work"},
		filepath.Join(t.TempDir(), "coverage.out"),
		".",
		[]string{"example/pkg/config"},
		"run coverage",
		nil,
	); err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v", err)
	}

	if len(invocations) != 1 {
		t.Fatalf("go invocations = %d, want one authoritative coverage invocation", len(invocations))
	}
	diagnostics, raw := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusComplete {
		t.Fatalf("diagnostic status = %q, want complete", diagnostics.Status)
	}
	if diagnostics.SchemaVersion != 2 || diagnostics.GoVersion == "" || !strings.HasPrefix(diagnostics.CoverageInvocationHash, "sha256:") {
		t.Fatalf("diagnostic identity = %+v, want schema v2, Go version, and invocation hash", diagnostics)
	}
	if diagnostics.ActionCache.PrimaryKey != "build-key" || diagnostics.ActionCache.MatchedKey != "build-key" || !diagnostics.ActionCache.ExactPrimaryHit {
		t.Fatalf("action-cache identity = %+v, want an exact primary hit", diagnostics.ActionCache)
	}
	invocation := diagnostics.CoverageInvocation
	if invocation.ExpectedPackages != 1 || invocation.CompilerCommands != 0 || invocation.LinkerCommands != 0 || invocation.BuildActions != 0 {
		t.Fatalf("coverage invocation summary = %+v, want one zero-action invocation", invocation)
	}
	if invocation.CacheReuse != coverageBuildCacheReuseExactInvocationHit || invocation.CommandResult != coverageBuildCommandResultPassed {
		t.Fatalf("coverage invocation classification = %+v, want exact hit and passed", invocation)
	}
	if invocation.WallSeconds < 0 || diagnostics.TotalMeasuredSeconds != invocation.WallSeconds {
		t.Fatalf("diagnostic timing = %+v, want one authoritative wall measurement", diagnostics)
	}
	for _, forbidden := range []string{"compileProbe", "inferredHits", "misses", "testRunWallSeconds"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("schema-v2 diagnostic retains removed field %q: %s", forbidden, raw)
		}
	}
	if !strings.Contains(stdout.String(), "cache-reuse=exact-invocation-hit") || stderr.Len() != 0 {
		t.Fatalf("diagnostic streams = stdout %q stderr %q, want concise stdout evidence only", stdout.String(), stderr.String())
	}
}

func TestCoverageBuildDiagnosticReportsObservedCompileWorkWithoutInferredHits(t *testing.T) {
	setActionCacheIdentity(t, "build-key", "older-key", "false")

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return "coverage test ran once\n", strings.Join([]string{
			"WORK=/tmp/go-build",
			"mkdir -p $WORK/b001/",
			`"/go/pkg/tool/linux_amd64/compile" -o "$WORK/b001/_pkg_.a" -p example.test`,
			`"/go/pkg/tool/linux_amd64/link" -o "$WORK/b001/example.test"`,
		}, "\n"), nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	if err := executeCoverageInvocationPlan(config{coverageBuildDiagnosticsOutput: diagnosticPath}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil); err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v", err)
	}
	diagnostics, raw := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.CoverageInvocation.CompilerCommands != 1 || diagnostics.CoverageInvocation.LinkerCommands != 1 || diagnostics.CoverageInvocation.BuildActions != 2 {
		t.Fatalf("coverage invocation summary = %+v, want observed compiler/linker actions", diagnostics.CoverageInvocation)
	}
	if diagnostics.CoverageInvocation.CacheReuse != coverageBuildCacheReuseCompileWorkObserved {
		t.Fatalf("cache reuse = %q, want compile-work-observed", diagnostics.CoverageInvocation.CacheReuse)
	}
	if strings.Contains(raw, "inferredHits") || strings.Contains(raw, "misses") {
		t.Fatalf("diagnostic contains inferred cache arithmetic: %s", raw)
	}
}

func TestCoverageBuildDiagnosticFailureDoesNotClaimExactReuse(t *testing.T) {
	setActionCacheIdentity(t, "build-key", "build-key", "true")

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return "WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", "original test failure", errors.New("exit status 1")
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	err := executeCoverageInvocationPlan(config{coverageBuildDiagnosticsOutput: diagnosticPath}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil)
	if err == nil || !strings.Contains(err.Error(), "original test failure") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want original test failure", err)
	}
	diagnostics, _ := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusFailed || diagnostics.CoverageInvocation.CommandResult != coverageBuildCommandResultFailed {
		t.Fatalf("failed diagnostic = %+v, want failed command and artifact", diagnostics)
	}
	if diagnostics.CoverageInvocation.CacheReuse == coverageBuildCacheReuseExactInvocationHit {
		t.Fatalf("failed diagnostic claimed exact reuse: %+v", diagnostics.CoverageInvocation)
	}
}

func TestCoverageBuildDiagnosticMalformedIdentityIsFailClosed(t *testing.T) {
	setActionCacheIdentity(t, "build-key", "different-key", "true")

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return "WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", "", nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	err := executeCoverageInvocationPlan(config{coverageBuildDiagnosticsOutput: diagnosticPath}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil)
	if err == nil || !strings.Contains(err.Error(), "exact cache hit identity is inconsistent") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want invalid identity", err)
	}
	diagnostics, _ := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.ActionCache.ExactPrimaryHit || diagnostics.CoverageInvocation.CacheReuse == coverageBuildCacheReuseExactInvocationHit {
		t.Fatalf("malformed identity produced exact reuse: %+v / %+v", diagnostics.ActionCache, diagnostics.CoverageInvocation)
	}
}

func TestCoverageBuildDiagnosticUnclassifiableTraceIsFailClosed(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return "coverage test ran once\n", "", nil
	}

	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	err := executeCoverageInvocationPlan(config{coverageBuildDiagnosticsOutput: diagnosticPath}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil)
	if err == nil || !strings.Contains(err.Error(), "unclassifiable go test -x trace") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want unclassifiable trace diagnostic", err)
	}
	diagnostics, _ := readCoverageBuildDiagnostics(t, diagnosticPath)
	if diagnostics.Status != coverageBuildDiagnosticsStatusFailed || diagnostics.CoverageInvocation.CacheReuse == coverageBuildCacheReuseExactInvocationHit {
		t.Fatalf("unclassifiable diagnostic = %+v, want failed non-exact result", diagnostics)
	}
}

func TestCoverageBuildDiagnosticWriterFailurePreservesCoverageFailure(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return "WORK=/tmp/go-build\nmkdir -p $WORK/b001/\n", "original coverage failure", errors.New("exit status 2")
	}

	outputDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDirectory, "child"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("prepare diagnostic output directory: %v", err)
	}
	err := executeCoverageInvocationPlan(config{coverageBuildDiagnosticsOutput: outputDirectory}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil)
	if err == nil || !strings.Contains(err.Error(), "original coverage failure") || !strings.Contains(err.Error(), "write coverage build diagnostics json") {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want coverage and writer failures", err)
	}
}

func TestCoverageBuildDiagnosticDisabledPreservesSingleUntracedInvocation(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		return "", "", nil
	}
	diagnosticPath := filepath.Join(t.TempDir(), "coverage-build-diagnostics.json")
	if err := executeCoverageInvocationPlan(config{}, diagnosticCoveragePlan(), []string{"example/tests/functional/work"}, "coverage.out", ".", nil, "run coverage", nil); err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v", err)
	}
	if len(invocations) != 1 || slicesContains(invocations[0].args, "-x") || slicesContains(invocations[0].args, "-json") || slicesContains(invocations[0].args, "-c") {
		t.Fatalf("disabled diagnostic invocations = %+v, want one unchanged untraced invocation", invocations)
	}
	if _, err := os.Stat(diagnosticPath); !os.IsNotExist(err) {
		t.Fatalf("disabled diagnostic file exists, stat error = %v", err)
	}
}

func TestCoverageActionCacheIdentityAcceptsPrefixAndMissResults(t *testing.T) {
	for _, tc := range []struct {
		name    string
		matched string
		hit     string
	}{
		{name: "prefix", matched: "older-key", hit: "false"},
		{name: "miss", matched: "", hit: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setActionCacheIdentity(t, "build-key", tc.matched, tc.hit)
			identity, err := coverageActionCacheIdentityFromEnvironment()
			if err != nil {
				t.Fatalf("coverageActionCacheIdentityFromEnvironment() error = %v", err)
			}
			if !identity.configured || identity.primaryKey != "build-key" || identity.matchedKey != tc.matched || identity.exactPrimaryHit {
				t.Fatalf("identity = %+v, want non-exact %s result", identity, tc.name)
			}
		})
	}
}

func TestCoverageInvocationHashIgnoresProfilePath(t *testing.T) {
	left := coverageInvocationPlan{invocations: []commandInvocation{{name: "go", args: []string{"test", "-coverprofile=/tmp/left.out", "./tests/functional"}}}}
	right := coverageInvocationPlan{invocations: []commandInvocation{{name: "go", args: []string{"test", "-coverprofile=/tmp/right.out", "./tests/functional"}}}}
	if coverageInvocationHash(left) != coverageInvocationHash(right) {
		t.Fatal("coverageInvocationHash() changed when only the profile path changed")
	}
}

func diagnosticCoveragePlan() coverageInvocationPlan {
	return coverageInvocationPlan{
		invocations: []commandInvocation{{
			name: "go",
			args: []string{"test", "-coverpkg=example/pkg/...", "-coverprofile=coverage.out", "example/tests/functional/work"},
		}},
		cleanup: func() error { return nil },
	}
}

func setActionCacheIdentity(t *testing.T, primary, matched, exact string) {
	t.Helper()
	t.Setenv(coverageBuildActionCachePrimaryKeyEnv, primary)
	t.Setenv(coverageBuildActionCacheMatchedKeyEnv, matched)
	t.Setenv(coverageBuildActionCacheExactHitEnv, exact)
}

func readCoverageBuildDiagnostics(t *testing.T, path string) (coverageBuildDiagnosticsJSON, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage build diagnostics: %v", err)
	}
	var diagnostics coverageBuildDiagnosticsJSON
	if err := json.Unmarshal(data, &diagnostics); err != nil {
		t.Fatalf("decode coverage build diagnostics: %v", err)
	}
	return diagnostics, string(data)
}

func countArgs(args []string, wanted string) int {
	count := 0
	for _, arg := range args {
		if arg == wanted {
			count++
		}
	}
	return count
}

func indexOf(args []string, wanted string) int {
	for index, arg := range args {
		if arg == wanted {
			return index
		}
	}
	return len(args)
}
