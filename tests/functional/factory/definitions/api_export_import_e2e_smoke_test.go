package definitions

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type exportImportSmokeHarnessOptions struct {
	sourceFactoryName string
	importFactoryName string
	beforeExport      func(*testing.T, string)
}

type exportImportSmokeHarnessOption func(*exportImportSmokeHarnessOptions)

type exportImportSmokeHarnessResult struct {
	env              []string
	workingDirectory string
	fixture          serviceSimpleExportImportFixture
	ExportedFactory  factoryapi.Factory
	ImportRequest    factoryapi.Factory
	ImportedFactory  factoryapi.Factory
	CurrentFactory   factoryapi.Factory
	SourceFactoryDir string
	ImportedDir      string
}

func withExportImportSmokeBeforeExport(fn func(*testing.T, string)) exportImportSmokeHarnessOption {
	return func(options *exportImportSmokeHarnessOptions) {
		options.beforeExport = fn
	}
}

func runExportImportSmokeViaCLI(
	t *testing.T,
	fixture serviceSimpleExportImportFixture,
	opts ...exportImportSmokeHarnessOption,
) exportImportSmokeHarnessResult {
	t.Helper()

	options := exportImportSmokeHarnessOptions{
		sourceFactoryName: "exported-service-simple",
		importFactoryName: "reimported-service-simple",
	}
	for _, opt := range opts {
		opt(&options)
	}

	runner := support.NewRecordingCommandRunner("export/import smoke must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	namedFactoriesRoot := initializeImportExportCustomerHome(t, env, workingDir)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "factory.json")
	if err := os.WriteFile(sourcePath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write export source factory.json: %v", err)
	}
	sourceFactoryDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		options.sourceFactoryName,
		sourcePath,
	)
	if options.beforeExport != nil {
		options.beforeExport(t, sourceFactoryDir)
	}

	exportedPayload, err := flattenFactoryConfigWithEdges(
		t,
		edges,
		filepath.Join(sourceFactoryDir, "factory.json"),
	)
	if err != nil {
		t.Fatalf("flattenFactoryConfigWithEdges(exported source): %v", err)
	}
	exportedFactory, err := decodeFactoryDefinitionForTest(exportedPayload)
	if err != nil {
		t.Fatalf("decode exported factory: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), options.importFactoryName+".json")
	if err := os.WriteFile(exportPath, exportedPayload, 0o644); err != nil {
		t.Fatalf("write reexported factory payload: %v", err)
	}

	importRequest := exportedFactory
	importRequest.Name = factoryapi.FactoryName(options.importFactoryName)

	importedDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		options.importFactoryName,
		exportPath,
	)
	importedFactory, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode imported factory: %v", err)
	}
	currentFactory, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode current factory after import: %v", err)
	}

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during export/import smoke", runner.CallCount())
	}

	return exportImportSmokeHarnessResult{
		env:              env,
		workingDirectory: workingDir,
		fixture:          fixture,
		ExportedFactory:  exportedFactory,
		ImportRequest:    importRequest,
		ImportedFactory:  importedFactory,
		CurrentFactory:   currentFactory,
		SourceFactoryDir: sourceFactoryDir,
		ImportedDir:      importedDir,
	}
}

func (result exportImportSmokeHarnessResult) assertAPIContractSuccess(t *testing.T) {
	t.Helper()

	fixture := result.fixture
	if result.ExportedFactory.Name == "" {
		t.Fatal("exported current factory name is empty")
	}
	if comparableServiceSimpleExportImportFactoryJSON(result.ExportedFactory) !=
		comparableServiceSimpleExportImportFactoryJSON(fixture.GeneratedExportFactor) {
		t.Fatalf(
			"exported current factory diverged from canonical generated payload\ngot JSON:\n%s\nwant JSON:\n%s",
			comparableServiceSimpleExportImportFactoryJSON(result.ExportedFactory),
			comparableServiceSimpleExportImportFactoryJSON(fixture.GeneratedExportFactor),
		)
	}
	if comparableServiceSimpleExportImportFactoryJSON(result.ImportedFactory) !=
		comparableServiceSimpleExportImportFactoryJSON(result.ImportRequest) {
		t.Fatalf(
			"imported factory diverged from submitted payload\ngot JSON:\n%s\nwant JSON:\n%s",
			comparableServiceSimpleExportImportFactoryJSON(result.ImportedFactory),
			comparableServiceSimpleExportImportFactoryJSON(result.ImportRequest),
		)
	}
	if comparableServiceSimpleExportImportFactoryJSON(result.CurrentFactory) !=
		comparableServiceSimpleExportImportFactoryJSON(result.ImportRequest) {
		t.Fatalf(
			"current factory readback diverged from imported payload\ngot JSON:\n%s\nwant JSON:\n%s",
			comparableServiceSimpleExportImportFactoryJSON(result.CurrentFactory),
			comparableServiceSimpleExportImportFactoryJSON(result.ImportRequest),
		)
	}
}

