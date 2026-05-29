package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarizeBackendVerificationLogFindsFirstGoTestFailureOutsideTail(t *testing.T) {
	const failingPackage = "github.com/portpowered/infinite-you/pkg/service"
	logLines := []string{
		"ok  \tgithub.com/portpowered/infinite-you/pkg/config\t0.031s\tcoverage: 72.4% of statements",
		"=== RUN   TestFactoryServiceStarts",
		"    factory_service_test.go:42: expected startup work to be queued",
		"--- FAIL: TestFactoryServiceStarts (0.01s)",
		"FAIL",
		"FAIL\t" + failingPackage + "\t0.128s",
	}
	for index := 0; index < 90; index++ {
		logLines = append(logLines, "ok  \tgithub.com/portpowered/infinite-you/pkg/filler"+string(rune('a'+index%20))+"\t0.001s\tcoverage: 80.0% of statements")
	}
	logLines = append(logLines, "run go test coverage lane: exit status 1")

	summary := summarizeBackendVerificationLog(strings.Join(logLines, "\n"))

	if summary.Kind != "go test failure" {
		t.Fatalf("summary.Kind = %q, want go test failure", summary.Kind)
	}
	if summary.CommandPhase != "run go test coverage lane" {
		t.Fatalf("summary.CommandPhase = %q, want run go test coverage lane", summary.CommandPhase)
	}
	if summary.Package != failingPackage {
		t.Fatalf("summary.Package = %q, want %q", summary.Package, failingPackage)
	}
	if summary.TestName != "TestFactoryServiceStarts" {
		t.Fatalf("summary.TestName = %q, want TestFactoryServiceStarts", summary.TestName)
	}

	excerpt := strings.Join(summary.Excerpt, "\n")
	for _, want := range []string{
		"=== RUN   TestFactoryServiceStarts",
		"factory_service_test.go:42: expected startup work to be queued",
		"--- FAIL: TestFactoryServiceStarts (0.01s)",
		"FAIL\t" + failingPackage + "\t0.128s",
	} {
		if !strings.Contains(excerpt, want) {
			t.Fatalf("summary excerpt = %q, want %q", excerpt, want)
		}
	}
	if strings.Contains(excerpt, "pkg/filler") {
		t.Fatalf("summary excerpt = %q, did not expect final successful package tail", excerpt)
	}
}

func TestWriteMarkdownSummaryIncludesBackendFailureContract(t *testing.T) {
	summary := failureSummary{
		Kind:         "go test failure",
		CommandPhase: "run go test coverage lane",
		Package:      "github.com/portpowered/infinite-you/pkg/service",
		TestName:     "TestFactoryServiceStarts",
		Excerpt: []string{
			"=== RUN   TestFactoryServiceStarts",
			"--- FAIL: TestFactoryServiceStarts (0.01s)",
			"FAIL\tgithub.com/portpowered/infinite-you/pkg/service\t0.128s",
		},
		Inferred: true,
	}

	var output bytes.Buffer
	writeMarkdownSummary(&output, config{
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}, summary)

	got := output.String()
	for _, want := range []string{
		"- Result: `failed`",
		"- Failure type: `go test failure`",
		"- Command phase: `run go test coverage lane`",
		"- Package: `github.com/portpowered/infinite-you/pkg/service`",
		"- Test: `TestFactoryServiceStarts`",
		"- Local rerun: `make test-backend-verification`",
		"- Retained artifact: `backend-verification-failure-artifacts` for 14 days",
		"- Primary log: `.artifacts/backend-verification/command.log`",
		"#### First actionable failure excerpt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown summary = %q, want %q", got, want)
		}
	}
}

