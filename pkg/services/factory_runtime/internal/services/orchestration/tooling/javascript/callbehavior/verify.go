package callbehavior

import (
	"fmt"
	"sort"
	"strings"
)

// ForbiddenRootGlobals are script-visible roots that must not appear in the
// call-behavior inventory.
var ForbiddenRootGlobals = []string{"context", "orchestrator"}

// VerifyProjectedInstalledCallBehavior projects the canonical inventory and fails
// when paths are missing, duplicated, unexpected, unsorted, or forbidden.
func VerifyProjectedInstalledCallBehavior() error {
	return VerifyInventory(ProjectInstalledCallBehavior())
}

// VerifyInventory fails when record paths are missing, duplicated, unexpected,
// unsorted, or include forbidden globals.
func VerifyInventory(inv Inventory) error {
	if inv.FormatVersion != FormatVersion {
		return fmt.Errorf("formatVersion = %q, want %q", inv.FormatVersion, FormatVersion)
	}
	if err := verifyNoDuplicateRecordPaths(inv.Records); err != nil {
		return err
	}
	if err := verifyForbiddenGlobalsAbsent(inv.Records); err != nil {
		return err
	}
	if err := verifyExpectedRecordPaths(inv.Records); err != nil {
		return err
	}
	return verifyRecordsSortedByPath(inv.Records)
}

func verifyRecordsSortedByPath(records []CallBehaviorRecord) error {
	for i := 1; i < len(records); i++ {
		prev := records[i-1].Path
		curr := records[i].Path
		if prev > curr {
			return fmt.Errorf("records not sorted by path at index %d: %q > %q", i, prev, curr)
		}
	}
	return nil
}

func verifyNoDuplicateRecordPaths(records []CallBehaviorRecord) error {
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if _, exists := seen[record.Path]; exists {
			return fmt.Errorf("duplicate record path %q", record.Path)
		}
		seen[record.Path] = struct{}{}
	}
	return nil
}

func verifyExpectedRecordPaths(records []CallBehaviorRecord) error {
	expected := ExpectedInstalledPaths()
	got := recordPaths(records)
	return compareRecordPathSets(expected, got)
}

func compareRecordPathSets(expected, got []string) error {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, path := range expected {
		expectedSet[path] = struct{}{}
	}

	for _, path := range got {
		if _, ok := expectedSet[path]; !ok {
			return fmt.Errorf("unexpected record path %q", path)
		}
		delete(expectedSet, path)
	}

	missing := make([]string, 0, len(expectedSet))
	for path := range expectedSet {
		missing = append(missing, path)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("missing record path %q", missing[0])
	}
	return nil
}

func verifyForbiddenGlobalsAbsent(records []CallBehaviorRecord) error {
	for _, record := range records {
		if isForbiddenRecordPath(record.Path) {
			return fmt.Errorf("forbidden record path %q", record.Path)
		}
	}
	return nil
}

func isForbiddenRecordPath(path string) bool {
	for _, forbidden := range ForbiddenRootGlobals {
		if path == forbidden || strings.HasPrefix(path, forbidden+".") {
			return true
		}
	}
	return false
}

func recordPaths(records []CallBehaviorRecord) []string {
	paths := make([]string, len(records))
	for i, record := range records {
		paths[i] = record.Path
	}
	return paths
}
