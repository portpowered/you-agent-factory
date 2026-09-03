package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestCompactGoPackagePatternsPreserveSelectedPackageSet(t *testing.T) {
	root := "example.com/project/pkg"
	allPackages := []string{
		root + "/alpha",
		root + "/alpha/one",
		root + "/alpha/two",
		root + "/beta",
		root + "/beta/excluded",
		root + "/gamma/one",
		root + "/gamma/two",
	}
	selectedPackages := []string{
		root + "/alpha",
		root + "/alpha/one",
		root + "/alpha/two",
		root + "/beta",
		root + "/gamma/one",
		root + "/gamma/two",
	}

	patterns, err := compactGoPackagePatterns(allPackages, selectedPackages, root)
	if err != nil {
		t.Fatalf("compactGoPackagePatterns() error = %v", err)
	}
	want := []string{root + "/alpha/...", root + "/beta", root + "/gamma/..."}
	if !reflect.DeepEqual(patterns, want) {
		t.Fatalf("compactGoPackagePatterns() = %v, want %v", patterns, want)
	}

	matched := make(map[string]struct{})
	for _, pattern := range patterns {
		subtree := strings.HasSuffix(pattern, "/...")
		prefix := strings.TrimSuffix(pattern, "/...")
		for _, packagePath := range allPackages {
			if packagePath == prefix || (subtree && strings.HasPrefix(packagePath, prefix+"/")) {
				matched[packagePath] = struct{}{}
			}
		}
	}
	if len(matched) != len(selectedPackages) {
		t.Fatalf("compact patterns matched %d packages, want %d", len(matched), len(selectedPackages))
	}
	for _, packagePath := range selectedPackages {
		if _, ok := matched[packagePath]; !ok {
			t.Fatalf("compact patterns omitted selected package %q", packagePath)
		}
	}
	if _, ok := matched[root+"/beta/excluded"]; ok {
		t.Fatal("compact patterns included excluded package")
	}
}

func TestCompactUnitTestPackageArgsUsesExactPackageUniverse(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	root := modulePath + "/pkg"
	allPackages := []string{
		root + "/alpha",
		root + "/alpha/one",
		root + "/alpha/two",
		root + "/beta",
		root + "/beta/excluded",
	}
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "list" {
			return "", "", fmt.Errorf("unexpected package-list invocation: %v", invocation.args)
		}
		lines := make([]string, 0, len(allPackages))
		for _, packagePath := range allPackages {
			lines = append(lines, packagePath+"\t1")
		}
		return strings.Join(lines, "\n"), "", nil
	}

	selectedPackages := []string{root + "/alpha", root + "/alpha/one", root + "/alpha/two", root + "/beta"}
	patterns := compactUnitTestPackageArgs(config{}, selectedPackages, "windows")
	want := []string{root + "/alpha/...", root + "/beta"}
	if !reflect.DeepEqual(patterns, want) {
		t.Fatalf("compactUnitTestPackageArgs() = %v, want %v", patterns, want)
	}
}

