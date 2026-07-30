//go:build !windows

package verification

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites prove verify-fast invokes only short owned suites in order.
func TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":             "@printf '%s\\n' 'stub:typecheck'\n",
		"mcp-contract-check":    "@printf '%s\\n' 'stub:mcp-contract-check'\n",
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
		"Running fast verification tier: typecheck + MCP contract boundary + short UI/unit suite + short Go suite",
		"==> dashboard typecheck [make typecheck]",
		"stub:typecheck",
		"==> MCP contract boundary [make mcp-contract-check]",
		"stub:mcp-contract-check",
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

// TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand prove verify-fast failure output reports the owning suite rerun command.
func TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":          "@printf '%s\\n' 'stub:typecheck'\n",
		"mcp-contract-check": "@printf '%s\\n' 'stub:mcp-contract-check'\n",
		"ui-test":            "@printf '%s\\n' 'stub:ui-test'\n\t@exit 17\n",
		"test":               "@printf '%s\\n' 'stub:test'\n",
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

// TestVerifyFastCommandSmoke_ContractFailureStopsLaterSuites prove verify-fast stops after a contract-boundary failure without running later suites.
func TestVerifyFastCommandSmoke_ContractFailureStopsLaterSuites(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":          "@printf '%s\\n' 'stub:typecheck'\n",
		"mcp-contract-check": "@printf '%s\\n' 'stub:mcp-contract-check'\n\t@exit 23\n",
		"ui-test":            "@printf '%s\\n' 'stub:ui-test'\n",
		"test":               "@printf '%s\\n' 'stub:test'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-fast")
	if err == nil {
		t.Fatalf("verify-fast unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: MCP contract boundary [make mcp-contract-check] failed. Rerun with: make mcp-contract-check") {
		t.Fatalf("verify-fast failure output missing MCP rerun hint:\n%s", output)
	}
	for _, laterSuite := range []string{"stub:ui-test", "stub:test"} {
		if strings.Contains(output, laterSuite) {
			t.Fatalf("verify-fast continued to %q after the failing MCP contract check:\n%s", laterSuite, output)
		}
	}
}

