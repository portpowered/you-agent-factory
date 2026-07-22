package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/exemptionbudget"
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
	writeGoFile(t, repoRoot, "pkg/transports/http/generated/server.gen.go", strings.Join([]string{
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

func TestRunHonorsExplicitFileAndFunctionIgnoreDirectives(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/api/ignored_file.go", strings.Join([]string{
		"// backendsizecheck:ignore-file legacy integration surface stays together until dedicated refactor work lands.",
		"package api",
		"",
		"func IgnoredFile() {",
		"\tprintln(\"line1\")",
		"\tprintln(\"line2\")",
		"\tprintln(\"line3\")",
		"\tprintln(\"line4\")",
		"\tprintln(\"line5\")",
		"}",
	}, "\n"))
	writeGoFile(t, repoRoot, "pkg/service/ignored_function.go", strings.Join([]string{
		"package service",
		"",
		"// backendsizecheck:ignore-function this long integration test remains inline until the legacy service builder is split.",
		"func IgnoredFunction() {",
		"\tprintln(\"line1\")",
		"\tprintln(\"line2\")",
		"\tprintln(\"line3\")",
		"\tprintln(\"line4\")",
		"\tprintln(\"line5\")",
		"}",
		"",
		"func Reported() {",
		"\tprintln(\"line1\")",
		"\tprintln(\"line2\")",
		"\tprintln(\"line3\")",
		"\tprintln(\"line4\")",
		"\tprintln(\"line5\")",
		"}",
	}, "\n"))
	writeExemptionBaseline(t, repoRoot,
		exemptionbudget.Entry{Rule: exemptionbudget.RuleBackendFile, Target: "pkg/api/ignored_file.go", Owner: "api-maintainers", RemovalReason: "Split the legacy integration surface by responsibility."},
		exemptionbudget.Entry{Rule: exemptionbudget.RuleBackendFunction, Target: "pkg/service/ignored_function.go#IgnoredFunction", Owner: "service-maintainers", RemovalReason: "Extract the integration harness from this function."},
	)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:          repoRoot,
		fileLineLimit: 20,
		funcLineLimit: 5,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want one remaining function violation")
	}

	errOutput := stderr.String()
	if strings.Contains(errOutput, "ignored_file.go") {
		t.Fatalf("run() stderr = %q, want file directive to skip ignored_file.go", errOutput)
	}
	if strings.Contains(errOutput, "IgnoredFunction") {
		t.Fatalf("run() stderr = %q, want function directive to skip IgnoredFunction", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | function Reported in pkg/service/ignored_function.go has 7 lines (limit 5)") {
		t.Fatalf("run() stderr = %q, want remaining function violation", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:backend-size] found 1 size violation(s)" {
		t.Fatalf("run() error = %q, want one violation count", got)
	}
}

func TestRunRejectsAllUnregisteredDirectivesInDeterministicOrder(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/zeta/z.go", "// backendsizecheck:ignore-file split this file.\npackage zeta\n")
	writeGoFile(t, repoRoot, "cmd/alpha/main.go", strings.Join([]string{
		"package main",
		"",
		"// backendsizecheck:ignore-function extract this function.",
		"func main() {}",
	}, "\n"))

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, fileLineLimit: 100, funcLineLimit: 100}, &bytes.Buffer{}, stderr)
	if err == nil || err.Error() != "[agent-factory:backend-size] found 2 exemption budget violation(s)" {
		t.Fatalf("run() error = %v, want two budget violations", err)
	}
	want := strings.Join([]string{
		"exemption budget rule=backendsizecheck:ignore-file target=pkg/zeta/z.go is unregistered; add a sorted docs/internal/baselines/backend-exemption-budget.json entry with a non-empty owner and actionable removalReason",
		"exemption budget rule=backendsizecheck:ignore-function target=cmd/alpha/main.go#main is unregistered; add a sorted docs/internal/baselines/backend-exemption-budget.json entry with a non-empty owner and actionable removalReason",
		"",
	}, "\n")
	if got := stderr.String(); got != want {
		t.Fatalf("run() stderr = %q, want deterministic diagnostics %q", got, want)
	}
}

