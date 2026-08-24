package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestMainFailsWhenCoverageBelowMinimumViaFailf(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=100.1",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/service", got)
	}
	if got := stdout.String(); strings.Contains(got, "meets minimum") {
		t.Fatalf("main() stdout = %q, did not expect success message", got)
	}
	wantFailure := "go coverage 100.0% is below minimum 100.1%\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainReportsPassingCoverageWithoutFailing(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 0 {
		t.Fatalf("main() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "Go coverage 100.0% meets minimum 80.0%.") {
		t.Fatalf("main() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("main() stderr = %q, want empty stderr", got)
	}
}

func TestMainPackageManifestEpsilonCases(t *testing.T) {
	configPackage := modulePath + "/pkg/config"
	cases := []struct {
		name        string
		minimum     string
		epsilon     string
		wantExit    int
		wantWarning bool
		wantFailure bool
	}{
		{name: "default boundary warning", minimum: "0.25", wantWarning: true},
		{name: "beyond default epsilon", minimum: "0.26", wantExit: 1, wantFailure: true},
		{name: "strict zero", minimum: "0.25", epsilon: "0", wantExit: 1, wantFailure: true},
		{name: "exact floor", minimum: "0.00"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := writePackageMinimumManifest(t, "unit", configPackage, tc.minimum)
			args := []string{
				"-min=0",
				"-suite=unit",
				"-package-manifest=" + manifestPath,
				"-coverpkg=" + configPackage,
				"-packages=./pkg/config",
			}
			if tc.epsilon != "" {
				args = append(args, "-package-floor-epsilon="+tc.epsilon)
			}
			_, stderr, exitCode := runMainForTest(t, args, fakeGoCoverageCommandWithMeasuredZeroConfig)

			if exitCode != tc.wantExit {
				t.Fatalf("main() exit code = %d, want %d; stderr=%q", exitCode, tc.wantExit, stderr)
			}
			if tc.wantWarning {
				got := stderr
				for _, want := range []string{
					"package coverage warning: tolerated drift",
					"package=" + configPackage,
					"lane=unit",
					"expected-minimum=0.25%",
					"actual=0.0000%",
					"delta=-0.2500 percentage-points",
					"epsilon=0.2500 percentage-points",
				} {
					if !strings.Contains(got, want) {
						t.Fatalf("main() stderr = %q, want warning containing %q", got, want)
					}
				}
				if strings.Contains(got, "update-manifest") {
					t.Fatalf("main() stderr = %q, did not expect failure remediation", got)
				}
			} else if stderr != "" && !tc.wantFailure {
				t.Fatalf("main() stderr = %q, want empty stderr", stderr)
			}
			if tc.wantFailure {
				got := stderr
				if !strings.Contains(got, "package coverage regression: package="+configPackage) || !strings.Contains(got, "-update-manifest") {
					t.Fatalf("main() stderr = %q, want regression and remediation", got)
				}
				if strings.Contains(got, "drift tolerated") {
					t.Fatalf("main() stderr = %q, did not expect tolerated warning", got)
				}
			}
		})
	}
}

