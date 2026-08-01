package persistence_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalogpersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/persistence"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

func validPersistenceValidationRequest() factorydefinitions.DefinitionValidationRequest {
	return factorydefinitions.DefinitionValidationRequest{
		Config:           &factorydefinitions.FactoryConfig{},
		CanonicalPayload: []byte(`{}`),
		CanonicalFactoryLoader: func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, nil
		},
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestServiceRoutesPersistenceThroughFlatCapabilities(t *testing.T) {
	t.Parallel()

	validator := factoryvalidation.New(nil)
	prepared := &factorydefinitions.PreparedFactoryLayoutPayload{}
	var preparedWith factorydefinitions.Validator
	mapCalls := 0
	canonicalLoads := 0
	writeCalls := 0

	service, err := catalogpersistence.New(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			if string(payload) != "payload" {
				t.Fatalf("map payload = %q", payload)
			}
			mapCalls++
			request := validPersistenceValidationRequest()
			request.Profile = factorydefinitions.ValidationProfileTopology
			request.CanonicalFactoryLoader = func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				canonicalLoads++
				return nil, nil
			}
			return request, nil
		},
		func(
			_ context.Context,
			segment string,
			payload []byte,
			gotValidator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			if segment != "alpha" || string(payload) != "payload" {
				t.Fatalf("prepare values = %q, %q", segment, payload)
			}
			preparedWith = gotValidator
			return prepared, nil
		},
		func(
			stagingDir string,
			got *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			if got != prepared {
				t.Fatalf("write prepared = %p", got)
			}
			if filepath.Base(sourcePath) != factorydefinitions.FactoryConfigFile {
				t.Fatalf("write source path = %q", sourcePath)
			}
			if info, err := os.Stat(stagingDir); err != nil || !info.IsDir() {
				t.Fatalf("write staging directory = %q: %v", stagingDir, err)
			}
			writeCalls++
			return os.WriteFile(
				filepath.Join(stagingDir, factorydefinitions.FactoryConfigFile),
				[]byte("{}"),
				0o644,
			)
		},
		func(stagingDir string) error {
			if info, err := os.Stat(stagingDir); err != nil || !info.IsDir() {
				t.Fatalf("validate staging directory = %q: %v", stagingDir, err)
			}
			return nil
		},
		func(path string) ([]byte, error) {
			return []byte("flattened:" + path), nil
		},
		func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return path + "-expanded", factorydefinitions.LayoutExpansionReport{
				FactoryConfigPaths: 1,
			}, nil
		},
		nil,
		platformfilesystem.Local{},
		persistenceTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	gotPrepared, err := service.PrepareFactoryLayout(context.Background(), "alpha", []byte("payload"))
	if err != nil || gotPrepared != prepared || preparedWith != validator || mapCalls != 1 || canonicalLoads != 1 {
		t.Fatalf(
			"PrepareFactoryLayout() = %p, %v; prepare validator = %#v; map calls = %d; canonical loads = %d",
			gotPrepared,
			err,
			preparedWith,
			mapCalls,
			canonicalLoads,
		)
	}
	rootDir := t.TempDir()
	targetDir := filepath.Join(rootDir, "alpha")
	if got, err := service.CreateNamedFactory(rootDir, "alpha", prepared); err != nil || got != targetDir {
		t.Fatalf("CreateNamedFactory() = %q, %v", got, err)
	}
	if got, err := service.ReplaceNamedFactory(rootDir, "alpha", prepared); err != nil || got != targetDir {
		t.Fatalf("ReplaceNamedFactory() = %q, %v", got, err)
	}
	if writeCalls != 2 {
		t.Fatalf("write calls = %d, want 2", writeCalls)
	}
	if flattened, err := service.FlattenFactoryLayout("alpha"); err != nil ||
		string(flattened) != "flattened:alpha" {
		t.Fatalf("FlattenFactoryLayout() = %q, %v", flattened, err)
	}
	expandedDir, report, err := service.ExpandFactoryLayout("alpha")
	if err != nil || expandedDir != "alpha-expanded" || report.FactoryConfigPaths != 1 {
		t.Fatalf("ExpandFactoryLayout() = %q, %#v, %v", expandedDir, report, err)
	}
	replacement, err := service.ReplaceFactoryLayout(targetDir, prepared)
	if err != nil || replacement == nil {
		t.Fatalf("ReplaceFactoryLayout() = %p, %v", replacement, err)
	}
	if replacement.DiscardBackup != nil {
		replacement.DiscardBackup()
	}
}

