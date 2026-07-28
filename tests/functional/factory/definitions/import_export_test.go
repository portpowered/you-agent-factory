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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	importExportWorkType        = "task"
	importExportSourceName      = "export-source"
	importExportImportedName    = "imported-roundtrip"
	importExportPortableName    = "imported-portable"
	importExportWorkerName      = "worker-a"
	importExportWorkstationName = "process"

	importExportPortableDocPath    = "factory/docs/standards/review.md"
	importExportPortableDocBody   = "# Review standards\n"
	importExportPortableScriptPath = "factory/scripts/execute-task.sh"
	importExportPortableScriptBody = "#!/bin/sh\necho 'portable script'\n"
	importExportPortableNoteBody   = "Portable guidance remains literal."
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

// TestImportExportPreservesNestedDocsScriptsAndMetadata proves nested docs,
// scripts, and portable layout metadata authored with a Factory survive export
// and import through the public flatten and create customer paths.
func TestImportExportPreservesNestedDocsScriptsAndMetadata(t *testing.T) {
	sourceDir := support.ScaffoldFactory(t, importExportPortableFactoryConfig())
	support.WriteAgentConfig(
		t,
		sourceDir,
		importExportWorkerName,
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	seedImportExportPortableFilesOnDisk(t, sourceDir)

	exported, err := support.FlattenFactoryConfig(t, filepath.Join(sourceDir, "factory.json"))
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(export source): %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("exported factory payload is empty")
	}

	exportedFactory, err := support.DecodeFactoryDefinition(exported)
	if err != nil {
		t.Fatalf("decode exported factory payload: %v", err)
	}
	assertImportExportPortableBundledFileInline(
		t,
		exportedFactory,
		factoryapi.BundledFileTypeDOC,
		importExportPortableDocPath,
		importExportPortableDocBody,
		"exported factory",
	)
	assertImportExportPortableBundledFileInline(
		t,
		exportedFactory,
		factoryapi.BundledFileTypeSCRIPT,
		importExportPortableScriptPath,
		importExportPortableScriptBody,
		"exported factory",
	)
	assertImportExportPortableLayoutNote(t, exportedFactory, "exported factory")

	exportPath := filepath.Join(t.TempDir(), "exported-portable-factory.json")
	if err := os.WriteFile(exportPath, exported, 0o644); err != nil {
		t.Fatalf("write exported factory payload: %v", err)
	}

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	importedDir := support.CreateNamedFactory(
		t,
		homeDir,
		workingDir,
		importExportPortableName,
		exportPath,
	)

	assertImportExportPortableFileOnDisk(
		t,
		filepath.Join(importedDir, "docs", "standards", "review.md"),
		importExportPortableDocBody,
	)
	assertImportExportPortableFileOnDisk(
		t,
		filepath.Join(importedDir, "scripts", "execute-task.sh"),
		importExportPortableScriptBody,
	)

	importedFactory, err := support.LoadedFactory(t, filepath.Join(importedDir, "factory.json"))
	if err != nil {
		t.Fatalf("load imported factory through public flatten readback: %v", err)
	}
	assertImportExportPortableBundledFile(
		t,
		importedFactory,
		factoryapi.BundledFileTypeDOC,
		importExportPortableDocPath,
		"imported factory",
	)
	assertImportExportPortableBundledFile(
		t,
		importedFactory,
		factoryapi.BundledFileTypeSCRIPT,
		importExportPortableScriptPath,
		"imported factory",
	)
	assertImportExportPortableLayoutNote(t, importedFactory, "imported factory")
}

func seedImportExportPortableFilesOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	docPath := filepath.Join(factoryDir, "docs", "standards", "review.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(docPath), err)
	}
	if err := os.WriteFile(docPath, []byte(importExportPortableDocBody), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", docPath, err)
	}

	scriptPath := filepath.Join(factoryDir, "scripts", "execute-task.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(scriptPath), err)
	}
	if err := os.WriteFile(scriptPath, []byte(importExportPortableScriptBody), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", scriptPath, err)
	}
}

func assertImportExportPortableFileOnDisk(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertImportExportPortableBundledFile(
	t *testing.T,
	factory factoryapi.Factory,
	wantType factoryapi.BundledFileType,
	targetPath string,
	contextLabel string,
) {
	t.Helper()

	bundledFile, ok := findImportExportBundledFileByTargetPath(factory, targetPath)
	if !ok {
		t.Fatalf("%s missing bundled file target %q", contextLabel, targetPath)
	}
	if bundledFile.Type != wantType {
		t.Fatalf(
			"%s bundled file %q type = %q, want %q",
			contextLabel,
			targetPath,
			bundledFile.Type,
			wantType,
		)
	}
}

func assertImportExportPortableBundledFileInline(
	t *testing.T,
	factory factoryapi.Factory,
	wantType factoryapi.BundledFileType,
	targetPath string,
	wantInline string,
	contextLabel string,
) {
	t.Helper()

	assertImportExportPortableBundledFile(t, factory, wantType, targetPath, contextLabel)

	bundledFile, _ := findImportExportBundledFileByTargetPath(factory, targetPath)
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf(
			"%s bundled file %q inline = %q, want %q",
			contextLabel,
			targetPath,
			bundledFile.Content.Inline,
			wantInline,
		)
	}
}

func assertImportExportPortableLayoutNote(t *testing.T, factory factoryapi.Factory, contextLabel string) {
	t.Helper()

	if factory.Layout == nil || factory.Layout.Annotations == nil {
		t.Fatalf("%s missing portable layout annotations", contextLabel)
	}
	for _, annotation := range *factory.Layout.Annotations {
		if annotation.Kind != factoryapi.FactoryLayoutAnnotationKindNOTE {
			continue
		}
		if annotation.Note == nil {
			continue
		}
		if annotation.Note.Body == importExportPortableNoteBody {
			return
		}
	}
	t.Fatalf("%s missing portable layout note %q", contextLabel, importExportPortableNoteBody)
}

func findImportExportBundledFileByTargetPath(
	factory factoryapi.Factory,
	targetPath string,
) (factoryapi.BundledFile, bool) {
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		return factoryapi.BundledFile{}, false
	}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile, true
		}
	}
	return factoryapi.BundledFile{}, false
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

func importExportPortableFactoryConfig() map[string]any {
	cfg := importExportFactoryConfig()
	cfg["name"] = importExportSourceName
	cfg["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"id":         importExportPortableDocPath,
				"type":       "DOC",
				"targetPath": importExportPortableDocPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   importExportPortableDocBody,
				},
			},
			{
				"id":         importExportPortableScriptPath,
				"type":       "SCRIPT",
				"targetPath": importExportPortableScriptPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   importExportPortableScriptBody,
				},
			},
		},
	}
	cfg["layout"] = map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id":       "workstation:" + importExportWorkstationName,
			"position": map[string]any{"x": 128, "y": 256},
		}},
		"annotations": []map[string]any{{
			"id":       "portable-note",
			"kind":     "NOTE",
			"position": map[string]any{"x": -80, "y": 120},
			"note": map[string]any{
				"body": importExportPortableNoteBody,
				"tone": "INFO",
			},
		}},
		"viewport": map[string]any{"x": 40, "y": 60, "zoom": 0.9},
	}
	return cfg
}