func TestMainPackageFloorPolicyAdvisoryReportsRegressionAndBlockingRestoresEnforcement(t *testing.T) {
	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "80.00")

	for _, tc := range []struct {
		name           string
		policy         string
		wantExit       int
		wantBanner     bool
		wantSuccess    bool
		wantStatus     string
		wantDiagnostic string
	}{
		{
			name:           "advisory",
			policy:         coverageFloorPolicyAdvisory,
			wantBanner:     true,
			wantSuccess:    true,
			wantStatus:     "WARN",
			wantDiagnostic: "package coverage regression: package=" + configPackage + " lane=unit expected-minimum=80.00% actual=0.0000% delta=-80.0000 percentage-points covered=0/3 statements",
		},
		{
			name:           "blocking",
			policy:         coverageFloorPolicyBlocking,
			wantExit:       1,
			wantStatus:     "FAIL",
			wantDiagnostic: "package coverage regression: package=" + configPackage + " lane=unit expected-minimum=80.00% actual=0.0000% delta=-80.0000 percentage-points covered=0/3 statements",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"-min=0",
				"-suite=unit",
				"-package-floor-policy=" + tc.policy,
				"-package-manifest=" + manifestPath,
				"-coverpkg=" + configPackage,
				"-packages=./pkg/config",
			}
			stdout, stderr, exitCode := runMainForTest(t, args, fakeGoCoverageCommandWithMeasuredZeroConfig)

			if exitCode != tc.wantExit {
				t.Fatalf("main() exit code = %d, want %d; stdout=%q stderr=%q", exitCode, tc.wantExit, stdout, stderr)
			}
			if tc.wantBanner {
				for _, want := range []string{
					"!!! COVERAGE FLOOR POLICY: advisory !!!",
					"Package floors and missing-manifest findings are report-only",
					"Set -package-floor-policy=blocking to restore blocking enforcement.",
				} {
					if !strings.Contains(stderr, want) {
						t.Fatalf("main() stderr = %q, want advisory banner containing %q", stderr, want)
					}
				}
			} else if strings.Contains(stderr, "COVERAGE FLOOR POLICY") {
				t.Fatalf("main() stderr = %q, did not expect advisory banner in blocking mode", stderr)
			}
			if !strings.Contains(stderr, tc.wantDiagnostic) {
				t.Fatalf("main() stderr = %q, want regression diagnostic containing %q", stderr, tc.wantDiagnostic)
			}
			wantVerdict := "package=" + configPackage + " coverage=0.0% floor=80.0% delta=-80.0pp status=" + tc.wantStatus + " lane=unit"
			if !strings.Contains(stdout, wantVerdict) {
				t.Fatalf("main() stdout = %q, want compact package verdict %q", stdout, wantVerdict)
			}
			if strings.Contains(stdout, "uncovered blocks:") || strings.Contains(stderr, "uncovered blocks:") {
				t.Fatalf("main() default diagnostics exposed uncovered source detail: stdout=%q stderr=%q", stdout, stderr)
			}
			if tc.wantSuccess {
				if !strings.Contains(stdout, "Go coverage 0.0% meets minimum 0.0%.") {
					t.Fatalf("main() stdout = %q, want successful aggregate message", stdout)
				}
			} else if strings.Contains(stdout, "meets minimum") {
				t.Fatalf("main() stdout = %q, did not expect success message", stdout)
			}
		})
	}

	for _, policy := range []string{coverageFloorPolicyAdvisory, coverageFloorPolicyBlocking} {
		t.Run(policy+" detailed diagnostics", func(t *testing.T) {
			args := []string{
				"-min=0",
				"-suite=unit",
				"-detailed-diagnostics",
				"-package-floor-policy=" + policy,
				"-package-manifest=" + manifestPath,
				"-coverpkg=" + configPackage,
				"-packages=./pkg/config",
			}
			_, stderr, _ := runMainForTest(t, args, fakeGoCoverageCommandWithMeasuredZeroConfig)
			if !strings.Contains(stderr, "uncovered blocks: pkg/config/config.go:1 (3 statements)") {
				t.Fatalf("main() detailed stderr = %q, want uncovered source detail", stderr)
			}
		})
	}
}