// TestVerifyPRCommandSmoke_UsesRequiredLanesOnce prove verify-pr runs each required CI-equivalent lane exactly once.
func TestVerifyPRCommandSmoke_UsesRequiredLanesOnce(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-root-process-acceptance":         "@printf '%s\\n' 'stub:test-root-process-acceptance'\n",
		"run-concurrent-ui-verification-lanes": "@printf '%s\\n' 'stub:run-concurrent-ui-verification-lanes'\n",
		"test-ui-storybook-integration":        "@printf '%s\\n' 'stub:test-ui-storybook-integration'\n",
		"test-ui-durable-session-real-backend": "@printf '%s\\n' 'stub:test-ui-durable-session-real-backend'\n",
		"test-unit-coverage":                   "@printf '%s\\n' 'stub:test-unit-coverage'\n",
		"test-functional-coverage":             "@printf '%s\\n' 'stub:test-functional-coverage'\n",
		"verify":                               "@printf '%s\\n' 'unexpected:verify'\n\t@exit 99\n",
		"test-backend-functional":              "@printf '%s\\n' 'unexpected:test-backend-functional'\n\t@exit 99\n",
		"ui-test":                              "@printf '%s\\n' 'unexpected:ui-test'\n\t@exit 99\n",
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
		"Running required CI-equivalent test lanes: maintenance + integration + contract + release surface + root-process S24 acceptance + concurrent UI coverage/browser integration + Storybook + UI backend integration + independent backend unit and functional coverage",
		"==> Backend Maintenance lane [make test-maintenance]",
		"stub:test-maintenance",
		"==> Backend Integration lane [make test-integration]",
		"stub:test-integration",
		"==> Backend Contract lane [make test-contract]",
		"stub:test-contract",
		"==> Release surface smoke lane [make release-surface-smoke]",
		"stub:release-surface-smoke",
		"==> Root-process S24 acceptance lane [make test-root-process-acceptance]",
		"stub:test-root-process-acceptance",
		"==> Concurrent UI Coverage + UI Browser Integration lanes [make run-concurrent-ui-verification-lanes]",
		"stub:run-concurrent-ui-verification-lanes",
		"==> UI Storybook Integration lane [make test-ui-storybook-integration]",
		"stub:test-ui-storybook-integration",
		"==> UI Backend Integration lane [make test-ui-durable-session-real-backend]",
		"stub:test-ui-durable-session-real-backend",
		"==> Backend Unit Coverage lane [make test-unit-coverage]",
		"stub:test-unit-coverage",
		"==> Backend Functional Coverage lane [make test-functional-coverage]",
		"stub:test-functional-coverage",
	)

	for _, expected := range []string{
		"stub:verify-build-contracts",
		"stub:test-maintenance",
		"stub:test-integration",
		"stub:test-contract",
		"stub:release-surface-smoke",
		"stub:test-root-process-acceptance",
		"stub:run-concurrent-ui-verification-lanes",
		"stub:test-ui-storybook-integration",
		"stub:test-ui-durable-session-real-backend",
		"stub:test-unit-coverage",
		"stub:test-functional-coverage",
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

// TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun prove verify-pr failure output reports the exact failing lane rerun command.
func TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-root-process-acceptance":         "@printf '%s\\n' 'stub:test-root-process-acceptance'\n",
		"run-concurrent-ui-verification-lanes": "@printf '%s\\n' 'stub:run-concurrent-ui-verification-lanes'\n\t@exit 23\n",
		"test-unit-coverage":                   "@printf '%s\\n' 'stub:test-unit-coverage'\n",
		"test-functional-coverage":             "@printf '%s\\n' 'stub:test-functional-coverage'\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-pr")
	if err == nil {
		t.Fatalf("verify-pr unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: Concurrent UI Coverage + UI Browser Integration lanes [make run-concurrent-ui-verification-lanes] failed. Rerun with: make run-concurrent-ui-verification-lanes") {
		t.Fatalf("verify-pr failure output missing exact lane rerun hint:\n%s", output)
	}
	if strings.Contains(output, "stub:test-unit-coverage") || strings.Contains(output, "stub:test-functional-coverage") {
		t.Fatalf("verify-pr continued after the failing required lane:\n%s", output)
	}
}

// TestBackendCoverageAliasesSmoke_RedirectToIndependentLanes prove backend coverage aliases delegate to independent unit and functional lanes.
func TestBackendCoverageAliasesSmoke_RedirectToIndependentLanes(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"test-unit-coverage":       "@printf '%s\\n' 'stub:test-unit-coverage'\n",
		"test-functional-coverage": "@printf '%s\\n' 'stub:test-functional-coverage'\n",
	})

	coverageOutput, err := runMakefileTarget(repoRoot, makefilePath, "test-backend-coverage")
	if err != nil {
		t.Fatalf("run test-backend-coverage wrapper: %v\n%s", err, coverageOutput)
	}
	if count := strings.Count(coverageOutput, "stub:test-unit-coverage"); count != 1 {
		t.Fatalf("test-backend-coverage should delegate to unit coverage exactly once, found %d:\n%s", count, coverageOutput)
	}
	if strings.Contains(coverageOutput, "stub:test-functional-coverage") {
		t.Fatalf("test-backend-coverage unexpectedly ran functional coverage:\n%s", coverageOutput)
	}

	functionalOutput, err := runMakefileTarget(repoRoot, makefilePath, "test-backend-functional")
	if err != nil {
		t.Fatalf("run test-backend-functional wrapper: %v\n%s", err, functionalOutput)
	}
	if count := strings.Count(functionalOutput, "stub:test-functional-coverage"); count != 1 {
		t.Fatalf("test-backend-functional should delegate to functional coverage exactly once, found %d:\n%s", count, functionalOutput)
	}
	if strings.Contains(functionalOutput, "stub:test-unit-coverage") {
		t.Fatalf("test-backend-functional unexpectedly ran unit coverage:\n%s", functionalOutput)
	}
}