func TestSummarizeBackendVerificationLogFindsCoverageGateFailure(t *testing.T) {
	logLines := []string{
		"ok  \tgithub.com/portpowered/infinite-you/cmd/factory\t0.041s\tcoverage: 71.0% of statements in github.com/portpowered/infinite-you/cmd/factory,github.com/portpowered/infinite-you/pkg/config",
		"ok  \tgithub.com/portpowered/infinite-you/pkg/config\t0.031s\tcoverage: 79.3% of statements",
		"total:                                      (statements)                 82.5%",
		"go coverage 82.5% is below minimum 90.0%",
		"make[1]: *** [test-backend-verification] Error 1",
		"run go test coverage lane: exit status 2",
	}

	summary := summarizeBackendVerificationLog(strings.Join(logLines, "\n"))

	if summary.Kind != "coverage gate failure" {
		t.Fatalf("summary.Kind = %q, want coverage gate failure", summary.Kind)
	}
	if summary.CommandPhase != "run go test coverage lane" {
		t.Fatalf("summary.CommandPhase = %q, want run go test coverage lane", summary.CommandPhase)
	}
	if summary.Measured != "82.5%" {
		t.Fatalf("summary.Measured = %q, want 82.5%%", summary.Measured)
	}
	if summary.Required != "90.0%" {
		t.Fatalf("summary.Required = %q, want 90.0%%", summary.Required)
	}
	if summary.Package != "" || summary.TestName != "" {
		t.Fatalf("coverage summary package/test = %q/%q, want empty identities", summary.Package, summary.TestName)
	}

	excerpt := strings.Join(summary.Excerpt, "\n")
	for _, want := range []string{
		"total:                                      (statements)                 82.5%",
		"go coverage 82.5% is below minimum 90.0%",
		"run go test coverage lane: exit status 2",
	} {
		if !strings.Contains(excerpt, want) {
			t.Fatalf("summary excerpt = %q, want %q", excerpt, want)
		}
	}
	if strings.Contains(excerpt, "cmd/factory") {
		t.Fatalf("summary excerpt = %q, did not expect full package output", excerpt)
	}
}

func TestWriteMarkdownSummaryIncludesCoverageGateDetails(t *testing.T) {
	summary := failureSummary{
		Kind:         "coverage gate failure",
		CommandPhase: "run go test coverage lane",
		Measured:     "82.5%",
		Required:     "90.0%",
		Excerpt: []string{
			"total:                                      (statements)                 82.5%",
			"go coverage 82.5% is below minimum 90.0%",
		},
		Inferred: true,
	}

	var output bytes.Buffer
	writeMarkdownSummary(&output, config{
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}, summary)

	got := output.String()
	for _, want := range []string{
		"- Result: `failed`",
		"- Failure type: `coverage gate failure`",
		"- Command phase: `run go test coverage lane`",
		"- Measured coverage: `82.5%`",
		"- Required coverage: `90.0%`",
		"- Local rerun: `make test-backend-verification`",
		"- Retained artifact: `backend-verification-failure-artifacts` for 14 days",
		"- Primary log: `.artifacts/backend-verification/command.log`",
		"#### First actionable failure excerpt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown summary = %q, want %q", got, want)
		}
	}
}

func TestSummarizeBackendVerificationLogFallsBackToBoundedTail(t *testing.T) {
	logLines := []string{
		"run go test coverage lane: starting",
	}
	for index := 0; index < 90; index++ {
		logLines = append(logLines, "ok  \tgithub.com/portpowered/infinite-you/pkg/filler\t0.001s")
	}
	logLines = append(logLines,
		"panic: bootstrap environment missing required provider fixture",
		"goroutine 1 [running]:",
		"exit status 2",
		"run go test coverage lane: exit status 2",
	)

	summary := summarizeBackendVerificationLog(strings.Join(logLines, "\n"))

	if summary.Kind != "unclassified backend verification failure" {
		t.Fatalf("summary.Kind = %q, want unclassified backend verification failure", summary.Kind)
	}
	if summary.Inferred {
		t.Fatalf("summary.Inferred = true, want false")
	}
	excerpt := strings.Join(summary.Excerpt, "\n")
	for _, want := range []string{
		"... excerpt truncated ...",
		"panic: bootstrap environment missing required provider fixture",
		"run go test coverage lane: exit status 2",
	} {
		if !strings.Contains(excerpt, want) {
			t.Fatalf("summary excerpt = %q, want %q", excerpt, want)
		}
	}
	if strings.Contains(excerpt, "run go test coverage lane: starting") {
		t.Fatalf("summary excerpt = %q, did not expect unbounded log start", excerpt)
	}
	if len(summary.Excerpt) > excerptLineLimit+1 {
		t.Fatalf("summary excerpt has %d lines, want at most %d", len(summary.Excerpt), excerptLineLimit+1)
	}
}

