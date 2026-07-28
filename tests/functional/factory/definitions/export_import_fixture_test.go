package definitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	serviceSimpleExportImportMakefilePath = "Makefile"
	serviceSimpleExportImportMakefileBody = "test:\n\tgo test ./...\n"
	serviceSimpleExportImportDocPath      = "factory/docs/README.md"
	serviceSimpleExportImportDocBody        = "# Portable factory\n"
	serviceSimpleExportImportScriptPath   = "factory/scripts/execute-story.ps1"
	serviceSimpleExportImportScriptBody   = "Write-Output 'portable script'\n"
)

type serviceSimpleExportImportExpectations struct {
	TerminalPlaceID  string
	WorkTypeName     string
	WorkstationNames []string
}

type serviceSimpleExportImportFixture struct {
	AuthoredFactoryDir    string
	CanonicalFactoryJSON  []byte
	Expected              serviceSimpleExportImportExpectations
	FlattenedFactory      factoryapi.Factory
	GeneratedExportFactor factoryapi.Factory
}

func newServiceSimpleExportImportFixture(t *testing.T) serviceSimpleExportImportFixture {
	t.Helper()

	authoredFactoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))

	runner := support.NewRecordingCommandRunner("fixture flatten must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	canonicalFactoryJSON, err := flattenFactoryConfigWithEdges(
		t,
		edges,
		filepath.Join(authoredFactoryDir, "factory.json"),
	)
	if err != nil {
		t.Fatalf("flattenFactoryConfigWithEdges(%s): %v", authoredFactoryDir, err)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during flatten", runner.CallCount())
	}

	canonicalFactoryJSON = withServiceSimpleExportImportPortableBundledFiles(t, canonicalFactoryJSON)
	canonicalFactoryJSON = withServiceSimpleExportImportPortableLayout(t, canonicalFactoryJSON)

	flattenedFactory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(canonicalFactoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}

	return serviceSimpleExportImportFixture{
		AuthoredFactoryDir:    authoredFactoryDir,
		CanonicalFactoryJSON:  canonicalFactoryJSON,
		Expected:              buildServiceSimpleExportImportExpectations(t, flattenedFactory),
		FlattenedFactory:      flattenedFactory,
		GeneratedExportFactor: flattenedFactory,
	}
}

func withServiceSimpleExportImportPortableLayout(t *testing.T, canonicalFactoryJSON []byte) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(canonicalFactoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(canonical export/import fixture): %v", err)
	}

	payload["layout"] = map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id":         "workstation:step-one",
			"position":   map[string]any{"x": 128, "y": 256},
			"size":       map[string]any{"width": 320, "height": 180},
			"locked":     true,
			"emptyState": map[string]any{"text": "No work is waiting."},
		}},
		"edges": []map[string]any{{
			"id":        "workstation-output:workstation:step-one->work-state:task:processing",
			"waypoints": []map[string]any{{"x": 180, "y": 220}},
		}},
		"groups": []map[string]any{{
			"id":      "group-1",
			"label":   "Main lane",
			"nodeIds": []string{"workstation:step-one"},
			"bounds":  map[string]any{"x": 100, "y": 200, "width": 400, "height": 240},
		}},
		"annotations": []map[string]any{
			{
				"id":       "portable-note",
				"kind":     "NOTE",
				"position": map[string]any{"x": -80, "y": 120},
				"note": map[string]any{
					"body": "Portable guidance\nremains literal.",
					"tone": "INFO",
				},
			},
			{
				"id":       "portable-image",
				"kind":     "IMAGE",
				"position": map[string]any{"x": 560, "y": 120},
				"size":     map[string]any{"width": 160, "height": 90},
				"image": map[string]any{
					"source": map[string]any{
						"kind":      "EMBEDDED",
						"mediaType": "image/png",
						"data":      "AQID",
					},
					"alternativeText": "Portable workflow illustration",
				},
			},
		},
		"viewport":    map[string]any{"x": 40, "y": 60, "zoom": 0.9},
		"preferences": map[string]any{"direction": "RIGHT"},
	}

	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(canonical export/import fixture with layout): %v", err)
	}
	return updated
}

