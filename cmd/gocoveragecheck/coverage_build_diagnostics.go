package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const coverageBuildDiagnosticsSchemaVersion = 1

const (
	coverageBuildDiagnosticsStatusComplete = "complete"
	coverageBuildDiagnosticsStatusFailed   = "failed"
)

var coverageBuildToolCommandPattern = map[string]*regexp.Regexp{
	"compile": regexp.MustCompile(`(?i)(?:^|[\s"'\\/])(?:[^\s"']+[\\/])?compile(?:\.exe)?(?:$|[\s"'\\/])`),
	"link":    regexp.MustCompile(`(?i)(?:^|[\s"'\\/])(?:[^\s"']+[\\/])?link(?:\.exe)?(?:$|[\s"'\\/])`),
}

// coverageBuildDiagnosticsJSON is a versioned, non-canonical observation of
// the coverage build. It intentionally contains counts and timings only; the
// raw -x trace is useful while a command runs but is too noisy and may expose
// paths that do not belong in a retained artifact.
type coverageBuildDiagnosticsJSON struct {
	SchemaVersion          int                      `json:"schemaVersion"`
	GoVersion              string                   `json:"goVersion"`
	CoverageInvocationHash string                   `json:"coverageInvocationHash"`
	CompileProbe           coverageCompileProbeJSON `json:"compileProbe"`
	TestRunWallSeconds     float64                  `json:"testRunWallSeconds"`
	TotalMeasuredSeconds   float64                  `json:"totalMeasuredSeconds"`
	Status                 string                   `json:"status"`
}

type coverageCompileProbeJSON struct {
	WallSeconds      float64 `json:"wallSeconds"`
	ExpectedPackages int     `json:"expectedPackages"`
	CompilerCommands int     `json:"compilerCommands"`
	LinkerCommands   int     `json:"linkerCommands"`
	InferredHits     int     `json:"inferredHits"`
	Misses           int     `json:"misses"`
}

type coverageBuildDiagnosticRun struct {
	diagnostics coverageBuildDiagnosticsJSON
}

// runCoverageBuildProbe runs the compile-only form of every existing
// coverage invocation. The existing plan remains the source of truth for
// package selection and coverage flags, so the probe cannot accidentally
// select a different test corpus or alter the real test run.
func runCoverageBuildProbe(cfg config, plan coverageInvocationPlan, testPackages []string) (*coverageBuildDiagnosticRun, error) {
	outputPath := strings.TrimSpace(cfg.coverageBuildDiagnosticsOutput)
	if outputPath == "" {
		return nil, nil
	}

	run := newCoverageBuildDiagnosticRun(plan, testPackages)
	run.diagnostics.Status = coverageBuildDiagnosticsStatusFailed
	if err := writeCoverageBuildDiagnostics(outputPath, run.diagnostics); err != nil {
		return nil, fmt.Errorf("prepare coverage build diagnostics output: %w", err)
	}
	run.diagnostics.Status = coverageBuildDiagnosticsStatusComplete
	started := time.Now()
	probeDir := filepath.Join(filepath.Dir(outputPath), "compile-probe-bin")
	if err := os.RemoveAll(probeDir); err != nil {
		return nil, runFailedCoverageBuildDiagnostic(
			outputPath,
			run,
			started,
			"",
			fmt.Errorf("clear coverage compile probe directory: %w", err),
		)
	}
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		return nil, runFailedCoverageBuildDiagnostic(
			outputPath,
			run,
			started,
			"",
			fmt.Errorf("create coverage compile probe directory: %w", err),
		)
	}
	defer func() { _ = os.RemoveAll(probeDir) }()

	var trace strings.Builder
	probeIndex := 0
	for _, invocation := range plan.invocations {
		probes, buildErr := buildCoverageCompileProbeInvocations(invocation, probeDir)
		if buildErr != nil {
			return nil, runFailedCoverageBuildDiagnostic(outputPath, run, started, trace.String(), buildErr)
		}
		for _, probe := range probes {
			stdout, stderr, commandErr := runCommand(probe)
			appendCoverageBuildTrace(&trace, stdout, stderr)
			if commandErr != nil {
				failure := coverageBuildProbeFailure(probeIndex, len(probes), commandErr, stdout, stderr)
				return nil, runFailedCoverageBuildDiagnostic(outputPath, run, started, trace.String(), failure)
			}
			probeIndex++
		}
	}

	run.diagnostics.CompileProbe.WallSeconds = roundTimingSeconds(time.Since(started).Seconds())
	probe, err := parseCoverageBuildTrace(trace.String(), run.diagnostics.CompileProbe.ExpectedPackages)
	if err != nil {
		return nil, runFailedCoverageBuildDiagnostic(outputPath, run, started, trace.String(), err)
	}
	run.diagnostics.CompileProbe.CompilerCommands = probe.CompilerCommands
	run.diagnostics.CompileProbe.LinkerCommands = probe.LinkerCommands
	run.diagnostics.CompileProbe.InferredHits = probe.InferredHits
	run.diagnostics.CompileProbe.Misses = probe.Misses
	return run, nil
}

