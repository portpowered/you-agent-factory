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
			FactoryComplete bool   `json:"factory_complete"`
			OpenPR          int    `json:"open_pr"`
			HoldReason      string `json:"hold_reason"`
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

func TestEnginePipelinePreStartGate_ClnFoldBlocksDeletionUntilFactoryComplete(t *testing.T) {
	t.Parallel()

	manifest := loadPrestartGateManifest(t)
	if manifest.Gates.ClnRunFoldEnginePipeline.FactoryComplete {
		t.Fatal("CLN-RUN-FOLD-ENGINE-PIPELINE gate is complete; flip deletion_hold_active and begin story 002")
	}
	if manifest.Gates.ClnRunFoldEnginePipeline.OpenPR <= 0 {
		t.Fatal("CLN-RUN-FOLD-ENGINE-PIPELINE gate must record the open PR number while incomplete")
	}
	if manifest.Gates.ClnRunFoldEnginePipeline.HoldReason == "" {
		t.Fatal("CLN-RUN-FOLD-ENGINE-PIPELINE gate must document the hold reason while incomplete")
	}
	if !manifest.DeletionHoldActive {
		t.Fatal("deletion_hold_active must remain true while CLN-RUN-FOLD-ENGINE-PIPELINE is incomplete")
	}

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range []string{
		"build", "engine", "javascript", "runtime", "scheduler", "state", "subsystems", "token",
	} {
		path := filepath.Join(runtimeRoot, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("pre-start hold: transitional pipeline public package %q missing unexpectedly: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("pre-start hold: transitional pipeline public package %q is not a directory", name)
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