func TestBuildCoverageInvocationPlanBatchesOversizedWindowsCommand(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-short",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := make([]string, 0, 500)
	for index := 0; index < 500; index++ {
		testPackages = append(testPackages, modulePath+"/pkg/coverage_test_"+strings.Repeat("x", 32)+strconv.Itoa(index))
	}
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	legacyArgs := buildCoverageTestArgs(commonArgs, profilePath, false, testPackages)
	if got := windowsCommandLine(legacyArgs); got <= windowsCoverageCommandLineLimit {
		t.Fatalf("old single invocation length = %d, want it above safe limit %d", got, windowsCoverageCommandLineLimit)
	}

	plan, err := buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, false, "windows")
	if err != nil {
		t.Fatalf("buildCoverageInvocationPlan() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plan.cleanup(); cleanupErr != nil {
			t.Fatalf("cleanup batch profiles: %v", cleanupErr)
		}
	})

	if len(plan.invocations) < 2 {
		t.Fatalf("planned invocations = %d, want batching for oversized command", len(plan.invocations))
	}
	if len(plan.profilePaths) != len(plan.invocations) {
		t.Fatalf("profile paths = %d, invocations = %d, want one profile per invocation", len(plan.profilePaths), len(plan.invocations))
	}

	seen := make(map[string]int, len(testPackages))
	for index, invocation := range plan.invocations {
		if got := windowsCommandLine(invocation.args); got > windowsCoverageCommandLineLimit {
			t.Fatalf("invocation %d length = %d, want <= safe limit %d", index, got, windowsCoverageCommandLineLimit)
		}
		batchProfile := helperCoverProfilePath(invocation.args)
		if batchProfile == "" || batchProfile == profilePath {
			t.Fatalf("invocation %d profile = %q, want isolated batch profile", index, batchProfile)
		}
		if got := plan.profilePaths[index]; got != batchProfile {
			t.Fatalf("profilePaths[%d] = %q, invocation profile = %q", index, got, batchProfile)
		}
		for _, testPackage := range testPackages {
			if slicesContains(invocation.args, testPackage) {
				seen[testPackage]++
			}
		}
	}

	if len(seen) != len(testPackages) {
		t.Fatalf("planned package count = %d, want %d", len(seen), len(testPackages))
	}
	for _, testPackage := range testPackages {
		if seen[testPackage] != 1 {
			t.Fatalf("test package %q appears %d times, want exactly once", testPackage, seen[testPackage])
		}
	}

	batchDir := filepath.Dir(plan.profilePaths[0])
	if _, err := os.Stat(batchDir); err != nil {
		t.Fatalf("batch directory stat error = %v, want it to exist before cleanup", err)
	}
	if err := plan.cleanup(); err != nil {
		t.Fatalf("plan cleanup error = %v", err)
	}
	if _, err := os.Stat(batchDir); !os.IsNotExist(err) {
		t.Fatalf("batch directory still exists after cleanup, stat error = %v", err)
	}
	plan.cleanup = func() error { return nil }
}

func TestOversizedWindowsInvocationFailsBeforeBoundedPlanPasses(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-short",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := makeOversizedTestPackages(500)
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	legacyArgs := buildCoverageTestArgs(commonArgs, profilePath, false, testPackages)
	if got := windowsCommandLine(legacyArgs); got <= windowsCoverageCommandLineLimit {
		t.Fatalf("legacy invocation length = %d, want it above safe limit %d", got, windowsCoverageCommandLineLimit)
	}

	rejectOversizedInvocation := func(invocation commandInvocation) error {
		if windowsCommandLine(invocation.args) > windowsCoverageCommandLineLimit {
			return errors.New("fork/exec go.exe: The filename or extension is too long")
		}
		return nil
	}
	if err := rejectOversizedInvocation(commandInvocation{name: "go", args: legacyArgs}); err == nil || !strings.Contains(err.Error(), "The filename or extension is too long") {
		t.Fatalf("legacy invocation error = %v, want Windows process-length failure", err)
	}

	plan, err := buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, false, "windows")
	if err != nil {
		t.Fatalf("buildCoverageInvocationPlan() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plan.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup bounded invocation plan: %v", cleanupErr)
		}
	})

	for index, invocation := range plan.invocations {
		if err := rejectOversizedInvocation(invocation); err != nil {
			t.Fatalf("bounded invocation %d failed process-length check: %v", index, err)
		}
	}
}

func TestBuildCoverageInvocationPlanUsesBoundedCompactPackageArguments(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-short",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := makeOversizedTestPackages(500)
	compactPackageArgs := []string{modulePath + "/pkg/initializer/...", modulePath + "/pkg/root/..."}
	profilePath := filepath.Join(t.TempDir(), "coverage.out")

	plan, err := buildCoverageInvocationPlan(commonArgs, testPackages, profilePath, false, "windows", compactPackageArgs)
	if err != nil {
		t.Fatalf("buildCoverageInvocationPlan() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := plan.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup compact invocation plan: %v", cleanupErr)
		}
	})
	if len(plan.invocations) != 1 {
		t.Fatalf("planned invocations = %d, want one compact invocation", len(plan.invocations))
	}
	invocation := plan.invocations[0]
	if got := windowsCommandLine(invocation.args); got > windowsCoverageCommandLineLimit {
		t.Fatalf("compact invocation length = %d, want <= safe limit %d", got, windowsCoverageCommandLineLimit)
	}
	for _, packageArg := range compactPackageArgs {
		if !slicesContains(invocation.args, packageArg) {
			t.Fatalf("compact invocation args = %v, missing package pattern %q", invocation.args, packageArg)
		}
	}
	if slicesContains(invocation.args, testPackages[0]) {
		t.Fatalf("compact invocation unexpectedly contains concrete package %q", testPackages[0])
	}
}

