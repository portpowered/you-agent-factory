package wire_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const prestartGateManifestRel = "docs/internal/processes/del-run-engine-pipeline-prestart-gates.json"

type prestartGateManifest struct {
	DeletionHoldActive bool `json:"deletion_hold_active"`
	Gates              struct {
		DelRunService struct {
			FactoryComplete bool `json:"factory_complete"`
			MergedPR        int  `json:"merged_pr"`
		} `json:"DEL-RUN-SERVICE"`
		ClnRunFoldEnginePipeline struct {
			FactoryComplete bool `json:"factory_complete"`
			MergedPR        int  `json:"merged_pr"`
		} `json:"CLN-RUN-FOLD-ENGINE-PIPELINE"`
	} `json:"gates"`
}

// DEL-RUN-ENGINE-PIPELINE story 001 confirms prerequisite gates before leased
// pipeline deletion or baseline burn-down begins.

func TestEnginePipelinePreStartGate_DelRunServiceFactoryComplete(t *testing.T) {
	t.Parallel()

	manifest := loadPrestartGateManifest(t)
	if !manifest.Gates.DelRunService.FactoryComplete {
		t.Fatal("DEL-RUN-SERVICE gate must be Factory-complete before this packet proceeds")
	}
	if manifest.Gates.DelRunService.MergedPR <= 0 {
		t.Fatal("DEL-RUN-SERVICE gate must record the merged PR number")
	}

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, rel := range []string{"service", filepath.Join("service", "host")} {
		path := filepath.Join(runtimeRoot, rel)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("DEL-RUN-SERVICE gate incomplete: deleted public package still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestEnginePipelinePreStartGate_ClnFoldCompleteDeletionHoldCleared(t *testing.T) {
	t.Parallel()

	manifest := loadPrestartGateManifest(t)
	if !manifest.Gates.ClnRunFoldEnginePipeline.FactoryComplete {
		t.Fatal("CLN-RUN-FOLD-ENGINE-PIPELINE gate must be Factory-complete before pipeline deletion begins")
	}
	if manifest.Gates.ClnRunFoldEnginePipeline.MergedPR <= 0 {
		t.Fatal("CLN-RUN-FOLD-ENGINE-PIPELINE gate must record the merged PR number")
	}
	if manifest.DeletionHoldActive {
		t.Fatal("deletion_hold_active must be false once CLN-RUN-FOLD-ENGINE-PIPELINE is Factory-complete")
	}

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range deletedEnginePipelinePublicTopLevelChildren() {
		path := filepath.Join(runtimeRoot, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("fold complete: deleted pipeline public package %q still exists at %s", name, path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func loadPrestartGateManifest(t *testing.T) prestartGateManifest {
	t.Helper()

	root := serviceDeletionRepoRoot(t)
	path := filepath.Join(root, prestartGateManifestRel)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pre-start gate manifest %s: %v", path, err)
	}
	var manifest prestartGateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode pre-start gate manifest: %v", err)
	}
	return manifest
}
