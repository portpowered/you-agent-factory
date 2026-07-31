package definitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	importExportCurrentName     = "current-safe"
	importExportWorkerName      = "worker-a"
	importExportWorkstationName = "process"

	importExportPortableDocPath    = "factory/docs/standards/review.md"
	importExportPortableDocBody    = "# Review standards\n"
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

// TestInvalidImportDoesNotReplaceCurrentFactory proves an invalid import through
// the public update path is rejected with a customer-visible failure and leaves
// the prior Current Factory definition unchanged on public readback.
func TestInvalidImportDoesNotReplaceCurrentFactory(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute during invalid import")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	namedFactoriesRoot := initializeImportExportCustomerHome(t, env, workingDir)

	sourceDir := support.ScaffoldFactory(t, importExportCurrentFactoryConfig())
	support.WriteAgentConfig(
		t,
		sourceDir,
		importExportWorkerName,
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	sourcePath := filepath.Join(sourceDir, "factory.json")

	factoryDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		importExportCurrentName,
		sourcePath,
	)

	baselineFactory, err := support.LoadedFactory(t, filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("load baseline current factory: %v", err)
	}
	if baselineFactory.Name != factoryapi.FactoryName(importExportCurrentName) {
		t.Fatalf(
			"baseline factory name = %q, want %q",
			baselineFactory.Name,
			importExportCurrentName,
		)
	}

	invalidDir := support.ScaffoldFactory(t, importExportInvalidImportFactoryConfig())
	invalidPath := filepath.Join(invalidDir, "factory.json")

	updateInputs := support.FakeInputs(t.Context(), []string{
		"you",
		"factory",
		"update",
		importExportCurrentName,
		"--from",
		invalidPath,
		"--dir",
		namedFactoriesRoot,
	})
	updateInputs.Input.Env = env
	updateInputs.Input.WorkingDirectory = workingDir
	updateErr := support.BuildProcess(t, edges).Execute(updateInputs.Input)
	if updateErr == nil {
		t.Fatalf(
			"Process.Execute(factory update invalid import) error = nil, want customer-visible failure; stdout=%q stderr=%q",
			updateInputs.Stdout(),
			updateInputs.Stderr(),
		)
	}
	diagnostic := updateErr.Error() + "\n" + updateInputs.Stdout() + "\n" + updateInputs.Stderr()
	if !strings.Contains(diagnostic, "invalid factory config") {
		t.Fatalf("diagnostic missing invalid import marker:\n%s", diagnostic)
	}
	if !strings.Contains(diagnostic, validationCodeDanglingWorkerReference) {
		t.Fatalf(
			"diagnostic missing validation code %q:\n%s",
			validationCodeDanglingWorkerReference,
			diagnostic,
		)
	}

	reloadedFactory, err := support.LoadedFactory(t, filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("reload current factory after rejected import: %v", err)
	}
	assertImportExportCurrentFactoryUnchanged(t, baselineFactory, reloadedFactory)

	listInputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	listInputs.Input.Env = env
	listInputs.Input.WorkingDirectory = workingDir
	if err := support.BuildProcess(t, edges).Execute(listInputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			listInputs.Stdout(),
			listInputs.Stderr(),
		)
	}
	var listEntries []importExportFactoryListEntry
	if err := json.Unmarshal([]byte(listInputs.Stdout()), &listEntries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, listInputs.Stdout())
	}
	foundCurrent := false
	for _, entry := range listEntries {
		if entry.Name != importExportCurrentName {
			continue
		}
		foundCurrent = true
		if entry.FactoryDirectory != factoryDir {
			t.Fatalf(
				"current factory directory = %q, want %q",
				entry.FactoryDirectory,
				factoryDir,
			)
		}
		if !entry.Current {
			t.Fatalf("factory list current flag = false, want true for %q", importExportCurrentName)
		}
	}
	if !foundCurrent {
		t.Fatalf("factory list missing current factory %q; entries=%#v", importExportCurrentName, listEntries)
	}

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during rejected import", runner.CallCount())
	}
}

type importExportFactoryListEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	Current          bool   `json:"current"`
}

func initializeImportExportCustomerHome(t *testing.T, env []string, workingDirectory string) string {
	t.Helper()

	homeDir := importExportEnvironmentHome(env)
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory,
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf(
			"Process.Execute(run missing Factory) error = %v, want missing Factory diagnostic\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return namedFactoriesRoot
}

func createImportExportActivatedNamedFactory(
	t *testing.T,
	env []string,
	workingDirectory string,
	namedFactoriesRoot string,
	name string,
	factoryConfigPath string,
) string {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--json",
		"factory",
		"create",
		name,
		"--from",
		factoryConfigPath,
		"--dir",
		namedFactoriesRoot,
		"--set-current",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory create %q --set-current) error = %v\nstdout:\n%s\nstderr:\n%s",
			name,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var result struct {
		FactoryDir string `json:"factoryDir"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode factory create result: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	if _, err := os.Stat(filepath.Join(result.FactoryDir, "factory.json")); err != nil {
		t.Fatalf(
			"activated named Factory %q missing factory.json at %s: %v",
			name,
			result.FactoryDir,
			err,
		)
	}
	return result.FactoryDir
}

func importExportEnvironmentHome(env []string) string {
	for index := len(env) - 1; index >= 0; index-- {
		item := env[index]
		if strings.HasPrefix(item, "HOME=") {
			return strings.TrimPrefix(item, "HOME=")
		}
		if strings.HasPrefix(item, "USERPROFILE=") {
			return strings.TrimPrefix(item, "USERPROFILE=")
		}
	}
	return ""
}

func assertImportExportCurrentFactoryUnchanged(
	t *testing.T,
	baseline factoryapi.Factory,
	reloaded factoryapi.Factory,
) {
	t.Helper()

	if reloaded.Name != baseline.Name {
		t.Fatalf("reloaded factory name = %q, want unchanged %q", reloaded.Name, baseline.Name)
	}
	if reloaded.Id == nil || baseline.Id == nil || *reloaded.Id != *baseline.Id {
		t.Fatalf("reloaded factory id = %#v, want unchanged %#v", reloaded.Id, baseline.Id)
	}
	if baseline.Version != nil && reloaded.Version != nil && *reloaded.Version != *baseline.Version {
		t.Fatalf("reloaded factory version = %#v, want unchanged %#v", reloaded.Version, baseline.Version)
	}
	assertImportExportWorkTypePresent(t, reloaded, importExportWorkType, "reloaded current factory")
}

func assertImportExportWorkTypePresent(
	t *testing.T,
	factory factoryapi.Factory,
	workType string,
	contextLabel string,
) {
	t.Helper()

	if factory.WorkTypes == nil {
		t.Fatalf("%s workTypes = nil, want %q", contextLabel, workType)
	}
	for _, candidate := range *factory.WorkTypes {
		if candidate.Name == workType {
			return
		}
	}
	t.Fatalf("%s missing work type %q in %#v", contextLabel, workType, factory.WorkTypes)
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

func importExportCurrentFactoryConfig() map[string]any {
	cfg := importExportFactoryConfig()
	cfg["name"] = importExportCurrentName
	cfg["id"] = importExportCurrentName
	return cfg
}

func importExportInvalidImportFactoryConfig() map[string]any {
	return map[string]any{
		"name": "invalid-import-payload",
		"id":   "invalid-import-payload",
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
				"worker":    "missing-worker",
				"inputs":    []map[string]string{{"workType": importExportWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": importExportWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": importExportWorkType, "state": "failed"}},
			},
		},
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
