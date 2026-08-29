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

// TestFunctionalLongCompileGate_UsesRealTaggedFixtureOutcome proves the
// compile-only gate observes real tagged source without executing the test.
func TestFunctionalLongCompileGate_UsesRealTaggedFixtureOutcome(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, nil)

	validPackage := writeFunctionallongFixture(t, repoRoot, false)
	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"FUNCTIONAL_LONG_PACKAGES="+validPackage,
		"test-functional-long-compile",
	)
	if err != nil {
		t.Fatalf("valid tagged fixture failed compile-only gate: %v\n%s", err, output)
	}

	invalidPackage := writeFunctionallongFixture(t, repoRoot, true)
	output, err = runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"FUNCTIONAL_LONG_PACKAGES="+invalidPackage,
		"test-functional-long-compile",
	)
	if err == nil {
		t.Fatalf("invalid tagged fixture unexpectedly passed compile-only gate:\n%s", output)
	}
	if !strings.Contains(output, "functionallongCompileGateIntentionalError") {
		t.Fatalf("compile-only gate failed without reporting the invalid tagged fixture:\n%s", output)
	}
}

// TestFunctionalLaneTargetsSeparateCachedAndFreshModes proves the two public
// Make targets keep the same boundary and runner settings while selecting
// Go's cache mode versus explicit execution.
func TestFunctionalLaneTargetsSeparateCachedAndFreshModes(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	fakeGo := writeExecutableScript(t, "fake-go-functional-lane", `#!/bin/sh
if [ "$1" = "run" ]; then
  printf '%s\n' "$*" >> "$FUNCTIONAL_LANE_ARGS"
fi
if [ "$1" = "run" ] && [ "${FUNCTIONAL_LANE_FAIL:-0}" = "1" ]; then
  printf '%s\n' '--- FAIL: TestRepresentativeFunctionalFailure'
  exit 23
fi
`)

	for _, tc := range []struct {
		target   string
		wantMode string
	}{
		{target: "test-functional", wantMode: "cached"},
		{target: "test-functional-fresh", wantMode: "fresh"},
	} {
		t.Run(tc.wantMode+" success", func(t *testing.T) {
			argsPath := filepath.Join(t.TempDir(), "go-args.txt")
			t.Setenv("FUNCTIONAL_LANE_ARGS", argsPath)
			makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
				"functional-boundary-check": "@printf '%s\\n' 'stub:functional-boundary'\n",
			})

			output, err := runMakefileTargetWithArgs(
				repoRoot,
				makefilePath,
				fmt.Sprintf("GO=%s", fakeGo),
				"FUNCTIONAL_DEFAULT_JOBS=3",
				"GO_TEST_TIMEOUT=17s",
				tc.target,
			)
			if err != nil {
				t.Fatalf("run %s: %v\\n%s", tc.target, err, output)
			}
			if count := strings.Count(output, "stub:functional-boundary"); count != 1 {
				t.Fatalf("%s ran boundary check %d times, want once:\\n%s", tc.target, count, output)
			}

			args, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read fake go args: %v", err)
			}
			got := strings.TrimSpace(string(args))
			wantBase := "run ./cmd/functionallane -jobs 3 -timeout 17s"
			if tc.wantMode == "cached" && got != wantBase {
				t.Fatalf("cached target args = %q, want %q", got, wantBase)
			}
			if tc.wantMode == "fresh" && got != "run ./cmd/functionallane -jobs 3 -count=1 -timeout 17s" {
				t.Fatalf("fresh target args = %q, want explicit count", got)
			}
		})

		t.Run(tc.wantMode+" failure", func(t *testing.T) {
			argsPath := filepath.Join(t.TempDir(), "go-args.txt")
			t.Setenv("FUNCTIONAL_LANE_ARGS", argsPath)
			t.Setenv("FUNCTIONAL_LANE_FAIL", "1")
			makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
				"functional-boundary-check": "@printf '%s\\n' 'stub:functional-boundary'\n",
			})

			output, err := runMakefileTargetWithArgs(
				repoRoot,
				makefilePath,
				fmt.Sprintf("GO=%s", fakeGo),
				"FUNCTIONAL_DEFAULT_JOBS=3",
				"GO_TEST_TIMEOUT=17s",
				tc.target,
			)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded:\\n%s", tc.target, output)
			}
			if !strings.Contains(output, "functional-lane") {
				t.Fatalf("%s failure did not include the runner command:\\n%s", tc.target, output)
			}
			if !strings.Contains(output, "TestRepresentativeFunctionalFailure") {
				t.Fatalf("%s failure did not preserve the failing functional test output:\\n%s", tc.target, output)
			}
		})
	}
}

// TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites prove verify-fast selects only short owned suites in order.
func TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	executionSentinel := filepath.Join(t.TempDir(), "verify-fast-suite-executed")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"typecheck":             "@printf '%s\\n' 'stub:typecheck'\n",
		"mcp-contract-check":    "@printf '%s\\n' 'stub:mcp-contract-check'\n",
		"ui-test":               "@printf '%s\\n' 'stub:ui-test'\n",
		"test":                  fmt.Sprintf("@printf '%%s\\n' 'stub:test'\n@printf '%%s\\n' 'selected-suite-executed' > %q\n", executionSentinel),
		"test-unit":             fmt.Sprintf("@printf '%%s\\n' 'simulated-unit-failure'\n@printf '%%s\\n' 'unit-suite-executed' > %q\n@exit 73\n", executionSentinel),
		"ui-install-playwright": "@printf '%s\\n' 'unexpected:ui-install-playwright'\n\t@exit 99\n",
		"ui-integration-test":   "@printf '%s\\n' 'unexpected:ui-integration-test'\n\t@exit 99\n",
		"test-functional-long":  "@printf '%s\\n' 'unexpected:test-functional-long'\n\t@exit 99\n",
		"long-tests":            "@printf '%s\\n' 'unexpected:long-tests'\n\t@exit 99\n",
	})

	output, err := runMakefileTargetDryRun(repoRoot, makefilePath, "verify-fast")
	if err != nil {
		t.Fatalf("dry-run verify-fast wrapper: %v\n%s", err, output)
	}
	if _, err := os.Stat(executionSentinel); !os.IsNotExist(err) {
		t.Fatalf("dry-run verify-fast executed a suite recipe; sentinel error = %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"Running fast verification tier: typecheck + MCP contract boundary + short UI/unit suite + short Go suite",
		"==> dashboard typecheck [make typecheck]",
		"typecheck ||",
		"==> MCP contract boundary [make mcp-contract-check]",
		"mcp-contract-check ||",
		"==> short UI/unit suite [make ui-test]",
		"ui-test ||",
		"==> short Go suite [make test]",
		"test ||",
	)

	for _, unwanted := range []string{
		"[make ui-install-playwright]",
		"[make ui-integration-test]",
		"[make test-functional-long]",
		"[make long-tests]",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("verify-fast unexpectedly selected %q:\n%s", unwanted, output)
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

	output, err := runMakefileTargetWithPrerequisitesMarkedOld(
		repoRoot,
		makefilePath,
		"verify-fast",
		"test-unit",
		"test-ci-workflows",
	)
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

	output, err := runMakefileTargetWithPrerequisitesMarkedOld(
		repoRoot,
		makefilePath,
		"verify-fast",
		"test-unit",
		"test-ci-workflows",
	)
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

// TestFunctionalTestVizLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog
// now proves CI invokes the supervisor whose coverage child owns the single
// Make-owned functional report entrypoint.
func TestFunctionalTestVizLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	workflow, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	body := string(workflow)
	if !strings.Contains(body, "run: bash scripts/ci/run-functional-coverage-with-quarantine.sh") {
		t.Fatalf("functional CI lane does not invoke the coverage supervisor:\n%s", body)
	}
	supervisor, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "ci", "run-functional-coverage-with-quarantine.sh"))
	if err != nil {
		t.Fatalf("read functional coverage supervisor: %v", err)
	}
	if !strings.Contains(string(supervisor), "coverage_command=(make functional-test-viz)") {
		t.Fatalf("functional coverage supervisor does not invoke the canonical Make target:\n%s", string(supervisor))
	}
	if strings.Contains(body, "run-functional-test-viz.sh") {
		t.Fatalf("functional CI lane still invokes the retired Bash runner:\n%s", body)
	}
}

// TestFunctionalTestVizLaneScriptSmoke_PreservesFailureExitAndLog proves the
// Make-owned entrypoint retains the full stream and returns its captured status.
func TestFunctionalTestVizLaneScriptSmoke_PreservesFailureExitAndLog(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	body := string(makefile)
	if !strings.Contains(body, `-log "$(FUNCTIONAL_TEST_VIZ_LOG)"`) {
		t.Fatalf("functional-test-viz does not pass the owned command log to its Go runner")
	}
	runner, err := os.ReadFile(filepath.Join(repoRoot, "cmd", "functionaltestviz", "suite.go"))
	if err != nil {
		t.Fatalf("read functional test Go runner: %v", err)
	}
	for _, required := range []string{"cmd.Stdout = log", "cmd.Stderr = log", "suiteExitError"} {
		if !strings.Contains(string(runner), required) {
			t.Fatalf("functional test Go runner missing failure/log contract %q", required)
		}
	}
}

// TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier prove verify-extended runs PR verification then only explicit long and specialty suites.
func TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"verify-pr":                  "@printf '%s\\n' 'stub:verify-pr'\n",
		"test-ui-performance":        "@printf '%s\\n' 'stub:test-ui-performance'\n",
		"long-tests-managed-runtime": "@printf '%s\\n' 'stub:long-tests-managed-runtime'\n",
		"test-functional-long":       "@printf '%s\\n' 'unexpected:test-functional-long'\n\t@exit 99\n",
		"test-backend-functional":    "@printf '%s\\n' 'unexpected:test-backend-functional'\n\t@exit 99\n",
		"ui-integration-test":        "@printf '%s\\n' 'unexpected:ui-integration-test'\n\t@exit 99\n",
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
		"Running opt-in long and specialty suites: UI performance + managed runtime coverage",
		"==> UI Performance specialty lane [make test-ui-performance]",
		"stub:test-ui-performance",
		"==> Managed Runtime specialty lane [make long-tests-managed-runtime]",
		"stub:long-tests-managed-runtime",
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
		"test-ui-performance":        "@printf '%s\\n' 'stub:test-ui-performance'\n",
		"long-tests-managed-runtime": "@printf '%s\\n' 'stub:long-tests-managed-runtime'\n\t@exit 29\n",
	})

	output, err := runMakefileTarget(repoRoot, makefilePath, "long-tests")
	if err == nil {
		t.Fatalf("long-tests unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "FAIL: Managed Runtime specialty lane [make long-tests-managed-runtime] failed. Rerun with: make long-tests-managed-runtime") {
		t.Fatalf("long-tests failure output missing exact specialty rerun hint:\n%s", output)
	}
}