func (result exportImportSmokeHarnessResult) assertDashboardActivationSuccess(t *testing.T) {
	t.Helper()

	assertServiceSimpleExportImportCurrentFactorySignals(
		t,
		result.fixture,
		result.env,
		result.workingDirectory,
		string(result.ImportRequest.Name),
		result.ImportedDir,
	)
}

// TestExportImportSmokeExportedFactoryCanBeReimportedThroughCustomerPath proves an
// exported Factory can be reimported through public factory create, activated as
// Current Factory, and accept work while rejecting stale work types.
func TestExportImportSmokeExportedFactoryCanBeReimportedThroughCustomerPath(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)
	result := runExportImportSmokeViaCLI(t, fixture)

	result.assertAPIContractSuccess(t)
	result.assertDashboardActivationSuccess(t)

	runner := support.NewRecordingCommandRunner("work submission smoke must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                result.ImportedDir,
		WorkingDirectory:          result.workingDirectory,
		Env:                       result.env,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)
	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)

	importedSubmit := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         ptrString("export-import-smoke"),
		WorkTypeName: fixture.Expected.WorkTypeName,
		Payload:      []byte(`{"title":"reimported-service-simple"}`),
	})
	if importedSubmit.TraceId == "" {
		t.Fatal("imported factory should accept work through POST /work")
	}

	legacyResp := submitExportImportSmokeWorkExpectStatus(
		t,
		server.URL(),
		"legacy-"+fixture.Expected.WorkTypeName,
		"legacy",
		http.StatusBadRequest,
	)
	var legacyErr factoryapi.ErrorResponse
	decodeExportImportSmokeJSONResponse(t, legacyResp, &legacyErr, "decode legacy work type error response")
	if legacyErr.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("legacy work type error code = %q, want BAD_REQUEST", legacyErr.Code)
	}

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during work submission smoke", runner.CallCount())
	}
}

// TestExportImportSmokePreservesBatchInboxGitkeepAfterImport proves batch inbox
// .gitkeep sentinels seeded before export survive import through public paths.
func TestExportImportSmokePreservesBatchInboxGitkeepAfterImport(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)
	result := runExportImportSmokeViaCLI(
		t,
		fixture,
		withExportImportSmokeBeforeExport(seedExportImportBatchInboxGitkeep),
	)

	result.assertAPIContractSuccess(t)
	assertExportImportBatchInboxGitkeepOnDisk(t, result.ImportedDir)
}

// TestExportImportSmokePreservesPortableLayoutThroughExportImportAndActivation proves
// portable layout metadata survives export, import, and Current Factory activation.
func TestExportImportSmokePreservesPortableLayoutThroughExportImportAndActivation(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)
	result := runExportImportSmokeViaCLI(t, fixture)

	result.assertAPIContractSuccess(t)
	assertExportImportPortableLayoutResponse(t, result.ExportedFactory.Layout, "exported factory")
	assertExportImportPortableLayoutResponse(t, result.ImportedFactory.Layout, "imported factory")
	assertExportImportPortableLayoutResponse(t, result.CurrentFactory.Layout, "current factory after import")

	factoryJSON, err := os.ReadFile(filepath.Join(result.ImportedDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(imported factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(imported factory.json): %v", err)
	}
	assertExportImportPortableLayoutPayload(t, persisted["layout"], "persisted imported factory")
}

// TestExportImportSmokeImportedFactoryPersistsThinSplitRuntimeLayout proves imported
// factories persist thin split runtime bodies on disk and reload them through the
// public flatten readback boundary.
func TestExportImportSmokeImportedFactoryPersistsThinSplitRuntimeLayout(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)
	result := runExportImportSmokeViaCLI(t, fixture)

	assertImportedFactoryLayoutOmitsInlineRuntimeBodies(t, result.ImportedDir)
	assertImportedPortableBundledFilesPersistThinAndMaterializeOnDisk(t, result.ImportedDir)
	assertImportedWorkerBodiesPersistOnlyInAgentsFiles(
		t,
		result.ImportedDir,
		exportImportValueOrEmpty(result.ImportedFactory.Workers),
	)
	assertImportedWorkstationBodiesPersistOnlyInAgentsFiles(
		t,
		result.ImportedDir,
		exportImportValueOrEmpty(result.ImportedFactory.Workstations),
	)
	assertImportedFactoryRuntimeReloadPreservesBodies(
		t,
		result.ImportedDir,
		exportImportValueOrEmpty(result.ImportedFactory.Workers),
		exportImportValueOrEmpty(result.ImportedFactory.Workstations),
	)
}

