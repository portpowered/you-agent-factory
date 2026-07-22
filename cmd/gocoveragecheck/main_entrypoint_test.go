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