func newCoverageBuildDiagnosticRun(plan coverageInvocationPlan, testPackages []string) *coverageBuildDiagnosticRun {
	return &coverageBuildDiagnosticRun{diagnostics: coverageBuildDiagnosticsJSON{
		SchemaVersion:          coverageBuildDiagnosticsSchemaVersion,
		GoVersion:              runtime.Version(),
		CoverageInvocationHash: coverageInvocationHash(plan),
		CompileProbe: coverageCompileProbeJSON{
			ExpectedPackages: expectedCoveragePackageCount(testPackages),
		},
		Status: coverageBuildDiagnosticsStatusComplete,
	}}
}

func expectedCoveragePackageCount(packages []string) int {
	seen := make(map[string]struct{}, len(packages))
	for _, packagePath := range packages {
		packagePath = strings.TrimSpace(packagePath)
		if packagePath != "" {
			seen[packagePath] = struct{}{}
		}
	}
	return len(seen)
}

func buildCoverageCompileProbeInvocation(invocation commandInvocation, outputDir string) (commandInvocation, error) {
	return buildCoverageCompileProbeInvocationForPackages(invocation, outputDir, nil)
}

func buildCoverageCompileProbeInvocations(invocation commandInvocation, outputDir string) ([]commandInvocation, error) {
	packages, err := coverageInvocationPackageArgs(invocation.args)
	if err != nil {
		return nil, err
	}

	type probePackageGroup struct {
		names    map[string]struct{}
		packages []string
	}
	groups := make([]probePackageGroup, 0, len(packages))
	for _, packageArg := range packages {
		packageName := path.Base(strings.TrimSuffix(strings.TrimSpace(packageArg), "/"))
		if packageName == "." || packageName == "..." || packageName == "" {
			packageName = packageArg
		}
		groupIndex := -1
		for index := range groups {
			if _, exists := groups[index].names[packageName]; !exists {
				groupIndex = index
				break
			}
		}
		if groupIndex == -1 {
			groupIndex = len(groups)
			groups = append(groups, probePackageGroup{names: make(map[string]struct{})})
		}
		groups[groupIndex].names[packageName] = struct{}{}
		groups[groupIndex].packages = append(groups[groupIndex].packages, packageArg)
	}

	probes := make([]commandInvocation, 0, len(groups))
	for _, group := range groups {
		probe, err := buildCoverageCompileProbeInvocationForPackages(invocation, outputDir, group.packages)
		if err != nil {
			return nil, err
		}
		probes = append(probes, probe)
	}
	return probes, nil
}

func buildCoverageCompileProbeInvocationForPackages(invocation commandInvocation, outputDir string, selectedPackages []string) (commandInvocation, error) {
	if len(invocation.args) == 0 || invocation.args[0] != "test" {
		return commandInvocation{}, errors.New("prepare coverage compile probe: expected a go test invocation")
	}
	packageStart, packages, err := coverageInvocationPackageArgsWithStart(invocation.args)
	if err != nil {
		return commandInvocation{}, err
	}
	if len(selectedPackages) == 0 {
		selectedPackages = packages
	}
	args := []string{"test", "-c", "-o", outputDir, "-x"}
	for index := 1; index < packageStart; index++ {
		arg := invocation.args[index]
		switch {
		case arg == "-json":
			continue
		case arg == "-coverprofile":
			if index+1 >= packageStart {
				return commandInvocation{}, errors.New("prepare coverage compile probe: -coverprofile has no value")
			}
			index++
			continue
		case strings.HasPrefix(arg, "-coverprofile="):
			continue
		default:
			args = append(args, arg)
		}
	}
	args = append(args, selectedPackages...)
	return commandInvocation{
		name: invocation.name,
		args: args,
		env:  invocation.env,
		dir:  invocation.dir,
	}, nil
}

