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

func TestRunSucceedsWhenOwnedPkgFilesStayWithinLimitsAndGeneratedRootsAreIgnored(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/service.go", strings.Join([]string{
		"package service",
		"",
		"func Service() {",
		"\tprintln(\"ok\")",
		"}",
	}, "\n"))
	writeGoFile(t, repoRoot, "pkg/transports/http/generated/server.gen.go", strings.Join([]string{
		"package generated",
		"",
		"func Generated() {",
		"\tif true {",
		"\t\tprintln(\"ignored\")",
		"\t}",
		"\tif true {",
		"\t\tprintln(\"ignored\")",
		"\t}",
		"\tif true {",
		"\t\tprintln(\"ignored\")",
		"\t}",
		"}",
	}, "\n"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:              repoRoot,
		fileLineLimit:     8,
		functionLineLimit: 4,
		cyclomaticLimit:   3,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:pkg-maint] pkg maintainability passed") {
		t.Fatalf("run() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunReportsFileFunctionAndComplexityViolationsWithClearFields(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/violations.go", strings.Join([]string{
		"package service",
		"",
		"func Oversized(value int) int {",
		"\tif value > 0 && value < 10 {",
		"\t\tvalue++",
		"\t}",
		"\tfor i := 0; i < 2; i++ {",
		"\t\tif i%2 == 0 {",
		"\t\t\tvalue += i",
		"\t\t}",
		"\t}",
		"\tswitch value {",
		"\tcase 1:",
		"\t\tvalue++",
		"\tcase 2:",
		"\t\tvalue += 2",
		"\tdefault:",
		"\t\tvalue += 3",
		"\t}",
		"\treturn value",
		"}",
	}, "\n"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:              repoRoot,
		fileLineLimit:     10,
		functionLineLimit: 8,
		cyclomaticLimit:   7,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want maintainability violations")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "pkg/service | rule=file-lines target=pkg/service/violations.go actual=21 limit=10") {
		t.Fatalf("run() stderr = %q, want file-line violation details", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | rule=function-lines target=Oversized file=pkg/service/violations.go actual=19 limit=8") {
		t.Fatalf("run() stderr = %q, want function-line violation details", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | rule=cyclomatic-complexity target=Oversized file=pkg/service/violations.go actual=8 limit=7") {
		t.Fatalf("run() stderr = %q, want complexity violation details", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-maint] found 3 maintainability violation(s)" {
		t.Fatalf("run() error = %q, want violation count", got)
	}
}

func TestRunHonorsRuleScopedIgnoreDirectives(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/ignored.go", strings.Join([]string{
		"// pkgmaintcheck:ignore-file-lines legacy transport surface remains together until the dedicated split story lands.",
		"package service",
		"",
		"// pkgmaintcheck:ignore-function-lines legacy scenario coverage stays inline until the harness is extracted.",
		"// pkgmaintcheck:ignore-cyclomatic-complexity legacy scenario coverage stays inline until the harness is extracted.",
		"func IgnoreLines(value int) int {",
		"\tif value > 0 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 1 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 2 {",
		"\t\tvalue++",
		"\t}",
		"\treturn value",
		"}",
		"",
		"// pkgmaintcheck:ignore-function-lines boundary normalization keeps this branchy helper together for now.",
		"// pkgmaintcheck:ignore-cyclomatic-complexity boundary normalization keeps this branchy helper together for now.",
		"func IgnoreComplexity(value int) int {",
		"\tif value > 0 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 1 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 2 {",
		"\t\tvalue++",
		"\t}",
		"\treturn value",
		"}",
		"",
		"func Reported(value int) int {",
		"\tif value > 0 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 1 {",
		"\t\tvalue++",
		"\t}",
		"\tif value > 2 {",
		"\t\tvalue++",
		"\t}",
		"\treturn value",
		"}",
	}, "\n"))
	writeExemptionBaseline(t, repoRoot,
		exemptionbudget.Entry{Rule: exemptionbudget.RulePackageComplexity, Target: "pkg/service/ignored.go#IgnoreComplexity", Owner: "service-maintainers", RemovalReason: "Replace branches with typed handlers."},
		exemptionbudget.Entry{Rule: exemptionbudget.RulePackageComplexity, Target: "pkg/service/ignored.go#IgnoreLines", Owner: "service-maintainers", RemovalReason: "Replace branches with typed handlers."},
		exemptionbudget.Entry{Rule: exemptionbudget.RulePackageFileLines, Target: "pkg/service/ignored.go", Owner: "service-maintainers", RemovalReason: "Split the file by responsibility."},
		exemptionbudget.Entry{Rule: exemptionbudget.RulePackageFuncLines, Target: "pkg/service/ignored.go#IgnoreComplexity", Owner: "service-maintainers", RemovalReason: "Extract the processing stages."},
		exemptionbudget.Entry{Rule: exemptionbudget.RulePackageFuncLines, Target: "pkg/service/ignored.go#IgnoreLines", Owner: "service-maintainers", RemovalReason: "Extract the processing stages."},
	)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:              repoRoot,
		fileLineLimit:     10,
		functionLineLimit: 7,
		cyclomaticLimit:   3,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want one remaining function-line and complexity violation")
	}

	errOutput := stderr.String()
	if strings.Contains(errOutput, "rule=file-lines") {
		t.Fatalf("run() stderr = %q, want file-line directive to suppress file violation", errOutput)
	}
	if strings.Contains(errOutput, "target=IgnoreLines file=") {
		t.Fatalf("run() stderr = %q, want function-line directive to suppress IgnoreLines", errOutput)
	}
	if strings.Contains(errOutput, "target=IgnoreComplexity file=") {
		t.Fatalf("run() stderr = %q, want complexity directive to suppress IgnoreComplexity", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | rule=function-lines target=Reported file=pkg/service/ignored.go actual=12 limit=7") {
		t.Fatalf("run() stderr = %q, want remaining function-line violation", errOutput)
	}
	if !strings.Contains(errOutput, "pkg/service | rule=cyclomatic-complexity target=Reported file=pkg/service/ignored.go actual=4 limit=3") {
		t.Fatalf("run() stderr = %q, want remaining complexity violation", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-maint] found 2 maintainability violation(s)" {
		t.Fatalf("run() error = %q, want two violation count", got)
	}
}