func TestBuildCoverageInvocationPlanRejectsSingleOversizedWindowsPackage(t *testing.T) {
	commonArgs := []string{
		"test",
		"-coverpkg=" + strings.Repeat("example.com/backend/", 1200),
		"-p=2",
		"-count=1",
	}
	testPackage := modulePath + "/pkg/one-oversized-package"
	profilePath := filepath.Join(t.TempDir(), "coverage.out")

	_, err := buildCoverageInvocationPlan(commonArgs, []string{testPackage}, profilePath, false, "windows")
	if err == nil {
		t.Fatal("buildCoverageInvocationPlan() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "above the safe Windows limit") || !strings.Contains(err.Error(), testPackage) {
		t.Fatalf("buildCoverageInvocationPlan() error = %q, want package and bound diagnostic", err)
	}
}

func TestDefaultLinuxUnitCoverageUsesBinaryCovdata(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	plan, err := planGoTestCoverageLane(
		[]string{"test", "-coverpkg=" + modulePath + "/pkg/...", "-p=4", "-covermode=set"},
		[]string{modulePath + "/pkg/platform/clock"},
		profilePath,
		config{suite: unitCoverageSuite, timingOutput: "timing.json"},
		"linux",
	)
	if err != nil {
		t.Fatalf("planGoTestCoverageLane() error = %v", err)
	}
	covdataDir := plan.covdataDir
	t.Cleanup(func() {
		if cleanupErr := plan.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup unit covdata plan: %v", cleanupErr)
		}
	})
	if covdataDir == "" {
		t.Fatal("unit coverage plan has no binary covdata directory")
	}
	args := plan.invocations[0].args
	if !slicesContains(args, "-cover") {
		t.Fatalf("unit coverage args = %v, want -cover", args)
	}
	if !slicesContains(args, "-test.gocoverdir="+covdataDir) {
		t.Fatalf("unit coverage args = %v, want covdata destination", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-coverprofile=") {
			t.Fatalf("unit coverage args retained text profile: %v", args)
		}
	}
}