func coverageInvocationPackageArgs(args []string) ([]string, error) {
	_, packages, err := coverageInvocationPackageArgsWithStart(args)
	return packages, err
}

func coverageInvocationPackageArgsWithStart(args []string) (int, []string, error) {
	if len(args) == 0 || args[0] != "test" {
		return 0, nil, errors.New("prepare coverage compile probe: expected a go test invocation")
	}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-coverprofile":
			if index+2 > len(args) {
				return 0, nil, errors.New("prepare coverage compile probe: -coverprofile has no value")
			}
			packageStart := index + 2
			if packageStart < len(args) && args[packageStart] == "-json" {
				packageStart++
			}
			return coverageInvocationPackageArgsFromStart(args, packageStart)
		case strings.HasPrefix(arg, "-coverprofile="):
			packageStart := index + 1
			if packageStart < len(args) && args[packageStart] == "-json" {
				packageStart++
			}
			return coverageInvocationPackageArgsFromStart(args, packageStart)
		}
	}
	return 0, nil, errors.New("prepare coverage compile probe: coverage invocation has no -coverprofile argument")
}

func coverageInvocationPackageArgsFromStart(args []string, packageStart int) (int, []string, error) {
	if packageStart >= len(args) {
		return 0, nil, errors.New("prepare coverage compile probe: coverage invocation has no test packages")
	}
	packages := append([]string(nil), args[packageStart:]...)
	return packageStart, packages, nil
}

func appendCoverageBuildTrace(trace *strings.Builder, stdout, stderr string) {
	if stdout != "" {
		trace.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			trace.WriteByte('\n')
		}
	}
	if stderr != "" {
		trace.WriteString(stderr)
		if !strings.HasSuffix(stderr, "\n") {
			trace.WriteByte('\n')
		}
	}
}

func parseCoverageBuildTrace(trace string, expectedPackages int) (coverageCompileProbeJSON, error) {
	if expectedPackages < 1 {
		return coverageCompileProbeJSON{}, errors.New("classify coverage compile probe: no expected test packages")
	}

	var compilerCommands, linkerCommands int
	workSeen := false
	traceActivity := false
	scanner := bufio.NewScanner(strings.NewReader(trace))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "WORK=") {
			workSeen = true
			continue
		}
		if isCoverageBuildToolCommand(line, "compile") {
			compilerCommands++
			traceActivity = true
			continue
		}
		if isCoverageBuildToolCommand(line, "link") {
			linkerCommands++
			traceActivity = true
			continue
		}
		if isCoverageBuildTraceActivity(line) {
			traceActivity = true
		}
	}
	if err := scanner.Err(); err != nil {
		return coverageCompileProbeJSON{}, fmt.Errorf("classify coverage compile probe trace: %w", err)
	}
	if !workSeen || !traceActivity {
		return coverageCompileProbeJSON{}, errors.New("classify coverage compile probe: unclassifiable go test -x trace")
	}

	misses := compilerCommands + linkerCommands
	inferredHits := expectedPackages - misses
	if inferredHits < 0 {
		inferredHits = 0
	}
	return coverageCompileProbeJSON{
		ExpectedPackages: expectedPackages,
		CompilerCommands: compilerCommands,
		LinkerCommands:   linkerCommands,
		InferredHits:     inferredHits,
		Misses:           misses,
	}, nil
}

func isCoverageBuildToolCommand(line, tool string) bool {
	pattern := coverageBuildToolCommandPattern[tool]
	if pattern == nil || !pattern.MatchString(line) {
		return false
	}
	return strings.Contains(line, " -o ") || strings.Contains(line, " -o=")
}

func isCoverageBuildTraceActivity(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "mkdir ") ||
		strings.HasPrefix(lower, "cd ") ||
		strings.Contains(lower, "packagefile ") ||
		strings.Contains(lower, "importcfg") ||
		strings.Contains(lower, "go tool ")
}