func TestRunDoesNotSuppressPunctuationAdjacentDirectiveLookalikes(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/lookalike.go", strings.Join([]string{
		"// pkgmaintcheck:ignore-file-lines: punctuation is not part of the directive grammar.",
		"package service",
		"",
		"// pkgmaintcheck:ignore-function-lines: punctuation is not part of the directive grammar.",
		"// pkgmaintcheck:ignore-cyclomatic-complexity: punctuation is not part of the directive grammar.",
		"func Branchy(value int) int {",
		"\tif value > 0 {",
		"\t\treturn 1",
		"\t}",
		"\treturn 0",
		"}",
	}, "\n"))

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, fileLineLimit: 8, functionLineLimit: 4, cyclomaticLimit: 1}, &bytes.Buffer{}, stderr)
	if err == nil || err.Error() != "[agent-factory:pkg-maint] found 3 maintainability violation(s)" {
		t.Fatalf("run() error = %v, want all three maintainability violations", err)
	}
	for _, want := range []string{"rule=file-lines", "rule=function-lines", "rule=cyclomatic-complexity"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsStalePackageRegistration(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/service.go", "package service\n\nfunc Service() {}\n")
	writeExemptionBaseline(t, repoRoot, exemptionbudget.Entry{
		Rule: exemptionbudget.RulePackageFuncLines, Target: "pkg/service/service.go#Removed", Owner: "service-maintainers", RemovalReason: "Extract the processing stages.",
	})

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, fileLineLimit: 100, functionLineLimit: 100, cyclomaticLimit: 100}, &bytes.Buffer{}, stderr)
	if err == nil || err.Error() != "[agent-factory:pkg-maint] found 1 exemption budget violation(s)" {
		t.Fatalf("run() error = %v, want stale budget violation", err)
	}
	if got := stderr.String(); !strings.Contains(got, "rule=pkgmaintcheck:ignore-function-lines target=pkg/service/service.go#Removed is stale") ||
		!strings.Contains(got, "remove its docs/internal/baselines/backend-exemption-budget.json entry or restore the directive") {
		t.Fatalf("run() stderr = %q, want stale registration remediation", got)
	}
}

