package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestC01EqualBaselinePassesAndReportsExactCounts(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "observed=1 baseline=1 packages=1") {
		t.Fatalf("stdout = %q, want exact count report", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestC02DecreasePassesWithoutEditingBaseline(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", twoSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 2})
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	baselineBefore := readFixtureFile(t, fixture.baselinePath)

	if err := os.WriteFile(fixture.sourcePath, []byte(oneSpawnSource), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	sites, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("rescan reduced fixture: %v", err)
	}
	fixture.sites = sites
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, want decrease pass", err)
	}
	if stderr != "" || !strings.Contains(stdout, "decreased=1") {
		t.Fatalf("stdout=%q stderr=%q, want one reported decrease", stdout, stderr)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on decrease: before=%s after=%s", baselineBefore, baselineAfter)
	}
}

func TestC05UnjustifiedIncreaseFailsWithPairedUpdateGuidance(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 0})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})

	_, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want an unpaired increase failure")
	}
	expected := "AST site " + fixture.sites[0].SiteID + " at " + fixture.sites[0].SourcePath + ":" + strconv.Itoa(fixture.sites[0].SourceLine) + " has no inventory verdict; update the baseline together with an INTENTIONAL-OS inventory row naming an allowed OS property"
	if !strings.Contains(stderr, expected) {
		t.Fatalf("stderr = %q, want exact diagnostic %q", stderr, expected)
	}
	if !strings.Contains(stderr, "LINT_VIOLATION_COUNT: 1") {
		t.Fatalf("stderr = %q, want one lint violation", stderr)
	}
}

func TestC03IntentionalAdmissionAllowsRaisedBaselineForNewSite(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", twoSpawnSource)
	fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites[:1])
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "observed=2 baseline=1") {
		t.Fatalf("stdout = %q, want raised-baseline counts", stdout)
	}
}

func TestC03ExistingIntentionalCannotAdmitNewAccidentalSite(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", twoSpawnSource)
	fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites[:1])
	records := fixture.siteRecordsForSites(intentionalVerdict)
	accidentalRecords := fixture.siteRecordsForSites(accidentalVerdict)
	records[1] = accidentalRecords[1]
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})

	_, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want existing-intentional/new-accidental failure")
	}
	if !strings.Contains(stderr, fixture.sites[1].SiteID) || !strings.Contains(stderr, "unadmitted=1") {
		t.Fatalf("stderr = %q, want the new accidental site identified", stderr)
	}
}

func TestC04EmptyTreePassesWithZeroTotals(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", "package fixture\n")
	fixture.writeBaseline(t, map[string]int{})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "observed=0 baseline=0 packages=0 intentional=0 accidental=0 decreased=0") {
		t.Fatalf("stdout = %q, want zero totals", stdout)
	}
}

func TestC07AccidentalIncreaseFailsEvenWithAnInventoryRecord(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 0})
	fixture.writeInventory(t, fixture.inventoryForSites(accidentalVerdict))

	_, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want accidental increase failure")
	}
	if !strings.Contains(stderr, "paired INTENTIONAL-OS") {
		t.Fatalf("stderr = %q, want intentional-admission diagnosis", stderr)
	}
}

func TestC06MissingInventorySiteFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})

	_, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want missing inventory failure")
	}
	if !strings.Contains(stderr, fixture.sites[0].SiteID) || !strings.Contains(stderr, "no inventory verdict") {
		t.Fatalf("stderr = %q, want site identity and missing-verdict context", stderr)
	}
}

func TestC07ExtraInventorySiteFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	records := fixture.siteRecordsForSites(intentionalVerdict)
	records = append(records, inventorySpawnSite{
		SiteID:            "OSSPAWN-tests-functional-fixture-missing-test-Spawn-01",
		PackagePath:       "tests/functional/fixture",
		SourcePath:        "tests/functional/fixture/missing_test.go",
		SourceLine:        1,
		EnclosingIdentity: "Spawn",
		LauncherKind:      "test-harness",
		Occurrence:        1,
		Verdict:           intentionalVerdict,
		RequiredProperty:  stringPointer("exit-status"),
		AssertionEvidence: "the fixture observes exit status",
	})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})

	_, stderr, err := fixture.run(t)
	if err == nil || !strings.Contains(stderr, "not present in the AST census") {
		t.Fatalf("run() error=%v stderr=%q, want extra-site diagnosis", err, stderr)
	}
}

