package main

import (
	"encoding/json"
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
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath:  modulePath + "/pkg/covered",
				GoFiles:     []string{"covered.go", "other.go"},
				TestGoFiles: []string{"covered_test.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/pkg/testless",
				GoFiles:    []string{"testless.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/pkg/transports/http/client",
				GoFiles:    []string{"client.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/internal/testutil/runtimefixtures",
				GoFiles:    []string{"fixture.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/cmd/factory",
				GoFiles:    []string{"main.go"},
			}),
		}, "\n"), "", nil
	}

	discovery, coverPackages, testPackages, err := resolveCoverageLaneWithDiscovery(config{suite: unitCoverageSuite})
	if err != nil {
		t.Fatalf("resolveCoverageLaneWithDiscovery() error = %v", err)
	}
	wantCover := []string{modulePath + "/pkg/covered", modulePath + "/pkg/testless"}
	wantTest := []string{modulePath + "/pkg/covered"}
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
	if got := listCalls[0].args[1:]; !reflect.DeepEqual(got, []string{"-e", coverageUnitGoListJSONFields, "-find", "./pkg/..."}) {
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

func marshalCoverageGoListPackage(t *testing.T, pkg coverageGoListPackage) string {
	t.Helper()
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("marshal coverage go list package: %v", err)
	}
	return string(data)
}

func TestResolveCoverageLaneSelectsBuildAwareUnitTestPackages(t *testing.T) {
	originalCommandRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalCommandRunner })

	var listedInvocation commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		listedInvocation = invocation
		return strings.Join([]string{
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath:  modulePath + "/pkg/internal-tests",
				GoFiles:     []string{"value.go"},
				TestGoFiles: []string{"value_test.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath:   modulePath + "/pkg/external-tests",
				GoFiles:      []string{"value.go"},
				XTestGoFiles: []string{"value_external_test.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/pkg/testless",
				GoFiles:    []string{"value.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath: modulePath + "/pkg/generated",
				GoFiles:    []string{"generated.go"},
			}),
			marshalCoverageGoListPackage(t, coverageGoListPackage{
				ImportPath:   modulePath + "/pkg/platform",
				GoFiles:      []string{"value.go"},
				TestGoFiles:  []string{"value_windows_test.go"},
				XTestGoFiles: []string{},
			}),
		}, "\n"), "", nil
	}

	discovery, coverPackages, testPackages, err := resolveCoverageLaneWithDiscoveryForOS(config{suite: unitCoverageSuite}, "windows")
	if err != nil {
		t.Fatalf("resolveCoverageLaneWithDiscoveryForOS() error = %v", err)
	}
	wantCoverage := []string{
		modulePath + "/pkg/external-tests",
		modulePath + "/pkg/generated",
		modulePath + "/pkg/internal-tests",
		modulePath + "/pkg/platform",
		modulePath + "/pkg/testless",
	}
	if !reflect.DeepEqual(coverPackages, wantCoverage) {
		t.Fatalf("coverage packages = %v, want unchanged measurement universe %v", coverPackages, wantCoverage)
	}
	wantTests := []string{
		modulePath + "/pkg/external-tests",
		modulePath + "/pkg/internal-tests",
		modulePath + "/pkg/platform",
	}
	if !reflect.DeepEqual(testPackages, wantTests) {
		t.Fatalf("execution packages = %v, want only build-selected test packages %v", testPackages, wantTests)
	}
	if !reflect.DeepEqual(discovery.allPackages, wantCoverage) {
		t.Fatalf("package universe = %v, want all listed packages %v", discovery.allPackages, wantCoverage)
	}
	if got := listedInvocation.env; !environmentContains(got, "GOOS=windows") {
		t.Fatalf("go list environment = %v, want target GOOS=windows", got)
	}
}

func TestBuildAwareUnitPackageDiscoveryFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantDetail string
	}{
		{name: "empty metadata", output: "", wantDetail: "no package metadata"},
		{name: "malformed metadata", output: "not-json", wantDetail: "decode go list metadata"},
		{name: "missing import path", output: `{"GoFiles":["value.go"]}`, wantDetail: "without an import path"},
		{name: "incomplete metadata", output: `{"ImportPath":"` + modulePath + `/pkg/incomplete","Incomplete":true}`, wantDetail: "incomplete metadata"},
		{name: "package error", output: `{"ImportPath":"` + modulePath + `/pkg/broken","Error":{"Err":"syntax error"}}`, wantDetail: "syntax error"},
		{
			name: "contradictory duplicate metadata",
			output: strings.Join([]string{
				`{"ImportPath":"` + modulePath + `/pkg/duplicate","GoFiles":["value.go"],"TestGoFiles":["value_test.go"]}`,
				`{"ImportPath":"` + modulePath + `/pkg/duplicate","GoFiles":["value.go"],"TestGoFiles":["other_test.go"]}`,
			}, "\n"),
			wantDetail: "contradictory metadata",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalCommandRunner := commandRunner
			t.Cleanup(func() { commandRunner = originalCommandRunner })
			commandRunner = func(commandInvocation) (string, string, error) {
				return test.output, "", nil
			}

			_, err := listUnitGoPackageListings([]string{"./pkg/..."}, "linux")
			if err == nil || !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf("listUnitGoPackageListings() error = %v, want detail %q", err, test.wantDetail)
			}
		})
	}
}

func environmentContains(environment []string, expected string) bool {
	for _, entry := range environment {
		if entry == expected {
			return true
		}
	}
	return false
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
