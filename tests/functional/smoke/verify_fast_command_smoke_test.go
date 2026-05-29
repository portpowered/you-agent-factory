package smoke

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":             "@printf '%s\\n' 'stub:typecheck'\n",
		"ui-test":               "@printf '%s\\n' 'stub:ui-test'\n",
		"test":                  "@printf '%s\\n' 'stub:test'\n",
		"ui-install-playwright": "@printf '%s\\n' 'unexpected:ui-install-playwright'\n\t@exit 99\n",
		"ui-integration-test":   "@printf '%s\\n' 'unexpected:ui-integration-test'\n\t@exit 99\n",
		"test-functional-long":  "@printf '%s\\n' 'unexpected:test-functional-long'\n\t@exit 99\n",
		"long-tests":            "@printf '%s\\n' 'unexpected:long-tests'\n\t@exit 99\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-fast")
	if err != nil {
		t.Fatalf("run verify-fast wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running fast verification tier: typecheck + short UI/unit suite + short Go suite",
		"==> dashboard typecheck [make typecheck]",
		"stub:typecheck",
		"==> short UI/unit suite [make ui-test]",
		"stub:ui-test",
		"==> short Go suite [make test]",
		"stub:test",
	)

	for _, unwanted := range []string{
		"unexpected:ui-install-playwright",
		"unexpected:ui-integration-test",
		"unexpected:test-functional-long",
		"unexpected:long-tests",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-fast unexpectedly ran %q:\n%s", unwanted, output)
		}
	}
}

func TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck": "@printf '%s\\n' 'stub:typecheck'\n",
		"ui-test":   "@printf '%s\\n' 'stub:ui-test'\n\t@exit 17\n",
		"test":      "@printf '%s\\n' 'stub:test'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-fast")
	if err == nil {
		t.Fatalf("verify-fast unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: short UI/unit suite [make ui-test] failed. Rerun with: make ui-test") {
		t.Fatalf("verify-fast failure output missing rerun hint:\n%s", output)
	}
	if strings.Contains(output, "stub:test") {
		t.Fatalf("verify-fast continued after the failing owned suite:\n%s", output)
	}
}

func TestVerifyPRCommandSmoke_UsesRequiredLanesOnce(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":    "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-ui-coverage":          "@printf '%s\\n' 'stub:test-ui-coverage'\n",
		"ui-integration-test":       "@printf '%s\\n' 'stub:ui-integration-test'\n",
		"test-backend-verification": "@printf '%s\\n' 'stub:test-backend-verification'\n",
		"verify":                    "@printf '%s\\n' 'unexpected:verify'\n\t@exit 99\n",
		"test-backend-functional":   "@printf '%s\\n' 'unexpected:test-backend-functional'\n\t@exit 99\n",
		"ui-test":                   "@printf '%s\\n' 'unexpected:ui-test'\n\t@exit 99\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-pr")
	if err != nil {
		t.Fatalf("run verify-pr wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running pull-request verification tier: build contracts + required CI-equivalent test lanes",
		"==> build contracts and static verification [make verify-build-contracts]",
		"stub:verify-build-contracts",
		"==> required CI-equivalent test lanes [make verify-tests]",
		"Running required CI-equivalent test lanes: UI coverage + browser integration + backend verification",
		"==> UI Coverage lane [make test-ui-coverage]",
		"stub:test-ui-coverage",
		"==> UI Browser Integration lane [make ui-integration-test]",
		"stub:ui-integration-test",
		"==> Backend Verification lane [make test-backend-verification]",
		"stub:test-backend-verification",
	)

	for _, expected := range []string{
		"stub:verify-build-contracts",
		"stub:test-ui-coverage",
		"stub:ui-integration-test",
		"stub:test-backend-verification",
	} {
		if count := strings.Count(output, expected); count != 1 {
			t.Fatalf("expected %q exactly once, found %d:\n%s", expected, count, output)
		}
	}

	for _, unwanted := range []string{
		"unexpected:verify",
		"unexpected:test-backend-functional",
		"unexpected:ui-test",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-pr unexpectedly ran %q:\n%s", unwanted, output)
		}
	}
}

func TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":    "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-ui-coverage":          "@printf '%s\\n' 'stub:test-ui-coverage'\n",
		"ui-integration-test":       "@printf '%s\\n' 'stub:ui-integration-test'\n\t@exit 23\n",
		"test-backend-verification": "@printf '%s\\n' 'stub:test-backend-verification'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-pr")
	if err == nil {
		t.Fatalf("verify-pr unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: UI Browser Integration lane [make ui-integration-test] failed. Rerun with: make ui-integration-test") {
		t.Fatalf("verify-pr failure output missing exact lane rerun hint:\n%s", output)
	}
	if strings.Contains(output, "stub:test-backend-verification") {
		t.Fatalf("verify-pr continued after the failing required lane:\n%s", output)
	}
}