// TestUICoverageCommandSmoke_RunsPackageCoverageThenReplayCheck prove test-ui-coverage runs package coverage before the replay check.
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

// TestUIPackageCoverageCommandSmoke_InvokesPackageOwnedCoverageScript prove ui-test-coverage invokes the package-owned coverage script.
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

// TestVerifyCompatibilityAliasSmoke_RedirectsToCanonicalPRTier prove make verify remains a compatibility alias for verify-pr.
func TestVerifyCompatibilityAliasSmoke_RedirectsToCanonicalPRTier(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-root-process-acceptance":         "@printf '%s\\n' 'stub:test-root-process-acceptance'\n",
		"run-concurrent-ui-verification-lanes": "@printf '%s\\n' 'stub:run-concurrent-ui-verification-lanes'\n",
		"test-ui-storybook-integration":        "@printf '%s\\n' 'stub:test-ui-storybook-integration'\n",
		"test-ui-durable-session-real-backend": "@printf '%s\\n' 'stub:test-ui-durable-session-real-backend'\n",
		"test-unit-coverage":                   "@printf '%s\\n' 'stub:test-unit-coverage'\n",
		"test-functional-coverage":             "@printf '%s\\n' 'stub:test-functional-coverage'\n",
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
		"==> Backend Maintenance lane [make test-maintenance]",
		"stub:test-maintenance",
		"==> Backend Integration lane [make test-integration]",
		"stub:test-integration",
		"==> Backend Contract lane [make test-contract]",
		"stub:test-contract",
		"==> Release surface smoke lane [make release-surface-smoke]",
		"stub:release-surface-smoke",
		"==> Root-process S24 acceptance lane [make test-root-process-acceptance]",
		"stub:test-root-process-acceptance",
		"==> Concurrent UI Coverage + UI Browser Integration lanes [make run-concurrent-ui-verification-lanes]",
		"stub:run-concurrent-ui-verification-lanes",
		"==> UI Storybook Integration lane [make test-ui-storybook-integration]",
		"stub:test-ui-storybook-integration",
		"==> UI Backend Integration lane [make test-ui-durable-session-real-backend]",
		"stub:test-ui-durable-session-real-backend",
		"==> Backend Unit Coverage lane [make test-unit-coverage]",
		"stub:test-unit-coverage",
		"==> Backend Functional Coverage lane [make test-functional-coverage]",
		"stub:test-functional-coverage",
	)

	for _, expected := range []string{
		"stub:verify-build-contracts",
		"stub:test-maintenance",
		"stub:test-integration",
		"stub:test-contract",
		"stub:release-surface-smoke",
		"stub:test-root-process-acceptance",
		"stub:run-concurrent-ui-verification-lanes",
		"stub:test-ui-storybook-integration",
		"stub:test-ui-durable-session-real-backend",
		"stub:test-unit-coverage",
		"stub:test-functional-coverage",
	} {
		if count := strings.Count(output, expected); count != 1 {
			t.Fatalf("expected %q exactly once through the verify compatibility alias, found %d:\n%s", expected, count, output)
		}
	}
}

// TestBackendVerificationLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog prove run-backend-verification.sh invokes the canonical make target and captures command.log.
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

// TestConcurrentUIVerificationLanesScriptSmoke_RunsBothOwnedLanesConcurrently prove run-concurrent-ui-verification-lanes.sh runs both owned lanes concurrently with prefixed output.
func TestConcurrentUIVerificationLanesScriptSmoke_RunsBothOwnedLanesConcurrently(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-concurrent-ui", `#!/bin/sh
case "$1" in
  test-ui-coverage)
    printf '%s\n' "fake-make:test-ui-coverage"
    sleep 0.2
    ;;
  ui-integration-test)
    printf '%s\n' "fake-make:ui-integration-test"
    ;;
  *)
    printf '%s\n' "fake-make:unexpected:$*"
    exit 99
    ;;
