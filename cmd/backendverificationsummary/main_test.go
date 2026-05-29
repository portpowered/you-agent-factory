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
