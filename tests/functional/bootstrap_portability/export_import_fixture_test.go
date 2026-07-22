package bootstrap_portability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	exportImportPortableMakefilePath = "Makefile"
	exportImportPortableMakefileBody = "test:\n\tgo test ./...\n"
	exportImportPortableDocPath      = "factory/docs/README.md"
	exportImportPortableDocBody      = "# Portable factory\n"
	exportImportPortableScriptPath   = "factory/scripts/execute-story.ps1"
	exportImportPortableScriptBody   = "Write-Output 'portable script'\n"
)

type exportImportFixtureExpectations struct {
	TerminalPlaceID  string
	WorkTypeName     string
	WorkstationNames []string
}

type exportImportFixture struct {
	AuthoredFactoryDir    string
	CanonicalFactoryJSON  []byte
	Expected              exportImportFixtureExpectations
	FlattenedFactory      factoryapi.Factory
	GeneratedExportFactor factoryapi.Factory
}

func newExportImportFixture(t *testing.T) exportImportFixture {
	t.Helper()

	authoredFactoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))

	canonicalFactoryJSON, err := support.FlattenFactoryConfig(t, authoredFactoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(%s): %v", authoredFactoryDir, err)
	}
	canonicalFactoryJSON = withExportImportPortableBundledFiles(t, canonicalFactoryJSON)
	canonicalFactoryJSON = withExportImportPortableLayout(t, canonicalFactoryJSON)

	flattenedFactory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(canonicalFactoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}

	return exportImportFixture{
		AuthoredFactoryDir:    authoredFactoryDir,
		CanonicalFactoryJSON:  canonicalFactoryJSON,
		Expected:              buildExportImportFixtureExpectations(t, flattenedFactory),
		FlattenedFactory:      flattenedFactory,
		GeneratedExportFactor: flattenedFactory,
	}
}

func withExportImportPortableLayout(t *testing.T, canonicalFactoryJSON []byte) []byte {
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

func withExportImportPortableBundledFiles(t *testing.T, canonicalFactoryJSON []byte) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(canonicalFactoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(canonical export/import fixture): %v", err)
	}

	payload["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"id":         exportImportPortableMakefilePath,
				"type":       "ROOT_HELPER",
				"targetPath": exportImportPortableMakefilePath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   exportImportPortableMakefileBody,
				},
			},
			{
				"id":         exportImportPortableDocPath,
				"type":       "DOC",
				"targetPath": exportImportPortableDocPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   exportImportPortableDocBody,
				},
			},
			{
				"id":         exportImportPortableScriptPath,
				"type":       "SCRIPT",
				"targetPath": exportImportPortableScriptPath,
				"content": map[string]any{
					"encoding": "utf-8",
					"inline":   exportImportPortableScriptBody,
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

func buildExportImportFixtureExpectations(
	t *testing.T,
	factory factoryapi.Factory,
) exportImportFixtureExpectations {
	t.Helper()

	workTypes := valueOrEmpty(factory.WorkTypes)
	workstations := valueOrEmpty(factory.Workstations)
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

	return exportImportFixtureExpectations{
		TerminalPlaceID:  workType.Name + ":" + terminalState,
		WorkTypeName:     workType.Name,
		WorkstationNames: workstationNames,
	}
}

func (fixture exportImportFixture) namedFactory(name string) factoryapi.Factory {
	namedFactory := fixture.GeneratedExportFactor
	namedFactory.Name = factoryapi.FactoryName(name)
	return namedFactory
}

func (fixture exportImportFixture) persistAs(t *testing.T, rootDir, name string) string {
	return fixture.persistAtCustomerBoundary(t, rootDir, name, false)
}

func (fixture exportImportFixture) persistAndActivateAs(t *testing.T, rootDir, name string) string {
	return fixture.persistAtCustomerBoundary(t, rootDir, name, true)
}

func (fixture exportImportFixture) persistAtCustomerBoundary(t *testing.T, rootDir, name string, activate bool) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "factory.json")
	if err := os.WriteFile(sourcePath, fixture.CanonicalFactoryJSON, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	if activate {
		return support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
	}
	return support.CreateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func (fixture exportImportFixture) assertCurrentFactorySignals(
	t *testing.T,
	svc namedFactoryReadback,
	wantName, wantDir string,
) {
	t.Helper()

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory(%s): %v", wantName, err)
	}
	if current.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("current factory name = %q, want %q", current.Name, wantName)
	}
	if current.FactoryDirectory == nil || *current.FactoryDirectory != wantDir {
		t.Fatalf("current factory directory = %#v, want %q", current.FactoryDirectory, wantDir)
	}

	if !reflect.DeepEqual(
		comparableExportImportFactory(current),
		comparableExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"current factory readback diverged from fixture export contract\ngot:  %#v\nwant: %#v\ngot JSON:\n%s\nwant JSON:\n%s",
			comparableExportImportFactory(current),
			comparableExportImportFactory(fixture.GeneratedExportFactor),
			comparableExportImportFactoryJSON(current),
			comparableExportImportFactoryJSON(fixture.GeneratedExportFactor),
		)
	}

	workstations := valueOrEmpty(current.Workstations)
	gotWorkstationNames := make([]string, 0, len(workstations))
	for _, workstation := range workstations {
		gotWorkstationNames = append(gotWorkstationNames, workstation.Name)
	}
	if !reflect.DeepEqual(gotWorkstationNames, fixture.Expected.WorkstationNames) {
		t.Fatalf("current workstation names = %#v, want %#v", gotWorkstationNames, fixture.Expected.WorkstationNames)
	}
}

