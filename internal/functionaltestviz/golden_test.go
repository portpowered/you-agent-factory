package functionaltestviz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestAttachGoldenProvenanceLoadsManifestFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestRel := "tests/functional/internal/support/testdata/provider-sessions/harness/load-smoke/manifest.json"
	manifestAbs := filepath.Join(root, filepath.FromSlash(manifestRel))
	if err := os.MkdirAll(filepath.Dir(manifestAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const body = `{
  "schemaVersion": 1,
  "id": "harness-load-smoke",
  "provider": "harness",
  "fidelityClass": "partial-stream",
  "case": "load-smoke"
}
`
	if err := os.WriteFile(manifestAbs, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "workers/inference/harness/load_test.go",
			Package:        "harness",
			Name:           "TestLoadSmoke",
			Line:           12,
			Description:    "loads golden fixture",
			Golden:         manifestRel,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
	})

	enriched, err := functionaltestviz.AttachGoldenProvenance(records, root)
	if err != nil {
		t.Fatalf("AttachGoldenProvenance: %v", err)
	}
	if !enriched[0].Provenance.Present() {
		t.Fatal("expected provenance on golden-backed record")
	}
	got := enriched[0].Provenance
	if got.Provider != "harness" || got.Case != "load-smoke" || got.FidelityClass != "partial-stream" || got.ID != "harness-load-smoke" {
		t.Fatalf("provenance = %#v", got)
	}
	if got.ManifestPath != manifestRel {
		t.Fatalf("ManifestPath = %q, want %q", got.ManifestPath, manifestRel)
	}
	if enriched[1].Provenance.Present() {
		t.Fatal("non-golden record must not gain provenance")
	}
}

func TestAttachGoldenProvenanceFailsClosedForMissingAndMalformed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	missing := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "workers/inference/openai/missing_test.go",
			Package:        "openai",
			Name:           "TestMissing",
			Line:           4,
			Description:    "missing golden",
			Golden:         "missing/manifest.json",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
	})
	_, err := functionaltestviz.AttachGoldenProvenance(missing, root)
	if err == nil {
		t.Fatal("expected missing manifest error")
	}
	if !strings.Contains(err.Error(), "golden manifest not found") {
		t.Fatalf("error %q should mention missing manifest", err)
	}
	if !strings.Contains(err.Error(), "workers/inference/openai/missing_test.go::TestMissing") {
		t.Fatalf("error %q should include stable identity", err)
	}

	badRel := "bad/manifest.json"
	badAbs := filepath.Join(root, filepath.FromSlash(badRel))
	if err := os.MkdirAll(filepath.Dir(badAbs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(badAbs, []byte(`{"id":"only-id"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	malformed := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "workers/inference/openai/bad_test.go",
			Package:        "openai",
			Name:           "TestBad",
			Line:           8,
			Description:    "bad golden",
			Golden:         badRel,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
	})
	_, err = functionaltestviz.AttachGoldenProvenance(malformed, root)
	if err == nil {
		t.Fatal("expected malformed manifest error")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Fatalf("error %q should mention missing required fields", err)
	}
}

func TestRenderCatalogMarkdownFailsClosedWithoutAttachedProvenance(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "workers/inference/openai/invoke_test.go",
			Package:        "openai",
			Name:           "TestInvoke",
			Line:           42,
			Description:    "verifies provider replay",
			Golden:         "tests/functional/internal/support/testdata/provider-sessions/openai/invoke/manifest.json",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
	})
	_, err := functionaltestviz.RenderCatalogMarkdown(functionaltestviz.CatalogInputs{Records: records})
	if err == nil {
		t.Fatal("expected missing provenance error")
	}
	if !strings.Contains(err.Error(), "missing attached provenance") {
		t.Fatalf("error %q should mention missing attached provenance", err)
	}
}
