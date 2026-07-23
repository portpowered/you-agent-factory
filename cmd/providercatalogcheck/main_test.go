package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/providercatalog"
)

func TestRunReportsActionableRegenerationGuidance(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run(t.TempDir(), &stdout, &stderr); exitCode == 0 {
		t.Fatal("run() succeeded without authored inputs")
	}
	if !strings.Contains(stderr.String(), "[provider-catalog-check] check failed:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestReportDriftIncludesPathsAndRegenerationCommand(t *testing.T) {
	var stderr bytes.Buffer
	reportDrift(&stderr, providercatalog.Drift{
		Stale:   []string{providercatalog.CatalogPath},
		Missing: []string{providercatalog.ManifestSchemaPath},
	})
	for _, want := range []string{
		"stale: " + providercatalog.CatalogPath,
		"missing: " + providercatalog.ManifestSchemaPath,
		"run `make provider-catalog-generate`",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}
}