func comparableExportImportFactory(factory factoryapi.Factory) factoryapi.Factory {
	comparable := factory
	comparable.Name = ""
	comparable.FactoryDirectory = nil
	comparable.SourceDirectory = nil
	comparable.Metadata = nil
	comparable.Version = nil
	return comparable
}

func comparableExportImportFactoryJSON(factory factoryapi.Factory) string {
	data, err := json.MarshalIndent(comparableExportImportFactory(factory), "", "  ")
	if err != nil {
		return "<marshal error: " + err.Error() + ">"
	}
	return string(data)
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return append([]T(nil), (*value)...)
}

type namedFactoryReadback interface {
	GetCurrentFactory(context.Context) (factoryapi.Factory, error)
}

func buildExportImportFixtureService(t *testing.T, rootDir string) namedFactoryReadback {
	t.Helper()
	server := startFunctionalServer(t, rootDir, true)
	return HTTPNamedFactoryReadback{t: t, serverURL: server.URL()}
}

func TestExportImportFixture_BuildsCanonicalExportAndImportContractsFromAuthoredFixture(t *testing.T) {
	fixture := newExportImportFixture(t)

	if len(fixture.CanonicalFactoryJSON) == 0 {
		t.Fatal("fixture canonical factory json should not be empty")
	}
	if !json.Valid(fixture.CanonicalFactoryJSON) {
		t.Fatalf("fixture canonical factory json is invalid: %s", fixture.CanonicalFactoryJSON)
	}
	assertExportImportFixtureCanonicalRouteArraysJSON(t, fixture.CanonicalFactoryJSON, map[string]map[string]int{
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
		comparableExportImportFactory(fixture.GeneratedExportFactor),
		comparableExportImportFactory(fixture.FlattenedFactory),
	) {
		t.Fatalf(
			"generated export factory diverged from flattened canonical boundary\ngenerated: %#v\nflattened: %#v",
			comparableExportImportFactory(fixture.GeneratedExportFactor),
			comparableExportImportFactory(fixture.FlattenedFactory),
		)
	}
	assertExportImportFixtureGeneratedRouteArrays(t, fixture.FlattenedFactory, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})
	assertExportImportFixtureGeneratedRouteArrays(t, fixture.GeneratedExportFactor, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})

	importContract := fixture.namedFactory("imported-service-simple")
	if importContract.Name != factoryapi.FactoryName("imported-service-simple") {
		t.Fatalf("import contract name = %q, want imported-service-simple", importContract.Name)
	}
	if !reflect.DeepEqual(
		comparableExportImportFactory(importContract),
		comparableExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"import contract factory diverged from generated export factory\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(importContract),
			comparableExportImportFactory(fixture.GeneratedExportFactor),
		)
	}
	assertExportImportFixtureGeneratedRouteArrays(t, importContract, map[string]map[string]int{
		"step-one": {"onFailure": 1},
		"step-two": {"onFailure": 1},
	})
}

func TestExportImportFixture_PersistedFactoryExposesReusableCurrentFactorySignals(t *testing.T) {
	fixture := newExportImportFixture(t)
	rootDir := t.TempDir()

	selectedDir := fixture.persistAndActivateAs(t, rootDir, "beta")

	svc := buildExportImportFixtureService(t, rootDir)
	fixture.assertCurrentFactorySignals(t, svc, "beta", selectedDir)
}

func assertExportImportFixtureCanonicalRouteArraysJSON(
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

func assertExportImportFixtureGeneratedRouteArrays(
	t *testing.T,
	factory factoryapi.Factory,
	want map[string]map[string]int,
) {
	t.Helper()

	workstations := valueOrEmpty(factory.Workstations)
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