func TestRunDoesNotSuppressPunctuationAdjacentDirectiveLookalikes(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/lookalike.go", strings.Join([]string{
		"// backendsizecheck:ignore-file: punctuation is not part of the directive grammar.",
		"package service",
		"",
		"// backendsizecheck:ignore-function: punctuation is not part of the directive grammar.",
		"func Oversized() {",
		"\tprintln(\"one\")",
		"\tprintln(\"two\")",
		"}",
	}, "\n"))

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, fileLineLimit: 5, funcLineLimit: 3}, &bytes.Buffer{}, stderr)
	if err == nil || err.Error() != "[agent-factory:backend-size] found 2 size violation(s)" {
		t.Fatalf("run() error = %v, want file and function violations", err)
	}
	for _, want := range []string{"file pkg/service/lookalike.go", "function Oversized in pkg/service/lookalike.go"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsInvalidRegistrationMetadata(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/ignored.go", "// backendsizecheck:ignore-file split this file.\npackage service\n")
	writeExemptionBaseline(t, repoRoot, exemptionbudget.Entry{
		Rule: exemptionbudget.RuleBackendFile, Target: "pkg/service/ignored.go", RemovalReason: "Split the file by responsibility.",
	})

	err := run(config{root: repoRoot, fileLineLimit: 100, funcLineLimit: 100}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "rule=backendsizecheck:ignore-file") ||
		!strings.Contains(err.Error(), "target=pkg/service/ignored.go") || !strings.Contains(err.Error(), "set a non-empty owner") {
		t.Fatalf("run() error = %v, want rule, target, and owner remediation", err)
	}
}

func TestRunRejectsNonActionableRemovalReason(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/ignored.go", "// backendsizecheck:ignore-file split this file.\npackage service\n")
	writeExemptionBaseline(t, repoRoot, exemptionbudget.Entry{
		Rule: exemptionbudget.RuleBackendFile, Target: "pkg/service/ignored.go", Owner: "service-maintainers", RemovalReason: "x",
	})

	err := run(config{root: repoRoot, fileLineLimit: 100, funcLineLimit: 100}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "rule=backendsizecheck:ignore-file") ||
		!strings.Contains(err.Error(), "target=pkg/service/ignored.go") || !strings.Contains(err.Error(), "non-actionable removalReason") {
		t.Fatalf("run() error = %v, want rule, target, and actionable-removal remediation", err)
	}
}

func TestMakeBackendSizeAllowsRemovingDirectiveWithBaselineEntry(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	writeGoFile(t, fixtureRoot, "pkg/service/legacy.go", "// backendsizecheck:ignore-file split this file by responsibility.\npackage service\n")
	writeExemptionBaseline(t, fixtureRoot, exemptionbudget.Entry{
		Rule:          exemptionbudget.RuleBackendFile,
		Target:        "pkg/service/legacy.go",
		Owner:         "service-maintainers",
		RemovalReason: "Split the file by responsibility.",
	})

	assertMakeBackendSizePasses(t, repoRoot, fixtureRoot)

	writeGoFile(t, fixtureRoot, "pkg/service/legacy.go", "package service\n")
	writeExemptionBaseline(t, fixtureRoot)
	assertMakeBackendSizePasses(t, repoRoot, fixtureRoot)
}

func assertMakeBackendSizePasses(t *testing.T, repoRoot, fixtureRoot string) {
	t.Helper()
	cmd := exec.Command("make", "backend-size", "BACKEND_SIZE_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make backend-size error = %v; output:\n%s", err, output)
	}
	if got := string(output); !strings.Contains(got, "[agent-factory:backend-size] all owned Go backend files are within") {
		t.Fatalf("make backend-size output = %q, want success diagnostic", got)
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
	baselinePath := filepath.Join(repoRoot, exemptionbudget.BaselinePath)
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		writeExemptionBaseline(t, repoRoot)
	}
}

func writeExemptionBaseline(t *testing.T, repoRoot string, entries ...exemptionbudget.Entry) {
	t.Helper()
	data, err := json.Marshal(exemptionbudget.Baseline{Version: exemptionbudget.Version, Entries: entries})
	if err != nil {
		t.Fatalf("marshal exemption baseline: %v", err)
	}
	baselinePath := filepath.Join(repoRoot, exemptionbudget.BaselinePath)
	if err := os.MkdirAll(filepath.Dir(baselinePath), 0o755); err != nil {
		t.Fatalf("create exemption baseline directory: %v", err)
	}
	if err := os.WriteFile(baselinePath, data, 0o644); err != nil {
		t.Fatalf("write exemption baseline: %v", err)
	}
}
