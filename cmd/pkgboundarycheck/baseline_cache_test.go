package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordedBaselineIsMemoizedPerBaseCommit(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	cacheDir := t.TempDir()
	const recordedImport = repositoryImportPrefix + "pkg/transports/http"
	writeGoImportFile(t, repoRoot, "pkg/services/work/recorded.go", "work", recordedImport)
	commitRecordedBoundaryFixture(t, repoRoot)
	cfg := config{root: repoRoot, packageRoot: defaultScanRoot, baseRef: "HEAD", baselineCacheDir: cacheDir}

	if err := run(cfg, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first run() error = %v, want recorded debt to pass", err)
	}
	cached, err := filepath.Glob(filepath.Join(cacheDir, "pkgboundary-base-*.json"))
	if err != nil || len(cached) != 1 {
		t.Fatalf("cache files = %v (err %v), want one memoized base scan", cached, err)
	}
	if err := run(cfg, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("cached run() error = %v, want the same passing result", err)
	}

	// An empty cached baseline proves the second run reads the cache: the same
	// finding is then unrecorded and blocks.
	if err := os.WriteFile(cached[0], []byte("[]"), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	err = run(cfg, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "found 1 package-boundary violation") {
		t.Fatalf("run() with emptied cache error = %v, want the finding to become blocking", err)
	}

	cfg.baselineCacheDir = ""
	if err := run(cfg, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("uncached run() error = %v, want recorded debt to pass", err)
	}
}