func TestWriteMarkdownSummaryIncludesUnclassifiedFallbackContract(t *testing.T) {
	summary := failureSummary{
		Kind: "unclassified backend verification failure",
		Excerpt: []string{
			"panic: bootstrap environment missing required provider fixture",
			"run go test coverage lane: exit status 2",
		},
		Inferred: false,
	}

	var output bytes.Buffer
	writeMarkdownSummary(&output, config{
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}, summary)

	got := output.String()
	for _, want := range []string{
		"- Result: `failed`",
		"- Failure type: `unclassified backend verification failure`",
		"- Failure identity: specific failing package or test could not be inferred from the command log.",
		"- Local rerun: `make test-backend-verification`",
		"- Retained artifact: `backend-verification-failure-artifacts` for 14 days",
		"- Primary log: `.artifacts/backend-verification/command.log`",
		"#### First actionable failure excerpt",
		"panic: bootstrap environment missing required provider fixture",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown summary = %q, want %q", got, want)
		}
	}
}

func TestSummarizeBackendVerificationLogFallbackHandlesEmptyLog(t *testing.T) {
	summary := summarizeBackendVerificationLog("")

	if summary.Kind != "unclassified backend verification failure" {
		t.Fatalf("summary.Kind = %q, want unclassified backend verification failure", summary.Kind)
	}
	if summary.Inferred {
		t.Fatalf("summary.Inferred = true, want false")
	}
	if got := strings.Join(summary.Excerpt, "\n"); got != "command log was empty" {
		t.Fatalf("summary.Excerpt = %q, want command log was empty", got)
	}
}

func TestRunRequiresLogPath(t *testing.T) {
	err := run(config{}, ioDiscard())
	if err == nil || !strings.Contains(err.Error(), "-log is required") {
		t.Fatalf("run error = %v, want missing -log error", err)
	}
}

func TestRunReadsLogAndWritesSummary(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "command.log")
	if err := os.WriteFile(logPath, []byte(strings.Join([]string{
		"=== RUN   TestWorkerRoutes",
		"--- FAIL: TestWorkerRoutes (0.02s)",
		"FAIL",
		"FAIL\tgithub.com/portpowered/infinite-you/pkg/workers\t0.220s",
		"run go test coverage lane: exit status 1",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("write fixture log: %v", err)
	}

	var output bytes.Buffer
	err := run(config{
		logPath:      logPath,
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}, &output)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "- Test: `TestWorkerRoutes`") {
		t.Fatalf("run() output = %q, want failing test", got)
	}
	if !strings.Contains(got, "- Package: `github.com/portpowered/infinite-you/pkg/workers`") {
		t.Fatalf("run() output = %q, want failing package", got)
	}
}

func TestRunWritesSummaryFromLogFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "command.log")
	logBody := strings.Join([]string{
		"=== RUN   TestFactoryServiceStarts",
		"    factory_service_test.go:42: expected startup work to be queued",
		"--- FAIL: TestFactoryServiceStarts (0.01s)",
		"FAIL",
		"FAIL\tgithub.com/portpowered/infinite-you/pkg/service\t0.128s",
		"run go test coverage lane: exit status 1",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(logBody), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var output bytes.Buffer
	err := run(config{
		logPath:      logPath,
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}, &output)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got := output.String()
	for _, want := range []string{
		"- Failure type: `go test failure`",
		"- Package: `github.com/portpowered/infinite-you/pkg/service`",
		"- Test: `TestFactoryServiceStarts`",
		"#### First actionable failure excerpt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary output = %q, want %q", got, want)
		}
	}
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}