func TestUnitCovdataIsLimitedToDefaultNonWindowsUnitCoverage(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config
		targetOS string
		want     bool
	}{
		{name: "default Linux unit", cfg: config{suite: unitCoverageSuite}, targetOS: "linux", want: true},
		{name: "implicit Linux unit", cfg: config{}, targetOS: "linux", want: true},
		{name: "Windows unit", cfg: config{suite: unitCoverageSuite}, targetOS: "windows"},
		{name: "functional", cfg: config{suite: functionalCoverageSuite}, targetOS: "linux"},
		{name: "custom packages", cfg: config{suite: unitCoverageSuite, packages: "example/pkg"}, targetOS: "linux"},
		{name: "custom cover packages", cfg: config{suite: unitCoverageSuite, coverpkg: "example/pkg"}, targetOS: "linux"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := useUnitCovdataProfile(test.cfg, test.targetOS); got != test.want {
				t.Fatalf("useUnitCovdataProfile() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRunForOSBatchesAndMergesCoverageProfiles(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutWriter = &stdout
	stderrWriter = &stderr

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	testPackages := makeOversizedTestPackages(500)
	profilePath := filepath.Join(t.TempDir(), "merged-coverage.out")
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "test" {
			return "", "", errors.New("unexpected command in batched coverage test")
		}
		if got := windowsCommandLine(invocation.args); got > windowsCoverageCommandLineLimit {
			return "", "", errors.New("batched coverage command exceeded safe Windows limit")
		}
		invocations = append(invocations, invocation)
		batchIndex := len(invocations)
		coverage := strings.Join([]string{
			"mode: count",
			configPackage + "/config.go:1.1,2.1 3 " + strconv.Itoa(batchIndex-1),
			servicePackage + "/factory.go:1.1,2.1 5 0",
			"",
		}, "\n")
		return "", "", writeFakeCoverageProfile(helperCoverProfilePath(invocation.args), coverage)
	}

	result, err := runForOS(config{
		min:       0,
		totalOnly: true,
		coverpkg:  strings.Join([]string{configPackage, servicePackage}, ","),
		packages:  strings.Join(testPackages, " "),
		profile:   profilePath,
	}, "windows")
	if err != nil {
		t.Fatalf("runForOS() error = %v", err)
	}
	if len(invocations) < 2 {
		t.Fatalf("coverage invocations = %d, want multiple Windows batches", len(invocations))
	}
	if result.actual != 37.5 {
		t.Fatalf("merged coverage = %v, want 37.5", result.actual)
	}
	if result.packageTotals[configPackage] != (packageCoverageTotals{coveredStatements: 3, totalStatements: 3}) {
		t.Fatalf("config totals = %+v, want 3/3", result.packageTotals[configPackage])
	}
	if result.packageTotals[servicePackage] != (packageCoverageTotals{coveredStatements: 0, totalStatements: 5}) {
		t.Fatalf("service totals = %+v, want 0/5", result.packageTotals[servicePackage])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("merged profile stat error = %v", err)
	}
	for _, invocation := range invocations {
		batchProfile := helperCoverProfilePath(invocation.args)
		if _, err := os.Stat(filepath.Dir(batchProfile)); !os.IsNotExist(err) {
			t.Fatalf("batch directory for %q still exists, stat error = %v", batchProfile, err)
		}
	}
}

func TestRunForOSKeepsCoverageResultsIdenticalAcrossDirectAndBatchedProfiles(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	coverPackages := []string{configPackage, servicePackage}
	testPackages := makeOversizedTestPackages(500)
	manifestPath := writeCoverageEquivalenceManifest(t, configPackage, servicePackage)
	profile := strings.Join([]string{
		"mode: count",
		configPackage + "/config.go:1.1,2.1 4 0",
		configPackage + "/config.go:1.1,2.1 4 2",
		configPackage + "/config.go:3.1,4.1 6 0",
		servicePackage + "/factory.go:1.1,2.1 8 1",
		"",
	}, "\n")

	direct, err := runCoverageEquivalenceScenario(t, "linux", manifestPath, coverPackages, testPackages, profile)
	if err != nil {
		t.Fatalf("direct runForOS() error = %v", err)
	}
	batched, err := runCoverageEquivalenceScenario(t, "windows", manifestPath, coverPackages, testPackages, profile)
	if err != nil {
		t.Fatalf("batched runForOS() error = %v", err)
	}

	if len(direct.invocations) != 1 {
		t.Fatalf("direct invocations = %d, want one", len(direct.invocations))
	}
	if len(batched.invocations) < 2 {
		t.Fatalf("batched invocations = %d, want multiple", len(batched.invocations))
	}
	if direct.output != batched.output || direct.output != "total: (statements) 66.7%\n" {
		t.Fatalf("direct output = %q, batched output = %q, want identical 66.7%% total", direct.output, batched.output)
	}
	if !bytes.Equal(direct.profile, batched.profile) {
		t.Fatalf("canonical coverage profiles differ:\ndirect=%s\nbatched=%s", direct.profile, batched.profile)
	}
	assertCoverageEquivalence(t, direct.result, batched.result, configPackage, servicePackage)
}

func assertCoverageEquivalence(t *testing.T, direct coverageResult, batched coverageResult, configPackage string, servicePackage string) {
	t.Helper()
	if direct.actual != batched.actual {
		t.Fatalf("actual coverage direct=%v, batched=%v, want identical", direct.actual, batched.actual)
	}
	if !reflect.DeepEqual(direct.packageTotals, batched.packageTotals) {
		t.Fatalf("package totals differ:\ndirect=%v\nbatched=%v", direct.packageTotals, batched.packageTotals)
	}
	if !reflect.DeepEqual(direct.packageSummaries, batched.packageSummaries) {
		t.Fatalf("package summaries differ:\ndirect=%v\nbatched=%v", direct.packageSummaries, batched.packageSummaries)
	}
	if !reflect.DeepEqual(direct.insufficientCoveragePackages, batched.insufficientCoveragePackages) ||
		!reflect.DeepEqual(direct.zeroCoveragePackages, batched.zeroCoveragePackages) ||
		!reflect.DeepEqual(direct.packageMinimumFailures, batched.packageMinimumFailures) ||
		!reflect.DeepEqual(direct.packageMinimumWarnings, batched.packageMinimumWarnings) ||
		!reflect.DeepEqual(direct.packageGates, batched.packageGates) {
		t.Fatalf("coverage gate results differ:\ndirect=%+v\nbatched=%+v", direct, batched)
	}

	wantTotals := map[string]packageCoverageTotals{
		configPackage:  {coveredStatements: 4, totalStatements: 10},
		servicePackage: {coveredStatements: 8, totalStatements: 8},
	}
	if !reflect.DeepEqual(direct.packageTotals, wantTotals) {
		t.Fatalf("package totals = %v, want %v", direct.packageTotals, wantTotals)
	}
	wantSummaries := []packageCoverageSummary{
		{importPath: configPackage, coverage: 40},
		{importPath: servicePackage, coverage: 100},
	}
	if !reflect.DeepEqual(direct.packageSummaries, wantSummaries) {
		t.Fatalf("package summaries = %v, want %v", direct.packageSummaries, wantSummaries)
	}
	if len(direct.packageMinimumFailures) != 1 || !strings.Contains(direct.packageMinimumFailures[0], "package="+configPackage) {
		t.Fatalf("package minimum failures = %v, want the uncovered package gate failure", direct.packageMinimumFailures)
	}
	if direct.packageGates[configPackage].Floor == nil || *direct.packageGates[configPackage].Floor != 41 {
		t.Fatalf("config package gate = %+v, want 41%% floor", direct.packageGates[configPackage])
	}
}

type coverageEquivalenceRun struct {
	result      coverageResult
	output      string
	invocations []commandInvocation
	profile     []byte
}

func runCoverageEquivalenceScenario(t *testing.T, targetOS string, manifestPath string, coverPackages []string, testPackages []string, profile string) (coverageEquivalenceRun, error) {
	t.Helper()
	var invocations []commandInvocation
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutWriter = &stdout
	stderrWriter = &stderr
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "test" {
			return "", "", fmt.Errorf("unexpected command in coverage equivalence test: %v", invocation.args)
		}
		if targetOS == "windows" && !coverageCommandFitsWindowsLimit(invocation.args) {
			return "", "", errors.New("bounded coverage invocation exceeded safe Windows limit")
		}
		invocations = append(invocations, invocation)
		if err := writeFakeCoverageProfile(helperCoverProfilePath(invocation.args), profile); err != nil {
			return "", "", err
		}
		return "", "", nil
	}

	profilePath := filepath.Join(t.TempDir(), targetOS+"-coverage.out")
	result, err := runForOS(config{
		min:                 0,
		suite:               "unit",
		coverpkg:            strings.Join(coverPackages, ","),
		packages:            strings.Join(testPackages, " "),
		packageManifest:     manifestPath,
		packageFloorEpsilon: 0.25,
		profile:             profilePath,
	}, targetOS)
	if err != nil {
		return coverageEquivalenceRun{result: result, output: stdout.String(), invocations: invocations}, err
	}
	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		return coverageEquivalenceRun{}, err
	}
	return coverageEquivalenceRun{
		result:      result,
		output:      stdout.String(),
		invocations: invocations,
		profile:     profileData,
	}, nil
}

