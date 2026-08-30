package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
)

const coverageBuildDiagnosticsSchemaVersion = 2

const (
	coverageBuildDiagnosticsStatusComplete = "complete"
	coverageBuildDiagnosticsStatusFailed   = "failed"
)

const (
	coverageBuildCacheReuseExactInvocationHit  = "exact-invocation-hit"
	coverageBuildCacheReuseCompileWorkObserved = "compile-work-observed"
	coverageBuildCacheReuseUnverified          = "cache-reuse-unverified"
	coverageBuildCacheReuseUnclassifiable      = "unclassifiable"
	coverageBuildCacheReuseCommandFailed       = "command-failed"
	coverageBuildCacheReuseIdentityInvalid     = "cache-identity-invalid"
)

const (
	coverageBuildCommandResultPassed = "passed"
	coverageBuildCommandResultFailed = "failed"
)

const (
	coverageBuildActionCachePrimaryKeyEnv = "FUNCTIONAL_COVERAGE_ACTION_CACHE_PRIMARY_KEY"
	coverageBuildActionCacheMatchedKeyEnv = "FUNCTIONAL_COVERAGE_ACTION_CACHE_MATCHED_KEY"
	coverageBuildActionCacheExactHitEnv   = "FUNCTIONAL_COVERAGE_ACTION_CACHE_EXACT_HIT"
)

var coverageBuildToolCommandPattern = map[string]*regexp.Regexp{
	"compile": regexp.MustCompile(`(?i)(?:^|[\s"'\\/])(?:[^\s"']+[\\/])?compile(?:\.exe)?(?:$|[\s"'\\/])`),
	"link":    regexp.MustCompile(`(?i)(?:^|[\s"'\\/])(?:[^\s"']+[\\/])?link(?:\.exe)?(?:$|[\s"'\\/])`),
}

// coverageBuildDiagnosticsJSON is a versioned, non-canonical observation of
// the authoritative coverage invocation. It intentionally contains counts
// and timings only; the raw -x trace is useful while a command runs but is
// too noisy and may expose paths that do not belong in a retained artifact.
type coverageBuildDiagnosticsJSON struct {
	SchemaVersion          int                               `json:"schemaVersion"`
	GoVersion              string                            `json:"goVersion"`
	CoverageInvocationHash string                            `json:"coverageInvocationHash"`
	ActionCache            coverageActionCacheJSON           `json:"actionCache"`
	CoverageInvocation     coverageInvocationDiagnosticsJSON `json:"coverageInvocation"`
	TotalMeasuredSeconds   float64                           `json:"totalMeasuredSeconds"`
	Status                 string                            `json:"status"`
}

type coverageActionCacheJSON struct {
	PrimaryKey      string `json:"primaryKey"`
	MatchedKey      string `json:"matchedKey"`
	ExactPrimaryHit bool   `json:"exactPrimaryHit"`
}

type coverageInvocationDiagnosticsJSON struct {
	WallSeconds      float64 `json:"wallSeconds"`
	ExpectedPackages int     `json:"expectedPackages"`
	CompilerCommands int     `json:"compilerCommands"`
	LinkerCommands   int     `json:"linkerCommands"`
	BuildActions     int     `json:"buildActions"`
	CacheReuse       string  `json:"cacheReuse"`
	CommandResult    string  `json:"commandResult"`
}

type coverageActionCacheIdentity struct {
	primaryKey      string
	matchedKey      string
	exactPrimaryHit bool
	configured      bool
}

type coverageBuildTraceSummary struct {
	compilerCommands int
	linkerCommands   int
	buildActions     int
	classifiable     bool
}

type coverageBuildTraceEvent struct {
	Action string `json:"Action"`
	Output string `json:"Output"`
}

type coverageBuildDiagnosticRun struct {
	diagnostics     coverageBuildDiagnosticsJSON
	identityErr     error
	initialWriteErr error
}

// prepareCoverageBuildDiagnostic enables tracing on the planned coverage
// command and creates the in-progress schema-v2 document. It does not run a
// second command: the caller executes the same plan used for coverage and
// supplies its output to finalizeCoverageBuildDiagnostics.
func prepareCoverageBuildDiagnostic(cfg config, plan *coverageInvocationPlan, testPackages []string) (*coverageBuildDiagnosticRun, error) {
	outputPath := strings.TrimSpace(cfg.coverageBuildDiagnosticsOutput)
	if outputPath == "" {
		return nil, nil
	}
	if err := addCoverageBuildDiagnosticFlags(plan); err != nil {
		return nil, err
	}

	run := newCoverageBuildDiagnosticRun(*plan, testPackages)
	run.diagnostics.Status = coverageBuildDiagnosticsStatusFailed
	run.initialWriteErr = writeCoverageBuildDiagnostics(outputPath, run.diagnostics)
	return run, nil
}