func TestCreateNamedFactory_DiscardsStagingWhenLayoutValidationFails(t *testing.T) {
	t.Parallel()

	validator := factoryvalidation.New(nil)
	service, err := catalogpersistence.New(
		validator,
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validPersistenceValidationRequest(), nil
		},
		prepareLayoutForPersistenceTest,
		func(
			stagingDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			if err := writePreparedLayoutForPersistenceTest(
				stagingDir,
				prepared,
				sourcePath,
			); err != nil {
				return err
			}
			return os.WriteFile(
				filepath.Join(
					stagingDir,
					factorydefinitions.WorkstationsDir,
					"execute-broken",
					factorydefinitions.FactoryAgentsFileName,
				),
				[]byte("---\ntype: [\n"),
				0o644,
			)
		},
		validateLayoutForPersistenceTest,
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		persistenceTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	payload := []byte(`{
		"name":"broken",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","body":"Execute."}],
		"workstations":[{"name":"execute-broken","worker":"executor","type":"MODEL_WORKSTATION","body":"Execute.","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]
	}`)
	prepared, err := service.PrepareFactoryLayout(
		context.Background(),
		"broken",
		payload,
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}

	rootDir := t.TempDir()
	_, err = service.CreateNamedFactory(rootDir, "broken", prepared)
	if err == nil {
		t.Fatal("CreateNamedFactory() error = nil, want staged validation failure")
	}
	for _, want := range []string{
		`validate factory "broken" config`,
		"load workstation",
		"AGENTS.md missing closing frontmatter delimiter",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CreateNamedFactory() error = %v, want substring %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(statErr) {
		t.Fatalf("failed target stat error = %v, want not-exist", statErr)
	}
}

func TestReplaceFactoryLayout_ValidationFailureLeavesCommittedFactoryUnchanged(t *testing.T) {
	t.Parallel()

	validator := factoryvalidation.New(nil)
	service, err := catalogpersistence.New(
		validator,
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validPersistenceValidationRequest(), nil
		},
		prepareLayoutForPersistenceTest,
		func(
			stagingDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			if err := writePreparedLayoutForPersistenceTest(
				stagingDir,
				prepared,
				sourcePath,
			); err != nil {
				return err
			}
			return os.WriteFile(
				filepath.Join(
					stagingDir,
					factorydefinitions.WorkstationsDir,
					"execute-broken",
					factorydefinitions.FactoryAgentsFileName,
				),
				[]byte("---\ntype: [\n"),
				0o644,
			)
		},
		validateLayoutForPersistenceTest,
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		persistenceTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	targetDir := t.TempDir()
	initial := []byte(`{"name":"alpha","id":"alpha"}`)
	if err := os.WriteFile(
		filepath.Join(targetDir, factorydefinitions.FactoryConfigFile),
		initial,
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(initial factory.json): %v", err)
	}
	payload := []byte(`{
		"name":"broken",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","body":"Execute."}],
		"workstations":[{"name":"execute-broken","worker":"executor","type":"MODEL_WORKSTATION","body":"Execute.","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]
	}`)
	prepared, err := service.PrepareFactoryLayout(
		context.Background(),
		"broken",
		payload,
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}

	_, err = service.ReplaceFactoryLayout(targetDir, prepared)
	if err == nil {
		t.Fatal("ReplaceFactoryLayout() error = nil, want staged validation failure")
	}
	for _, want := range []string{
		`validate factory`,
		"AGENTS.md missing closing frontmatter delimiter",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ReplaceFactoryLayout() error = %v, want substring %q", err, want)
		}
	}
	got, readErr := os.ReadFile(
		filepath.Join(targetDir, factorydefinitions.FactoryConfigFile),
	)
	if readErr != nil {
		t.Fatalf("ReadFile(factory.json): %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after failed replace = %q, want %q", got, initial)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, factorydefinitions.WorkersDir)); !os.IsNotExist(statErr) {
		t.Fatalf("workers directory after failed replace stat error = %v, want not-exist", statErr)
	}
}
