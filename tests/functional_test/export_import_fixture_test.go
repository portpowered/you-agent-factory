package functional_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
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

	authoredFactoryDir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))

	loaded, err := config.LoadRuntimeConfig(authoredFactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", authoredFactoryDir, err)
	}

	canonicalFactoryJSON, err := config.FlattenFactoryConfig(authoredFactoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(%s): %v", authoredFactoryDir, err)
	}

	flattenedFactory, err := config.GeneratedFactoryFromOpenAPIJSON(canonicalFactoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}

	generatedExportFactory, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}

	return exportImportFixture{
		AuthoredFactoryDir:    authoredFactoryDir,
		CanonicalFactoryJSON:  canonicalFactoryJSON,
		Expected:              buildExportImportFixtureExpectations(t, flattenedFactory),
		FlattenedFactory:      flattenedFactory,
		GeneratedExportFactor: generatedExportFactory,
	}
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

func (fixture exportImportFixture) namedFactory(name string) factoryapi.NamedFactory {
	return factoryapi.NamedFactory{
		Name:    factoryapi.FactoryName(name),
		Factory: fixture.GeneratedExportFactor,
	}
}

func (fixture exportImportFixture) persistAs(t *testing.T, rootDir, name string) string {
	t.Helper()

	factoryDir, err := config.PersistNamedFactory(rootDir, name, fixture.CanonicalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}

func (fixture exportImportFixture) assertCurrentFactorySignals(
	t *testing.T,
	rootDir string,
	svc namedFactoryReadback,
	wantName string,
) {
	t.Helper()

	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer(%s): %v", wantName, err)
	} else if got != wantName {
		t.Fatalf("current factory pointer = %q, want %q", got, wantName)
	}

	wantDir := filepath.Join(rootDir, wantName)
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir(%s): %v", wantName, err)
	} else if got != wantDir {
		t.Fatalf("resolved current factory dir = %q, want %q", got, wantDir)
	}

	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory(%s): %v", wantName, err)
	}
	if current.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("current factory name = %q, want %q", current.Name, wantName)
	}

	if !reflect.DeepEqual(
		comparableExportImportFactory(current.Factory),
		comparableExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"current named factory readback diverged from fixture export contract\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(current.Factory),
			comparableExportImportFactory(fixture.GeneratedExportFactor),
		)
	}

	workstations := valueOrEmpty(current.Factory.Workstations)
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
	comparable.FactoryDir = nil
	comparable.SourceDirectory = nil
	comparable.WorkflowId = nil
	comparable.Metadata = nil
	return comparable
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return append([]T(nil), (*value)...)
}

type namedFactoryReadback interface {
	GetCurrentNamedFactory(context.Context) (factoryapi.NamedFactory, error)
}

func buildExportImportFixtureService(t *testing.T, rootDir string) namedFactoryReadback {
	t.Helper()

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(%s): %v", rootDir, err)
	}
	return svc
}

func TestExportImportFixture_BuildsCanonicalExportAndImportContractsFromAuthoredFixture(t *testing.T) {
	fixture := newExportImportFixture(t)

	if len(fixture.CanonicalFactoryJSON) == 0 {
		t.Fatal("fixture canonical factory json should not be empty")
	}
	if !json.Valid(fixture.CanonicalFactoryJSON) {
		t.Fatalf("fixture canonical factory json is invalid: %s", fixture.CanonicalFactoryJSON)
	}
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

	importContract := fixture.namedFactory("imported-service-simple")
	if importContract.Name != factoryapi.FactoryName("imported-service-simple") {
		t.Fatalf("import contract name = %q, want imported-service-simple", importContract.Name)
	}
	if !reflect.DeepEqual(
		comparableExportImportFactory(importContract.Factory),
		comparableExportImportFactory(fixture.GeneratedExportFactor),
	) {
		t.Fatalf(
			"import contract factory diverged from generated export factory\ngot:  %#v\nwant: %#v",
			comparableExportImportFactory(importContract.Factory),
			comparableExportImportFactory(fixture.GeneratedExportFactor),
		)
	}
}

func TestExportImportFixture_PersistedFactoryExposesReusableCurrentFactorySignals(t *testing.T) {
	fixture := newExportImportFixture(t)
	rootDir := t.TempDir()

	fixture.persistAs(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(beta): %v", err)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta")

	svc := buildExportImportFixtureService(t, rootDir)
	fixture.assertCurrentFactorySignals(t, rootDir, svc, "beta")
}
