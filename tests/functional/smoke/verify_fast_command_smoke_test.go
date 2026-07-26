//go:build !windows

package smoke

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

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

func TestVerifyPRCommandSmoke_UsesRequiredLanesOnce(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-built-cli-acceptance":            "@printf '%s\\n' 'stub:test-built-cli-acceptance'\n",
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
		"Running required CI-equivalent test lanes: maintenance + integration + contract + release surface + built-CLI S24 acceptance + concurrent UI coverage/browser integration + Storybook + UI backend integration + independent backend unit and functional coverage",
		"==> Backend Maintenance lane [make test-maintenance]",
		"stub:test-maintenance",
		"==> Backend Integration lane [make test-integration]",
		"stub:test-integration",
		"==> Backend Contract lane [make test-contract]",
		"stub:test-contract",
		"==> Release surface smoke lane [make release-surface-smoke]",
		"stub:release-surface-smoke",
		"==> Built-CLI S24 acceptance lane [make test-built-cli-acceptance]",
		"stub:test-built-cli-acceptance",
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
		"stub:test-built-cli-acceptance",
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

func TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-built-cli-acceptance":            "@printf '%s\\n' 'stub:test-built-cli-acceptance'\n",
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
		"verify-build-contracts":               "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"test-maintenance":                     "@printf '%s\\n' 'stub:test-maintenance'\n",
		"test-integration":                     "@printf '%s\\n' 'stub:test-integration'\n",
		"test-contract":                        "@printf '%s\\n' 'stub:test-contract'\n",
		"release-surface-smoke":                "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-built-cli-acceptance":            "@printf '%s\\n' 'stub:test-built-cli-acceptance'\n",
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
		"==> Built-CLI S24 acceptance lane [make test-built-cli-acceptance]",
		"stub:test-built-cli-acceptance",
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
		"stub:test-built-cli-acceptance",
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

func TestConcurrentUIVerificationLanesScriptSmoke_RunsBothOwnedLanesConcurrently(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-concurrent-ui", `#!/bin/sh
case "$1" in
  run-sharded-ui-coverage)
    printf '%s\n' "fake-make:run-sharded-ui-coverage"
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

	if !strings.Contains(output, "[UI Coverage] fake-make:run-sharded-ui-coverage") {
		t.Fatalf("concurrent UI verification script missing prefixed coverage output:\n%s", output)
	}
	if !strings.Contains(output, "[UI Browser Integration] fake-make:ui-integration-test") {
		t.Fatalf("concurrent UI verification script missing prefixed browser output:\n%s", output)
	}

	assertOutputOrder(t, output,
		"==> UI Coverage lane [make run-sharded-ui-coverage] (concurrent)",
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

func TestConcurrentUIVerificationLanesScriptSmoke_FailureReportsExactLaneRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-concurrent-ui-fail", `#!/bin/sh
case "$1" in
  run-sharded-ui-coverage)
    printf '%s\n' "fake-make:run-sharded-ui-coverage"
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

func TestShardedUICoverageScriptSmoke_RunsAllShardsThenMerge(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-sharded-ui", `#!/bin/sh
case "$1" in
  ui-test-coverage)
    printf '%s\n' "fake-make:ui-test-coverage UI_COVERAGE_SHARD=${UI_COVERAGE_SHARD:-unset}"
    ;;
  test-ui-coverage-merge)
    printf '%s\n' "fake-make:test-ui-coverage-merge"
    ;;
  *)
    printf '%s\n' "fake-make:unexpected:$*"
    exit 99
    ;;
esac
`)
	artifactRoot := filepath.Join(t.TempDir(), "sharded-ui-coverage-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-sharded-ui-coverage.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
		"UI_COVERAGE_SHARD_TOTAL=2",
	)
	if err != nil {
		t.Fatalf("run sharded UI coverage script: %v\n%s", err, output)
	}

	for _, expected := range []string{
		"==> Sharded UI Coverage (2 main covered Vitest shards + merge)",
		"[UI Coverage Shard 1/2] fake-make:ui-test-coverage UI_COVERAGE_SHARD=1/2",
		"[UI Coverage Shard 2/2] fake-make:ui-test-coverage UI_COVERAGE_SHARD=2/2",
		"==> UI Coverage merge lane [make test-ui-coverage-merge]",
		"fake-make:test-ui-coverage-merge",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("sharded UI coverage script missing %q:\n%s", expected, output)
		}
	}
}

func TestShardedUICoverageScriptSmoke_CleansStaleVitestReportBlobsBeforeShards(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	reportsDir := filepath.Join(repoRoot, "ui", ".vitest-reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("create vitest reports dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(reportsDir)
	})

	staleFiles := []string{
		filepath.Join(reportsDir, "main.json"),
		filepath.Join(reportsDir, "main-shard-99.json"),
		filepath.Join(reportsDir, "react-flow-current-activity-card.json"),
	}
	for _, stalePath := range staleFiles {
		if err := os.WriteFile(stalePath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write stale vitest report blob %s: %v", stalePath, err)
		}
	}

	makePath := writeExecutableScript(t, "fake-make-sharded-ui-clean", `#!/bin/sh