func TestBackendVerificationCompatibilityAliasesSmoke_RedirectToCanonicalLane(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"test-backend-verification": "@printf '%s\\n' 'stub:test-backend-verification'\n",
		"test-coverage-go":          "@printf '%s\\n' 'unexpected:test-coverage-go'\n\t@exit 99\n",
	})

	coverageOutput, err := runMakefileTarget(repoRoot, makefilePath, "test-backend-coverage")
	if err != nil {
		t.Fatalf("run test-backend-coverage wrapper: %v\n%s", err, coverageOutput)
	}
	if count := strings.Count(coverageOutput, "stub:test-backend-verification"); count != 1 {
		t.Fatalf("test-backend-coverage should delegate to the canonical backend lane exactly once, found %d:\n%s", count, coverageOutput)
	}
	if strings.Contains(coverageOutput, "unexpected:test-coverage-go") {
		t.Fatalf("test-backend-coverage bypassed the canonical backend lane:\n%s", coverageOutput)
	}

	functionalOutput, err := runMakefileTarget(repoRoot, makefilePath, "test-backend-functional")
	if err != nil {
		t.Fatalf("run test-backend-functional wrapper: %v\n%s", err, functionalOutput)
	}
	assertOutputOrder(t, functionalOutput,
		"Backend functional verification is merged into make test-backend-verification; rerun that target for the required PR lane.",
		"stub:test-backend-verification",
	)
	if count := strings.Count(functionalOutput, "stub:test-backend-verification"); count != 1 {
		t.Fatalf("test-backend-functional should delegate to the canonical backend lane exactly once, found %d:\n%s", count, functionalOutput)
	}
	if strings.Contains(functionalOutput, "unexpected:test-coverage-go") {
		t.Fatalf("test-backend-functional bypassed the canonical backend lane:\n%s", functionalOutput)
	}
}

func TestUICoverageCommandSmoke_RunsPackageCoverageThenReplayCheck(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"ui-test-coverage":         "@printf '%s\\n' 'stub:ui-test-coverage'\n",
		"ui-replay-coverage-check": "@printf '%s\\n' 'stub:ui-replay-coverage-check'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "test-ui-coverage")
	if err != nil {
		t.Fatalf("run test-ui-coverage wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"stub:ui-test-coverage",
		"stub:ui-replay-coverage-check",
	)
}

func TestUIPackageCoverageCommandSmoke_InvokesPackageOwnedCoverageScript(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	scriptPath := writeMakeEchoScript(t, "ui-script")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, nil)

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"ui-test-coverage",
		fmt.Sprintf("UI_SCRIPT=%s", scriptPath),
	)
	if err != nil {
		t.Fatalf("run ui-test-coverage wrapper: %v\n%s", err, output)
	}

	if !strings.Contains(output, "ui-script:test:coverage") {
		t.Fatalf("ui-test-coverage did not invoke package-owned test:coverage script:\n%s", output)
	}
}

func TestVerifyCompatibilityAliasSmoke_RedirectsToCanonicalPRTier(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":    "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-ui-coverage":          "@printf '%s\\n' 'stub:test-ui-coverage'\n",
		"ui-integration-test":       "@printf '%s\\n' 'stub:ui-integration-test'\n",
		"test-backend-verification": "@printf '%s\\n' 'stub:test-backend-verification'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify")
	if err != nil {
		t.Fatalf("run verify wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"make verify is a compatibility alias for the canonical pull-request tier; prefer make verify-pr",
		"Running pull-request verification tier: build contracts + required CI-equivalent test lanes",
		"==> build contracts and static verification [make verify-build-contracts]",
		"stub:verify-build-contracts",
		"==> required CI-equivalent test lanes [make verify-tests]",
		"==> UI Coverage lane [make test-ui-coverage]",
		"stub:test-ui-coverage",
		"==> UI Browser Integration lane [make ui-integration-test]",
		"stub:ui-integration-test",
		"==> Backend Verification lane [make test-backend-verification]",
		"stub:test-backend-verification",
	)

	for _, expected := range []string{
		"stub:verify-build-contracts",
		"stub:test-ui-coverage",
		"stub:ui-integration-test",
		"stub:test-backend-verification",
	} {
		if count := strings.Count(output, expected); count != 1 {
			t.Fatalf("expected %q exactly once through the verify compatibility alias, found %d:\n%s", expected, count, output)
		}
	}
}

func TestBackendVerificationLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make", "#!/bin/sh\nprintf '%s\\n' \"fake-make:$*\"\n")
	artifactRoot := filepath.Join(t.TempDir(), "backend-verification-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-backend-verification.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err != nil {
		t.Fatalf("run backend verification script: %v\n%s", err, output)
	}
	if !strings.Contains(output, "fake-make:test-backend-verification") {
		t.Fatalf("backend verification script did not invoke canonical make target:\n%s", output)
	}

	logBody, err := os.ReadFile(filepath.Join(artifactRoot, "command.log"))
	if err != nil {
		t.Fatalf("read backend verification command log: %v", err)
	}
	if !strings.Contains(string(logBody), "fake-make:test-backend-verification") {
		t.Fatalf("backend verification command log missing canonical command output:\n%s", string(logBody))
	}
}

