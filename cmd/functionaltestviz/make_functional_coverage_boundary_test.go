//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFunctionalCoverageCommandSmoke_InvokesCoverage proves the Make surface
// invokes the functional coverage runner exactly once with the requested suite.
func TestFunctionalCoverageCommandSmoke_InvokesCoverage(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, nil)
	goStub := writeMakeEchoScript(t, "stub-go")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"test-functional-coverage",
		"GO="+goStub,
	)
	if err != nil {
		t.Fatalf("run test-functional-coverage wrapper: %v\n%s", err, output)
	}
	if count := strings.Count(output, "stub-go:run ./cmd/gocoveragecheck"); count != 1 {
		t.Fatalf("expected one gocoveragecheck invocation, found %d:\n%s", count, output)
	}
	if !strings.Contains(output, "-suite functional") {
		t.Fatalf("test-functional-coverage missing functional suite flag:\n%s", output)
	}
}

// TestFunctionalCoverageCommandSmoke_CoverageFailurePropagates proves a
// failed coverage runner preserves its diagnostics and exits non-zero.
func TestFunctionalCoverageCommandSmoke_CoverageFailurePropagates(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, nil)
	goStub := writeMakeEchoScript(t, "stub-go")
	if err := os.WriteFile(goStub, []byte("#!/bin/sh\nprintf '%s\\n' \"stub:coverage-fail $*\"\nexit 23\n"), 0o755); err != nil {
		t.Fatalf("write failing coverage runner: %v", err)
	}

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"test-functional-coverage",
		"GO="+goStub,
	)
	if err == nil {
		t.Fatalf("test-functional-coverage unexpectedly succeeded after coverage failure:\n%s", output)
	}
	if !strings.Contains(output, "stub:coverage-fail run ./cmd/gocoveragecheck -suite functional") {
		t.Fatalf("test-functional-coverage output missing coverage failure diagnostics:\n%s", output)
	}
}

// TestBackendCoverageAliasesSmoke_UnitLaneRunsOnlyUnitCoverage
// proves the unit coverage matrix alias delegates only to unit coverage.
func TestBackendCoverageAliasesSmoke_UnitLaneRunsOnlyUnitCoverage(t *testing.T) {
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
}
