package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/migrationledgercheck"
)

func TestRunPassesOnRepositoryLedger(t *testing.T) {
	t.Parallel()

	cfg := config{
		root:          findRepoRoot(t),
		ledgerPath:    migrationledgercheck.DefaultLedgerPath,
		checklistPath: migrationledgercheck.DefaultChecklistPath,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := run(cfg, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "customer scenarios mapped") {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
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