case "$1" in
  ui-test-coverage)
    printf '%s\n' "fake-make:ui-test-coverage UI_COVERAGE_SHARD=${UI_COVERAGE_SHARD:-unset}"
    ;;
  test-ui-coverage-merge)
    printf '%s\n' "fake-make:test-ui-coverage-merge"
    ;;
  *)
    printf '%s\n' "fake-make:unexpected:$*"
    exit 99
    ;;
esac
`)
	artifactRoot := filepath.Join(t.TempDir(), "sharded-ui-coverage-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-sharded-ui-coverage.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
		"UI_COVERAGE_SHARD_TOTAL=2",
	)
	if err != nil {
		t.Fatalf("run sharded UI coverage script: %v\n%s", err, output)
	}

	for _, stalePath := range staleFiles {
		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Fatalf("stale vitest report blob still present after sharded run: %s (stat err=%v)", stalePath, statErr)
		}
	}
}

func TestShardedUICoverageScriptSmoke_FailureReportsExactShardRerun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-sharded-ui-fail", `#!/bin/sh
case "$1" in
  ui-test-coverage)
    case "${UI_COVERAGE_SHARD:-}" in
      2/2)
        printf '%s\n' "fake-make:ui-test-coverage UI_COVERAGE_SHARD=${UI_COVERAGE_SHARD}"
        exit 19
        ;;
      *)
        printf '%s\n' "fake-make:ui-test-coverage UI_COVERAGE_SHARD=${UI_COVERAGE_SHARD:-unset}"
        ;;
    esac
    ;;
  test-ui-coverage-merge)
    printf '%s\n' "fake-make:test-ui-coverage-merge"
    exit 99
    ;;
  *)
    printf '%s\n' "fake-make:unexpected:$*"
    exit 99
    ;;
esac
`)
	artifactRoot := filepath.Join(t.TempDir(), "sharded-ui-coverage-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-sharded-ui-coverage.sh"),
		fmt.Sprintf("ARTIFACT_ROOT=%s", artifactRoot),
		fmt.Sprintf("MAKE_BIN=%s", makePath),
		"UI_COVERAGE_SHARD_TOTAL=2",
	)
	if err == nil {
		t.Fatalf("sharded UI coverage script unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: UI Coverage Shard 2/2 failed. Rerun with: UI_COVERAGE_SHARD=2/2 make ui-test-coverage") {
		t.Fatalf("sharded UI coverage script missing exact shard rerun hint:\n%s", output)
	}
	if strings.Contains(output, "fake-make:test-ui-coverage-merge") {
		t.Fatalf("sharded UI coverage script should not run merge after shard failure:\n%s", output)
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

func TestVerifyPRInferenceCommandSmoke_StaysOutsideRequiredPRAndExtendedTiers(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-build-contracts":        "@printf '%s\\n' 'stub:verify-build-contracts'\n",
		"release-surface-smoke":         "@printf '%s\\n' 'stub:release-surface-smoke'\n",
		"test-ui-coverage":              "@printf '%s\\n' 'stub:test-ui-coverage'\n",
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

func writeVerifyFastWrapperMakefile(t *testing.T, repoRoot string, overrides map[string]string) string {
	t.Helper()
	requirePOSIXShell(t)

	var body strings.Builder
	body.WriteString("SHELL := sh\n")
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
	return filepath.ToSlash(path)
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
		"SHELL=sh",
		fmt.Sprintf("MAKE=%s -f %s", filepath.ToSlash(makePath), filepath.ToSlash(makefilePath)),
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
	requirePOSIXShell(t)

	path := filepath.Join(t.TempDir(), label)
	body := fmt.Sprintf("#!/bin/sh\nprintf '%%s:' %q\nprintf '%%s\\n' \"$*\"\n", label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write echo script: %v", err)
	}
	return filepath.ToSlash(path)
}

func writeExecutableScript(t *testing.T, label string, body string) string {
	t.Helper()
	requirePOSIXShell(t)

	path := filepath.Join(t.TempDir(), label)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable script: %v", err)
	}
	return filepath.ToSlash(path)
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("POSIX shell smoke test requires sh: %v", err)
	}
}

func runScript(repoRoot string, scriptPath string, env ...string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("sh", filepath.ToSlash(scriptPath))
	} else {
		cmd = exec.Command(scriptPath)
	}
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