func TestRunGoTestCoverageLanePropagatesBatchedFailureAndCleansProfiles(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := makeOversizedTestPackages(1000)
	profilePath := filepath.Join(t.TempDir(), "merged-coverage.out")
	repoRoot, err := repoRootDir()
	if err != nil {
		t.Fatalf("repoRootDir() error = %v", err)
	}

	var invocations []commandInvocation
	var batchDir string
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		batchProfile := helperCoverProfilePath(invocation.args)
		if batchDir == "" {
			batchDir = filepath.Dir(batchProfile)
		}
		if len(invocations) == 2 {
			return "", "synthetic subprocess failure", errors.New("exit status 23")
		}
		if err := writeFakeCoverageProfile(batchProfile, "mode: count\n"+modulePath+"/pkg/config/config.go:1.1,2.1 3 1\n"); err != nil {
			return "", "", err
		}
		return "", "", nil
	}

	err = runGoTestCoverageLane(
		config{},
		commonArgs,
		testPackages,
		profilePath,
		repoRoot,
		[]string{modulePath + "/pkg/config"},
		"windows",
		"run unit coverage",
	)
	if err == nil {
		t.Fatal("runGoTestCoverageLane() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "run unit coverage (batch 2/") || !strings.Contains(err.Error(), "synthetic subprocess failure") {
		t.Fatalf("runGoTestCoverageLane() error = %q, want batch context and subprocess detail", err)
	}
	if len(invocations) < 3 {
		t.Fatalf("coverage invocations = %d, want later batches after second-batch failure", len(invocations))
	}
	seen := make(map[string]int, len(testPackages))
	for _, invocation := range invocations {
		for _, testPackage := range testPackages {
			if slicesContains(invocation.args, testPackage) {
				seen[testPackage]++
			}
		}
	}
	for _, testPackage := range testPackages {
		if seen[testPackage] != 1 {
			t.Fatalf("test package %q invoked %d times, want exactly once across failed lane", testPackage, seen[testPackage])
		}
	}
	profileData, readErr := os.ReadFile(profilePath)
	if readErr != nil {
		t.Fatalf("merged failure profile read error = %v, want profiles from completed batches", readErr)
	}
	if got, want := string(profileData), "mode: count\n"+modulePath+"/pkg/config/config.go:1.1,2.1 3 1\n"; got != want {
		t.Fatalf("merged failure profile = %q, want deterministic completed-batch profile %q", got, want)
	}
	if batchDir == "" {
		t.Fatal("batch directory was not captured")
	}
	if _, statErr := os.Stat(batchDir); !os.IsNotExist(statErr) {
		t.Fatalf("batch directory still exists after failure, stat error = %v", statErr)
	}
}

func TestRunGoTestCoverageLaneRejectsIncompleteOrIncompatibleBatchProfiles(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	commonArgs := []string{
		"test",
		"-coverpkg=" + modulePath + "/pkg/...",
		"-p=2",
		"-count=1",
		"-covermode=count",
		"-timeout=10m",
	}
	testPackages := makeOversizedTestPackages(1000)
	repoRoot, err := repoRootDir()
	if err != nil {
		t.Fatalf("repoRootDir() error = %v", err)
	}

	tests := []struct {
		name       string
		profileFor func(batch int) (string, bool)
		wantError  string
	}{
		{
			name: "successful batch without profile",
			profileFor: func(batch int) (string, bool) {
				return "mode: count\n" + modulePath + "/pkg/config/config.go:1.1,2.1 3 1\n", batch != 2
			},
			wantError: "go coverage batch 2 completed without writing profile",
		},
		{
			name: "incompatible profile mode",
			profileFor: func(batch int) (string, bool) {
				mode := "count"
				if batch == 2 {
					mode = "atomic"
				}
				return "mode: " + mode + "\n" + modulePath + "/pkg/config/config.go:1.1,2.1 3 1\n", true
			},
			wantError: "merge go coverage profiles: mode headers differ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profilePath := filepath.Join(t.TempDir(), "merged-coverage.out")
			invocationCount := 0
			commandRunner = func(invocation commandInvocation) (string, string, error) {
				invocationCount++
				profileData, writeProfile := tc.profileFor(invocationCount)
				if writeProfile {
					if err := writeFakeCoverageProfile(helperCoverProfilePath(invocation.args), profileData); err != nil {
						return "", "", err
					}
				}
				return "", "", nil
			}

			err := runGoTestCoverageLane(
				config{},
				commonArgs,
				testPackages,
				profilePath,
				repoRoot,
				[]string{modulePath + "/pkg/config"},
				"windows",
				"run unit coverage",
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("runGoTestCoverageLane() error = %v, want diagnostic containing %q", err, tc.wantError)
			}
			if invocationCount < 3 {
				t.Fatalf("coverage invocations = %d, want multiple batches", invocationCount)
			}
		})
	}
}

func makeOversizedTestPackages(count int) []string {
	packages := make([]string, 0, count)
	for index := 0; index < count; index++ {
		packages = append(packages, modulePath+"/pkg/coverage_test_"+strings.Repeat("x", 32)+strconv.Itoa(index))
	}
	return packages
}

func writeCoverageEquivalenceManifest(t *testing.T, configPackage string, servicePackage string) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), "coverage-minimums.json")
	manifest := fmt.Sprintf(`{"version":1,"lane":"unit","packages":[{"package":%q,"minimum":41.00},{"package":%q,"minimum":100.00}]}
`, configPackage, servicePackage)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write coverage equivalence manifest: %v", err)
	}
	return manifestPath
}
