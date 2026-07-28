package wire_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func completeDependencies() authoringlayout.Dependencies {
	return authoringlayout.Dependencies{
		Validator:            factoryvalidation.New(nil),
		MapInput:             func([]byte) (factorydefinitions.DefinitionValidationRequest, error) { return factorydefinitions.DefinitionValidationRequest{}, nil },
		Prepare:              func(context.Context, string, []byte, factorydefinitions.Validator) (*factorydefinitions.PreparedFactoryLayoutPayload, error) { return nil, nil },
		Write:                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error { return nil },
		Validate:             func(string) error { return nil },
		Flatten:              func(string) ([]byte, error) { return nil, nil },
		Expand:               func(string) (string, factorydefinitions.LayoutExpansionReport, error) { return "", factorydefinitions.LayoutExpansionReport{}, nil },
		FileSystem:           stubPersistenceFileSystem{},
		RequireDefinitionDir: func(string) error { return nil },
		Directories:          stubDirectoryReplacementStore{},
	}
}

type stubPersistenceFileSystem struct{}

func (stubPersistenceFileSystem) MkdirTemp(string, string) (string, error) { return "", nil }
func (stubPersistenceFileSystem) RemoveAll(string) error                   { return nil }
func (stubPersistenceFileSystem) Rename(string, string) error              { return nil }
func (stubPersistenceFileSystem) Stat(string) (fs.FileInfo, error) { return nil, nil }
func (stubPersistenceFileSystem) MkdirAll(string, fs.FileMode) error { return nil }

type stubDirectoryReplacementStore struct{}

func (stubDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}
func (stubDirectoryReplacementStore) Restore(string, string) {}

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	complete := completeDependencies()
	requiredFields := []struct {
		name string
		mutate func(*authoringlayout.Dependencies)
		want string
	}{
		{
			name:   "validator",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Validator = nil },
			want:   "validator is required",
		},
		{
			name:   "payload mapper",
			mutate: func(deps *authoringlayout.Dependencies) { deps.MapInput = nil },
			want:   "payload mapper is required",
		},
		{
			name:   "layout preparer",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Prepare = nil },
			want:   "layout preparer is required",
		},
		{
			name:   "layout writer",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Write = nil },
			want:   "layout writer is required",
		},
		{
			name:   "layout validator",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Validate = nil },
			want:   "layout validator is required",
		},
		{
			name:   "layout flattener",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Flatten = nil },
			want:   "layout flattener is required",
		},
		{
			name:   "layout expander",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Expand = nil },
			want:   "layout expander is required",
		},
		{
			name:   "persistence filesystem",
			mutate: func(deps *authoringlayout.Dependencies) { deps.FileSystem = nil },
			want:   "persistence filesystem is required",
		},
		{
			name:   "definition directory validator",
			mutate: func(deps *authoringlayout.Dependencies) { deps.RequireDefinitionDir = nil },
			want:   "definition directory validator is required",
		},
		{
			name:   "directory replacement store",
			mutate: func(deps *authoringlayout.Dependencies) { deps.Directories = nil },
			want:   "directory replacement store is required",
		},
	}

	for _, tc := range requiredFields {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deps := complete
			tc.mutate(&deps)
			svc, err := authoringlayoutwire.NewService(deps)
			if err == nil || svc != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewService(%s) = %#v, %v; want error containing %q", tc.name, svc, err, tc.want)
			}
		})
	}

	svc, err := authoringlayoutwire.NewService(complete)
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ authoringlayout.Service = svc
}