func TestMainDefaultPackageFloorPolicyBlocksRegression(t *testing.T) {
	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "80.00")
	stdout, stderr, exitCode := runMainForTest(t, []string{
		"-min=0",
		"-suite=unit",
		"-package-manifest=" + manifestPath,
		"-coverpkg=" + configPackage,
		"-packages=./pkg/config",
	}, fakeGoCoverageCommandWithMeasuredZeroConfig)

	if exitCode != 1 {
		t.Fatalf("main() default policy exit code = %d, want 1; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}
	if strings.Contains(stderr, "COVERAGE FLOOR POLICY: advisory") || strings.Contains(stderr, "report-only") {
		t.Fatalf("main() default policy emitted advisory guidance: %q", stderr)
	}
	want := "package coverage regression: package=" + configPackage + " lane=unit expected-minimum=80.00% actual=0.0000% delta=-80.0000 percentage-points"
	if !strings.Contains(stderr, want) {
		t.Fatalf("main() default policy stderr = %q, want %q", stderr, want)
	}
}

func TestMainPackageFloorPolicyAdvisoryReportsMissingManifestEntryAndBlockingRestoresEnforcement(t *testing.T) {
	rootObservationPackage := modulePath + "/pkg/services/factory_runtime/internal/rootobservation"
	manifestPath := writePackageMinimumManifest(t, "unit", rootObservationPackage, "0.00")

	for _, tc := range []struct {
		name       string
		policy     string
		wantExit   int
		wantBanner bool
	}{
		{name: "advisory", policy: coverageFloorPolicyAdvisory, wantBanner: true},
		{name: "blocking", policy: coverageFloorPolicyBlocking, wantExit: 1},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"-min=0",
				"-suite=unit",
				"-package-floor-policy=" + tc.policy,
				"-package-manifest=" + manifestPath,
				"-coverpkg=" + rootObservationPackage,
				"-packages=./pkg/services/factory_runtime/internal/rootobservation",
			}
			stdout, stderr, exitCode := runMainForTest(t, args, fakeGoCoverageCommandWithRootObservationRegression)

			if exitCode != tc.wantExit {
				t.Fatalf("main() exit code = %d, want %d; stdout=%q stderr=%q", exitCode, tc.wantExit, stdout, stderr)
			}
			wantDiagnostic := "coverage manifest missing entry: package=" + modulePath + "/pkg/services/factory_runtime lane=unit"
			if !tc.wantBanner {
				wantDiagnostic = "measured unit services have no root manifest entry"
			}
			if !strings.Contains(stderr, wantDiagnostic) {
				t.Fatalf("main() stderr = %q, want missing-manifest diagnostic containing %q", stderr, wantDiagnostic)
			}
			if tc.wantBanner && !strings.Contains(stderr, "COVERAGE FLOOR POLICY: advisory") {
				t.Fatalf("main() stderr = %q, want advisory banner", stderr)
			}
			if tc.wantBanner {
				if !strings.Contains(stdout, "Go coverage 95.6% meets minimum 0.0%.") {
					t.Fatalf("main() stdout = %q, want successful aggregate message", stdout)
				}
			} else if strings.Contains(stdout, "meets minimum") {
				t.Fatalf("main() stdout = %q, did not expect success message", stdout)
			}
		})
	}
}

func TestMainPackageFloorPolicyAdvisoryDoesNotMaskFailedTests(t *testing.T) {
	for _, tc := range []struct {
		name       string
		policy     string
		wantBanner bool
	}{
		{name: "advisory", policy: coverageFloorPolicyAdvisory, wantBanner: true},
		{name: "blocking", policy: coverageFloorPolicyBlocking},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runMainForTest(t, []string{
				"-package-floor-policy=" + tc.policy,
				"-coverpkg=" + modulePath + "/pkg/config",
				"-packages=./pkg/config",
			}, fakeGoCoverageCommandTestFailsWithObservedFailures)

			if exitCode != 1 {
				t.Fatalf("main() exit code = %d, want 1; stdout=%q stderr=%q", exitCode, stdout, stderr)
			}
			want := "coverage not evaluated: 2 failed tests observed; package floors were NOT checked because the coverage test run failed"
			if !strings.Contains(stderr, want) {
				t.Fatalf("main() stderr = %q, want exact failed-test diagnostic containing %q", stderr, want)
			}
			if strings.Contains(stderr, "package coverage regression") || strings.Contains(stdout, "meets minimum") {
				t.Fatalf("failed-test run was reclassified: stdout=%q stderr=%q", stdout, stderr)
			}
			if tc.wantBanner && !strings.Contains(stderr, "COVERAGE FLOOR POLICY: advisory") {
				t.Fatalf("main() stderr = %q, want advisory banner", stderr)
			}
			if !tc.wantBanner && strings.Contains(stderr, "COVERAGE FLOOR POLICY") {
				t.Fatalf("main() stderr = %q, did not expect advisory banner in blocking mode", stderr)
			}
		})
	}
}