// TestExportImportSmokePublicShareImportSurfaceCarriesDetachedStarterWork proves
// public share import carries detached starter Work bundles that remain isolated
// across source and imported factories.
func TestExportImportSmokePublicShareImportSurfaceCarriesDetachedStarterWork(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)

	sourceHome := t.TempDir()
	sourceWorkingDir := t.TempDir()
	sourceEnv := append(os.Environ(), "HOME="+sourceHome, "USERPROFILE="+sourceHome)
	sourceFactoriesRoot := initializeImportExportCustomerHome(t, sourceEnv, sourceWorkingDir)

	importHome := t.TempDir()
	importWorkingDir := t.TempDir()
	importEnv := append(os.Environ(), "HOME="+importHome, "USERPROFILE="+importHome)
	importFactoriesRoot := initializeImportExportCustomerHome(t, importEnv, importWorkingDir)

	const (
		importBootstrapFactoryName = "seeded-share-bootstrap"
		sourceFactoryName          = "seeded-share-source"
		importFactoryName          = "seeded-share-imported"
	)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "factory.json")
	if err := os.WriteFile(sourcePath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write source factory.json: %v", err)
	}
	sourceFactoryDir := createImportExportActivatedNamedFactory(
		t,
		sourceEnv,
		sourceWorkingDir,
		sourceFactoriesRoot,
		sourceFactoryName,
		sourcePath,
	)

	bootstrapDir := t.TempDir()
	bootstrapPath := filepath.Join(bootstrapDir, "factory.json")
	if err := os.WriteFile(bootstrapPath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write bootstrap factory.json: %v", err)
	}
	createImportExportActivatedNamedFactory(
		t,
		importEnv,
		importWorkingDir,
		importFactoriesRoot,
		importBootstrapFactoryName,
		bootstrapPath,
	)

	sourceSnapshot := map[string]string{
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/default/starter.md":    "source starter markdown\n",
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/exec-123/request.json": "{\"title\":\"starter request\"}\n",
	}
	writeExportImportSeededStarterInputs(t, sourceFactoryDir, sourceSnapshot)

	runner := support.NewRecordingCommandRunner("public share import smoke must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	exportedPayload := mustFlattenFactoryConfigWithEdges(
		t,
		edges,
		filepath.Join(sourceFactoryDir, "factory.json"),
	)
	exportedFactory, err := decodeFactoryDefinitionForTest(exportedPayload)
	if err != nil {
		t.Fatalf("decode exported source factory: %v", err)
	}
	assertExportImportStarterBundledFiles(t, exportedFactory, sourceSnapshot)

	exportPath := filepath.Join(t.TempDir(), importFactoryName+".json")
	if err := os.WriteFile(exportPath, exportedPayload, 0o644); err != nil {
		t.Fatalf("write shared export payload: %v", err)
	}
	importedDir := createImportExportActivatedNamedFactory(
		t,
		importEnv,
		importWorkingDir,
		importFactoriesRoot,
		importFactoryName,
		exportPath,
	)
	importedFactory, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode imported factory: %v", err)
	}
	assertExportImportStarterBundledFileTargets(t, importedFactory, sourceSnapshot)

	importedCurrent, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode imported current factory: %v", err)
	}
	assertExportImportStarterBundledFiles(t, importedCurrent, sourceSnapshot)

	sourceUpdatedSnapshot := map[string]string{
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/default/starter.md":    "source starter markdown updated\n",
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/exec-123/request.json": "{\"title\":\"source request updated\"}\n",
	}
	importedSnapshot := map[string]string{
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/default/starter.md":    "imported starter markdown\n",
		"factory/inputs/" + fixture.Expected.WorkTypeName + "/exec-123/request.json": "{\"title\":\"imported request\"}\n",
	}
	writeExportImportSeededStarterInputs(t, sourceFactoryDir, sourceUpdatedSnapshot)
	writeExportImportSeededStarterInputs(t, importedDir, importedSnapshot)

	sourceCurrent, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(sourceFactoryDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode updated source factory: %v", err)
	}
	assertExportImportStarterBundledFiles(t, sourceCurrent, sourceUpdatedSnapshot)

	importedUpdatedCurrent, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(t, edges, filepath.Join(importedDir, "factory.json")),
	)
	if err != nil {
		t.Fatalf("decode updated imported current factory: %v", err)
	}
	assertExportImportStarterBundledFiles(t, importedUpdatedCurrent, importedSnapshot)

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during public share import smoke", runner.CallCount())
	}
}

