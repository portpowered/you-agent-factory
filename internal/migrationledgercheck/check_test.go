package migrationledgercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPassesOnRepositoryLedger(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := Check(repoRoot, DefaultLedgerPath, DefaultChecklistPath); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestLoadChecklistPathsFindsDestinationCells(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	paths, err := LoadChecklistPaths(filepath.Join(repoRoot, DefaultChecklistPath))
	if err != nil {
		t.Fatalf("LoadChecklistPaths() error = %v", err)
	}
	if _, ok := paths["tests/functional/transport/http/server/generated_client_test.go"]; !ok {
		t.Fatal("expected generated_client_test.go checklist cell")
	}
}

func TestScanLiveScenariosMatchesLedgerSummary(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := ScanLiveScenarios(repoRoot)
	if err != nil {
		t.Fatalf("ScanLiveScenarios() error = %v", err)
	}
	ledger, err := LoadLedger(filepath.Join(repoRoot, DefaultLedgerPath))
	if err != nil {
		t.Fatalf("LoadLedger() error = %v", err)
	}
	if len(live) != ledger.Summary.CustomerTopLevelTestScenarios {
		t.Fatalf("live=%d summary=%d", len(live), ledger.Summary.CustomerTopLevelTestScenarios)
	}
}

func TestCheckRejectsUnmappedDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixtureTree(t, root)
	ledgerPath := filepath.Join(root, "ledger.json")
	checklistPath := filepath.Join(root, "checklist.md")
	writeFixture(t, checklistPath, "- [ ] `tests/functional/example/dest_test.go`\n")
	writeFixture(t, ledgerPath, `{
  "rows": [{
    "source_path": "tests/functional/example/source_test.go",
    "package": "you-agent-factory/tests/functional/example",
    "scenario": "TestExample",
    "lane": "short",
    "destination": "TBD",
    "catch_all": "none",
    "specialty_targets": "none",
    "deletion_only_batch": "n/a"
  }],
  "summary": {
    "customer_top_level_Test_scenarios": 1,
    "lane_short": 1,
    "lane_functionallong": 0
  }
}`)

	err := Check(root, "ledger.json", "checklist.md")
	if err == nil || !strings.Contains(err.Error(), "unmapped destination") {
		t.Fatalf("Check() error = %v, want unmapped destination", err)
	}
}

func TestCheckRejectsUnknownSpecialtyTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixtureTree(t, root)
	ledgerPath := filepath.Join(root, "ledger.json")
	checklistPath := filepath.Join(root, "checklist.md")
	writeFixture(t, checklistPath, "- [ ] `tests/functional/example/dest_test.go`\n")
	writeFixture(t, ledgerPath, `{
  "rows": [{
    "source_path": "tests/functional/example/source_test.go",
    "package": "you-agent-factory/tests/functional/example",
    "scenario": "TestExample",
    "lane": "short",
    "destination": "tests/functional/example/dest_test.go",
    "catch_all": "none",
    "specialty_targets": "not-a-real-specialty-target",
    "deletion_only_batch": "n/a"
  }],
  "summary": {
    "customer_top_level_Test_scenarios": 1,
    "lane_short": 1,
    "lane_functionallong": 0
  }
}`)

	err := Check(root, "ledger.json", "checklist.md")
	if err == nil || !strings.Contains(err.Error(), "unknown specialty target") {
		t.Fatalf("Check() error = %v, want unknown specialty target", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func writeFixtureTree(t *testing.T, root string) {
	t.Helper()
	sourceDir := filepath.Join(root, "tests", "functional", "example")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFixture(t, filepath.Join(sourceDir, "source_test.go"), `package example

import "testing"

func TestExample(t *testing.T) {}
`)
}

func writeFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