func withServiceSimpleExportImportPortableBundledFiles(t *testing.T, canonicalFactoryJSON []byte) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(canonicalFactoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(canonical export/import fixture): %v", err)
	}

	payload["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"id":         serviceSimpleExportImportMakefilePath,
				"type":       "ROOT_HELPER",
				"targetPath": serviceSimpleExportImportMakefilePath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   serviceSimpleExportImportMakefileBody,
				},
			},
			{
				"id":         serviceSimpleExportImportDocPath,
				"type":       "DOC",
				"targetPath": serviceSimpleExportImportDocPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   serviceSimpleExportImportDocBody,
				},
			},
			{
				"id":         serviceSimpleExportImportScriptPath,
				"type":       "SCRIPT",
				"targetPath": serviceSimpleExportImportScriptPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   serviceSimpleExportImportScriptBody,
				},
			},
		},
	}

	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(canonical export/import fixture with bundled files): %v", err)
	}
	return updated
}

func buildServiceSimpleExportImportExpectations(
	t *testing.T,
	factory factoryapi.Factory,
) serviceSimpleExportImportExpectations {
	t.Helper()

	workTypes := exportImportValueOrEmpty(factory.WorkTypes)
	workstations := exportImportValueOrEmpty(factory.Workstations)
	if len(workTypes) == 0 {
		t.Fatal("fixture factory must expose at least one work type")
	}
	if len(workstations) == 0 {
		t.Fatal("fixture factory must expose at least one workstation")
	}

	workType := workTypes[0]
	terminalState := ""
	for _, state := range workType.States {
		if state.Type == factoryapi.WorkStateTypeTERMINAL {
			terminalState = state.Name
			break
		}
	}
	if terminalState == "" {
		t.Fatalf("fixture work type %q is missing a terminal state", workType.Name)
	}

	workstationNames := make([]string, 0, len(workstations))
	for _, workstation := range workstations {
		workstationNames = append(workstationNames, workstation.Name)
	}

	return serviceSimpleExportImportExpectations{
		TerminalPlaceID:  workType.Name + ":" + terminalState,
		WorkTypeName:     workType.Name,
		WorkstationNames: workstationNames,
	}
}

func (fixture serviceSimpleExportImportFixture) namedFactory(name string) factoryapi.Factory {
	namedFactory := fixture.GeneratedExportFactor
	namedFactory.Name = factoryapi.FactoryName(name)
	return namedFactory
}

func mustFlattenFactoryConfigWithEdges(
	t *testing.T,
	edges serviceedges.Edges,
	factoryConfigPath string,
) []byte {
	t.Helper()

	payload, err := flattenFactoryConfigWithEdges(t, edges, factoryConfigPath)
	if err != nil {
		t.Fatalf("flattenFactoryConfigWithEdges(%s): %v", factoryConfigPath, err)
	}
	return payload
}

func comparableServiceSimpleExportImportFactory(factory factoryapi.Factory) factoryapi.Factory {
	comparable := factory
	comparable.Name = ""
	comparable.FactoryDirectory = nil
	comparable.SourceDirectory = nil
	comparable.Metadata = nil
	comparable.Version = nil
	return comparable
}

func comparableServiceSimpleExportImportFactoryJSON(factory factoryapi.Factory) string {
	data, err := json.MarshalIndent(comparableServiceSimpleExportImportFactory(factory), "", "  ")
	if err != nil {
		return "<marshal error: " + err.Error() + ">"
	}
	return string(data)
}

func exportImportValueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return append([]T(nil), (*value)...)
}