func assertExportImportPortableLayoutResponse(t *testing.T, layout *factoryapi.FactoryLayout, contextLabel string) {
	t.Helper()

	if layout == nil {
		t.Fatalf("%s layout = nil, want portable layout metadata", contextLabel)
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("%s layout schemaVersion = %d, want 1", contextLabel, layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 || (*layout.Nodes)[0].Id != "workstation:step-one" {
		t.Fatalf("%s layout nodes = %#v, want workstation:step-one", contextLabel, layout.Nodes)
	}
	if (*layout.Nodes)[0].EmptyState == nil || (*layout.Nodes)[0].EmptyState.Text == nil || *(*layout.Nodes)[0].EmptyState.Text != "No work is waiting." {
		t.Fatalf("%s node empty state = %#v, want literal text", contextLabel, (*layout.Nodes)[0].EmptyState)
	}
	if layout.Edges == nil || len(*layout.Edges) != 1 || (*layout.Edges)[0].Id != "workstation-output:workstation:step-one->work-state:task:processing" {
		t.Fatalf("%s layout edges = %#v, want step-one output edge", contextLabel, layout.Edges)
	}
	if layout.Annotations == nil || len(*layout.Annotations) != 2 {
		t.Fatalf("%s layout annotations = %#v, want note and image", contextLabel, layout.Annotations)
	}
	annotations := *layout.Annotations
	if annotations[0].Note == nil || annotations[0].Note.Body != "Portable guidance\nremains literal." {
		t.Fatalf("%s note annotation = %#v, want literal note", contextLabel, annotations[0])
	}
	if annotations[1].Image == nil || annotations[1].Image.Source.MediaType != factoryapi.FactoryLayoutImageSourceMediaType("image/png") || annotations[1].Image.AlternativeText != "Portable workflow illustration" {
		t.Fatalf("%s image annotation = %#v, want embedded PNG", contextLabel, annotations[1])
	}
	if layout.Viewport == nil || math.Abs(float64(layout.Viewport.Zoom)-0.9) > 1e-6 {
		t.Fatalf("%s layout viewport = %#v, want zoom 0.9", contextLabel, layout.Viewport)
	}
}

func assertExportImportPortableLayoutPayload(t *testing.T, value any, contextLabel string) {
	t.Helper()

	layout, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s layout = %#v, want object", contextLabel, value)
	}
	if got := layout["schemaVersion"]; got != float64(1) {
		t.Fatalf("%s layout schemaVersion = %#v, want 1", contextLabel, got)
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("%s layout nodes = %#v, want one node", contextLabel, layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:step-one" {
		t.Fatalf("%s layout node = %#v, want workstation:step-one", contextLabel, nodes[0])
	}
	emptyState, ok := node["emptyState"].(map[string]any)
	if !ok || emptyState["text"] != "No work is waiting." {
		t.Fatalf("%s node empty state = %#v, want literal text", contextLabel, node["emptyState"])
	}
	annotations, ok := layout["annotations"].([]any)
	if !ok || len(annotations) != 2 {
		t.Fatalf("%s annotations = %#v, want note and image", contextLabel, layout["annotations"])
	}
	image, ok := annotations[1].(map[string]any)["image"].(map[string]any)
	if !ok || image["alternativeText"] != "Portable workflow illustration" {
		t.Fatalf("%s image annotation = %#v, want portable alternative text", contextLabel, annotations[1])
	}
}

func assertImportedFactoryLayoutOmitsInlineRuntimeBodies(t *testing.T, factoryDir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}

	for _, workerEntry := range requireExportImportObjectSlice(t, payload["workers"], "workers") {
		if _, ok := workerEntry["body"]; ok {
			t.Fatalf("imported factory.json worker should omit inline body: %#v", workerEntry)
		}
	}
	for _, workstationEntry := range requireExportImportObjectSlice(t, payload["workstations"], "workstations") {
		if _, ok := workstationEntry["body"]; ok {
			t.Fatalf("imported factory.json workstation should omit inline body: %#v", workstationEntry)
		}
	}
}

func assertImportedWorkerBodiesPersistOnlyInAgentsFiles(t *testing.T, factoryDir string, workers []factoryapi.Worker) {
	t.Helper()

	for _, worker := range workers {
		if worker.Body == nil {
			t.Fatalf("expected imported worker %q to expose a runtime body", worker.Name)
		}
		agentsPath := filepath.Join(factoryDir, "workers", worker.Name, "AGENTS.md")
		contents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", agentsPath, err)
		}
		got := string(contents)
		if got != *worker.Body {
			t.Fatalf("imported worker AGENTS.md for %q = %q, want exact body-only content %q", worker.Name, got, *worker.Body)
		}
		if strings.HasPrefix(got, "---") {
			t.Fatalf("imported worker AGENTS.md for %q should be body-only, got frontmatter:\n%s", worker.Name, got)
		}
	}
}

