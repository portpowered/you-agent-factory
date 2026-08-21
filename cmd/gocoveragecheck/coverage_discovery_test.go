package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveCoverageLaneUsesOneFreshSweepForDefaultUnitSets(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	var listCalls []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "list" {
			return fakeGoCoverageCommandPassing(invocation)
		}
		listCalls = append(listCalls, invocation)
		return strings.Join([]string{
			modulePath + "/pkg/covered\t2",
			modulePath + "/pkg/testless\t0",
			modulePath + "/pkg/transports/http/client\t4",
			modulePath + "/internal/testutil/runtimefixtures\t1",
			modulePath + "/cmd/factory\t1",
			modulePath + "/pkg/covered\t2",
		}, "\n"), "", nil
	}

	discovery, coverPackages, testPackages, err := resolveCoverageLaneWithDiscovery(config{suite: unitCoverageSuite})
	if err != nil {
		t.Fatalf("resolveCoverageLaneWithDiscovery() error = %v", err)
	}
	wantCover := []string{modulePath + "/pkg/covered"}
	wantTest := []string{modulePath + "/pkg/covered", modulePath + "/pkg/testless"}
	if !reflect.DeepEqual(coverPackages, wantCover) {
		t.Fatalf("cover packages = %v, want %v", coverPackages, wantCover)
	}
	if !reflect.DeepEqual(testPackages, wantTest) {
		t.Fatalf("test packages = %v, want %v", testPackages, wantTest)
	}
	wantUniverse := []string{
		modulePath + "/cmd/factory",
		modulePath + "/internal/testutil/runtimefixtures",
		modulePath + "/pkg/covered",
		modulePath + "/pkg/testless",
		modulePath + "/pkg/transports/http/client",
	}
	if !reflect.DeepEqual(discovery.allPackages, wantUniverse) {
		t.Fatalf("discovered package universe = %v, want %v", discovery.allPackages, wantUniverse)
	}
	if len(listCalls) != 1 {
		t.Fatalf("go list calls = %d, want one fresh sweep: %#v", len(listCalls), listCalls)
	}
	if got := listCalls[0].args[3:]; !reflect.DeepEqual(got, []string{"./pkg/..."}) {
		t.Fatalf("go list patterns = %v, want one default unit pattern", got)
	}

	_, _, _, err = resolveCoverageLaneWithDiscovery(config{suite: unitCoverageSuite})
	if err != nil {
		t.Fatalf("second resolveCoverageLaneWithDiscovery() error = %v", err)
	}
	if len(listCalls) != 2 {
		t.Fatalf("go list calls after second invocation = %d, want fresh discovery per invocation", len(listCalls))
	}
}

func TestCompactUnitTestPackageArgsUsesProvidedUniverse(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		t.Fatalf("unexpected package-list sweep while planning from provided universe: %#v", invocation.args)
		return "", "", nil
	}

	root := modulePath + "/pkg"
	allPackages := []string{
		root + "/alpha",
		root + "/alpha/one",
		root + "/alpha/two",
		root + "/beta",
		root + "/beta/excluded",
	}
	selectedPackages := []string{root + "/alpha", root + "/alpha/one", root + "/alpha/two", root + "/beta"}
	want := []string{root + "/alpha/...", root + "/beta"}

	got := compactUnitTestPackageArgs(config{}, selectedPackages, "windows", allPackages)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compactUnitTestPackageArgs() = %v, want %v", got, want)
	}
}

func TestResolveCoverageLaneUsesOneSweepForFunctionalCoverAndTestSets(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	var listCalls []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if len(invocation.args) == 0 || invocation.args[0] != "list" {
			return fakeGoCoverageCommandPassing(invocation)
		}
		listCalls = append(listCalls, invocation)
		return strings.Join([]string{
			modulePath + "/pkg/covered\t2",
			modulePath + "/tests/functional/runtime_api\t1",
			modulePath + "/tests/functional/internal/support\t1",
		}, "\n"), "", nil
	}

	_, coverPackages, testPackages, err := resolveCoverageLaneWithDiscovery(config{suite: functionalCoverageSuite})
	if err != nil {
		t.Fatalf("resolveCoverageLaneWithDiscovery() error = %v", err)
	}
	if !reflect.DeepEqual(coverPackages, []string{modulePath + "/pkg/covered"}) {
		t.Fatalf("functional cover packages = %v", coverPackages)
	}
	if !reflect.DeepEqual(testPackages, []string{modulePath + "/tests/functional/runtime_api"}) {
		t.Fatalf("functional test packages = %v", testPackages)
	}
	if len(listCalls) != 1 {
		t.Fatalf("go list calls = %d, want one combined sweep", len(listCalls))
	}
	if got := listCalls[0].args[3:]; !reflect.DeepEqual(got, []string{"./pkg/...", "./tests/functional/..."}) {
		t.Fatalf("functional go list patterns = %v", got)
	}
}

func TestCanonicalizeBlocksCanBeReusedWithoutChangingEvaluation(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(t.TempDir())
	profilePath := writeCoverageProfile(t, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/service/factory.go:3.1,4.1 2 0",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
		modulePath + "/pkg/service/factory.go:3.1,4.1 2 4",
		"",
	}, "\n"))
	coverPackages := []string{modulePath + "/pkg/config", modulePath + "/pkg/service"}

	canonicalBlocks, err := canonicalizeCoverageProfileWithBlocks(profilePath, repoRoot, coverPackages)
	if err != nil {
		t.Fatalf("canonicalizeCoverageProfileWithBlocks() error = %v", err)
	}
	readBlocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		t.Fatalf("readCoverageProfileBlocks() error = %v", err)
	}
	if !reflect.DeepEqual(canonicalBlocks, readBlocks) {
		t.Fatalf("reused canonical blocks = %#v, reread blocks = %#v", canonicalBlocks, readBlocks)
	}

	baseline := map[string]struct{}{}
	fromProfile, fromProfileLine, err := evaluateCoverage("", "", profilePath, repoRoot, coverPackages, 80, baseline)
	if err != nil {
		t.Fatalf("evaluateCoverage() error = %v", err)
	}
	fromBlocks, fromBlocksLine, err := evaluateCoverageBlocks(canonicalBlocks, coverPackages, 80, baseline)
	if err != nil {
		t.Fatalf("evaluateCoverageBlocks() error = %v", err)
	}
	if !reflect.DeepEqual(fromBlocks, fromProfile) || fromBlocksLine != fromProfileLine {
		t.Fatalf("reused evaluation = %#v/%q, reread evaluation = %#v/%q", fromBlocks, fromBlocksLine, fromProfile, fromProfileLine)
	}
}
