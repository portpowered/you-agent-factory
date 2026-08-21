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
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
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
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
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
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
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
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
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
	if got := stdout.String(); !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 0.0% of statements") {
		t.Fatalf("main() stdout = %q, want package summary for pkg/config", got)
	}
	wantFailure := "go coverage found non-baselined backend packages below 80.0% statement coverage: " + modulePath + "/pkg/config (0.0%)\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}