func assertImportedWorkstationBodiesPersistOnlyInAgentsFiles(
	t *testing.T,
	factoryDir string,
	workstations []factoryapi.Workstation,
) {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Body == nil {
			t.Fatalf("expected imported workstation %q to expose a runtime body", workstation.Name)
		}
		agentsPath := filepath.Join(factoryDir, "workstations", workstation.Name, "AGENTS.md")
		contents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", agentsPath, err)
		}
		got := string(contents)
		if got != *workstation.Body {
			t.Fatalf(
				"imported workstation AGENTS.md for %q = %q, want exact body-only content %q",
				workstation.Name,
				got,
				*workstation.Body,
			)
		}
		if strings.HasPrefix(got, "---") {
			t.Fatalf("imported workstation AGENTS.md for %q should be body-only, got frontmatter:\n%s", workstation.Name, got)
		}
	}
}

func assertImportedFactoryRuntimeReloadPreservesBodies(
	t *testing.T,
	factoryDir string,
	workers []factoryapi.Worker,
	workstations []factoryapi.Workstation,
) {
	t.Helper()

	loaded, err := support.LoadedFactory(t, filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", factoryDir, err)
	}

	for _, worker := range workers {
		runtimeWorker, ok := support.FindFactoryWorker(loaded, worker.Name)
		if !ok {
			t.Fatalf("expected imported runtime worker %q to load", worker.Name)
		}
		if worker.Body == nil || runtimeWorker.Body == nil || *runtimeWorker.Body != *worker.Body {
			t.Fatalf(
				"runtime worker %q body = %q, want %q",
				worker.Name,
				exportImportStringPtrValue(runtimeWorker.Body),
				exportImportStringPtrValue(worker.Body),
			)
		}
	}
	for _, workstation := range workstations {
		runtimeWorkstation, ok := support.FindFactoryWorkstation(loaded, workstation.Name)
		if !ok {
			t.Fatalf("expected imported runtime workstation %q to load", workstation.Name)
		}
		if workstation.Body == nil || runtimeWorkstation.Body == nil || *runtimeWorkstation.Body != *workstation.Body {
			t.Fatalf(
				"runtime workstation %q body = %q, want %q",
				workstation.Name,
				exportImportStringPtrValue(runtimeWorkstation.Body),
				exportImportStringPtrValue(workstation.Body),
			)
		}
	}
}