func TestC08DuplicateSiteIdentityFailsSchemaValidation(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	records := fixture.siteRecordsForSites(intentionalVerdict)
	records = append(records, records[0])
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "duplicates siteId") {
		t.Fatalf("run() error = %v, want duplicate identity diagnosis", err)
	}
}

func TestC09MalformedInventoryFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	if err := os.WriteFile(fixture.inventoryPath, []byte("{not-json\n"), 0o644); err != nil {
		t.Fatalf("write malformed inventory: %v", err)
	}
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "parse inventory") {
		t.Fatalf("run() error = %v, want parse inventory diagnosis", err)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "malformed inventory")
}

func TestC10MalformedBaselineFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	if err := os.WriteFile(fixture.baselinePath, []byte("{not-json\n"), 0o644); err != nil {
		t.Fatalf("write malformed baseline: %v", err)
	}
	baselineBefore := readFixtureFile(t, fixture.baselinePath)

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "parse baseline") {
		t.Fatalf("run() error = %v, want parse baseline diagnosis", err)
	}
	assertFixtureFileUnchanged(t, fixture.baselinePath, baselineBefore, "malformed baseline")
}

func TestC11BaselineSchemaRejectsBadVersionUnitAndOrdering(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "version", content: `{"version":2,"countUnit":"static-os-spawn-site","packages":[]}`, want: "version must be 1"},
		{name: "count unit", content: `{"version":1,"countUnit":"process-start","packages":[]}`, want: "countUnit must be"},
		{name: "ordering", content: `{"version":1,"countUnit":"static-os-spawn-site","packages":[{"packagePath":"tests/functional/z","count":0},{"packagePath":"tests/functional/a","count":0}]}`, want: "unique and sorted"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newFixture(t, "tests/functional/fixture/process_test.go", "package fixture\n")
			fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})
			if err := os.WriteFile(fixture.baselinePath, []byte(testCase.content+"\n"), 0o644); err != nil {
				t.Fatalf("write baseline: %v", err)
			}
			_, _, err := fixture.run(t)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("run() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestC11MalformedGoFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/fixture/process_test.go", "package fixture\nfunc Broken( {\n")

	_, err := scanFunctionalOSSpawns(root)
	if err == nil || !strings.Contains(err.Error(), "parse functional source tests/functional/fixture/process_test.go") {
		t.Fatalf("scan() error = %v, want source parse context", err)
	}
}

func TestC12InventorySchemaRejectsBadVersion(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", "package fixture\n")
	fixture.writeBaseline(t, map[string]int{})
	if err := os.WriteFile(fixture.inventoryPath, []byte(`{"formatVersion":2,"testRows":[],"osSpawnSites":[]}`+"\n"), 0o644); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "formatVersion must be 3") {
		t.Fatalf("run() error = %v, want format version diagnosis", err)
	}
}

func TestC13MissingIsolatedRowVerdictFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", "package fixture\n")
	fixture.writeBaseline(t, map[string]int{})
	fixture.writeInventory(t, inventoryDocument{
		FormatVersion: inventoryFormatVersion,
		TestRows:      []inventoryTestRow{{Classification: "isolated-with-reason"}},
		OSSpawnSites:  []inventorySpawnSite{},
	})

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "missing osBoundaryIntentionality") {
		t.Fatalf("run() error = %v, want missing row verdict diagnosis", err)
	}
}