func assertServiceSimpleExportImportCurrentFactorySignals(
	t *testing.T,
	fixture serviceSimpleExportImportFixture,
	env []string,
	workingDirectory string,
	wantName, wantDir string,
) {
	t.Helper()

	runner := support.NewRecordingCommandRunner("current factory readback must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	current, err := decodeFactoryDefinitionForTest(
		mustFlattenFactoryConfigWithEdges(
			t,
			edges,
			filepath.Join(wantDir, "factory.json"),
		),
	)
	if err != nil {
		t.Fatalf("decode current factory %s: %v", wantName, err)
	}

	if comparableServiceSimpleExportImportFactoryJSON(current) !=
		comparableServiceSimpleExportImportFactoryJSON(fixture.GeneratedExportFactor) {
		t.Fatalf(
			"current factory readback diverged from fixture export contract\ngot JSON:\n%s\nwant JSON:\n%s",
			comparableServiceSimpleExportImportFactoryJSON(current),
			comparableServiceSimpleExportImportFactoryJSON(fixture.GeneratedExportFactor),
		)
	}

	workstations := exportImportValueOrEmpty(current.Workstations)
	gotWorkstationNames := make([]string, 0, len(workstations))
	for _, workstation := range workstations {
		gotWorkstationNames = append(gotWorkstationNames, workstation.Name)
	}
	if !reflect.DeepEqual(gotWorkstationNames, fixture.Expected.WorkstationNames) {
		t.Fatalf("current workstation names = %#v, want %#v", gotWorkstationNames, fixture.Expected.WorkstationNames)
	}

	listInputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	listInputs.Input.Env = env
	listInputs.Input.WorkingDirectory = workingDirectory
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
		if entry.Name != wantName {
			continue
		}
		foundCurrent = true
		if entry.FactoryDirectory != wantDir {
			t.Fatalf(
				"current factory directory = %q, want %q",
				entry.FactoryDirectory,
				wantDir,
			)
		}
		if !entry.Current {
			t.Fatalf("factory list current flag = false, want true for %q", wantName)
		}
	}
	if !foundCurrent {
		t.Fatalf("factory list missing current factory %q; entries=%#v", wantName, listEntries)
	}

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during current factory readback", runner.CallCount())
	}
}

// TestExportImportFixtureBuildsCanonicalExportAndImportContractsFromAuthoredFixture proves
// the service_simple authored fixture flattens into canonical export/import contracts
// with stable route arrays and import naming without external provider execution.
func TestExportImportFixtureBuildsCanonicalExportAndImportContractsFromAuthoredFixture(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)

	if len(fixture.CanonicalFactoryJSON) == 0 {
		t.Fatal("fixture canonical factory json should not be empty")
	}
	if !json.Valid(fixture.CanonicalFactoryJSON) {
		t.Fatalf("fixture canonical factory json is invalid: %s", fixture.CanonicalFactoryJSON)
	}
	assertServiceSimpleExportImportCanonicalRouteArraysJSON(t, fixture.CanonicalFactoryJSON, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})
	if fixture.Expected.WorkTypeName != "task" {
		t.Fatalf("fixture work type = %q, want task", fixture.Expected.WorkTypeName)
	}
	if fixture.Expected.TerminalPlaceID != "task:complete" {
		t.Fatalf("fixture terminal place = %q, want task:complete", fixture.Expected.TerminalPlaceID)
	}
	if !reflect.DeepEqual(fixture.Expected.WorkstationNames, []string{"step-one", "step-two"}) {
		t.Fatalf("fixture workstation names = %#v, want [step-one step-two]", fixture.Expected.WorkstationNames)
	}

	if !reflect.DeepEqual(
		comparableServiceSimpleExportImportFactory(fixture.GeneratedExportFactor),
		comparableServiceSimpleExportImportFactory(fixture.FlattenedFactory),
	) {
		t.Fatalf(
			"generated export factory diverged from flattened canonical boundary\ngenerated: %#v\nflattened: %#v",
			comparableServiceSimpleExportImportFactory(fixture.GeneratedExportFactor),
			comparableServiceSimpleExportImportFactory(fixture.FlattenedFactory),
		)
	}
	assertServiceSimpleExportImportGeneratedRouteArrays(t, fixture.FlattenedFactory, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})
	assertServiceSimpleExportImportGeneratedRouteArrays(t, fixture.GeneratedExportFactor, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})

	importContract := fixture.namedFactory("imported-service-simple")
	if importContract.Name != factoryapi.FactoryName("imported-service-simple") {
		t.Fatalf("import contract name = %q, want imported-service-simple", importContract.Name)
	}
	if !reflect.DeepEqual(
		comparableServiceSimpleExportImportFactory(importContract),
		comparableServiceSimpleExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"import contract factory diverged from generated export factory\ngot:  %#v\nwant: %#v",
			comparableServiceSimpleExportImportFactory(importContract),
			comparableServiceSimpleExportImportFactory(fixture.GeneratedExportFactor),
		)
	}
	assertServiceSimpleExportImportGeneratedRouteArrays(t, importContract, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})
}

