package functionaltestmetadata_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

func TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	functionalRoot := filepath.Join(repoRoot, "tests", "functional")
	baselinePath := filepath.Join(repoRoot, "docs", "internal", "baselines", "functional-undocumented-tests.json")

	records, err := functionaltestmetadata.Parse(functionalRoot)
	if err != nil {
		t.Fatalf("Parse(%s) error = %v", functionalRoot, err)
	}
	baseline, err := functionaltestmetadata.LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline(%s) error = %v", baselinePath, err)
	}

	current := functionaltestmetadata.BaselineFromRecords(records)
	if len(current.Entries) != len(baseline.Entries) {
		t.Fatalf("committed baseline has %d entries, parser discovered %d undocumented customer tests", len(baseline.Entries), len(current.Entries))
	}
	if err := functionaltestmetadata.CheckAgainstBaseline(records, baseline); err != nil {
		t.Fatalf("CheckAgainstBaseline(repo) error = %v", err)
	}
	// Exact match: no stale committed entries either for the initial freeze.
	if err := functionaltestmetadata.ValidateBaselineUpdate(current, baseline); err != nil {
		t.Fatalf("committed baseline drifted from parser discovery: %v", err)
	}
	if err := functionaltestmetadata.ValidateBaselineUpdate(baseline, current); err != nil {
		t.Fatalf("parser discovery drifted from committed baseline: %v", err)
	}
}