func TestC14InvalidIntentionalPropertyFailsClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	records := fixture.siteRecordsForSites(intentionalVerdict)
	records[0].RequiredProperty = stringPointer("listener")
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "one allowed OS property") {
		t.Fatalf("run() error = %v, want invalid property diagnosis", err)
	}
	for _, property := range allowedProperties {
		if !strings.Contains(err.Error(), property) {
			t.Fatalf("run() error = %v, want allowed property %q", err, property)
		}
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "invalid intentional inventory")
}

func TestC15InvalidAccidentalFieldsFailClosed(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	records := fixture.siteRecordsForSites(accidentalVerdict)
	records[0].RequiredProperty = stringPointer("exit-status")
	records[0].ConversionObligation = nil
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "accidental verdict must have requiredProperty=null") {
		t.Fatalf("run() error = %v, want accidental-field diagnosis", err)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "invalid accidental inventory")
}

func TestInventoryReconciliationToleratesSourceLineDriftByIdentity(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites)
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	baselineBefore := readFixtureFile(t, fixture.baselinePath)
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)
	originalSite := fixture.sites[0]
	callOffset := strings.Index(oneSpawnSource, "_ = exec.Command")
	if callOffset < 0 {
		t.Fatal("oneSpawnSource does not contain the fixture spawn")
	}
	driftedSource := oneSpawnSource[:callOffset] + "\n\n" + oneSpawnSource[callOffset:]
	writeFixtureSource(t, fixture.root, "tests/functional/fixture/process_test.go", driftedSource)
	driftedSites, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("scan drifted fixture: %v", err)
	}
	if len(driftedSites) != 1 || driftedSites[0].SiteID != originalSite.SiteID || driftedSites[0].SourceLine <= originalSite.SourceLine {
		t.Fatalf("drifted sites = %#v, want unchanged identity and moved source line", driftedSites)
	}

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%q", err, stderr)
	}
	want := fmt.Sprintf("[agent-factory:functional-os-boundary] tolerated sourceLine drift for inventory site %s: recorded=%d observed=%d; siteId matched\n[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=1 baseline=1 packages=1 intentional=1 accidental=0 decreased=0\n[agent-factory:functional-os-boundary] reconciled 1 inventory OS-spawn records\n", originalSite.SiteID, originalSite.SourceLine, driftedSites[0].SourceLine)
	if stdout != want {
		t.Fatalf("stdout = %q, want exact drift and success report %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on line drift: before=%s after=%s", baselineBefore, baselineAfter)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "metadata-drift inventory")
}

func TestInventoryRejectsUninventoriedSpawnIncrease(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 0})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})
	baselineBefore := readFixtureFile(t, fixture.baselinePath)
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	stdout, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want un-inventoried increase failure")
	}
	want := fmt.Sprintf("[agent-factory:functional-os-boundary] AST site %s at %s:%d has no inventory verdict; update the baseline together with an INTENTIONAL-OS inventory row naming an allowed OS property\nLINT_VIOLATION_COUNT: 1\n", fixture.sites[0].SiteID, fixture.sites[0].SourcePath, fixture.sites[0].SourceLine)
	if stdout != "" || stderr != want {
		t.Fatalf("stdout=%q stderr=%q, want stdout empty and exact admission diagnostic %q", stdout, stderr, want)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on un-inventoried increase: before=%s after=%s", baselineBefore, baselineAfter)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "un-inventoried increase inventory")
}

func TestInventoryReconciliationRejectsNonLineMetadataDrift(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites)
	records := fixture.siteRecordsForSites(intentionalVerdict)
	recordedPath := records[0].SourcePath
	records[0].SourcePath = "tests/functional/fixture/renamed.go"
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})
	baselineBefore := readFixtureFile(t, fixture.baselinePath)
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	stdout, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want non-line metadata failure")
	}
	want := fmt.Sprintf("[agent-factory:functional-os-boundary] inventory site %s sourcePath=%q does not match AST sourcePath=%q\nLINT_VIOLATION_COUNT: 1\n", fixture.sites[0].SiteID, records[0].SourcePath, recordedPath)
	if stdout != "" || stderr != want {
		t.Fatalf("stdout=%q stderr=%q, want stdout empty and exact metadata diagnostic %q", stdout, stderr, want)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on non-line metadata failure: before=%s after=%s", baselineBefore, baselineAfter)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "non-line metadata inventory")
}

