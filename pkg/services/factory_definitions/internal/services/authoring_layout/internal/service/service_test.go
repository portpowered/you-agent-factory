package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

type stubPersistenceFileSystem struct{}

func (stubPersistenceFileSystem) MkdirTemp(string, string) (string, error) { return "", nil }
func (stubPersistenceFileSystem) RemoveAll(string) error                   { return nil }
func (stubPersistenceFileSystem) Rename(string, string) error              { return nil }
func (stubPersistenceFileSystem) Stat(string) (fs.FileInfo, error)         { return nil, nil }
func (stubPersistenceFileSystem) MkdirAll(string, fs.FileMode) error       { return nil }

type stubDirectoryReplacementStore struct{}

func (stubDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}
func (stubDirectoryReplacementStore) Restore(string, string) {}

func newAuthoringLayoutService(t *testing.T) authoringlayout.Service {
	t.Helper()

	mapper := factorymapping.NewFactoryConfigMapper()
	svc, err := authoringlayoutwire.NewService(authoringlayout.Dependencies{
		Validator: factoryvalidation.New(nil),
		MapInput: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, func(
				payload []byte,
				_ factorydefinitions.WorkstationLoader,
			) (factorydefinitions.MutableLoadedFactorySource, error) {
				cfg, decodeErr := mapper.Expand(payload)
				if decodeErr != nil {
					return nil, decodeErr
				}
				return stubLoadedSource{cfg: cfg}, nil
			})
		},
		DecodeFactory:     mapper.Expand,
		NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		EncodeFactory:     mapper.Flatten,
		Write:             func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error { return nil },
		Validate:          func(string) error { return nil },
		Flatten:           func(path string) ([]byte, error) { return []byte("flattened:" + path), nil },
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return path + "-expanded", factorydefinitions.LayoutExpansionReport{FactoryConfigPaths: 1}, nil
		},
		FileSystem:           stubPersistenceFileSystem{},
		RequireDefinitionDir: func(string) error { return nil },
		Directories:          stubDirectoryReplacementStore{},
	})
	if err != nil {
		t.Fatalf("construct authoring_layout: %v", err)
	}
	return svc
}

type stubLoadedSource struct {
	cfg *factorydefinitions.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                                 { return "" }
func (s stubLoadedSource) RuntimeBaseDir() string                             { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                           {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error {
	return nil
}
func (s stubLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func validAlphaPayload(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(factory): %v", err)
	}
	return payload
}

func TestPrepareFactoryLayout_ReturnsPreparedAggregateForValidPayload(t *testing.T) {
	t.Parallel()

	payload := validAlphaPayload(t)
	svc := newAuthoringLayoutService(t)

	result, err := svc.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if result.Prepared.Config == nil {
		t.Fatal("PrepareFactoryLayout prepared config is nil")
	}
	if len(result.Prepared.Canonical) == 0 {
		t.Fatal("PrepareFactoryLayout prepared canonical is empty")
	}
}

func TestPrepareFactoryLayout_RejectsMalformedPayloadWithoutFilesystemEffects(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	_, err := svc.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte("{")},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf("PrepareFactoryLayout malformed error = %v, want %v", err, factorydefinitions.ErrInvalidNamedFactory)
	}
}

func TestFlattenFactoryLayout_ReturnsCanonicalBytes(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	result, err := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if string(result.Canonical) != "flattened:/factories/alpha" {
		t.Fatalf("FlattenFactoryLayout canonical = %q, want flattened path marker", result.Canonical)
	}
}

func TestExpandFactoryLayout_ReturnsFactoryDirAndReport(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	result, err := svc.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if result.FactoryDir != "/factories/alpha-expanded" {
		t.Fatalf("ExpandFactoryLayout factoryDir = %q, want expanded path", result.FactoryDir)
	}
	if result.Report.FactoryConfigPaths != 1 {
		t.Fatalf("ExpandFactoryLayout report = %#v, want one factory config path", result.Report)
	}
}

func TestFlattenExpandFactoryLayout_RejectsEmptyPath(t *testing.T) {
	t.Parallel()

	svc := newAuthoringLayoutService(t)
	_, flattenErr := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{},
	)
	if flattenErr == nil || flattenErr.Error() != "factory path is required" {
		t.Fatalf("FlattenFactoryLayout empty path error = %v, want required path failure", flattenErr)
	}

	_, expandErr := svc.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{},
	)
	if expandErr == nil || expandErr.Error() != "factory path is required" {
		t.Fatalf("ExpandFactoryLayout empty path error = %v, want required path failure", expandErr)
	}
}