// TestExportImportFixturePersistedFactoryExposesReusableCurrentFactorySignals proves
// a persisted and activated named Factory exposes reusable Current Factory signals
// through public CLI flatten readback and factory list without external provider execution.
func TestExportImportFixturePersistedFactoryExposesReusableCurrentFactorySignals(t *testing.T) {
	fixture := newServiceSimpleExportImportFixture(t)

	homeDir := t.TempDir()
	workingDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	namedFactoriesRoot := initializeImportExportCustomerHome(t, env, workingDir)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "factory.json")
	if err := os.WriteFile(sourcePath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write customer Factory source beta: %v", err)
	}
	selectedDir := createImportExportActivatedNamedFactory(
		t,
		env,
		workingDir,
		namedFactoriesRoot,
		"beta",
		sourcePath,
	)

	assertServiceSimpleExportImportCurrentFactorySignals(
		t,
		fixture,
		env,
		workingDir,
		"beta",
		selectedDir,
	)
}

func assertServiceSimpleExportImportCanonicalRouteArraysJSON(
	t *testing.T,
	data []byte,
	want map[string]map[string]int,
) {
	t.Helper()

	var payload struct {
		Workstations []map[string]any `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal canonical export/import fixture json: %v", err)
	}
	if len(payload.Workstations) == 0 {
		t.Fatal("expected canonical export/import fixture to include workstations")
	}

	found := map[string]bool{}
	for _, workstation := range payload.Workstations {
		name, _ := workstation["name"].(string)
		expectedRoutes, ok := want[name]
		if !ok {
			continue
		}
		found[name] = true
		for field, expectedCount := range expectedRoutes {
			routes, ok := workstation[field].([]any)
			if !ok {
				t.Fatalf("workstation %q field %q = %#v, want JSON array", name, field, workstation[field])
			}
			if len(routes) != expectedCount {
				t.Fatalf("workstation %q field %q len = %d, want %d", name, field, len(routes), expectedCount)
			}
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("expected workstation %q in canonical export/import json", name)
		}
	}
}

func assertServiceSimpleExportImportGeneratedRouteArrays(
	t *testing.T,
	factory factoryapi.Factory,
	want map[string]map[string]int,
) {
	t.Helper()

	workstations := exportImportValueOrEmpty(factory.Workstations)
	if len(workstations) == 0 {
		t.Fatal("expected generated export/import fixture to include workstations")
	}

	found := map[string]bool{}
	for _, workstation := range workstations {
		expectedRoutes, ok := want[workstation.Name]
		if !ok {
			continue
		}
		found[workstation.Name] = true
		for field, expectedCount := range expectedRoutes {
			switch field {
			case "onContinue":
				if workstation.OnContinue == nil || len(*workstation.OnContinue) != expectedCount {
					t.Fatalf("workstation %q onContinue = %#v, want %d route(s)", workstation.Name, workstation.OnContinue, expectedCount)
				}
			case "onRejection":
				if workstation.OnRejection == nil || len(*workstation.OnRejection) != expectedCount {
					t.Fatalf("workstation %q onRejection = %#v, want %d route(s)", workstation.Name, workstation.OnRejection, expectedCount)
				}
			case "onFailure":
				if workstation.OnFailure == nil || len(*workstation.OnFailure) != expectedCount {
					t.Fatalf("workstation %q onFailure = %#v, want %d route(s)", workstation.Name, workstation.OnFailure, expectedCount)
				}
			default:
				t.Fatalf("unsupported route field assertion %q", field)
			}
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("expected workstation %q in generated export/import factory", name)
		}
	}
}