func newCoverageBuildDiagnosticRun(plan coverageInvocationPlan, testPackages []string) *coverageBuildDiagnosticRun {
	identity, identityErr := coverageActionCacheIdentityFromEnvironment()
	return &coverageBuildDiagnosticRun{
		diagnostics: coverageBuildDiagnosticsJSON{
			SchemaVersion:          coverageBuildDiagnosticsSchemaVersion,
			GoVersion:              runtime.Version(),
			CoverageInvocationHash: coverageInvocationHash(plan),
			ActionCache: coverageActionCacheJSON{
				PrimaryKey:      identity.primaryKey,
				MatchedKey:      identity.matchedKey,
				ExactPrimaryHit: identity.exactPrimaryHit,
			},
			CoverageInvocation: coverageInvocationDiagnosticsJSON{
				ExpectedPackages: expectedCoveragePackageCount(testPackages),
				CacheReuse:       coverageBuildCacheReuseUnclassifiable,
			},
			Status: coverageBuildDiagnosticsStatusFailed,
		},
		identityErr: identityErr,
	}
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

func addCoverageBuildDiagnosticFlags(plan *coverageInvocationPlan) error {
	for index := range plan.invocations {
		invocation := &plan.invocations[index]
		packageStart, err := coverageInvocationPackageStart(invocation.args)
		if err != nil {
			return fmt.Errorf("prepare coverage build diagnostics for invocation %d: %w", index+1, err)
		}

		args := append([]string(nil), invocation.args[:packageStart]...)
		if !coverageInvocationHasFlag(args, "-x") {
			args = append(args, "-x")
		}
		if !coverageInvocationHasFlag(args, "-json") {
			args = append(args, "-json")
		}
		args = append(args, invocation.args[packageStart:]...)
		invocation.args = args
	}
	return nil
}

func coverageInvocationHasFlag(args []string, wanted string) bool {
	for _, arg := range args {
		if arg == wanted {
			return true
		}
	}
	return false
}

func coverageInvocationPackageStart(args []string) (int, error) {
	if len(args) == 0 || args[0] != "test" {
		return 0, errors.New("expected a go test invocation")
	}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-coverprofile":
			if index+1 >= len(args) {
				return 0, errors.New("coverage invocation -coverprofile has no value")
			}
			return coverageInvocationPackageStartAfterFlags(args, index+2)
		case strings.HasPrefix(arg, "-coverprofile="):
			return coverageInvocationPackageStartAfterFlags(args, index+1)
		}
	}
	return 0, errors.New("coverage invocation has no -coverprofile argument")
}

func coverageInvocationPackageStartAfterFlags(args []string, packageStart int) (int, error) {
	for packageStart < len(args) && (args[packageStart] == "-json" || args[packageStart] == "-x") {
		packageStart++
	}
	if packageStart >= len(args) {
		return 0, errors.New("coverage invocation has no test packages")
	}
	return packageStart, nil
}

func coverageActionCacheIdentityFromEnvironment() (coverageActionCacheIdentity, error) {
	primaryKey, primarySet := os.LookupEnv(coverageBuildActionCachePrimaryKeyEnv)
	matchedKey, matchedSet := os.LookupEnv(coverageBuildActionCacheMatchedKeyEnv)
	exactHit, exactSet := os.LookupEnv(coverageBuildActionCacheExactHitEnv)
	identity := coverageActionCacheIdentity{
		primaryKey: strings.TrimSpace(primaryKey),
		matchedKey: strings.TrimSpace(matchedKey),
		configured: primarySet || matchedSet || exactSet,
	}
	if !identity.configured {
		return identity, nil
	}
	if !primarySet || identity.primaryKey == "" {
		return identity, fmt.Errorf("coverage build diagnostics: %s is missing", coverageBuildActionCachePrimaryKeyEnv)
	}
	if primaryKey != identity.primaryKey || strings.ContainsAny(primaryKey, "\r\n") {
		return identity, fmt.Errorf("coverage build diagnostics: %s is malformed", coverageBuildActionCachePrimaryKeyEnv)
	}
	if matchedKey != identity.matchedKey || strings.ContainsAny(matchedKey, "\r\n") {
		return identity, fmt.Errorf("coverage build diagnostics: %s is malformed", coverageBuildActionCacheMatchedKeyEnv)
	}
	if !exactSet {
		return identity, fmt.Errorf("coverage build diagnostics: %s is missing", coverageBuildActionCacheExactHitEnv)
	}
	var exactPrimaryHit bool
	switch strings.ToLower(strings.TrimSpace(exactHit)) {
	case "true":
		exactPrimaryHit = true
	case "false":
		exactPrimaryHit = false
	default:
		return identity, fmt.Errorf("coverage build diagnostics: %s must be true or false", coverageBuildActionCacheExactHitEnv)
	}
	if exactPrimaryHit && (identity.matchedKey == "" || identity.matchedKey != identity.primaryKey) {
		return identity, errors.New("coverage build diagnostics: exact cache hit identity is inconsistent")
	}
	if !exactPrimaryHit && identity.matchedKey == identity.primaryKey {
		return identity, errors.New("coverage build diagnostics: non-exact cache result matches the primary key")
	}
	identity.exactPrimaryHit = exactPrimaryHit
	return identity, nil
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

func parseCoverageBuildTrace(trace string) (coverageBuildTraceSummary, error) {
	var summary coverageBuildTraceSummary
	workSeen := false
	traceActivity := false
	scanner := bufio.NewScanner(strings.NewReader(trace))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		for _, outputLine := range coverageBuildTraceOutputLines(scanner.Text()) {
			line := strings.TrimSpace(outputLine)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "WORK=") {
				workSeen = true
				continue
			}
			if isCoverageBuildToolCommand(line, "compile") {
				summary.compilerCommands++
				traceActivity = true
				continue
			}
			if isCoverageBuildToolCommand(line, "link") {
				summary.linkerCommands++
				traceActivity = true
				continue
			}
			if isCoverageBuildTraceActivity(line) {
				traceActivity = true
			}
		}
	}
	summary.buildActions = summary.compilerCommands + summary.linkerCommands
	if err := scanner.Err(); err != nil {
		return summary, fmt.Errorf("classify coverage build trace: %w", err)
	}
	if !workSeen || !traceActivity {
		return summary, errors.New("classify coverage build trace: unclassifiable go test -x trace")
	}
	summary.classifiable = true
	return summary, nil
}

