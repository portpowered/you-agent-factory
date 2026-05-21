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
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
		"-min=90",
		"-coverpkg=" + strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	if got := stdout.String(); strings.Contains(got, "meets minimum") {
		t.Fatalf("main() stdout = %q, did not expect success message", got)
	}
	wantFailure := "go coverage 82.5% is below minimum 90.0%\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainReportsPassingCoverageWithoutFailing(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
	execCommand = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 0 {
		t.Fatalf("main() exit code = %d, want 0", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "Go coverage 82.5% meets minimum 80.0%.") {
		t.Fatalf("main() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("main() stderr = %q, want empty stderr", got)
	}
}

func TestMainFailsWhenZeroCoveragePackagesDetectedViaFailf(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoveragePackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandWithOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}

func TestMainFailsWithZeroCoverageCoverpkgOKPackageSummary(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	originalExecCommand := execCommand
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	originalExit := exitFunc
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
		execCommand = originalExecCommand
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
			modulePath + "/pkg/generatedclient",
		}, ","),
		"-packages=./pkg/config",
	}
	execCommand = fakeGoCoverageCommandWithCoverpkgOKSummary
	stdoutWriter = &stdout
	stderrWriter = &stderr
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if exitCode != 1 {
		t.Fatalf("main() exit code = %d, want 1", exitCode)
	}
	if got := stdout.String(); !strings.Contains(got, "total: (statements) 82.5%") {
		t.Fatalf("main() stdout = %q, want total coverage line", got)
	}
	wantFailure := "go coverage found backend-owned packages with 0% statement coverage: " + modulePath + "/pkg/config\n"
	if got := stderr.String(); got != wantFailure {
		t.Fatalf("main() stderr = %q, want %q", got, wantFailure)
	}
}