esac
`)
	artifactRoot := filepath.Join(t.TempDir(), "concurrent-ui-verification-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-concurrent-ui-verification-lanes.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err != nil {
		t.Fatalf("run concurrent UI verification script: %v\n%s", err, output)
	}

	if !strings.Contains(output, "[UI Coverage] fake-make:test-ui-coverage") {
		t.Fatalf("concurrent UI verification script missing prefixed coverage output:\n%s", output)
	}
	if !strings.Contains(output, "[UI Browser Integration] fake-make:ui-integration-test") {
		t.Fatalf("concurrent UI verification script missing prefixed browser output:\n%s", output)
	}

	assertOutputOrder(t, output,
		"==> UI Coverage lane [make test-ui-coverage] (concurrent)",
		"==> UI Browser Integration lane [make ui-integration-test] (concurrent)",
	)

	for _, lane := range []struct {
		label    string
		filename string
	}{
		{"UI Coverage", "ui-coverage.log"},
		{"UI Browser Integration", "ui-browser-integration.log"},
	} {
		logBody, readErr := os.ReadFile(filepath.Join(artifactRoot, lane.filename))
		if readErr != nil {
			t.Fatalf("read %s lane log: %v", lane.label, readErr)
		}
		if len(logBody) == 0 {
			t.Fatalf("%s lane log is empty", lane.label)
		}
	}
}

// TestConcurrentUIVerificationLanesScriptSmoke_FailureReportsExactLaneRerun prove concurrent UI verification failure output reports the exact browser lane rerun command.
func TestConcurrentUIVerificationLanesScriptSmoke_FailureReportsExactLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-concurrent-ui-fail", `#!/bin/sh
case "$1" in
  test-ui-coverage)
    printf '%s\n' "fake-make:test-ui-coverage"
    ;;
  ui-integration-test)
    printf '%s\n' "fake-make:ui-integration-test"
    exit 23
    ;;
  *)
    printf '%s\n' "fake-make:unexpected:$*"
    exit 99
    ;;
