package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	functionalQuarantineVersion = 1
	functionalSuiteName         = "functional"
	functionalBucketEnvironment = "ENVIRONMENT-DEPENDENT"
	functionalBucketFailure     = "GENUINELY FAILING"
)

var functionalTestNamePattern = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

type functionalQuarantine struct {
	Version int                         `json:"version"`
	Suite   string                      `json:"suite"`
	Entries []functionalQuarantineEntry `json:"entries"`
}

type functionalQuarantineEntry struct {
	Package  string `json:"package"`
	Test     string `json:"test,omitempty"`
	Bucket   string `json:"bucket"`
	Reason   string `json:"reason"`
	FollowUp string `json:"followUp,omitempty"`
}

type functionalTestInventory struct {
	Packages []string
	Tests    map[string][]string
}

type functionalCoverageSelection struct {
	Inventory                functionalTestInventory
	Groups                   []coverageRunGroup
	QuarantinedPackageCount  int
	QuarantinedTestSelectors int
	PackageExcludedTestCount int
	SelectedPackageCount     int
	SelectedTestCount        int
}

type coverageRunGroup struct {
	Packages   []string
	RunPattern string
}

type goTestListEvent struct {
	Action  string `json:"Action"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func resolveFunctionalCoverageSelection(path string, packages []string, timeout time.Duration, short bool, jobs int, repoRoot string) (functionalCoverageSelection, error) {
	manifest, err := readFunctionalQuarantineFile(path)
	if err != nil {
		return functionalCoverageSelection{}, err
	}
	inventory, err := discoverFunctionalTestInventory(packages, timeout, short, jobs, repoRoot)
	if err != nil {
		return functionalCoverageSelection{}, err
	}
	if err := validateFunctionalQuarantine(manifest, inventory); err != nil {
		return functionalCoverageSelection{}, err
	}
	return buildFunctionalCoverageSelection(manifest, inventory)
}

func readFunctionalQuarantineFile(path string) (functionalQuarantine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return functionalQuarantine{}, fmt.Errorf("read functional quarantine %q: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest functionalQuarantine
	if err := decoder.Decode(&manifest); err != nil {
		return functionalQuarantine{}, fmt.Errorf("parse functional quarantine %q: %w", path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return functionalQuarantine{}, fmt.Errorf("parse functional quarantine %q: %w", path, err)
	}
	return manifest, nil
}

func discoverFunctionalTestInventory(packages []string, timeout time.Duration, short bool, jobs int, repoRoot string) (functionalTestInventory, error) {
	packages = sortedUniqueStrings(packages)
	if len(packages) == 0 {
		return functionalTestInventory{}, errors.New("discover functional tests: no packages were selected")
	}
	args := []string{"test", "-list=^Test", "-json", fmt.Sprintf("-p=%d", maxFunctionalDiscoveryJobs(jobs)), "-count=1"}
	if short {
		args = append(args, "-short")
	}
	args = append(args, fmt.Sprintf("-timeout=%s", timeout))
	args = append(args, packages...)
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  repoRoot,
	})
	if err != nil {
		detail := mergeGoTestFailureDetail(stderr, stdout)
		if detail != "" {
			return functionalTestInventory{}, fmt.Errorf("discover functional tests: %w\n%s", err, detail)
		}
		return functionalTestInventory{}, fmt.Errorf("discover functional tests: %w", err)
	}

	inventory := functionalTestInventory{
		Packages: packages,
		Tests:    make(map[string][]string, len(packages)),
	}
	for _, packagePath := range packages {
		inventory.Tests[packagePath] = nil
	}
	seenPackages := make(map[string]struct{}, len(packages))
	if err := parseFunctionalTestList(stdout, inventory, seenPackages); err != nil {
		return functionalTestInventory{}, err
	}
	for _, packagePath := range packages {
		if _, ok := seenPackages[packagePath]; !ok {
			return functionalTestInventory{}, fmt.Errorf("discover functional tests: package %q did not report a terminal list event", packagePath)
		}
		tests := sortedUniqueStrings(inventory.Tests[packagePath])
		inventory.Tests[packagePath] = tests
	}
	return inventory, nil
}

func maxFunctionalDiscoveryJobs(jobs int) int {
	if jobs < 1 {
		return 1
	}
	return jobs
}

func parseFunctionalTestList(output string, inventory functionalTestInventory, seenPackages map[string]struct{}) error {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event goTestListEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("discover functional tests: decode go test list event: %w", err)
		}
		if event.Package == "" {
			continue
		}
		if _, known := inventory.Tests[event.Package]; !known {
			return fmt.Errorf("discover functional tests: go test reported unexpected package %q", event.Package)
		}
		seenPackages[event.Package] = struct{}{}
		if event.Test != "" {
			if err := addFunctionalTestName(inventory.Tests, event.Package, event.Test); err != nil {
				return err
			}
		}
		for _, outputLine := range strings.Split(event.Output, "\n") {
			name := strings.TrimSpace(outputLine)
			if name == "" {
				continue
			}
			if functionalTestNamePattern.MatchString(name) {
				if err := addFunctionalTestName(inventory.Tests, event.Package, name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addFunctionalTestName(tests map[string][]string, packagePath, name string) error {
	if !functionalTestNamePattern.MatchString(name) {
		return fmt.Errorf("discover functional tests: package %q reported invalid top-level test name %q", packagePath, name)
	}
	tests[packagePath] = append(tests[packagePath], name)
	return nil
}

func validateFunctionalQuarantine(manifest functionalQuarantine, inventory functionalTestInventory) error {
	if manifest.Version != functionalQuarantineVersion {
		return fmt.Errorf("validate functional quarantine: version %d is unsupported; expected %d", manifest.Version, functionalQuarantineVersion)
	}
	if manifest.Suite != functionalSuiteName {
		return fmt.Errorf("validate functional quarantine: suite %q is unsupported; expected %q", manifest.Suite, functionalSuiteName)
	}
	knownPackages := make(map[string]struct{}, len(inventory.Packages))
	for _, packagePath := range inventory.Packages {
		knownPackages[packagePath] = struct{}{}
	}
	seen := make(map[string]struct{}, len(manifest.Entries))
	packageSelectors := make(map[string]struct{})
	previous := ""
	for index, entry := range manifest.Entries {
		if err := validateFunctionalQuarantineEntry(entry, index, knownPackages, inventory.Tests); err != nil {
			return err
		}
		key := functionalSelectorKey(entry.Package, entry.Test)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("validate functional quarantine: duplicate selector %q", functionalSelectorDisplay(entry))
		}
		if previous != "" && key < previous {
			return fmt.Errorf("validate functional quarantine: entries must be sorted by package and test; %q appears after %q", key, previous)
		}
		if entry.Test == "" {
			packageSelectors[entry.Package] = struct{}{}
		} else if _, packageSelected := packageSelectors[entry.Package]; packageSelected {
			return fmt.Errorf("validate functional quarantine: package selector %q overlaps test selector %q; remove the test entry or quarantine the package only", entry.Package, functionalSelectorDisplay(entry))
		}
		seen[key] = struct{}{}
		previous = key
	}
	return nil
}

func validateFunctionalQuarantineEntry(entry functionalQuarantineEntry, index int, knownPackages map[string]struct{}, tests map[string][]string) error {
	if strings.TrimSpace(entry.Package) == "" {
		return fmt.Errorf("validate functional quarantine: entries[%d].package is required", index)
	}
	if _, known := knownPackages[entry.Package]; !known {
		return fmt.Errorf("validate functional quarantine: selector package %q is not discoverable in the functional package set", entry.Package)
	}
	if entry.Test != "" {
		if !functionalTestNamePattern.MatchString(entry.Test) {
			return fmt.Errorf("validate functional quarantine: selector %q has invalid top-level test name", functionalSelectorDisplay(entry))
		}
		if !slices.Contains(tests[entry.Package], entry.Test) {
			return fmt.Errorf("validate functional quarantine: selector %q is not discoverable in package %q", entry.Test, entry.Package)
		}
	}
	if entry.Bucket != functionalBucketEnvironment && entry.Bucket != functionalBucketFailure {
		return fmt.Errorf("validate functional quarantine: selector %q has unsupported bucket %q; expected %s or %s", functionalSelectorDisplay(entry), entry.Bucket, functionalBucketEnvironment, functionalBucketFailure)
	}
	if strings.TrimSpace(entry.Reason) == "" {
		return fmt.Errorf("validate functional quarantine: selector %q requires a non-empty reason", functionalSelectorDisplay(entry))
	}
	if entry.Bucket == functionalBucketFailure && strings.TrimSpace(entry.FollowUp) == "" {
		return fmt.Errorf("validate functional quarantine: genuinely failing selector %q requires a non-empty followUp", functionalSelectorDisplay(entry))
	}
	return nil
}

func buildFunctionalCoverageSelection(manifest functionalQuarantine, inventory functionalTestInventory) (functionalCoverageSelection, error) {
	packageExcluded := make(map[string]struct{})
	testExcluded := make(map[string]map[string]struct{})
	for _, entry := range manifest.Entries {
		if entry.Test == "" {
			packageExcluded[entry.Package] = struct{}{}
			continue
		}
		if testExcluded[entry.Package] == nil {
			testExcluded[entry.Package] = make(map[string]struct{})
		}
		testExcluded[entry.Package][entry.Test] = struct{}{}
	}

	selection := functionalCoverageSelection{
		Inventory:                inventory,
		QuarantinedPackageCount:  len(packageExcluded),
		QuarantinedTestSelectors: len(manifest.Entries) - len(packageExcluded),
	}
	groups := make(map[string][]string)
	for _, packagePath := range inventory.Packages {
		if _, excluded := packageExcluded[packagePath]; excluded {
			selection.PackageExcludedTestCount += len(inventory.Tests[packagePath])
			continue
		}
		excludedTests := testExcluded[packagePath]
		selectedTests := make([]string, 0, len(inventory.Tests[packagePath]))
		for _, testName := range inventory.Tests[packagePath] {
			if _, excluded := excludedTests[testName]; excluded {
				continue
			}
			selectedTests = append(selectedTests, testName)
		}
		if len(inventory.Tests[packagePath]) > 0 && len(selectedTests) == 0 {
			continue
		}
		pattern := ""
		if len(excludedTests) > 0 {
			pattern = exactFunctionalTestRunPattern(selectedTests)
		}
		groups[pattern] = append(groups[pattern], packagePath)
		selection.SelectedTestCount += len(selectedTests)
	}

	patterns := make([]string, 0, len(groups))
	for pattern := range groups {
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	for _, pattern := range patterns {
		packagePaths := sortedUniqueStrings(groups[pattern])
		selection.Groups = append(selection.Groups, coverageRunGroup{Packages: packagePaths, RunPattern: pattern})
		selection.SelectedPackageCount += len(packagePaths)
	}
	if len(selection.Groups) == 0 {
		return functionalCoverageSelection{}, errors.New("build functional coverage selection: quarantine removes every runnable functional test; retain at least one selected test")
	}
	return selection, nil
}

func functionalTestCount(inventory functionalTestInventory) int {
	count := 0
	for _, tests := range inventory.Tests {
		count += len(tests)
	}
	return count
}

func selectedFunctionalPackages(selection functionalCoverageSelection) []string {
	packages := make([]string, 0, selection.SelectedPackageCount)
	for _, group := range selection.Groups {
		packages = append(packages, group.Packages...)
	}
	return sortedUniqueStrings(packages)
}

func exactFunctionalTestRunPattern(testNames []string) string {
	quoted := make([]string, 0, len(testNames))
	for _, name := range sortedUniqueStrings(testNames) {
		quoted = append(quoted, regexp.QuoteMeta(name))
	}
	return "^(?:" + strings.Join(quoted, "|") + ")$"
}

func functionalSelectorKey(packagePath, testName string) string {
	return packagePath + "\x00" + testName
}

func functionalSelectorDisplay(entry functionalQuarantineEntry) string {
	if entry.Test == "" {
		return entry.Package
	}
	return entry.Package + "#" + entry.Test
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
