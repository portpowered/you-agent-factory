package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExecuteReportsPassingCoverage(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	if strings.Contains(got, "\t\tcoverage: 75.0% of statements") {
		t.Fatalf("execute() stdout = %q, did not expect raw go test coverage output", got)
	}
	wantSuccess := "Go coverage 100.0% meets minimum 80.0%."
	if !strings.Contains(got, wantSuccess) {
		t.Fatalf("execute() stdout = %q, want success message %q", got, wantSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteReportsPackageSummariesWhenManifestValidationFails(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	workRootPackage := modulePath + "/pkg/services/work"
	workInternalPackage := modulePath + "/pkg/services/work/internal"
	manifestPath := writePackageMinimumManifest(t, "unit", workInternalPackage, "100.00")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:             0,
		suite:           "unit",
		coverpkg:        strings.Join([]string{workRootPackage, workInternalPackage}, ","),
		packages:        "./pkg/services/work/internal",
		packageManifest: manifestPath,
	})
	if err == nil || !strings.Contains(err.Error(), "measured unit service \""+workRootPackage+"\" has no root manifest entry") {
		t.Fatalf("execute() error = %v, want missing-service-root validation failure", err)
	}

	wantSummaries := workRootPackage + "\tcoverage: 0.0% of statements\n" + workInternalPackage + "\tcoverage: 0.0% of statements\n"
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 0.0%") || !strings.Contains(got, wantSummaries) {
		t.Fatalf("execute() stdout = %q, want total and exact package summaries", got)
	}
	if strings.Contains(stdout.String(), "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteReportsPackageSummariesForCompleteManifest(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "100.00")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:             0,
		suite:           "unit",
		coverpkg:        configPackage,
		packages:        "./pkg/config",
		packageManifest: manifestPath,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	wantSummary := "package=" + configPackage + " coverage=100.0% floor=100.0% delta=+0.0pp status=PASS lane=unit\n"
	if strings.Count(got, wantSummary) != 1 {
		t.Fatalf("execute() stdout = %q, want one package summary %q", got, wantSummary)
	}
	if !strings.Contains(got, "Go coverage 100.0% meets minimum 0.0%.") {
		t.Fatalf("execute() stdout = %q, want success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteReportsManifestPackagesWithoutProfileMeasurement(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	configPackage := modulePath + "/pkg/config"
	initializerPackage := modulePath + "/pkg/initializer"
	servicePackage := modulePath + "/pkg/service"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: configPackage, minimum: "0.00"},
		{importPath: initializerPackage, exception: unitUnmeasurableMeasurementException()},
		{importPath: servicePackage, exception: unitUnmeasurableMeasurementException()},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithMeasuredZeroConfig
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:             0,
		suite:           "unit",
		coverpkg:        strings.Join([]string{configPackage, initializerPackage, servicePackage}, ","),
		packages:        "./pkg/config",
		packageManifest: manifestPath,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	wantStderr := strings.Join([]string{
		"coverage not evaluated: package=" + initializerPackage + " lane=unit (no measurement in profile)",
		"coverage not evaluated: package=" + servicePackage + " lane=unit (no measurement in profile)",
		"",
	}, "\n")
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("execute() stderr = %q, want deterministic unmeasured diagnostics %q", got, wantStderr)
	}
	if strings.Contains(stderr.String(), "package="+configPackage) {
		t.Fatalf("execute() stderr = %q, did not report measured package as unmeasured", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Go coverage 0.0% meets minimum 0.0%.") {
		t.Fatalf("execute() stdout = %q, want unchanged successful gate result", stdout.String())
	}
}

func TestExecuteReportsUncoveredBlocksForManifestRegression(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "100.00")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithMeasuredZeroConfig
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:                 0,
		suite:               "unit",
		coverpkg:            configPackage,
		packages:            "./pkg/config",
		packageManifest:     manifestPath,
		detailedDiagnostics: true,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	for _, want := range []string{
		"package coverage regression: package=" + configPackage,
		"lane=unit expected-minimum=100.00%",
		"uncovered blocks: pkg/config/config.go:1 (3 statements)",
		"restore coverage before running `go run ./cmd/gocoveragecheck",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("execute() error = %q, want diagnostic containing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "pkg/service") {
		t.Fatalf("execute() error = %q, did not expect another package's uncovered blocks", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteReportsRootObservationCoverageRegressionWithExactCountsAndBlocks(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	rootObservationPackage := modulePath + "/pkg/services/factory_runtime/internal/rootobservation"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: modulePath + "/pkg/services/factory_runtime", minimum: "0.00"},
		{importPath: rootObservationPackage, minimum: "100.00"},
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithRootObservationRegression
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:                 0,
		suite:               "unit",
		coverpkg:            strings.Join([]string{modulePath + "/pkg/services/factory_runtime", rootObservationPackage}, ","),
		packages:            "./pkg/services/factory_runtime/internal/rootobservation",
		packageManifest:     manifestPath,
		detailedDiagnostics: true,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	wantFailure := "package coverage regression: package=" + rootObservationPackage +
		" lane=unit expected-minimum=100.00% actual=95.5556% delta=-4.4444 percentage-points covered=43/45 statements; " +
		"uncovered blocks: pkg/services/factory_runtime/internal/rootobservation/project.go:21 (1 statement), " +
		"pkg/services/factory_runtime/internal/rootobservation/project.go:41 (1 statement); " +
		"restore coverage before running `go run ./cmd/gocoveragecheck -suite unit -profile <coverage-profile> -update-manifest " + manifestPath + "`"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want exact regression diagnostic %q", err, wantFailure)
	}
	if strings.Count(err.Error(), "uncovered blocks:") != 1 || strings.Count(err.Error(), "(1 statement)") != 2 {
		t.Fatalf("execute() error = %q, want exactly two uncovered blocks", err)
	}
	if !strings.Contains(stdout.String(), "package="+rootObservationPackage+" coverage=95.6% floor=100.0% delta=-4.4pp status=FAIL lane=unit") {
		t.Fatalf("execute() stdout = %q, want rootobservation package verdict", stdout.String())
	}
	wantDiagnostic := "coverage not evaluated: package=" + modulePath + "/pkg/services/factory_runtime lane=unit (no measurement in profile)\n"
	if got := stderr.String(); got != wantDiagnostic {
		t.Fatalf("execute() stderr = %q, want the service-root unmeasured diagnostic %q", got, wantDiagnostic)
	}
}

func TestExecuteDoesNotReportPackageSummariesWhenMeasurementFails(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandTestFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:      0,
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	if strings.Contains(stdout.String(), "\tcoverage:") || strings.Contains(stdout.String(), "total: (statements)") {
		t.Fatalf("execute() stdout = %q, want no package summaries without a valid measurement", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "coverage not evaluated") || !strings.Contains(got, "package floors were NOT checked") {
		t.Fatalf("execute() stderr = %q, want skipped-floor diagnostic", got)
	}
	if strings.Contains(stderr.String(), "failed tests observed") {
		t.Fatalf("execute() stderr = %q, did not expect an invented failure count", stderr.String())
	}
}

func TestExecuteTotalOnlyReportsIndependentSuiteCoverageWithPackageSummaries(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min:       80,
		coverpkg:  modulePath + "/pkg/config",
		packages:  "./tests/functional/runtime_api",
		totalOnly: true,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=n/a status=report-only lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary from the selected suite profile", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFunctionalFailsForNonBaselinedPackageBelowTarget(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		coverpkg:        modulePath + "/pkg/config",
		packages:        "./tests/functional/runtime_api",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
		packageMin:      80,
		suite:           "functional",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly accepted a new functional package below target")
	}
	want := "go coverage found non-baselined backend packages below 80.0% statement coverage: " +
		modulePath + "/pkg/config (0.0%)"
	if err.Error() != want {
		t.Fatalf("execute() error = %q, want %q", err.Error(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenCoverageBelowMinimum(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 100.1,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := "go coverage 100.0% is below minimum 100.1%"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteEnforcesAggregateAndManifestMinimumsIndependently(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	tests := []struct {
		name         string
		minimum      string
		aggregateMin float64
		command      commandRunnerFunc
		wantFailure  string
		rejectText   string
		wantStderr   string
	}{
		{
			name:         "aggregate pass cannot mask package regression",
			minimum:      "1.00",
			aggregateMin: 0,
			command:      fakeGoCoverageCommandWithMeasuredZeroConfig,
			wantFailure:  "package coverage regression: package=" + modulePath + "/pkg/config lane=unit expected-minimum=1.00% actual=0.0000%",
		},
		{
			name:         "package pass cannot mask aggregate regression",
			minimum:      "100.00",
			aggregateMin: 100.1,
			command:      fakeGoCoverageCommandPassing,
			wantFailure:  "go coverage 100.0% is below minimum 100.1%",
			rejectText:   "package coverage regression",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := writePackageMinimumManifest(t, "unit", modulePath+"/pkg/config", tc.minimum)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			commandRunner = tc.command
			stdoutWriter = &stdout
			stderrWriter = &stderr

			err := execute(config{
				suite:           "unit",
				min:             tc.aggregateMin,
				coverpkg:        modulePath + "/pkg/config",
				packages:        "./pkg/config",
				packageManifest: manifestPath,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantFailure) {
				t.Fatalf("execute() error = %v, want failure containing %q", err, tc.wantFailure)
			}
			if tc.rejectText != "" && strings.Contains(err.Error(), tc.rejectText) {
				t.Fatalf("execute() error = %q, did not expect %q", err.Error(), tc.rejectText)
			}
			if got := stderr.String(); got != tc.wantStderr {
				t.Fatalf("execute() stderr = %q, want %q", got, tc.wantStderr)
			}
		})
	}
}

func writePackageMinimumManifest(t *testing.T, lane string, importPath string, minimum string) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), lane+"-minimums.json")
	data := fmt.Sprintf("{\n  \"version\": 1,\n  \"lane\": %q,\n  \"packages\": [\n    {\n      \"package\": %q,\n      \"minimum\": %s\n    }\n  ]\n}\n", lane, importPath, minimum)
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write package minimum manifest: %v", err)
	}
	return manifestPath
}

func TestExecuteFailsWhenCoverageBelowMinimumAndZeroCoveragePackage(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 100.1,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := strings.Join([]string{
		"go coverage 100.0% is below minimum 100.1%",
		"go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)",
	}, "\n")
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteFailsWhenZeroCoveragePackageOnly(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestRunCreatesAndRemovesTempCoverageProfile(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithTempProfileReport
	stdoutWriter = &stdout
	stderrWriter = &stderr

	result, err := run(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if result.actual != 100 {
		t.Fatalf("actual coverage = %v, want 100", result.actual)
	}
	if len(result.zeroCoveragePackages) != 0 {
		t.Fatalf("zero coverage packages = %v, want none", result.zeroCoveragePackages)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty stderr", stderr.String())
	}

	markerPath := filepath.Join(os.TempDir(), tempProfileMarkerFilename)
	t.Cleanup(func() {
		_ = os.Remove(markerPath)
	})
	profileData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read temp profile marker: %v", err)
	}
	profilePath := strings.TrimSpace(string(profileData))
	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Fatalf("temp profile %q still exists after run(), stat err = %v", profilePath, err)
	}
	if !strings.Contains(stdout.String(), "total: (statements) 100.0%") {
		t.Fatalf("run() stdout = %q, want total coverage line", stdout.String())
	}
}

func TestRunWrapsCoverageLaneFailure(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandTestFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
		profile:  filepath.Join(t.TempDir(), "coverage.out"),
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	want := "run go test coverage lane: exit status 7"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("run() error = %q, want prefix %q", err.Error(), want)
	}
	if !strings.Contains(err.Error(), "raw failure output from go test") {
		t.Fatalf("run() error = %q, want raw go test output detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureUsingStderrDetail(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithStderr

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "list go packages: exit status 5") {
		t.Fatalf("listGoPackages() error = %q, want wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stderr detail from go list") {
		t.Fatalf("listGoPackages() error = %q, want stderr detail", err.Error())
	}
	if strings.Contains(err.Error(), "stdout detail from go list") {
		t.Fatalf("listGoPackages() error = %q, did not expect stdout fallback detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureUsingStdoutFallback(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithStdout

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "list go packages: exit status 6") {
		t.Fatalf("listGoPackages() error = %q, want wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "stdout detail from go list") {
		t.Fatalf("listGoPackages() error = %q, want stdout fallback detail", err.Error())
	}
	if strings.Contains(err.Error(), "stderr detail from go list") {
		t.Fatalf("listGoPackages() error = %q, did not expect stderr detail", err.Error())
	}
}

func TestListGoPackagesWrapsListFailureWithoutDetail(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandFailsWithoutDetail

	_, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err == nil {
		t.Fatal("listGoPackages() unexpectedly succeeded")
	}

	want := "list go packages: exit status 9"
	if err.Error() != want {
		t.Fatalf("listGoPackages() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenDefaultCoverageDiscoveryMatchesNoBackendPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandWithExcludedPackagesOnly

	_, _, err := resolveCoverageLane(config{})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveCoverageLaneFailsWhenFunctionalDiscoveryMatchesNoMaintainedPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	defer func() {
		commandRunner = originalCommandRunner
	}()

	commandRunner = fakeGoListCommandWithCoverageButNoTestPackages

	_, _, err := resolveCoverageLane(config{suite: "functional"})
	if err == nil {
		t.Fatal("resolveCoverageLane() unexpectedly succeeded")
	}

	want := "resolve go coverage lane: no packages matched"
	if err.Error() != want {
		t.Fatalf("resolveCoverageLane() error = %q, want %q", err.Error(), want)
	}
}

func TestListGoPackagesFiltersDuplicatesAndExcludedPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		commandRunner = originalCommandRunner
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nestedDir := filepath.Join(repoRoot, "cmd", "gocoveragecheck")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	commandRunner = fakeGoListCommandWithDuplicatesAndExcludedPackages

	packages, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, true)
	if err != nil {
		t.Fatalf("listGoPackages() error = %v", err)
	}

	want := []string{
		modulePath + "/pkg/config",
	}
	if !slices.Equal(packages, want) {
		t.Fatalf("listGoPackages() = %v, want %v", packages, want)
	}

	testPackages, err := listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage, false)
	if err != nil {
		t.Fatalf("listGoPackages() for test lane error = %v", err)
	}
	if slices.Contains(testPackages, modulePath+"/pkg/config/exhaustiontests") {
		t.Fatalf("listGoPackages() for test lane = %v, maintenance package must be excluded", testPackages)
	}
}
