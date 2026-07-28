package definitions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	importExportWorkType       = "task"
	importExportSourceName     = "export-source"
	importExportImportedName   = "imported-roundtrip"
	importExportWorkerName     = "worker-a"
	importExportWorkstationName = "process"
)

// TestExportedFactoryCanBeImportedAndRun proves a valid Factory exported through
// the public flatten path can be imported through factory create and then run to
// a customer-visible terminal success outcome.
func TestExportedFactoryCanBeImportedAndRun(t *testing.T) {
	sourceDir := support.ScaffoldFactory(t, importExportFactoryConfig())
	support.WriteAgentConfig(
		t,
		sourceDir,
		importExportWorkerName,
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)

	exported, err := support.FlattenFactoryConfig(t, filepath.Join(sourceDir, "factory.json"))
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(export source): %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("exported factory payload is empty")
	}

	exportPath := filepath.Join(t.TempDir(), "exported-factory.json")
	if err := os.WriteFile(exportPath, exported, 0o644); err != nil {
		t.Fatalf("write exported factory payload: %v", err)
	}

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	importedDir := support.CreateNamedFactory(
		t,
		homeDir,
		workingDir,
		importExportImportedName,
		exportPath,
	)

	if _, err := os.Stat(filepath.Join(importedDir, "factory.json")); err != nil {
		t.Fatalf("imported factory.json missing at %s: %v", importedDir, err)
	}
	importedFactory, err := support.LoadedFactory(t, filepath.Join(importedDir, "factory.json"))
	if err != nil {
		t.Fatalf("load imported factory through public flatten readback: %v", err)
	}
	if importedFactory.Name == "" {
		t.Fatalf("imported factory name = %q, want non-empty customer-visible identity", importedFactory.Name)
	}

	testutil.WriteSeedFile(
		t,
		importedDir,
		importExportWorkType,
		[]byte(`{"title":"exported factory imported and run"}`),
	)

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		importedDir,
		edges,
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, importExportWorkType+":complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, importExportWorkType+":failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
}

func importExportFactoryConfig() map[string]any {
	return map[string]any{
		"name": importExportSourceName,
		"workTypes": []map[string]any{
			{
				"name": importExportWorkType,
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": importExportWorkerName},
		},
		"workstations": []map[string]any{
			{
				"name":      importExportWorkstationName,
				"worker":    importExportWorkerName,
				"inputs":    []map[string]string{{"workType": importExportWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": importExportWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": importExportWorkType, "state": "failed"}},
			},
		},
	}
}