func TestMainRejectsUnknownPackageFloorPolicyBeforeCoverageWork(t *testing.T) {
	called := false
	_, stderr, exitCode := runMainForTest(t, []string{"-package-floor-policy=unknown"}, func(commandInvocation) (string, string, error) {
		called = true
		return "", "", nil
	})

	if called {
		t.Fatal("main() started coverage work for invalid package floor policy")
	}
	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "-package-floor-policy must be") {
		t.Fatalf("main() stderr = %q, want actionable policy diagnostic", stderr)
	}
}

func TestMainRejectsNegativePackageFloorEpsilonBeforeCoverageWork(t *testing.T) {
	called := false
	_, stderr, exitCode := runMainForTest(t, []string{"-package-floor-epsilon=-0.01"}, func(commandInvocation) (string, string, error) {
		called = true
		return "", "", nil
	})

	if called {
		t.Fatal("main() started coverage work for invalid epsilon")
	}
	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "finite non-negative percentage-point value") || !strings.Contains(stderr, "set it to 0 or greater") {
		t.Fatalf("main() stderr = %q, want actionable epsilon diagnostic", stderr)
	}
	if strings.Contains(stderr, "coverage not evaluated") {
		t.Fatalf("main() stderr = %q, did not expect test-failure-only warning", stderr)
	}
}

func TestMainReportsSkippedPackageFloorsWhenCoverageTestsFail(t *testing.T) {
	stdout, stderr, exitCode := runMainForTest(t, []string{
		"-coverpkg=" + modulePath + "/pkg/config",
		"-packages=./pkg/config",
	}, fakeGoCoverageCommandTestFailsWithObservedFailures)

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if strings.Contains(stdout, "\tcoverage:") || strings.Contains(stdout, "total: (statements)") {
		t.Fatalf("main() stdout = %q, want no coverage summaries without a valid measurement", stdout)
	}
	for _, want := range []string{
		"coverage not evaluated",
		"2 failed tests observed",
		"package floors were NOT checked",
		"raw failure output from go test",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("main() stderr = %q, want diagnostic containing %q", stderr, want)
		}
	}
}

func TestMainRejectsSingleProfileManifestUpdateBeforeCoverage(t *testing.T) {
	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "80.00")
	called := false
	_, stderr, exitCode := runMainForTest(t, []string{
		"-suite=unit",
		"-update-manifest=" + manifestPath,
		"-update-profiles=one.out",
	}, func(commandInvocation) (string, string, error) {
		called = true
		return "", "", nil
	})

	if called {
		t.Fatal("main() started coverage work for a single-profile manifest update")
	}
	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "requires at least 5 profiles") {
		t.Fatalf("main() stderr = %q, want actionable insufficient-sample diagnostic", stderr)
	}
}

func runMainForTest(t *testing.T, args []string, runner commandRunnerFunc) (string, string, int) {
	t.Helper()
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	defer func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := 0
	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = append([]string{"gocoveragecheck"}, args...)
	commandRunner = runner
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()
	return stdout.String(), stderr.String(), exitCode
}

func TestMainFailsWhenZeroCoveragePackagesDetectedViaFailf(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	baselinePath := emptyPackageCoverageBaselinePath(t)

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-package-baseline=" + baselinePath,
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoveragePackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	baselinePath := emptyPackageCoverageBaselinePath(t)

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-package-baseline=" + baselinePath,
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	baselinePath := emptyPackageCoverageBaselinePath(t)

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-package-baseline=" + baselinePath,
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommandWithOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageCoverpkgOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
		exitFunc = originalExit
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var exitCode int

	baselinePath := emptyPackageCoverageBaselinePath(t)

	flag.CommandLine = flag.NewFlagSet("gocoveragecheck", flag.ExitOnError)
	os.Args = []string{
		"gocoveragecheck",
		"-min=80",
		"-package-baseline=" + baselinePath,
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
			modulePath + "/pkg/transports/http/client",
		}, ","),
		"-packages=./pkg/config",
	}
	commandRunner = fakeGoCoverageCommandWithCoverpkgOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=0.0% floor=80.0% delta=-80.0pp status=FAIL lane=unit") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}