func coverageBuildTraceOutputLines(line string) []string {
	var event coverageBuildTraceEvent
	if err := json.Unmarshal([]byte(line), &event); err == nil && event.Action == "build-output" {
		return strings.Split(event.Output, "\n")
	}
	return []string{line}
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

func classifyCoverageBuildTrace(summary coverageBuildTraceSummary, identity coverageActionCacheIdentity, identityErr error, traceErr error, commandErr error) string {
	if summary.buildActions > 0 {
		return coverageBuildCacheReuseCompileWorkObserved
	}
	if traceErr != nil {
		return coverageBuildCacheReuseUnclassifiable
	}
	if commandErr != nil {
		return coverageBuildCacheReuseCommandFailed
	}
	if identityErr != nil {
		return coverageBuildCacheReuseIdentityInvalid
	}
	if identity.configured && identity.exactPrimaryHit && identity.primaryKey != "" && identity.primaryKey == identity.matchedKey {
		return coverageBuildCacheReuseExactInvocationHit
	}
	return coverageBuildCacheReuseUnverified
}

func finalizeCoverageBuildDiagnostics(run *coverageBuildDiagnosticRun, outputPath string, wallSeconds float64, trace string, commandErr error) error {
	wallSeconds = roundTimingSeconds(wallSeconds)
	run.diagnostics.CoverageInvocation.WallSeconds = wallSeconds
	run.diagnostics.TotalMeasuredSeconds = wallSeconds
	if commandErr == nil {
		run.diagnostics.CoverageInvocation.CommandResult = coverageBuildCommandResultPassed
	} else {
		run.diagnostics.CoverageInvocation.CommandResult = coverageBuildCommandResultFailed
	}

	parsed, traceErr := parseCoverageBuildTrace(trace)
	run.diagnostics.CoverageInvocation.CompilerCommands = parsed.compilerCommands
	run.diagnostics.CoverageInvocation.LinkerCommands = parsed.linkerCommands
	run.diagnostics.CoverageInvocation.BuildActions = parsed.buildActions
	identity, identityErr := coverageActionCacheIdentityFromEnvironment()
	if run.identityErr == nil {
		run.identityErr = identityErr
	}
	run.diagnostics.CoverageInvocation.CacheReuse = classifyCoverageBuildTrace(
		parsed,
		identity,
		run.identityErr,
		traceErr,
		commandErr,
	)
	run.diagnostics.Status = coverageBuildDiagnosticsStatusComplete
	if commandErr != nil || traceErr != nil || run.identityErr != nil || run.initialWriteErr != nil {
		run.diagnostics.Status = coverageBuildDiagnosticsStatusFailed
	}

	writeErr := writeCoverageBuildDiagnostics(outputPath, run.diagnostics)
	_, summaryErr := fmt.Fprintf(
		stdoutWriter,
		"Coverage build diagnostic: wall=%.3fs compiler=%d linker=%d build-actions=%d cache-reuse=%s command-result=%s status=%s\n",
		run.diagnostics.CoverageInvocation.WallSeconds,
		run.diagnostics.CoverageInvocation.CompilerCommands,
		run.diagnostics.CoverageInvocation.LinkerCommands,
		run.diagnostics.CoverageInvocation.BuildActions,
		run.diagnostics.CoverageInvocation.CacheReuse,
		run.diagnostics.CoverageInvocation.CommandResult,
		run.diagnostics.Status,
	)
	return errors.Join(run.initialWriteErr, run.identityErr, traceErr, writeErr, summaryErr)
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