func TestInventoryAllowsPairedRemovalAsDecrease(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", twoSpawnSource)
	fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 2}, fixture.sites)
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	baselineBefore := readFixtureFile(t, fixture.baselinePath)

	writeFixtureSource(t, fixture.root, "tests/functional/fixture/process_test.go", oneSpawnSource)
	remainingSites, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("scan reduced fixture: %v", err)
	}
	fixture.sites = remainingSites
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%q", err, stderr)
	}
	want := "[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=1 baseline=2 packages=1 intentional=1 accidental=0 decreased=1\n[agent-factory:functional-os-boundary] reconciled 1 inventory OS-spawn records\n"
	if stdout != want || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q, want exact decrease report %q and empty stderr", stdout, stderr, want)
	}
	if strings.Contains(stdout, "tolerated sourceLine drift") {
		t.Fatalf("stdout = %q, paired removal must not report line drift", stdout)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on paired removal: before=%s after=%s", baselineBefore, baselineAfter)
	}
}

func TestInventoryRejectsRenamedOrMovedIdentity(t *testing.T) {
	t.Run("enclosing function rename", func(t *testing.T) {
		fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
		fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites)
		fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
		baselineBefore := readFixtureFile(t, fixture.baselinePath)
		inventoryBefore := readFixtureFile(t, fixture.inventoryPath)
		oldSite := fixture.sites[0]

		renamedSource := strings.Replace(oneSpawnSource, "func Spawn()", "func RenamedSpawn()", 1)
		writeFixtureSource(t, fixture.root, "tests/functional/fixture/process_test.go", renamedSource)
		newSites, err := scanFunctionalOSSpawns(fixture.root)
		if err != nil {
			t.Fatalf("scan renamed fixture: %v", err)
		}
		if len(newSites) != 1 || newSites[0].SiteID == oldSite.SiteID {
			t.Fatalf("renamed sites = %#v, want a new identity", newSites)
		}
		assertIdentityChangeRejected(t, fixture, oldSite, newSites[0], baselineBefore, inventoryBefore)
	})

	t.Run("source move", func(t *testing.T) {
		fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
		fixture.writeBaselineWithSites(t, map[string]int{"tests/functional/fixture": 1}, fixture.sites)
		fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
		baselineBefore := readFixtureFile(t, fixture.baselinePath)
		inventoryBefore := readFixtureFile(t, fixture.inventoryPath)
		oldSite := fixture.sites[0]
		movedPath := "tests/functional/moved/process_test.go"

		writeFixtureSource(t, fixture.root, movedPath, oneSpawnSource)
		if err := os.Remove(fixture.sourcePath); err != nil {
			t.Fatalf("remove original source: %v", err)
		}
		newSites, err := scanFunctionalOSSpawns(fixture.root)
		if err != nil {
			t.Fatalf("scan moved fixture: %v", err)
		}
		if len(newSites) != 1 || newSites[0].SiteID == oldSite.SiteID {
			t.Fatalf("moved sites = %#v, want a new identity", newSites)
		}
		assertIdentityChangeRejected(t, fixture, oldSite, newSites[0], baselineBefore, inventoryBefore)
	})
}