func TestBackendVerificationLaneScriptSmoke_PreservesFailureExitAndLog(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-fail", "#!/bin/sh\nprintf '%s\\n' \"fake-make:$*\"\nexit 27\n")
	artifactRoot := filepath.Join(t.TempDir(), "backend-verification-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-backend-verification.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err == nil {
		t.Fatalf("backend verification script unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 27 {
		t.Fatalf("backend verification script exit = %v, want exit code 27\n%s", err, output)
	}

	logBody, readErr := os.ReadFile(filepath.Join(artifactRoot, "command.log"))
	if readErr != nil {
		t.Fatalf("read backend verification command log: %v", readErr)
	}
	if !strings.Contains(string(logBody), "fake-make:test-backend-verification") {
		t.Fatalf("backend verification command log missing failure output:\n%s", string(logBody))
	}
}

func TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-pr":                     "@printf '%s\\n' 'stub:verify-pr'\n",
		"long-tests-managed-runtime":    "@printf '%s\\n' 'stub:long-tests-managed-runtime'\n",
		"long-tests-functional-runtime": "@printf '%s\\n' 'stub:long-tests-functional-runtime'\n",
		"test-functional-long":          "@printf '%s\\n' 'unexpected:test-functional-long'\n\t@exit 99\n",
		"test-backend-functional":       "@printf '%s\\n' 'unexpected:test-backend-functional'\n\t@exit 99\n",
		"ui-integration-test":           "@printf '%s\\n' 'unexpected:ui-integration-test'\n\t@exit 99\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-extended")
	if err != nil {
		t.Fatalf("run verify-extended wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running extended verification tier: required PR verification + opt-in long and specialty suites",
		"==> pull-request verification tier [make verify-pr]",
		"stub:verify-pr",
		"==> opt-in long and specialty suites [make long-tests]",
		"Running opt-in long and specialty suites: managed runtime coverage + real local inference coverage",
		"==> Managed Runtime specialty lane [make long-tests-managed-runtime]",
		"stub:long-tests-managed-runtime",
		"==> Real Local Inference specialty lane [make long-tests-functional-runtime]",
		"stub:long-tests-functional-runtime",
	)

	for _, unwanted := range []string{
		"unexpected:test-functional-long",
		"unexpected:test-backend-functional",
		"unexpected:ui-integration-test",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-extended unexpectedly ran %q:\n%s", unwanted, output)
		}
	}
}

func TestLongTestsCommandSmoke_FailureReportsExactSpecialtyLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"long-tests-managed-runtime":    "@printf '%s\\n' 'stub:long-tests-managed-runtime'\n",
		"long-tests-functional-runtime": "@printf '%s\\n' 'stub:long-tests-functional-runtime'\n\t@exit 29\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "long-tests")
	if err == nil {
		t.Fatalf("long-tests unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: Real Local Inference specialty lane [make long-tests-functional-runtime] failed. Rerun with: make long-tests-functional-runtime") {
		t.Fatalf("long-tests failure output missing exact specialty rerun hint:\n%s", output)
	}
}

func writeVerifyFastWrapperMakefile(t *testing.T, repoRoot string, overrides map[string]string) string {
	t.Helper()

	var body strings.Builder
	body.WriteString(fmt.Sprintf("include %s\n\n", filepath.Join(repoRoot, "Makefile")))
	for target, recipe := range overrides {
		body.WriteString(fmt.Sprintf("%s:\n", target))
		for _, line := range strings.Split(strings.TrimSuffix(recipe, "\n"), "\n") {
			body.WriteString("\t")
			body.WriteString(line)
			body.WriteString("\n")
		}
		body.WriteString("\n")
	}

	path := filepath.Join(t.TempDir(), "verify-fast-wrapper.mk")
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatalf("write wrapper makefile: %v", err)
	}
	return path
}

func runMakefileTarget(repoRoot, makefilePath, target string) (string, error) {
	return runMakefileTargetWithArgs(repoRoot, makefilePath, target)
}

func runMakefileTargetWithArgs(repoRoot, makefilePath, target string, args ...string) (string, error) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		return "", err
	}

	makeArgs := []string{
		"-f", makefilePath,
		fmt.Sprintf("MAKE=%s -f %s", makePath, makefilePath),
	}
	makeArgs = append(makeArgs, args...)
	makeArgs = append(makeArgs, target)

	cmd := exec.Command(makePath, makeArgs...)
	cmd.Dir = repoRoot

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	return output.String(), err
}

func writeMakeEchoScript(t *testing.T, label string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), label)
	body := fmt.Sprintf("#!/bin/sh\nprintf '%%s:' %q\nprintf '%%s\\n' \"$*\"\n", label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write echo script: %v", err)
	}
	return path
}

func writeExecutableScript(t *testing.T, label string, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable script: %v", err)
	}
	return path
}

func runScript(repoRoot string, scriptPath string, env ...string) (string, error) {
	cmd := exec.Command(scriptPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), env...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	return output.String(), err
}

func assertOutputOrder(t *testing.T, output string, markers ...string) {
	t.Helper()

	cursor := 0
	for _, marker := range markers {
		next := strings.Index(output[cursor:], marker)
		if next < 0 {
			t.Fatalf("output missing marker %q:\n%s", marker, output)
		}
		cursor += next + len(marker)
	}
}