func assertImportedPortableBundledFilesPersistThinAndMaterializeOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	assertImportedPortableFile(t, filepath.Join(factoryDir, "Makefile"), serviceSimpleExportImportMakefileBody)
	assertImportedPortableFile(t, filepath.Join(factoryDir, "docs", "README.md"), serviceSimpleExportImportDocBody)
	assertImportedPortableFile(
		t,
		filepath.Join(factoryDir, "scripts", "execute-story.ps1"),
		serviceSimpleExportImportScriptBody,
	)

	data, err := os.ReadFile(filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}

	supportingFiles, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("expected 3 persisted bundled files, got %#v", supportingFiles["bundledFiles"])
	}

	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", bundledFile["content"])
		}

		targetPath, _ := bundledFile["targetPath"].(string)
		switch targetPath {
		case serviceSimpleExportImportMakefilePath:
			if got := content["inline"]; got != serviceSimpleExportImportMakefileBody {
				t.Fatalf("persisted root helper inline = %#v, want %q", got, serviceSimpleExportImportMakefileBody)
			}
		case serviceSimpleExportImportDocPath, serviceSimpleExportImportScriptPath:
			if _, ok := content["inline"]; ok {
				t.Fatalf("persisted bundled inline for %q should be omitted, got %#v", targetPath, content["inline"])
			}
		default:
			t.Fatalf("unexpected persisted bundled targetPath = %#v", targetPath)
		}
	}
}

func assertImportedPortableFile(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func writeExportImportSeededStarterInputs(t *testing.T, factoryDir string, files map[string]string) {
	t.Helper()

	for portablePath, content := range files {
		relativePath, found := strings.CutPrefix(portablePath, "factory/")
		if !found {
			t.Fatalf("portable starter path %q must begin with factory/", portablePath)
		}
		fullPath := filepath.Join(factoryDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", fullPath, err)
		}
	}
}

func assertExportImportStarterBundledFiles(t *testing.T, factory factoryapi.Factory, want map[string]string) {
	t.Helper()

	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("factory %q missing supportingFiles bundledFiles", factory.Name)
	}

	got := map[string]string{}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.Type != factoryapi.BundledFileType(interfaces.BundledFileTypeInput) {
			continue
		}
		got[bundledFile.TargetPath] = bundledFile.Content.Inline
	}
	if len(got) != len(want) {
		t.Fatalf("starter bundled files for %q = %#v, want %#v", factory.Name, got, want)
	}
	for targetPath, wantContent := range want {
		if got[targetPath] != wantContent {
			t.Fatalf(
				"starter bundled file %q for %q = %q, want %q",
				targetPath,
				factory.Name,
				got[targetPath],
				wantContent,
			)
		}
	}
}

func assertExportImportStarterBundledFileTargets(t *testing.T, factory factoryapi.Factory, want map[string]string) {
	t.Helper()

	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("factory %q missing supportingFiles bundledFiles", factory.Name)
	}

	got := map[string]struct{}{}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.Type != factoryapi.BundledFileType(interfaces.BundledFileTypeInput) {
			continue
		}
		got[bundledFile.TargetPath] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("starter bundled file targets for %q = %#v, want %#v", factory.Name, got, want)
	}
	for targetPath := range want {
		if _, ok := got[targetPath]; !ok {
			t.Fatalf("starter bundled file target %q missing from %q", targetPath, factory.Name)
		}
	}
}

func seedExportImportBatchInboxGitkeep(t *testing.T, factoryDir string) {
	t.Helper()

	gitkeepPath := filepath.Join(factoryDir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName, ".gitkeep")
	if err := os.MkdirAll(filepath.Dir(gitkeepPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(batch inbox): %v", err)
	}
	if err := os.WriteFile(gitkeepPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(batch .gitkeep): %v", err)
	}
}

func assertExportImportBatchInboxGitkeepOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	gitkeepPath := filepath.Join(factoryDir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName, ".gitkeep")
	info, err := os.Stat(gitkeepPath)
	if err != nil {
		t.Fatalf("inputs/BATCH/default/.gitkeep after import: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("inputs/BATCH/default/.gitkeep after import: got directory, want regular file")
	}
}

func submitExportImportSmokeWorkExpectStatus(
	t *testing.T,
	serverURL, workTypeName, title string,
	wantStatus int,
) *http.Response {
	t.Helper()

	name := "export-import-smoke"
	request := factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: workTypeName,
		Payload:      []byte(`{"title":"` + title + `"}`),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(serverURL, "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work status = %d, want %d", resp.StatusCode, wantStatus)
	}
	return resp
}

func decodeExportImportSmokeJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func requireExportImportObjectSlice(t *testing.T, value any, field string) []map[string]any {
	t.Helper()

	entries, ok := value.([]any)
	if !ok {
		t.Fatalf("expected %s to be an array, got %#v", field, value)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected %s entry to be an object, got %#v", field, entry)
		}
		out = append(out, obj)
	}
	return out
}

func exportImportStringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ptrString(value string) *string {
	return &value
}