func assertIdentityChangeRejected(t *testing.T, fixture *checkerFixture, oldSite, newSite spawnSite, baselineBefore, inventoryBefore []byte) {
	t.Helper()
	stdout, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want identity reconciliation failure")
	}
	newFinding := fmt.Sprintf("AST site %s at %s:%d has no inventory verdict; update the baseline together with an INTENTIONAL-OS inventory row naming an allowed OS property", newSite.SiteID, newSite.SourcePath, newSite.SourceLine)
	oldFinding := fmt.Sprintf("inventory site %s is not present in the AST census", oldSite.SiteID)
	findings := []string{newFinding, oldFinding}
	sort.Strings(findings)
	findings[0] = "[agent-factory:functional-os-boundary] " + findings[0]
	wantStderr := strings.Join(findings, "\n") + "\nLINT_VIOLATION_COUNT: 2\n"
	if stdout != "" || stderr != wantStderr {
		t.Fatalf("stdout=%q stderr=%q, want stdout empty and exact identity diagnostics %q", stdout, stderr, wantStderr)
	}
	if strings.Contains(stderr, "tolerated sourceLine drift") {
		t.Fatalf("stderr = %q, identity change must not be treated as line drift", stderr)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed on identity failure: before=%s after=%s", baselineBefore, baselineAfter)
	}
	assertFixtureFileUnchanged(t, fixture.inventoryPath, inventoryBefore, "identity failure inventory")
}

func TestInventoryReconciliationSortsSourceLineDriftBySiteID(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/zeta/process_test.go", oneSpawnSource)
	writeFixtureSource(t, root, "tests/functional/alpha/process_test.go", oneSpawnSource)
	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	fixture := &checkerFixture{root: root, baselinePath: filepath.Join(root, "baseline.json"), inventoryPath: filepath.Join(root, "inventory.json"), sites: sites}
	counts := map[string]int{"tests/functional/alpha": 1, "tests/functional/zeta": 1}
	fixture.writeBaselineWithSites(t, counts, sites)
	records := fixture.siteRecordsForSites(intentionalVerdict)
	for index := range records {
		records[index].SourceLine--
	}
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})

	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%q", err, stderr)
	}
	drifts := make([]string, 0, len(sites))
	for _, site := range sites {
		for _, record := range records {
			if record.SiteID == site.SiteID {
				drifts = append(drifts, fmt.Sprintf("[agent-factory:functional-os-boundary] tolerated sourceLine drift for inventory site %s: recorded=%d observed=%d; siteId matched", record.SiteID, record.SourceLine, site.SourceLine))
				break
			}
		}
	}
	sort.Strings(drifts)
	want := strings.Join(append(drifts, "[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=2 baseline=2 packages=2 intentional=2 accidental=0 decreased=0", "[agent-factory:functional-os-boundary] reconciled 2 inventory OS-spawn records"), "\n") + "\n"
	if stdout != want || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q, want sorted exact drift report %q and empty stderr", stdout, stderr, want)
	}
}

func TestC17StableIdentityIsDeterministicAndOccurrenceIsTwoDigits(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", twoSpawnSource)
	first, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	second, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated scans differ: first=%#v second=%#v", first, second)
	}
	if len(first) != 2 || first[0].Occurrence != 1 || first[1].Occurrence != 2 {
		t.Fatalf("sites = %#v, want two ordered occurrences", first)
	}
	if !strings.HasSuffix(first[1].SiteID, "-02") || first[0].SiteID == first[1].SiteID {
		t.Fatalf("site identities = %q, %q, want distinct two-digit identities", first[0].SiteID, first[1].SiteID)
	}
}

