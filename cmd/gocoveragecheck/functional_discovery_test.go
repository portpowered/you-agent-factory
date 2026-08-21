package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscoverFunctionalTestInventoryUsesGoListProcessResult(t *testing.T) {
	originalRunner := commandRunner
	defer func() { commandRunner = originalRunner }()

	packageA := modulePath + "/tests/functional/alpha"
	packageB := modulePath + "/tests/functional/beta"
	repoRoot := t.TempDir()
	packageADir := filepath.Join(repoRoot, "alpha")
	packageBDir := filepath.Join(repoRoot, "beta")
	if err := os.MkdirAll(packageADir, 0o755); err != nil {
		t.Fatalf("create package A fixture: %v", err)
	}
	if err := os.MkdirAll(packageBDir, 0o755); err != nil {
		t.Fatalf("create package B fixture: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageADir, "alpha_test.go", `package alpha

import "testing"

func TestAlpha(t *testing.T) {}
func TestAdded(*testing.T) {}
`)
	writeFunctionalDiscoveryFixture(t, packageBDir, "beta_external_test.go", `package beta_test

import "testing"

func TestBeta(t *testing.T) {}
`)
	var gotInvocation commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		gotInvocation = invocation
		return strings.Join([]string{
			marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: packageB, Dir: packageBDir, XTestGoFiles: []string{"beta_external_test.go"}}),
			marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: packageA, Dir: packageADir, TestGoFiles: []string{"alpha_test.go"}}),
		}, "\n"), "", nil
	}

	inventory, err := discoverFunctionalTestInventory([]string{packageB, packageA}, 2*time.Minute, true, 4, repoRoot)
	if err != nil {
		t.Fatalf("discoverFunctionalTestInventory() error = %v", err)
	}
	if !slices.Equal(inventory.Packages, []string{packageA, packageB}) {
		t.Fatalf("inventory packages = %v, want sorted package paths", inventory.Packages)
	}
	if !slices.Equal(inventory.Tests[packageA], []string{"TestAdded", "TestAlpha"}) || !slices.Equal(inventory.Tests[packageB], []string{"TestBeta"}) {
		t.Fatalf("inventory tests = %+v, want both package test lists", inventory.Tests)
	}
	if gotInvocation.name != "go" || gotInvocation.dir != repoRoot || !slices.Equal(gotInvocation.args, []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find", packageA, packageB}) {
		t.Fatalf("discovery invocation = %+v, want go list -json -find for sorted packages", gotInvocation)
	}
}

func TestDiscoverFunctionalTestInventoryWithPatternsFiltersUnrequestedPackages(t *testing.T) {
	packagePath := modulePath + "/tests/functional/selected"
	supportPath := modulePath + "/tests/functional/internal/support"
	repoRoot := t.TempDir()
	packageDir := filepath.Join(repoRoot, "selected")
	supportDir := filepath.Join(repoRoot, "support")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create selected package directory: %v", err)
	}
	if err := os.MkdirAll(supportDir, 0o755); err != nil {
		t.Fatalf("create support package directory: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageDir, "selected_test.go", `package selected

import "testing"

func TestSelected(t *testing.T) {}
`)

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	var gotInvocation commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		gotInvocation = invocation
		return strings.Join([]string{
			marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: supportPath, Dir: supportDir}),
			marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: packagePath, Dir: packageDir, TestGoFiles: []string{"selected_test.go"}}),
		}, "\n"), "", nil
	}

	inventory, err := discoverFunctionalTestInventoryWithPatterns([]string{"./tests/functional/..."}, []string{packagePath}, repoRoot)
	if err != nil {
		t.Fatalf("discoverFunctionalTestInventoryWithPatterns() error = %v", err)
	}
	if !slices.Equal(inventory.Packages, []string{packagePath}) || !slices.Equal(inventory.Tests[packagePath], []string{"TestSelected"}) {
		t.Fatalf("inventory = %+v, want selected package only", inventory)
	}
	if gotInvocation.name != "go" || gotInvocation.dir != repoRoot || !slices.Equal(gotInvocation.args, []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find", "./tests/functional/..."}) {
		t.Fatalf("discovery invocation = %+v, want one current-tree go list -find pattern", gotInvocation)
	}
}

func TestDiscoverFunctionalTestInventoryParallelizesConcretePackageBatches(t *testing.T) {
	const packageCount = 8
	repoRoot := t.TempDir()
	packages := make([]string, 0, packageCount)
	for index := 0; index < packageCount; index++ {
		packages = append(packages, modulePath+"/tests/functional/batch"+strconv.Itoa(index))
	}

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	var mu sync.Mutex
	var invocations [][]string
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		mu.Lock()
		invocations = append(invocations, append([]string(nil), invocation.args...))
		mu.Unlock()
		var output strings.Builder
		for _, packagePath := range invocation.args[4:] {
			output.WriteString(marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: packagePath, Dir: repoRoot}))
			output.WriteByte('\n')
		}
		return output.String(), "", nil
	}

	inventory, err := discoverFunctionalTestInventory(packages, 0, false, 4, repoRoot)
	if err != nil {
		t.Fatalf("discoverFunctionalTestInventory() error = %v", err)
	}
	if !slices.Equal(inventory.Packages, packages) || functionalTestCount(inventory) != 0 {
		t.Fatalf("inventory = %+v, want all %d concrete packages and no tests", inventory, packageCount)
	}
	if len(invocations) != 4 {
		t.Fatalf("go list invocations = %d, want four bounded batches: %+v", len(invocations), invocations)
	}
	seen := make(map[string]struct{}, packageCount)
	for _, invocation := range invocations {
		if !slices.Equal(invocation[:4], []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find"}) {
			t.Fatalf("go list invocation = %v, want metadata flags", invocation)
		}
		for _, packagePath := range invocation[4:] {
			seen[packagePath] = struct{}{}
		}
	}
	if len(seen) != packageCount {
		t.Fatalf("go list package coverage = %d, want %d: %v", len(seen), packageCount, seen)
	}
}

func TestResolveFunctionalTestPackagesWithMetadataUsesConcreteCurrentTreeMetadata(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	packagePath := modulePath + "/tests/functional/metadata"
	secondPackagePath := modulePath + "/tests/functional/metadata-second"
	repoRoot := t.TempDir()
	packageDir := filepath.Join(repoRoot, "tests", "functional", "metadata")
	secondPackageDir := filepath.Join(repoRoot, "tests", "functional", "metadata-second")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create package directory: %v", err)
	}
	if err := os.MkdirAll(secondPackageDir, 0o755); err != nil {
		t.Fatalf("create second package directory: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageDir, "metadata_test.go", `package metadata

import "testing"

func TestMetadata(t *testing.T) {}
`)
	writeFunctionalDiscoveryFixture(t, secondPackageDir, "second_test.go", `package metadatasecond

import "testing"

func TestSecond(t *testing.T) {}
`)

	var mu sync.Mutex
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		mu.Lock()
		invocations = append(invocations, invocation)
		mu.Unlock()
		if slices.Equal(invocation.args, []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find", packagePath, secondPackagePath}) {
			return strings.Join([]string{
				marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: packagePath, Dir: packageDir, TestGoFiles: []string{"metadata_test.go"}}),
				marshalFunctionalGoListPackage(t, functionalGoListPackage{ImportPath: secondPackagePath, Dir: secondPackageDir, TestGoFiles: []string{"second_test.go"}}),
			}, "\n"), "", nil
		}
		t.Fatalf("unexpected go list invocation: %+v", invocation)
		return "", "", nil
	}

	packages, listed, err := resolveFunctionalTestPackagesWithMetadata(config{suite: "functional"}, repoRoot)
	if err != nil {
		t.Fatalf("resolveFunctionalTestPackagesWithMetadata() error = %v", err)
	}
	if !slices.Equal(packages, []string{packagePath, secondPackagePath}) {
		t.Fatalf("packages = %v, want only runnable packages", packages)
	}
	if len(listed) != 2 || listed[0].ImportPath != packagePath || !slices.Equal(listed[0].TestGoFiles, []string{"metadata_test.go"}) || listed[1].ImportPath != secondPackagePath || !slices.Equal(listed[1].TestGoFiles, []string{"second_test.go"}) {
		t.Fatalf("listed metadata = %+v, want complete grouped build-selected metadata", listed)
	}
	if len(invocations) != 1 {
		t.Fatalf("go list invocations = %d, want one concrete current-tree metadata query: %+v", len(invocations), invocations)
	}
	for _, invocation := range invocations {
		if invocation.name != "go" || invocation.dir != repoRoot {
			t.Fatalf("go list invocation = %+v, want repo-root go command", invocation)
		}
	}
}

func TestResolveFunctionalTestPackagesWithMetadataListsCurrentTreeCandidatesOnce(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	const packageCount = 9
	repoRoot := t.TempDir()
	packages := make([]string, 0, packageCount)
	var packageDirs = make(map[string]string, packageCount)
	for index := 0; index < packageCount; index++ {
		name := "candidate" + strconv.Itoa(index)
		packagePath := modulePath + "/tests/functional/" + name
		packageDir := filepath.Join(repoRoot, "tests", "functional", name)
		if err := os.MkdirAll(packageDir, 0o755); err != nil {
			t.Fatalf("create package directory %s: %v", name, err)
		}
		writeFunctionalDiscoveryFixture(t, packageDir, "candidate_test.go", "package candidate\n")
		packages = append(packages, packagePath)
		packageDirs[packagePath] = packageDir
	}

	var mu sync.Mutex
	var invocations [][]string
	var runnerErr error
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		mu.Lock()
		defer mu.Unlock()
		invocations = append(invocations, append([]string(nil), invocation.args...))
		if invocation.name != "go" || invocation.dir != repoRoot || len(invocation.args) < 5 {
			runnerErr = fmt.Errorf("unexpected invocation: %+v", invocation)
			return "", "", runnerErr
		}
		var output strings.Builder
		for _, packagePath := range invocation.args[4:] {
			packageDir, ok := packageDirs[packagePath]
			if !ok {
				runnerErr = fmt.Errorf("unexpected package %q in invocation %+v", packagePath, invocation.args)
				return "", "", runnerErr
			}
			data, err := json.Marshal(functionalGoListPackage{
				ImportPath:  packagePath,
				Dir:         packageDir,
				TestGoFiles: []string{"candidate_test.go"},
			})
			if err != nil {
				runnerErr = err
				return "", "", runnerErr
			}
			output.Write(data)
			output.WriteByte('\n')
		}
		return output.String(), "", nil
	}

	gotPackages, listed, err := resolveFunctionalTestPackagesWithMetadata(config{suite: "functional"}, repoRoot)
	if err != nil {
		t.Fatalf("resolveFunctionalTestPackagesWithMetadata() error = %v", err)
	}
	if runnerErr != nil {
		t.Fatal(runnerErr)
	}
	if !slices.Equal(gotPackages, packages) || len(listed) != packageCount {
		t.Fatalf("packages/listed = %v/%d, want %v/%d", gotPackages, len(listed), packages, packageCount)
	}
	if len(invocations) != 1 {
		t.Fatalf("go list invocations = %d, want one concrete metadata query: %+v", len(invocations), invocations)
	}
	seen := make(map[string]struct{}, packageCount)
	for _, invocation := range invocations {
		if !slices.Equal(invocation[:4], []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find"}) {
			t.Fatalf("go list invocation = %v, want concrete metadata flags", invocation)
		}
		for _, packagePath := range invocation[4:] {
			seen[packagePath] = struct{}{}
		}
	}
	if len(seen) != packageCount {
		t.Fatalf("go list package coverage = %d, want %d: %v", len(seen), packageCount, seen)
	}
}

func TestResolveFunctionalTestPackagesWithMetadataSkipsBuildExcludedCandidates(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	packagePath := modulePath + "/tests/functional/visible"
	excludedPath := modulePath + "/tests/functional/excluded"
	repoRoot := t.TempDir()
	packageDir := filepath.Join(repoRoot, "tests", "functional", "visible")
	excludedDir := filepath.Join(repoRoot, "tests", "functional", "excluded")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create visible package directory: %v", err)
	}
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("create excluded package directory: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageDir, "visible_test.go", "package visible\n")
	writeFunctionalDiscoveryFixture(t, excludedDir, "excluded_functionallong_test.go", "package excluded\n")

	commandRunner = func(invocation commandInvocation) (string, string, error) {
		wantArgs := []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find", excludedPath, packagePath}
		if !slices.Equal(invocation.args, wantArgs) {
			t.Fatalf("go list invocation = %+v, want concrete current-tree candidates", invocation)
		}
		return strings.Join([]string{
			marshalFunctionalGoListPackage(t, functionalGoListPackage{
				ImportPath: packagePath,
				Dir:        packageDir,
			}),
			marshalFunctionalGoListPackage(t, functionalGoListPackage{
				ImportPath: excludedPath,
				Dir:        excludedDir,
				Error: &functionalGoListPackageError{
					Err: "build constraints exclude all Go files in " + excludedDir,
				},
			}),
		}, "\n"), "", nil
	}

	packages, listed, err := resolveFunctionalTestPackagesWithMetadata(config{suite: "functional"}, repoRoot)
	if err != nil {
		t.Fatalf("resolveFunctionalTestPackagesWithMetadata() error = %v", err)
	}
	if !slices.Equal(packages, []string{packagePath}) || len(listed) != 1 || listed[0].ImportPath != packagePath {
		t.Fatalf("packages/listed = %v/%+v, want only build-selected package", packages, listed)
	}
}

func TestFunctionalMetadataPatternBatchesBalanceCurrentTreePackages(t *testing.T) {
	packages := []string{
		modulePath + "/tests/functional/factory",
		modulePath + "/tests/functional/factory/one",
		modulePath + "/tests/functional/transport",
		modulePath + "/tests/functional/workers",
		modulePath + "/tests/functional/work",
	}
	batches := functionalMetadataPatternBatches(packages, 3)
	if len(batches) != 3 {
		t.Fatalf("metadata batches = %v, want three workers", batches)
	}
	seen := make(map[string]struct{})
	for _, batch := range batches {
		for _, pattern := range batch {
			if _, duplicate := seen[pattern]; duplicate {
				t.Fatalf("metadata pattern %q was assigned more than once", pattern)
			}
			seen[pattern] = struct{}{}
		}
	}
	wantPatterns := functionalMetadataPatterns(packages)
	if len(seen) != len(wantPatterns) {
		t.Fatalf("metadata patterns = %v, want every pattern exactly once from %v", seen, wantPatterns)
	}
	for _, pattern := range wantPatterns {
		if _, found := seen[pattern]; !found {
			t.Fatalf("metadata pattern %q was omitted from batches %v", pattern, batches)
		}
	}
}

func TestPrepareFunctionalCoverageRunUsesRetainedGoListMetadata(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/metadata-reuse"
	repoRoot := t.TempDir()
	packageDir := filepath.Join(repoRoot, "metadata-reuse")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create package directory: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageDir, "metadata_reuse_test.go", `package metadatareuse

import "testing"

func TestMetadataReuse(t *testing.T) {}
`)
	quarantinePath := filepath.Join(repoRoot, "functional-quarantine.json")
	if err := os.WriteFile(quarantinePath, []byte(`{"version":1,"suite":"functional","entries":[]}`), 0o644); err != nil {
		t.Fatalf("write quarantine fixture: %v", err)
	}

	commandRunner = func(invocation commandInvocation) (string, string, error) {
		t.Fatalf("unexpected subprocess after metadata reuse: %+v", invocation)
		return "", "", nil
	}
	var stdout bytes.Buffer
	stdoutWriter = &stdout
	started := startFunctionalDiscovery("current-tree")
	selection, selected, err := prepareFunctionalCoverageRunAfterStart(
		config{functionalQuarantine: quarantinePath, timeout: time.Minute},
		[]string{packagePath},
		"linux",
		4,
		repoRoot,
		[]functionalGoListPackage{{ImportPath: packagePath, Dir: packageDir, TestGoFiles: []string{"metadata_reuse_test.go"}}},
		started,
	)
	if err != nil {
		t.Fatalf("prepareFunctionalCoverageRunAfterStart() error = %v", err)
	}
	if !slices.Equal(selected, []string{packagePath}) || !slices.Equal(selection.Inventory.Tests[packagePath], []string{"TestMetadataReuse"}) {
		t.Fatalf("selection = %+v, want retained metadata inventory", selection)
	}
	if !strings.Contains(stdout.String(), "Functional discovery: end status=complete") || !strings.Contains(stdout.String(), "Functional gate: discovered-packages=1 discovered-tests=1") {
		t.Fatalf("lifecycle output = %q, want complete metadata-backed gate", stdout.String())
	}
}

func marshalFunctionalGoListPackage(t *testing.T, pkg functionalGoListPackage) string {
	t.Helper()
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("marshal go list package: %v", err)
	}
	return string(data)
}

func writeFunctionalDiscoveryFixture(t *testing.T, dir, name, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
		t.Fatalf("write discovery fixture %s: %v", name, err)
	}
}

func TestFunctionalDiscoveryFiltersDeclarationsByTestContract(t *testing.T) {
	packagePath := modulePath + "/tests/functional/fixture"
	repoRoot := functionalDiscoveryFixtureRoot(t)
	packageDir := filepath.Join(repoRoot, "cmd", "gocoveragecheck", "testdata", "functional-discovery")

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return marshalFunctionalGoListPackage(t, functionalGoListPackage{
			ImportPath:   packagePath,
			Dir:          packageDir,
			TestGoFiles:  []string{"in_package_test.go", "dot_import_test.go"},
			XTestGoFiles: []string{"external_test.go"},
		}), "", nil
	}

	inventory, err := discoverFunctionalTestInventory([]string{packagePath}, 0, false, 0, repoRoot)
	if err != nil {
		t.Fatalf("discoverFunctionalTestInventory() error = %v", err)
	}
	want := []string{"TestAlias", "TestDot", "TestExample", "TestExternal", "TestValid"}
	if !slices.Equal(inventory.Tests[packagePath], want) {
		t.Fatalf("discovered tests = %v, want %v", inventory.Tests[packagePath], want)
	}
}

func functionalDiscoveryFixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRootDir()
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func TestFunctionalDiscoveryFailsClosedForGoListAndFileErrors(t *testing.T) {
	packagePath := modulePath + "/tests/functional/fixture"
	fixtureDir := filepath.Join(functionalDiscoveryFixtureRoot(t), "cmd", "gocoveragecheck", "testdata", "functional-discovery")
	tests := []struct {
		name       string
		setup      func(*testing.T, string) string
		runnerErr  error
		stderr     string
		wantDetail string
	}{
		{
			name:       "go list failure",
			runnerErr:  errors.New("exit status 7"),
			stderr:     "go: no required module provides package " + packagePath,
			wantDetail: packagePath,
		},
		{
			name: "go list package error",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				return marshalFunctionalGoListPackage(t, functionalGoListPackage{
					ImportPath: packagePath,
					Dir:        root,
					Error: &functionalGoListPackageError{
						Pos: "broken.go:1:1",
						Err: "syntax error",
					},
				})
			},
			wantDetail: "syntax error",
		},
		{
			name: "missing listed file",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				return marshalFunctionalGoListPackage(t, functionalGoListPackage{
					ImportPath:  packagePath,
					Dir:         root,
					TestGoFiles: []string{"missing_test.go"},
				})
			},
			wantDetail: "missing_test.go",
		},
		{
			name: "parse failure",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				return marshalFunctionalGoListPackage(t, functionalGoListPackage{
					ImportPath:  packagePath,
					Dir:         fixtureDir,
					TestGoFiles: []string{"broken_test.txt"},
				})
			},
			wantDetail: "broken_test.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			originalRunner := commandRunner
			t.Cleanup(func() { commandRunner = originalRunner })
			commandRunner = func(commandInvocation) (string, string, error) {
				if test.runnerErr != nil {
					return "", test.stderr, test.runnerErr
				}
				return test.setup(t, root), "", nil
			}

			_, err := discoverFunctionalTestInventory([]string{packagePath}, 0, false, 0, root)
			if err == nil || !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf("discoverFunctionalTestInventory() error = %v, want detail %q", err, test.wantDetail)
			}
		})
	}
}

func TestFunctionalQuarantineRuntimeVerificationScopesListingToTestSelectors(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	packageLevel := modulePath + "/tests/functional/package-level"
	testLevel := modulePath + "/tests/functional/test-level"
	unquarantined := modulePath + "/tests/functional/unquarantined"
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		return strings.Join([]string{
			marshalFunctionalListEvent(goTestListEvent{Action: "start", Package: testLevel}),
			marshalFunctionalListEvent(goTestListEvent{Action: "output", Package: testLevel, Output: "TestRuntimeSelector\n"}),
			marshalFunctionalListEvent(goTestListEvent{Action: timingOutcomePass, Package: testLevel}),
		}, "\n"), "", nil
	}

	manifest := functionalQuarantine{Entries: []functionalQuarantineEntry{
		{Package: packageLevel, Bucket: functionalBucketEnvironment, Reason: "package precondition"},
		{Package: testLevel, Test: "TestRuntimeSelector", Bucket: functionalBucketEnvironment, Reason: "test precondition"},
	}}
	if err := verifyFunctionalTestQuarantineSelectors(manifest, 2*time.Minute, true, 4, t.TempDir()); err != nil {
		t.Fatalf("verifyFunctionalTestQuarantineSelectors() error = %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("runtime listing invocations = %d, want one for test-level selectors only", len(invocations))
	}
	invocation := invocations[0]
	wantArgs := []string{"test", "-vet=off", "-list=^Test", "-json", "-p=4", "-count=1", "-short", "-timeout=2m0s", testLevel}
	if invocation.name != "go" || !slices.Equal(invocation.args, wantArgs) {
		t.Fatalf("runtime listing invocation = %+v, want only test-level package %q with retained list flags", invocation, testLevel)
	}
	if strings.Contains(strings.Join(invocation.args, " "), packageLevel) || strings.Contains(strings.Join(invocation.args, " "), unquarantined) {
		t.Fatalf("runtime listing included a package without a test-level selector: %+v", invocation.args)
	}
}

func TestPrepareFunctionalCoverageRunReportsDiscoveryLifecycle(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/discovery-output"
	repoRoot := t.TempDir()
	packageDir := filepath.Join(repoRoot, "discovery-output")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create package directory: %v", err)
	}
	writeFunctionalDiscoveryFixture(t, packageDir, "discovery_test.go", `package discoveryoutput

import "testing"

func TestOne(t *testing.T) {}
func TestTwo(t *testing.T) {}
`)
	quarantinePath := filepath.Join(repoRoot, "functional-quarantine.json")
	quarantineData := []byte(`{"version":1,"suite":"functional","entries":[]}`)
	if err := os.WriteFile(quarantinePath, quarantineData, 0o644); err != nil {
		t.Fatalf("write quarantine fixture: %v", err)
	}

	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if invocation.name != "go" || !slices.Equal(invocation.args, []string{"list", functionalGoListErrorFlag, functionalGoListJSONFields, "-find", packagePath}) {
			t.Fatalf("discovery invocation = %+v, want go list -json -find", invocation)
		}
		return marshalFunctionalGoListPackage(t, functionalGoListPackage{
			ImportPath:  packagePath,
			Dir:         packageDir,
			TestGoFiles: []string{"discovery_test.go"},
		}), "", nil
	}

	var stdout bytes.Buffer
	stdoutWriter = &stdout
	_, selected, err := prepareFunctionalCoverageRun(
		config{functionalQuarantine: quarantinePath, timeout: time.Minute},
		[]string{packagePath},
		"linux",
		4,
		repoRoot,
	)
	if err != nil {
		t.Fatalf("prepareFunctionalCoverageRun() error = %v", err)
	}
	if !slices.Equal(selected, []string{packagePath}) {
		t.Fatalf("selected packages = %v, want %q", selected, packagePath)
	}

	output := stdout.String()
	begin := strings.Index(output, "Functional discovery: begin")
	end := strings.Index(output, "Functional discovery: end")
	gate := strings.Index(output, "Functional gate:")
	if begin < 0 || end < 0 || gate < 0 || !(begin < end && end < gate) {
		t.Fatalf("discovery lifecycle output ordering is wrong: %q", output)
	}
	if !strings.Contains(output, "Functional discovery: begin requested-packages=1") {
		t.Fatalf("discovery begin output = %q, want requested package count", output)
	}
	if !strings.Contains(output, "Functional discovery: end status=complete elapsed=") ||
		!strings.Contains(output, "discovered-packages=1 discovered-tests=2") {
		t.Fatalf("discovery end output = %q, want terminal counts and elapsed duration", output)
	}
	if !strings.Contains(output, "Functional gate: discovered-packages=1 discovered-tests=2") {
		t.Fatalf("functional gate output = %q, want unchanged discovered counts", output)
	}
}

func TestPrepareFunctionalCoverageRunReportsFailedDiscoveryEnd(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/discovery-output"
	repoRoot := t.TempDir()
	quarantinePath := filepath.Join(repoRoot, "functional-quarantine.json")
	if err := os.WriteFile(quarantinePath, []byte(`{"version":1,"suite":"functional","entries":[]}`), 0o644); err != nil {
		t.Fatalf("write quarantine fixture: %v", err)
	}
	commandRunner = func(commandInvocation) (string, string, error) {
		return "", "go: malformed discovery fixture", errors.New("exit status 1")
	}

	var stdout bytes.Buffer
	stdoutWriter = &stdout
	_, _, err := prepareFunctionalCoverageRun(
		config{functionalQuarantine: quarantinePath, timeout: time.Minute},
		[]string{packagePath},
		"linux",
		4,
		repoRoot,
	)
	if err == nil || !strings.Contains(err.Error(), "go list") {
		t.Fatalf("prepareFunctionalCoverageRun() error = %v, want fail-closed go list diagnostic", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Functional discovery: begin requested-packages=1") ||
		!strings.Contains(output, "Functional discovery: end status=failed elapsed=") ||
		strings.Contains(output, "Functional gate:") {
		t.Fatalf("failed discovery output = %q, want begin, failed end, and no gate", output)
	}
}

func TestFunctionalQuarantineRuntimeVerificationRejectsStaleSelector(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	packagePath := modulePath + "/tests/functional/test-level"
	commandRunner = func(commandInvocation commandInvocation) (string, string, error) {
		return strings.Join([]string{
			marshalFunctionalListEvent(goTestListEvent{Action: "start", Package: packagePath}),
			marshalFunctionalListEvent(goTestListEvent{Action: "output", Package: packagePath, Output: "TestCurrent\n"}),
			marshalFunctionalListEvent(goTestListEvent{Action: timingOutcomePass, Package: packagePath}),
		}, "\n"), "", nil
	}

	manifest := functionalQuarantine{Entries: []functionalQuarantineEntry{{
		Package: packagePath, Test: "TestStale", Bucket: functionalBucketEnvironment, Reason: "stale selector fixture",
	}}}
	err := verifyFunctionalTestQuarantineSelectors(manifest, time.Minute, false, 1, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `selector "TestStale" is not discoverable in package "`+packagePath+`"`) {
		t.Fatalf("verifyFunctionalTestQuarantineSelectors() error = %v, want unchanged stale-selector diagnostic", err)
	}
}

func TestFunctionalQuarantineRuntimeVerificationFailsClosedOnListErrors(t *testing.T) {
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })

	packagePath := modulePath + "/tests/functional/test-level"
	manifest := functionalQuarantine{Entries: []functionalQuarantineEntry{{
		Package: packagePath, Test: "TestCurrent", Bucket: functionalBucketEnvironment, Reason: "runtime list failure fixture",
	}}}
	for _, test := range []struct {
		name      string
		stdout    string
		stderr    string
		runnerErr error
		want      string
	}{
		{name: "command failure", stderr: "compile failed", runnerErr: errors.New("exit status 1"), want: "runtime go test list"},
		{name: "malformed event", stdout: "{not json", want: "decode go test list event"},
		{name: "missing terminal event", stdout: marshalFunctionalListEvent(goTestListEvent{Action: "output", Package: packagePath, Output: "TestCurrent\n"}), want: "terminal runtime list event"},
	} {
		t.Run(test.name, func(t *testing.T) {
			commandRunner = func(commandInvocation commandInvocation) (string, string, error) {
				return test.stdout, test.stderr, test.runnerErr
			}
			err := verifyFunctionalTestQuarantineSelectors(manifest, time.Minute, false, 1, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("verifyFunctionalTestQuarantineSelectors() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
