package functionaltestmetadata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

func TestCheckAgainstBaselineAcceptsIdenticalMatch(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/doc_test.go": `package smoke

import "testing"

// TestDocumented is covered by Go doc.
func TestDocumented(t *testing.T) {}
`,
		"smoke/undoc_test.go": `package smoke

import "testing"

func TestLegacyUndocumented(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	baseline := functionaltestmetadata.BaselineFromRecords(records)
	if len(baseline.Entries) != 1 {
		t.Fatalf("baseline entries = %d, want 1: %#v", len(baseline.Entries), baseline.Entries)
	}
	if err := functionaltestmetadata.CheckAgainstBaseline(records, baseline); err != nil {
		t.Fatalf("CheckAgainstBaseline() identical match error = %v", err)
	}
}

func TestCheckAgainstBaselineAcceptsDeletionSubset(t *testing.T) {
	t.Parallel()

	previousRoot := writeTree(t, map[string]string{
		"smoke/a_test.go": `package smoke

import "testing"

func TestAlpha(t *testing.T) {}

func TestBeta(t *testing.T) {}
`,
	})
	previousRecords, err := functionaltestmetadata.Parse(previousRoot)
	if err != nil {
		t.Fatalf("Parse(previous) error = %v", err)
	}
	baseline := functionaltestmetadata.BaselineFromRecords(previousRecords)
	if len(baseline.Entries) != 2 {
		t.Fatalf("previous baseline entries = %d, want 2", len(baseline.Entries))
	}

	// Suite and baseline both shrink by deleting TestBeta (deletion success).
	shrunkRoot := writeTree(t, map[string]string{
		"smoke/a_test.go": `package smoke

import "testing"

func TestAlpha(t *testing.T) {}
`,
	})
	shrunkRecords, err := functionaltestmetadata.Parse(shrunkRoot)
	if err != nil {
		t.Fatalf("Parse(shrunk) error = %v", err)
	}
	shrunkBaseline := functionaltestmetadata.Baseline{
		Version: functionaltestmetadata.BaselineVersion,
		Stage:   functionaltestmetadata.BaselineStage,
		Entries: []functionaltestmetadata.BaselineEntry{
			{File: "smoke/a_test.go", Name: "TestAlpha"},
		},
	}
	if err := functionaltestmetadata.ValidateBaselineUpdate(baseline, shrunkBaseline); err != nil {
		t.Fatalf("ValidateBaselineUpdate(deletion) error = %v", err)
	}
	if err := functionaltestmetadata.CheckAgainstBaseline(shrunkRecords, shrunkBaseline); err != nil {
		t.Fatalf("CheckAgainstBaseline(deletion) error = %v", err)
	}

	// Undocumented set may also be a strict subset while a stale baseline entry remains.
	if err := functionaltestmetadata.CheckAgainstBaseline(shrunkRecords, baseline); err != nil {
		t.Fatalf("CheckAgainstBaseline(subset with stale baseline entry) error = %v", err)
	}
}

func TestCheckAgainstBaselineRejectsNewUndocumentedCustomerTest(t *testing.T) {
	t.Parallel()

	baseline := functionaltestmetadata.Baseline{
		Version: functionaltestmetadata.BaselineVersion,
		Stage:   functionaltestmetadata.BaselineStage,
		Entries: []functionaltestmetadata.BaselineEntry{
			{File: "smoke/legacy_test.go", Name: "TestLegacy"},
		},
	}

	root := writeTree(t, map[string]string{
		"smoke/legacy_test.go": `package smoke

import "testing"

func TestLegacy(t *testing.T) {}
`,
		"smoke/new_test.go": `package smoke

import "testing"

func TestBrandNewUndocumented(t *testing.T) {}
`,
	})
	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	err = functionaltestmetadata.CheckAgainstBaseline(records, baseline)
	if err == nil {
		t.Fatal("CheckAgainstBaseline() error = nil, want new undocumented failure")
	}
	if !strings.Contains(err.Error(), "smoke/new_test.go::TestBrandNewUndocumented") {
		t.Fatalf("error %q missing actionable identity for new undocumented test", err)
	}
}

func TestValidateBaselineUpdateRejectsIllegalExpansion(t *testing.T) {
	t.Parallel()

	previous := functionaltestmetadata.Baseline{
		Version: functionaltestmetadata.BaselineVersion,
		Stage:   functionaltestmetadata.BaselineStage,
		Entries: []functionaltestmetadata.BaselineEntry{
			{File: "smoke/a_test.go", Name: "TestAlpha"},
		},
	}
	expanded := functionaltestmetadata.Baseline{
		Version: functionaltestmetadata.BaselineVersion,
		Stage:   functionaltestmetadata.BaselineStage,
		Entries: []functionaltestmetadata.BaselineEntry{
			{File: "smoke/a_test.go", Name: "TestAlpha"},
			{File: "smoke/b_test.go", Name: "TestBeta"},
		},
	}

	err := functionaltestmetadata.ValidateBaselineUpdate(previous, expanded)
	if err == nil {
		t.Fatal("ValidateBaselineUpdate() error = nil, want illegal expansion failure")
	}
	if !strings.Contains(err.Error(), "smoke/b_test.go::TestBeta") {
		t.Fatalf("error %q missing expanded identity", err)
	}
	if !strings.Contains(err.Error(), "deletion-only") {
		t.Fatalf("error %q should mention deletion-only policy", err)
	}
}

func TestUndocumentedBaselineExcludesHarnessAndInternal(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/customer_test.go": `package smoke

import "testing"

func TestCustomerUndocumented(t *testing.T) {}
`,
		"smoke/helpers_test.go": `package smoke

import "testing"

func TestHelperUndocumented(t *testing.T) {}
`,
		"internal/support/harness_test.go": `package support

import "testing"

func TestInternalUndocumented(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	identities := functionaltestmetadata.UndocumentedCustomerIdentities(records)
	if len(identities) != 1 || identities[0] != "smoke/customer_test.go::TestCustomerUndocumented" {
		t.Fatalf("UndocumentedCustomerIdentities = %#v, want only customer identity", identities)
	}
	baseline := functionaltestmetadata.BaselineFromRecords(records)
	if len(baseline.Entries) != 1 {
		t.Fatalf("baseline entries = %#v, want only customer", baseline.Entries)
	}
	for _, identity := range identities {
		if strings.Contains(identity, "helpers") || strings.Contains(identity, "internal/") {
			t.Fatalf("harness/internal identity leaked into customer baseline: %s", identity)
		}
	}
	if err := functionaltestmetadata.CheckAgainstBaseline(records, baseline); err != nil {
		t.Fatalf("CheckAgainstBaseline() error = %v", err)
	}
}

func TestLoadBaselineRoundTrip(t *testing.T) {
	t.Parallel()

	baseline := functionaltestmetadata.Baseline{
		Version: functionaltestmetadata.BaselineVersion,
		Stage:   functionaltestmetadata.BaselineStage,
		Entries: []functionaltestmetadata.BaselineEntry{
			{File: `smoke\windows_test.go`, Name: "TestWindows"},
			{File: "smoke/a_test.go", Name: "TestAlpha"},
		},
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := functionaltestmetadata.LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline() error = %v", err)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("loaded entries = %#v, want 2", loaded.Entries)
	}
	if loaded.Entries[0].File != "smoke/a_test.go" || loaded.Entries[0].Name != "TestAlpha" {
		t.Fatalf("first entry = %#v, want sorted smoke/a_test.go::TestAlpha", loaded.Entries[0])
	}
	if loaded.Entries[1].File != "smoke/windows_test.go" {
		t.Fatalf("Windows path not normalized: %#v", loaded.Entries[1])
	}
}
