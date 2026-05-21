package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSucceedsWhenOwnedFilesStayWithinLimitsAndExcludedRootsAreIgnored(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "cmd/factory/main.go", strings.Join([]string{
		"package main",
		"",
		"func main() {",
		"\tprintln(\"ok\")",
		"}",
	}, "\n"))
	writeGoFile(t, repoRoot, "pkg/api/generated/server.gen.go", strings.Join([]string{
		"package generated",
		"",
		"func Generated() {",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"}",
		"",
		"func GeneratedAgain() {}",
	}, "\n"))
	writeGoFile(t, repoRoot, "tests/functional/runtime_api/testdata/fixture.go", strings.Join([]string{
		"package testdata",
		"",
		"func Fixture() {",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"}",
	}, "\n"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:          repoRoot,
		fileLineLimit: 6,
		funcLineLimit: 4,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:backend-size] all owned Go backend files are within file limit 6 and function limit 4") {
		t.Fatalf("run() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunReportsFileAndFunctionViolationsWithPackageFileAndLimitDetails(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/oversized.go", strings.Join([]string{
		"package service",
		"",
		"func helper() string {",
		"\tvalue := \"line1\"",
		"\tvalue += \"line2\"",
		"\tvalue += \"line3\"",
		"\tvalue += \"line4\"",
		"\tvalue += \"line5\"",
		"\treturn value",
		"}",
		"",
		"func small() {}",
	}, "\n"))
	writeGoFile(t, repoRoot, "vendor/example/ignored.go", strings.Join([]string{
		"package example",
		"",
		"func Ignored() {",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"\tprintln(\"ignored\")",
		"}",
	}, "\n"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:          repoRoot,
		fileLineLimit: 10,
		funcLineLimit: 5,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want size violations")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "pkg/service | file pkg/service/oversized.go has 12 lines (limit 10)") {
		t.Fatalf("run() stderr = %q, want file violation details", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | function helper in pkg/service/oversized.go has 8 lines (limit 5)") {
		t.Fatalf("run() stderr = %q, want function violation details", errOutput)
	}
	if strings.Contains(errOutput, "vendor/example") {
		t.Fatalf("run() stderr = %q, want vendor paths excluded", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:backend-size] found 2 size violation(s)" {
		t.Fatalf("run() error = %q, want violation count", got)
	}
}

func TestRunRejectsNonPositiveLimits(t *testing.T) {
	t.Parallel()

	err := run(config{root: t.TempDir(), fileLineLimit: 0, funcLineLimit: 1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "file limit must be positive, got 0" {
		t.Fatalf("run() error = %v, want file-limit validation", err)
	}

	err = run(config{root: t.TempDir(), fileLineLimit: 1, funcLineLimit: -1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "function limit must be positive, got -1" {
		t.Fatalf("run() error = %v, want function-limit validation", err)
	}
}

func writeGoFile(t *testing.T, repoRoot string, relativePath string, content string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}