esac
`)
	artifactRoot := filepath.Join(t.TempDir(), "concurrent-ui-verification-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-concurrent-ui-verification-lanes.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err == nil {
		t.Fatalf("concurrent UI verification script unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: UI Browser Integration lane [make ui-integration-test] failed. Rerun with: make ui-integration-test") {
		t.Fatalf("concurrent UI verification script missing exact browser lane rerun hint:\n%s", output)
	}
}

// TestConcurrentUIVerificationLanesScriptSmoke_DoesNotWaitForDetachedLogHandle prove concurrent UI verification does not wait for a detached process holding output open.
func TestConcurrentUIVerificationLanesScriptSmoke_DoesNotWaitForDetachedLogHandle(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-concurrent-ui-detached", `#!/bin/sh
printf '%s\n' "fake-make:$1"
(sleep 2) &
`)
	artifactRoot := filepath.Join(t.TempDir(), "concurrent-ui-verification-artifacts")

	startedAt := time.Now()
	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-concurrent-ui-verification-lanes.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err != nil {
		t.Fatalf("run concurrent UI verification script: %v\n%s", err, output)
	}
	if elapsed := time.Since(startedAt); elapsed >= 1500*time.Millisecond {
		t.Fatalf("concurrent UI verification waited for a detached process holding output open: %s\n%s", elapsed, output)
	}
}

// TestBackendVerificationLaneScriptSmoke_PreservesFailureExitAndLog prove run-backend-verification.sh preserves failure exit codes and command.log output.
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

// TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier prove verify-extended runs PR verification then only explicit long and specialty suites.
func TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-pr":                     "@printf '%s\\n' 'stub:verify-pr'\n",
		"test-ui-performance":           "@printf '%s\\n' 'stub:test-ui-performance'\n",
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
		"Running opt-in long and specialty suites: UI performance + managed runtime coverage + real local inference coverage",
		"==> UI Performance specialty lane [make test-ui-performance]",
		"stub:test-ui-performance",
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

// TestLongTestsCommandSmoke_FailureReportsExactSpecialtyLaneRerun prove long-tests failure output reports the exact specialty lane rerun command.
func TestLongTestsCommandSmoke_FailureReportsExactSpecialtyLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"test-ui-performance":           "@printf '%s\\n' 'stub:test-ui-performance'\n",
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

// TestVerifyPRInferenceCommandSmoke_RunsSingleNamedRegressionOnly prove verify-pr-inference runs only the named PR inference approval regression.
func TestVerifyPRInferenceCommandSmoke_RunsSingleNamedRegressionOnly(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"pr-inference-approval":         "@printf '%s\\n' 'stub:pr-inference-approval'\n",
		"long-tests":                    "@printf '%s\\n' 'unexpected:long-tests'\n\t@exit 99\n",
		"long-tests-managed-runtime":    "@printf '%s\\n' 'unexpected:long-tests-managed-runtime'\n\t@exit 99\n",
		"long-tests-functional-runtime": "@printf '%s\\n' 'unexpected:long-tests-functional-runtime'\n\t@exit 99\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-pr-inference")
	if err != nil {
		t.Fatalf("run verify-pr-inference wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running PR-gated inference approval lane: TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio",
		"Required: export INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1",
		"Runtime: omnivoice-llamacpp on PATH, or set INFINITE_YOU_OMNIVOICE_COMMAND to the executable",
		"Optional: INFINITE_YOU_OMNIVOICE_CACHE_DIR to reuse managed model cache (omit to use a temp cache)",
		"Broader specialty sweep remains on make long-tests; this lane is merge-blocking PR inference approval only",
		"==> PR inference approval regression [make pr-inference-approval]",
		"stub:pr-inference-approval",
	)

	for _, unwanted := range []string{
		"unexpected:long-tests",
		"unexpected:long-tests-managed-runtime",
		"unexpected:long-tests-functional-runtime",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-pr-inference unexpectedly ran %q:\n%s", unwanted, output)
		}
	}
}

// TestVerifyPRInferenceCommandSmoke_FailureReportsOwnedRerunCommand prove verify-pr-inference failure output reports the owned rerun command.
func TestVerifyPRInferenceCommandSmoke_FailureReportsOwnedRerunCommand(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"pr-inference-approval": "@printf '%s\\n' 'stub:pr-inference-approval'\n\t@exit 31\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "verify-pr-inference")
	if err == nil {
		t.Fatalf("verify-pr-inference unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: PR inference approval regression [make pr-inference-approval] failed. Rerun with: make pr-inference-approval") {
		t.Fatalf("verify-pr-inference failure output missing exact rerun hint:\n%s", output)
	}
}

// TestVerifyPRInferenceCommandSmoke_StaysOutsideRequiredPRAndExtendedTiers prove verify-pr, verify-extended, and long-tests do not invoke verify-pr-inference.
func TestVerifyPRInferenceCommandSmoke_StaysOutsideRequiredPRAndExtendedTiers(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":        "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"release-surface-smoke":         "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-ui-coverage":              "@printf '%s\\n' 'stub:test-ui-coverage'\n",
		"test-ui-performance":           "@printf '%s\\n' 'stub:test-ui-performance'\n",
		"ui-integration-test":           "@printf '%s\\n' 'stub:ui-integration-test'\n",
		"test-backend-verification":     "@printf '%s\\n' 'stub:test-backend-verification'\n",
		"verify-pr":                     "@printf '%s\\n' 'stub:verify-pr'\n",
		"long-tests-managed-runtime":    "@printf '%s\\n' 'stub:long-tests-managed-runtime'\n",
		"long-tests-functional-runtime": "@printf '%s\\n' 'stub:long-tests-functional-runtime'\n",
		"pr-inference-approval":         "@printf '%s\\n' 'unexpected:pr-inference-approval'\n\t@exit 99\n",
	})

	for _, target := range []string{"verify-pr", "verify-extended", "long-tests"} {
		output, err := runMakefileTarget(repoRoot, makefilePath, target)
		if err != nil {
			t.Fatalf("run %s wrapper: %v\n%s", target, err, output)
		}
		if strings.Contains(output, "unexpected:pr-inference-approval") {
			t.Fatalf("%s unexpectedly ran verify-pr-inference lane:\n%s", target, output)
		}
	}
}
