package contractguard

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const functionalLongBuildTag = "//go:build functionallong"

var approvedLegacyFunctionalHelperFiles = map[string]struct{}{
	"agent_config_helpers_test.go":           {},
	"assertion_helpers_test.go":              {},
	"current_factory_compat_helpers_test.go": {},
	"event_history_dispatch_helpers_test.go": {},
	"generated_api_helpers_test.go":          {},
	"pipeline_helpers_test.go":               {},
	"provider_error_corpus_helpers_test.go":  {},
	"provider_harness_helpers_test.go":       {},
	"replay_compat_helpers_test.go":          {},
	"testhelpers_test.go":                    {},
	"token_identity_helpers_test.go":         {},
	"work_request_helpers_test.go":           {},
	"worker_config_compat_helpers_test.go":   {},
}

type functionalLayoutViolation struct {
	file   string
	kind   string
	detail string
}

func TestFunctionalLayoutContractGuard_RepositoryKeepsLongLaneAndLegacyHelpersBounded(t *testing.T) {
	t.Parallel()

	violations, err := scanFunctionalLayoutContractViolations(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("scan functional layout contract: %v", err)
	}
	if len(violations) == 0 {
		return
	}

	var details []string
	for _, violation := range violations {
		details = append(details, violation.file+": "+violation.kind+" ("+violation.detail+")")
	}
	t.Fatalf(
		"functional layout contract regression:\n%s\nKeep slow functional files behind %s under tests/functional/... and move new shared helpers into tests/functional/internal/support instead of growing tests/functional_test helper shims.",
		strings.Join(details, "\n"),
		functionalLongBuildTag,
	)
}

func TestFunctionalLayoutContractGuard_DetectsLongTestMissingBuildTag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFunctionalLayoutGuardFixture(t, root, "tests/functional/providers/cli_provider_error_long_test.go", "package providers\n")

	assertFunctionalLayoutViolationKinds(
		t,
		root,
		[]string{"tests/functional/providers/cli_provider_error_long_test.go:long test missing functionallong build tag"},
	)
}

func TestFunctionalLayoutContractGuard_DetectsFunctionallongTagOutsideDecomposedTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFunctionalLayoutGuardFixture(t, root, "tests/functional_test/provider_error_long_test.go", functionalLongBuildTag+"\n\npackage functional_test\n")

	assertFunctionalLayoutViolationKinds(
		t,
		root,
		[]string{"tests/functional_test/provider_error_long_test.go:functionallong file outside tests/functional"},
	)
}

func TestFunctionalLayoutContractGuard_DetectsNewLegacyHelperShim(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFunctionalLayoutGuardFixture(t, root, "tests/functional_test/new_cross_package_helpers_test.go", "package functional_test\n")

	assertFunctionalLayoutViolationKinds(
		t,
		root,
		[]string{"tests/functional_test/new_cross_package_helpers_test.go:unapproved legacy helper shim"},
	)
}

func TestFunctionalLayoutContractGuard_SkipsHiddenDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFunctionalLayoutGuardFixture(t, root, ".claude/tests/functional_test/new_cross_package_helpers_test.go", "package functional_test\n")
	writeFunctionalLayoutGuardFixture(t, root, "tests/functional_test/new_cross_package_helpers_test.go", "package functional_test\n")

	assertFunctionalLayoutViolationKinds(
		t,
		root,
		[]string{"tests/functional_test/new_cross_package_helpers_test.go:unapproved legacy helper shim"},
	)
}

func scanFunctionalLayoutContractViolations(root string) ([]functionalLayoutViolation, error) {
	root = filepath.Clean(root)

	var violations []functionalLayoutViolation
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if ShouldSkipDir(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fileViolations, err := scanFunctionalLayoutFile(path, filepath.ToSlash(filepath.Clean(rel)))
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file == violations[j].file {
			if violations[i].kind == violations[j].kind {
				return violations[i].detail < violations[j].detail
			}
			return violations[i].kind < violations[j].kind
		}
		return violations[i].file < violations[j].file
	})
	return violations, nil
}

func scanFunctionalLayoutFile(path string, rel string) ([]functionalLayoutViolation, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var violations []functionalLayoutViolation
	name := filepath.Base(rel)
	hasLongTag := hasFunctionalLongBuildTag(string(contents))
	inDecomposedTree := strings.HasPrefix(rel, "tests/functional/")
	isLongTestFile := strings.HasSuffix(name, "_long_test.go")

	if isLongTestFile && !hasLongTag {
		violations = append(violations, functionalLayoutViolation{
			file:   rel,
			kind:   "long test missing functionallong build tag",
			detail: "files suffixed _long_test.go must be excluded from make test-functional by the explicit long-lane build tag",
		})
	}
	if hasLongTag && !isLongTestFile {
		violations = append(violations, functionalLayoutViolation{
			file:   rel,
			kind:   "functionallong tag on non-long filename",
			detail: "rename the file to *_long_test.go so slow-lane ownership is obvious during review",
		})
	}
	if (hasLongTag || isLongTestFile) && !inDecomposedTree {
		violations = append(violations, functionalLayoutViolation{
			file:   rel,
			kind:   "functionallong file outside tests/functional",
			detail: "slow functional coverage must stay in the behavior package it validates under tests/functional/...",
		})
	}
	if isLegacyFunctionalHelperShim(rel, name) {
		if _, ok := approvedLegacyFunctionalHelperFiles[name]; !ok {
			violations = append(violations, functionalLayoutViolation{
				file:   rel,
				kind:   "unapproved legacy helper shim",
				detail: "new cross-package helpers belong in tests/functional/internal/support instead of tests/functional_test",
			})
		}
	}

	return violations, nil
}

func hasFunctionalLongBuildTag(contents string) bool {
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == functionalLongBuildTag {
			return true
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		return false
	}
	return false
}

func isLegacyFunctionalHelperShim(rel string, name string) bool {
	if !strings.HasPrefix(rel, "tests/functional_test/") {
		return false
	}
	return strings.Contains(name, "helper") || strings.Contains(name, "_compat_")
}

func writeFunctionalLayoutGuardFixture(t *testing.T, root, relativePath, contents string) {
	t.Helper()

	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFunctionalLayoutViolationKinds(t *testing.T, root string, want []string) {
	t.Helper()

	violations, err := scanFunctionalLayoutContractViolations(root)
	if err != nil {
		t.Fatalf("scan functional layout contract: %v", err)
	}

	got := make([]string, 0, len(violations))
	for _, violation := range violations {
		got = append(got, violation.file+":"+violation.kind)
	}
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("violation count = %d, want %d\nviolations = %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("violations = %v, want %v", got, want)
		}
	}
}