func TestFlattenExpandFactoryLayout_PreservesFactoryIdentityAcrossRoundTrip(t *testing.T) {
	t.Parallel()

	composition := newAuthoringLayoutTestComposition(t)
	validator := factoryvalidation.New(nil)
	rootDir := t.TempDir()
	payload := validAlphaPayload(t)

	factoryDir, err := composition.PersistNamedFactory(rootDir, "alpha", payload, validator)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	svc := newAuthoringLayoutServiceFromComposition(t, composition, validator)
	flattened, err := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: factoryDir},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}

	canonicalDir := t.TempDir()
	canonicalPath := filepath.Join(canonicalDir, "factory.json")
	if err := os.WriteFile(canonicalPath, flattened.Canonical, 0o644); err != nil {
		t.Fatalf("WriteFile(flattened canonical): %v", err)
	}

	expanded, err := svc.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: canonicalPath},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir == "" {
		t.Fatal("ExpandFactoryLayout returned empty factoryDir")
	}
	if expanded.Report.FactoryConfigPaths < 1 {
		t.Fatalf("ExpandFactoryLayout report = %#v, want factory config paths", expanded.Report)
	}

	flattenedAgain, err := svc.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: expanded.FactoryDir},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout(expanded layout): %v", err)
	}

	var before map[string]json.RawMessage
	if err := json.Unmarshal(flattened.Canonical, &before); err != nil {
		t.Fatalf("Unmarshal(first flatten): %v", err)
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(flattenedAgain.Canonical, &after); err != nil {
		t.Fatalf("Unmarshal(second flatten): %v", err)
	}
	if string(before["name"]) != string(after["name"]) {
		t.Fatalf("round-trip name = %s, want %s", after["name"], before["name"])
	}
	if string(before["id"]) != string(after["id"]) {
		t.Fatalf("round-trip id = %s, want %s", after["id"], before["id"])
	}
}

func newAuthoringLayoutTestComposition(t *testing.T) factorydefinitiontestcomposition.Composition {
	t.Helper()

	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	var composition factorydefinitiontestcomposition.Composition
	composition = factorydefinitiontestcomposition.New(factorydefinitiontestcomposition.Representation{
		DecodeFactory:     factorymapping.ExpandFactoryConfigForRuntimeLoad,
		DecodeAuthored:    mapper.Expand,
		EncodeFactory:     factorymapping.MarshalCanonicalFactoryConfig,
		NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		ParseWorker:       authoredmapping.ParseWorkerConfig,
		ParseWorkstation:  authoredmapping.ParseWorkstationConfig,
		ParseBody:         authoredmapping.ParseAgentsBody,
		RenderWorker:      authoredmapping.RenderWorkerAgentsMarkdown,
		RenderWorkstation: authoredmapping.RenderWorkstationAgentsMarkdown,
		RenderBody:        authoredmapping.RenderAgentsBody,
		SafeLayoutSegment: authoredmapping.SafeFactoryLayoutSegment,
		SafePromptPath:    authoredmapping.SafePromptFilePath,
		MapPersistence: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, composition.LoadCanonicalJSON)
		},
	}, fileSystem, directoryreplace.Local{}, factorydefinitiontestcomposition.Effects{
		Loading:             fileSystem,
		AuthoredReader:      fileSystem,
		AuthoredWriter:      fileSystem,
		Persistence:         fileSystem,
		NamedPaths:          fileSystem,
		NamedFactoryCatalog: fileSystem,
		InboxSentinels:      inboxgitkeep.NewLocal(fileSystem),
	}, mustAuthoringLayoutRequiredToolChecker())
	return composition
}

func mustAuthoringLayoutRequiredToolChecker() factorydefinitions.RequiredToolChecker {
	checker, err := factoryloading.NewPathRequiredToolChecker(
		exec.LookPath,
		func(path string, args ...string) ([]byte, error) {
			return exec.Command(path, args...).CombinedOutput()
		},
	)
	if err != nil {
		panic(err)
	}
	return checker
}

func newAuthoringLayoutServiceFromComposition(
	t *testing.T,
	composition factorydefinitiontestcomposition.Composition,
	validator factorydefinitions.Validator,
) authoringlayout.Service {
	t.Helper()

	persistence := composition.FactoryDefinitionPersistenceWithValidator(validator)
	svc, err := authoringlayoutwire.NewService(authoringlayout.Dependencies{
		Validator:            validator,
		MapInput:             composition.MapFactoryJSONForPersistence,
		DecodeFactory:        factorymapping.NewFactoryConfigMapper().Expand,
		NormalizeAuthored:    authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		EncodeFactory:        factorymapping.MarshalCanonicalFactoryConfig,
		Write:                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error { return nil },
		Validate:             persistence.ValidateFactoryLayout,
		Flatten:              composition.FactoryLayoutFlattener(),
		Expand:               persistence.ExpandFactoryLayout,
		FileSystem:           platformfilesystem.Local{},
		RequireDefinitionDir: composition.NamedPaths().RequireDefinitionDir,
		Directories:          directoryreplace.Local{},
	})
	if err != nil {
		t.Fatalf("construct authoring_layout from composition: %v", err)
	}
	return svc
}