func TestC18ASTCatalogRecognizesAliasesAndExcludesSupportAndTestdata(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/alias/alias_test.go", `package alias
import osexec "os/exec"
func Alias() { _ = osexec.Command("echo"); _, _ = osexec.LookPath("echo") }
`)
	writeFixtureSource(t, root, "tests/functional/alias/dot_test.go", `package alias
import . "os/exec"
func Dot() { _ = CommandContext(nil, "echo") }
`)
	writeFixtureSource(t, root, "tests/functional/internal/support/ignored.go", oneSpawnSource)
	writeFixtureSource(t, root, "tests/functional/alias/testdata/ignored.go", oneSpawnSource)
	writeFixtureSource(t, root, "tests/functional/alias/non_spawn.go", `package alias
type process struct{}
func (process) Start() {}
var comment = "exec.Command(\"not-a-process\")"
// exec.Command("also-not-a-process")
`)

	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(sites) != 2 || sites[0].SourcePath != "tests/functional/alias/alias_test.go" || sites[1].SourcePath != "tests/functional/alias/dot_test.go" {
		t.Fatalf("sites = %#v, want aliased and dot-imported production fixture sites", sites)
	}
}

func TestC19PairedRecoveryChangesFailureToPass(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 0})
	fixture.writeInventory(t, fixture.inventoryForSites(accidentalVerdict))
	if _, stderr, err := fixture.run(t); err == nil || !strings.Contains(stderr, "paired INTENTIONAL-OS") {
		t.Fatalf("initial run error=%v stderr=%q, want paired-admission failure", err, stderr)
	}

	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("recovered run() error = %v, stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "observed=1 baseline=1") {
		t.Fatalf("stdout = %q, want recovered equal-count report", stdout)
	}
}

func TestC20SlashNormalizedPathsKeepStableIdentity(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/path/process_test.go", oneSpawnSource)
	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	if len(sites) != 1 || sites[0].SourcePath != "tests/functional/path/process_test.go" || sites[0].PackagePath != "tests/functional/path" {
		t.Fatalf("sites = %#v, want slash-normalized source and package paths", sites)
	}
	if strings.Contains(sites[0].SiteID, "\\") {
		t.Fatalf("site identity = %q, want slash-independent identity", sites[0].SiteID)
	}
}

func TestC18PermutedInventoryFindingsRemainByteStable(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/zeta/process_test.go", oneSpawnSource)
	writeFixtureSource(t, root, "tests/functional/alpha/process_test.go", oneSpawnSource)
	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	fixture := &checkerFixture{
		root:          root,
		baselinePath:  filepath.Join(root, "baseline.json"),
		inventoryPath: filepath.Join(root, "inventory.json"),
		sites:         sites,
	}
	fixture.writeBaseline(t, map[string]int{"tests/functional/alpha": 0, "tests/functional/zeta": 0})
	records := fixture.siteRecordsForSites(accidentalVerdict)
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})
	_, firstStderr, firstErr := fixture.run(t)
	if firstErr == nil {
		t.Fatal("first run() error = nil, want baseline violations")
	}

	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: records})
	_, secondStderr, secondErr := fixture.run(t)
	if secondErr == nil {
		t.Fatal("second run() error = nil, want baseline violations")
	}
	if firstStderr != secondStderr {
		t.Fatalf("stderr changed after inventory permutation:\nfirst=%s\nsecond=%s", firstStderr, secondStderr)
	}
}

func TestC19MissingArtifactsAreActionable(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", "package fixture\n")
	fixture.writeBaseline(t, map[string]int{})
	fixture.writeInventory(t, inventoryDocument{FormatVersion: inventoryFormatVersion, TestRows: []inventoryTestRow{}, OSSpawnSites: []inventorySpawnSite{}})
	if err := os.Remove(fixture.baselinePath); err != nil {
		t.Fatalf("remove baseline: %v", err)
	}
	_, _, err := fixture.run(t)
	if err == nil || !strings.Contains(err.Error(), "read baseline") {
		t.Fatalf("run() error = %v, want missing baseline diagnosis", err)
	}
}

func TestC20PackageAggregationAndSortedOutput(t *testing.T) {
	root := t.TempDir()
	writeFixtureSource(t, root, "tests/functional/zeta/process_test.go", oneSpawnSource)
	writeFixtureSource(t, root, "tests/functional/alpha/process_test.go", oneSpawnSource)
	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan() error = %v", err)
	}
	fixture := &checkerFixture{root: root, sites: sites}
	fixture.baselinePath = filepath.Join(root, "baseline.json")
	fixture.inventoryPath = filepath.Join(root, "inventory.json")
	fixture.writeBaseline(t, map[string]int{
		"tests/functional/alpha": 1,
		"tests/functional/zeta":  0,
	})
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("run() error = %v, stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "observed=2 baseline=1 packages=2") {
		t.Fatalf("stdout = %q, want aggregate exact counts", stdout)
	}
}

func TestC21ScratchSpawnIsStaticOnlyAndRemovalRecovers(t *testing.T) {
	fixture := newFixture(t, "tests/functional/fixture/process_test.go", oneSpawnSource)
	fixture.writeBaseline(t, map[string]int{"tests/functional/fixture": 1})
	fixture.writeInventory(t, fixture.inventoryForSites(intentionalVerdict))
	baselineBefore := readFixtureFile(t, fixture.baselinePath)
	inventoryBefore := readFixtureFile(t, fixture.inventoryPath)

	sentinelPath := filepath.Join(fixture.root, "checker-sentinel-must-not-be-created")
	scratchPath := "tests/functional/fixture/scratch_test.go"
	scratchSource := `package fixture

import "os/exec"

func Scratch() {
	_ = exec.Command("sh", "-c", "printf executed > "+"` + filepath.ToSlash(sentinelPath) + `").Run()
}
`
	writeFixtureSource(t, fixture.root, scratchPath, scratchSource)

	_, stderr, err := fixture.run(t)
	if err == nil {
		t.Fatal("run() error = nil, want unadmitted scratch spawn failure")
	}
	if !strings.Contains(stderr, "INTENTIONAL-OS inventory row naming an allowed OS property") {
		t.Fatalf("stderr = %q, want paired intentional-admission guidance", stderr)
	}
	scratchSites, err := scanFunctionalOSSpawns(fixture.root)
	if err != nil {
		t.Fatalf("rescan scratch fixture: %v", err)
	}
	var scratchSite *spawnSite
	for index := range scratchSites {
		if scratchSites[index].SourcePath == scratchPath {
			scratchSite = &scratchSites[index]
			break
		}
	}
	if scratchSite == nil {
		t.Fatalf("scratch site missing from AST census: %#v", scratchSites)
	}
	expectedSiteID := stableSiteID(scratchPath, "Scratch", 1)
	if scratchSite.SiteID != expectedSiteID || !strings.Contains(stderr, expectedSiteID) {
		t.Fatalf("scratch site=%#v stderr=%q, want stable site %q in diagnostic", scratchSite, stderr, expectedSiteID)
	}
	if _, statErr := os.Stat(sentinelPath); !os.IsNotExist(statErr) {
		t.Fatalf("checker created sentinel at %q; stat error=%v", sentinelPath, statErr)
	}
	if baselineAfter := readFixtureFile(t, fixture.baselinePath); !bytes.Equal(baselineBefore, baselineAfter) {
		t.Fatalf("baseline changed during scratch rejection: before=%s after=%s", baselineBefore, baselineAfter)
	}
	if inventoryAfter := readFixtureFile(t, fixture.inventoryPath); !bytes.Equal(inventoryBefore, inventoryAfter) {
		t.Fatalf("inventory changed during scratch rejection: before=%s after=%s", inventoryBefore, inventoryAfter)
	}

	if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(scratchPath))); err != nil {
		t.Fatalf("remove scratch source: %v", err)
	}
	stdout, stderr, err := fixture.run(t)
	if err != nil {
		t.Fatalf("recovered run() error = %v, stderr=%s", err, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "observed=1 baseline=1 packages=1") {
		t.Fatalf("recovered stdout=%q stderr=%q, want exact clean pass", stdout, stderr)
	}
	if _, statErr := os.Stat(sentinelPath); !os.IsNotExist(statErr) {
		t.Fatalf("recovery created sentinel at %q; stat error=%v", sentinelPath, statErr)
	}
}

type checkerFixture struct {
	root          string
	sourcePath    string
	baselinePath  string
	inventoryPath string
	sites         []spawnSite
}

func newFixture(t *testing.T, sourcePath, source string) *checkerFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &checkerFixture{
		root:          root,
		sourcePath:    filepath.Join(root, filepath.FromSlash(sourcePath)),
		baselinePath:  filepath.Join(root, "baseline.json"),
		inventoryPath: filepath.Join(root, "inventory.json"),
	}
	writeFixtureSource(t, root, sourcePath, source)
	sites, err := scanFunctionalOSSpawns(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	fixture.sites = sites
	return fixture
}

func (fixture *checkerFixture) run(t *testing.T) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(config{
		root:      fixture.root,
		baseline:  filepath.Base(fixture.baselinePath),
		inventory: filepath.Base(fixture.inventoryPath),
	}, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func (fixture *checkerFixture) inventoryForSites(verdict string) inventoryDocument {
	return inventoryDocument{
		FormatVersion: inventoryFormatVersion,
		TestRows:      []inventoryTestRow{},
		OSSpawnSites:  fixture.siteRecordsForSites(verdict),
	}
}

func (fixture *checkerFixture) siteRecordsForSites(verdict string) []inventorySpawnSite {
	records := make([]inventorySpawnSite, 0, len(fixture.sites))
	for _, site := range fixture.sites {
		record := inventorySpawnSite{
			SiteID:            site.SiteID,
			PackagePath:       site.PackagePath,
			SourcePath:        site.SourcePath,
			SourceLine:        site.SourceLine,
			EnclosingIdentity: site.EnclosingIdentity,
			LauncherKind:      "test-harness",
			Occurrence:        site.Occurrence,
			Verdict:           verdict,
			AssertionEvidence: "the fixture observes the recorded process property",
		}
		if verdict == intentionalVerdict {
			record.RequiredProperty = stringPointer("exit-status")
		} else {
			record.ConversionObligation = stringPointer("move the assertion to the owning c15 edge")
		}
		records = append(records, record)
	}
	return records
}

func (fixture *checkerFixture) writeBaseline(t *testing.T, counts map[string]int) {
	t.Helper()
	fixture.writeBaselineWithSites(t, counts, nil)
}

func (fixture *checkerFixture) writeBaselineWithSites(t *testing.T, counts map[string]int, baselineSites []spawnSite) {
	t.Helper()
	paths := make([]string, 0, len(counts))
	for packagePath := range counts {
		paths = append(paths, packagePath)
	}
	// Deliberately use the production ordering contract in every fixture.
	for index := 0; index < len(paths); index++ {
		for next := index + 1; next < len(paths); next++ {
			if paths[next] < paths[index] {
				paths[index], paths[next] = paths[next], paths[index]
			}
		}
	}
	siteIDsByPackage := map[string][]string{}
	for _, site := range baselineSites {
		siteIDsByPackage[site.PackagePath] = append(siteIDsByPackage[site.PackagePath], site.SiteID)
	}
	packages := make([]baselinePackageRow, 0, len(paths))
	for _, packagePath := range paths {
		siteIDs := append([]string(nil), siteIDsByPackage[packagePath]...)
		sort.Strings(siteIDs)
		packages = append(packages, baselinePackageRow{PackagePath: packagePath, Count: counts[packagePath], SiteIDs: siteIDs})
	}
	writeFixtureJSON(t, fixture.baselinePath, baselineDocument{Version: baselineFormatVersion, CountUnit: spawnCountUnit, Packages: packages})
}

func (fixture *checkerFixture) writeInventory(t *testing.T, inventory inventoryDocument) {
	t.Helper()
	writeFixtureJSON(t, fixture.inventoryPath, inventory)
}

func writeFixtureSource(t *testing.T, root, relativePath, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir source directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
}

func writeFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture JSON: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture JSON directory: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture JSON: %v", err)
	}
}

func readFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture file: %v", err)
	}
	return data
}

func assertFixtureFileUnchanged(t *testing.T, path string, before []byte, label string) {
	t.Helper()
	after := readFixtureFile(t, path)
	if !bytes.Equal(before, after) {
		t.Fatalf("%s changed after checker failure: before=%s after=%s", label, before, after)
	}
}

func stringPointer(value string) *string {
	return &value
}

const oneSpawnSource = `package fixture

import "os/exec"

func Spawn() {
	_ = exec.Command("echo")
}
`

const twoSpawnSource = `package fixture

import "os/exec"

func Spawn() {
	_ = exec.Command("echo")
	_ = exec.Command("printf")
}
`
