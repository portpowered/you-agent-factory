package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
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
		Flatten:           func(string) ([]byte, error) { return nil, nil },
		Expand: func(string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return "", factorydefinitions.LayoutExpansionReport{}, nil
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