func TestMakePkgMaintRejectsStaleEntryThenAllowsCompleteRemoval(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	writeGoFile(t, fixtureRoot, "pkg/service/legacy.go", "package service\n")
	writeExemptionBaseline(t, fixtureRoot, exemptionbudget.Entry{
		Rule:          exemptionbudget.RulePackageFileLines,
		Target:        "pkg/service/legacy.go",
		Owner:         "service-maintainers",
		RemovalReason: "Split the file by responsibility.",
	})

	cmd := exec.Command("make", "pkg-maint", "PACKAGE_MAINT_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make pkg-maint succeeded with stale baseline entry; output:\n%s", output)
	}
	got := string(output)
	if !strings.Contains(got, "rule=pkgmaintcheck:ignore-file-lines target=pkg/service/legacy.go is stale") ||
		!strings.Contains(got, "remove its docs/internal/baselines/backend-exemption-budget.json entry or restore the directive") {
		t.Fatalf("make pkg-maint output = %q, want stale-entry remediation", got)
	}

	writeExemptionBaseline(t, fixtureRoot)
	cmd = exec.Command("make", "pkg-maint", "PACKAGE_MAINT_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make pkg-maint after baseline reduction error = %v; output:\n%s", err, output)
	}
	if got := string(output); !strings.Contains(got, "[agent-factory:pkg-maint] pkg maintainability passed") {
		t.Fatalf("make pkg-maint output = %q, want success after complete removal", got)
	}
}

func TestMakeLintRunsBothExemptionBudgetChecks(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	writeGoFile(t, fixtureRoot, "pkg/service/service.go", "package service\n")

	cmd := exec.Command(
		"make", "lint", "LINT_TARGETS=backend-size pkg-maint",
		"BACKEND_SIZE_ROOT="+fixtureRoot, "PACKAGE_MAINT_ROOT="+fixtureRoot,
	)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make lint focused budget path error = %v; output:\n%s", err, output)
	}
	got := string(output)
	for _, want := range []string{
		"[agent-factory:backend-size] all owned Go backend files are within",
		"[agent-factory:pkg-maint] pkg maintainability passed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("make lint output = %q, want %q", got, want)
		}
	}
}

func TestRunRequiresFileLineDirectiveAtFileScope(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoFile(t, repoRoot, "pkg/service/file_scope.go", strings.Join([]string{
		"package service",
		"",
		"// pkgmaintcheck:ignore-file-lines this comment is attached to the function, not the file.",
		"func HiddenFileWaiver() {",
		"\tprintln(\"line 1\")",
		"\tprintln(\"line 2\")",
		"}",
	}, "\n"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{
		root:              repoRoot,
		fileLineLimit:     4,
		functionLineLimit: 20,
		cyclomaticLimit:   20,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want file-line violation when directive is function-scoped")
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "pkg/service | rule=file-lines target=pkg/service/file_scope.go actual=7 limit=4") {
		t.Fatalf("run() stderr = %q, want file-line violation details", errOutput)
	}
	if strings.Contains(errOutput, "rule=function-lines") || strings.Contains(errOutput, "rule=cyclomatic-complexity") {
		t.Fatalf("run() stderr = %q, want only file-line violation", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-maint] found 1 maintainability violation(s)" {
		t.Fatalf("run() error = %q, want one violation count", got)
	}
}

func TestRunRejectsNonPositiveLimits(t *testing.T) {
	t.Parallel()

	err := run(config{root: t.TempDir(), fileLineLimit: 0, functionLineLimit: 1, cyclomaticLimit: 1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "file limit must be positive, got 0" {
		t.Fatalf("run() error = %v, want file-limit validation", err)
	}

	err = run(config{root: t.TempDir(), fileLineLimit: 1, functionLineLimit: -1, cyclomaticLimit: 1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "function limit must be positive, got -1" {
		t.Fatalf("run() error = %v, want function-limit validation", err)
	}

	err = run(config{root: t.TempDir(), fileLineLimit: 1, functionLineLimit: 1, cyclomaticLimit: 0}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "cyclomatic limit must be positive, got 0" {
		t.Fatalf("run() error = %v, want cyclomatic-limit validation", err)
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
