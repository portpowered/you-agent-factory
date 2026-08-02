package wire_test

import (
	"io/fs"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/contracts"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

type serviceArgs struct {
	validator            authoringlayoutcontracts.LayoutValidator
	validateDefinition   authoringlayoutcontracts.DefinitionValidationOperation
	mapInput             authoringlayoutcontracts.LayoutPayloadMapper
	decodeFactory        authoringlayoutcontracts.FactoryConfigJSONDecoder
	normalizeAuthored    authoringlayoutcontracts.AuthoredFactoryNormalizer
	encodeFactory        authoringlayoutcontracts.FactoryConfigJSONEncoder
	write                authoringlayoutcontracts.LayoutWriter
	validate             authoringlayoutcontracts.LayoutValidatorFunc
	flatten              authoringlayoutcontracts.LayoutFlattener
	expand               authoringlayoutcontracts.LayoutExpander
	fileSystem           authoringlayoutcontracts.PersistenceFileSystem
	requireDefinitionDir authoringlayoutcontracts.DefinitionDirectoryRequirer
	directories          authoringlayoutcontracts.DirectoryReplacementStore
}

func completeServiceArgs() serviceArgs {
	mapper := factorymapping.NewFactoryConfigMapper()
	validator := factoryvalidation.New(nil)
	return serviceArgs{
		validator:          validator,
		validateDefinition: validator,
		mapInput: func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{}, nil
		},
		decodeFactory:     mapper.Expand,
		normalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		encodeFactory:     mapper.Flatten,
		write:             func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error { return nil },
		validate:          func(string) error { return nil },
		flatten:           func(string) ([]byte, error) { return nil, nil },
		expand: func(string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return "", factorydefinitions.LayoutExpansionReport{}, nil
		},
		fileSystem:           stubPersistenceFileSystem{},
		requireDefinitionDir: func(string) error { return nil },
		directories:          stubDirectoryReplacementStore{},
	}
}

func (a serviceArgs) newService() (authoringlayout.Service, error) {
	return authoringlayoutwire.NewService(
		a.validator,
		a.validateDefinition,
		a.mapInput,
		a.decodeFactory,
		a.normalizeAuthored,
		a.encodeFactory,
		a.write,
		a.validate,
		a.flatten,
		a.expand,
		a.fileSystem,
		a.requireDefinitionDir,
		a.directories,
	)
}

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

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	complete := completeServiceArgs()
	requiredFields := []struct {
		name   string
		mutate func(*serviceArgs)
		want   string
	}{
		{
			name:   "validator",
			mutate: func(args *serviceArgs) { args.validator = nil },
			want:   "validator is required",
		},
		{
			name:   "definition validation operation",
			mutate: func(args *serviceArgs) { args.validateDefinition = nil },
			want:   "definition validation operation is required",
		},
		{
			name:   "payload mapper",
			mutate: func(args *serviceArgs) { args.mapInput = nil },
			want:   "payload mapper is required",
		},
		{
			name:   "factory decoder",
			mutate: func(args *serviceArgs) { args.decodeFactory = nil },
			want:   "factory decoder is required",
		},
		{
			name:   "authored normalizer",
			mutate: func(args *serviceArgs) { args.normalizeAuthored = nil },
			want:   "authored normalizer is required",
		},
		{
			name:   "factory encoder",
			mutate: func(args *serviceArgs) { args.encodeFactory = nil },
			want:   "factory encoder is required",
		},
		{
			name:   "layout writer",
			mutate: func(args *serviceArgs) { args.write = nil },
			want:   "layout writer is required",
		},
		{
			name:   "layout validator",
			mutate: func(args *serviceArgs) { args.validate = nil },
			want:   "layout validator is required",
		},
		{
			name:   "layout flattener",
			mutate: func(args *serviceArgs) { args.flatten = nil },
			want:   "layout flattener is required",
		},
		{
			name:   "layout expander",
			mutate: func(args *serviceArgs) { args.expand = nil },
			want:   "layout expander is required",
		},
		{
			name:   "persistence filesystem",
			mutate: func(args *serviceArgs) { args.fileSystem = nil },
			want:   "persistence filesystem is required",
		},
		{
			name:   "definition directory validator",
			mutate: func(args *serviceArgs) { args.requireDefinitionDir = nil },
			want:   "definition directory validator is required",
		},
		{
			name:   "directory replacement store",
			mutate: func(args *serviceArgs) { args.directories = nil },
			want:   "directory replacement store is required",
		},
	}

	for _, tc := range requiredFields {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := complete
			tc.mutate(&args)
			svc, err := args.newService()
			if err == nil || svc != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewService(%s) = %#v, %v; want error containing %q", tc.name, svc, err, tc.want)
			}
		})
	}

	svc, err := complete.newService()
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ authoringlayout.Service = svc
}