func coverageInvocationHash(plan coverageInvocationPlan) string {
	canonical := make([][]string, 0, len(plan.invocations))
	for _, invocation := range plan.invocations {
		canonical = append(canonical, append([]string{invocation.name}, canonicalCoverageInvocationArgs(invocation.args)...))
	}
	data, _ := json.Marshal(canonical)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func canonicalCoverageInvocationArgs(args []string) []string {
	canonical := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-coverprofile":
			canonical = append(canonical, "-coverprofile=<coverage-profile>")
			if index+1 < len(args) {
				index++
			}
		case strings.HasPrefix(arg, "-coverprofile="):
			canonical = append(canonical, "-coverprofile=<coverage-profile>")
		default:
			canonical = append(canonical, arg)
		}
	}
	return canonical
}

func runFailedCoverageBuildDiagnostic(outputPath string, run *coverageBuildDiagnosticRun, started time.Time, trace string, cause error) error {
	run.diagnostics.Status = coverageBuildDiagnosticsStatusFailed
	run.diagnostics.CompileProbe.WallSeconds = roundTimingSeconds(time.Since(started).Seconds())
	// The test invocation never starts after a failed probe, so the observed
	// compile-only interval is also the complete measured interval. Keep it in
	// the failed artifact without presenting the observation as complete.
	run.diagnostics.TotalMeasuredSeconds = run.diagnostics.CompileProbe.WallSeconds
	if parsed, err := parseCoverageBuildTrace(trace, run.diagnostics.CompileProbe.ExpectedPackages); err == nil {
		run.diagnostics.CompileProbe.CompilerCommands = parsed.CompilerCommands
		run.diagnostics.CompileProbe.LinkerCommands = parsed.LinkerCommands
		run.diagnostics.CompileProbe.InferredHits = parsed.InferredHits
		run.diagnostics.CompileProbe.Misses = parsed.Misses
	}
	writeErr := writeCoverageBuildDiagnostics(outputPath, run.diagnostics)
	return errors.Join(cause, writeErr)
}

func coverageBuildProbeFailure(index, total int, commandErr error, stdout, stderr string) error {
	detail := mergeGoTestFailureDetail(stderr, stdout)
	if detail == "" {
		return fmt.Errorf("run coverage compile probe (%d/%d): %w", index+1, total, commandErr)
	}
	return fmt.Errorf("run coverage compile probe (%d/%d): %w\n%s", index+1, total, commandErr, trimCoverageBuildFailureDetail(detail))
}

func trimCoverageBuildFailureDetail(detail string) string {
	const maxDetail = 8 * 1024
	if len(detail) <= maxDetail {
		return detail
	}
	const marker = "\n... coverage compile trace omitted ...\n"
	half := (maxDetail - len(marker)) / 2
	return detail[:half] + marker + detail[len(detail)-half:]
}

func finalizeCoverageBuildDiagnostics(run *coverageBuildDiagnosticRun, outputPath string, testRunWallSeconds float64) error {
	run.diagnostics.TestRunWallSeconds = roundTimingSeconds(testRunWallSeconds)
	run.diagnostics.TotalMeasuredSeconds = roundTimingSeconds(
		run.diagnostics.CompileProbe.WallSeconds + run.diagnostics.TestRunWallSeconds,
	)
	run.diagnostics.Status = coverageBuildDiagnosticsStatusComplete
	if err := writeCoverageBuildDiagnostics(outputPath, run.diagnostics); err != nil {
		return err
	}
	fmt.Fprintf(
		stdoutWriter,
		"Coverage build diagnostic: compile=%.3fs test=%.3fs total=%.3fs compiler=%d linker=%d inferred-hits=%d misses=%d status=%s\n",
		run.diagnostics.CompileProbe.WallSeconds,
		run.diagnostics.TestRunWallSeconds,
		run.diagnostics.TotalMeasuredSeconds,
		run.diagnostics.CompileProbe.CompilerCommands,
		run.diagnostics.CompileProbe.LinkerCommands,
		run.diagnostics.CompileProbe.InferredHits,
		run.diagnostics.CompileProbe.Misses,
		run.diagnostics.Status,
	)
	return nil
}

func writeCoverageBuildDiagnostics(path string, diagnostics coverageBuildDiagnosticsJSON) error {
	data, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		return fmt.Errorf("render coverage build diagnostics json: %w", err)
	}
	if err := writeAtomicDiagnosticFile(path, append(data, '\n')); err != nil {
		return fmt.Errorf("write coverage build diagnostics json: %w", err)
	}
	return nil
}
