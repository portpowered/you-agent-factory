package persistence_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalogpersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestPersistNamedFactoryOwnsCreateReplaceAndCurrentPointerPolicy(t *testing.T) {
	t.Parallel()

	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "request")
	var preparedContexts []context.Context
	var preparedNames []string
	var preparedPayloads []string
	var currentNames []string
	service, err := catalogpersistence.New(
		factoryvalidation.New(nil),
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validPersistenceValidationRequest(), nil
		},
		func(
			gotContext context.Context,
			name string,
			payload []byte,
			_ factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			preparedContexts = append(preparedContexts, gotContext)
			preparedNames = append(preparedNames, name)
			preparedPayloads = append(preparedPayloads, string(payload))
			return &factorydefinitions.PreparedFactoryLayoutPayload{}, nil
		},
		func(
			stagingDir string,
			_ *factorydefinitions.PreparedFactoryLayoutPayload,
			_ string,
		) error {
			return os.WriteFile(
				filepath.Join(stagingDir, factorydefinitions.FactoryConfigFile),
				[]byte("{}"),
				0o644,
			)
		},
		func(string) error { return nil },
		nil,
		nil,
		func(_ string, name string) error {
			currentNames = append(currentNames, name)
			return nil
		},
		platformfilesystem.Local{},
		persistenceTestNamedPaths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	rootDir := t.TempDir()
	created, err := service.PersistNamedFactory(
		ctx,
		factorydefinitions.NamedFactoryPersistenceRequest{
			Mode:       factorydefinitions.NamedFactoryPersistenceModeCreate,
			RootDir:    rootDir,
			Name:       " alpha ",
			Payload:    []byte("create"),
			SetCurrent: true,
		},
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory(create): %v", err)
	}
	wantDir := filepath.Join(rootDir, "alpha")
	if created.Name != "alpha" || created.FactoryDir != wantDir {
		t.Fatalf("create result = %#v, want alpha at %q", created, wantDir)
	}

	replaced, err := service.PersistNamedFactory(
		ctx,
		factorydefinitions.NamedFactoryPersistenceRequest{
			Mode:       factorydefinitions.NamedFactoryPersistenceModeReplace,
			RootDir:    rootDir,
			Name:       "alpha",
			Payload:    []byte("replace"),
			SetCurrent: true,
		},
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory(replace): %v", err)
	}
	if replaced != created {
		t.Fatalf("replace result = %#v, want %#v", replaced, created)
	}
	if len(preparedContexts) != 2 ||
		preparedContexts[0] != ctx ||
		preparedContexts[1] != ctx {
		t.Fatalf("prepared contexts = %#v, want propagated context twice", preparedContexts)
	}
	if got := preparedNames; len(got) != 2 || got[0] != "alpha" || got[1] != "alpha" {
		t.Fatalf("prepared names = %#v", got)
	}
	if got := preparedPayloads; len(got) != 2 || got[0] != "create" || got[1] != "replace" {
		t.Fatalf("prepared payloads = %#v", got)
	}
	if len(currentNames) != 1 || currentNames[0] != "alpha" {
		t.Fatalf("current-pointer writes = %#v, want create only", currentNames)
	}
}

func TestPersistNamedFactoryReturnsResolvedTargetWithPersistenceFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("prepare failed")
	service, err := catalogpersistence.New(
		factoryvalidation.New(nil),
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{}, wantErr
		},
		func(
			context.Context,
			string,
			[]byte,
			factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			t.Fatal("preparer called after input validation failed")
			return nil, nil
		},
		nil,
		nil,
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
	rootDir := t.TempDir()
	result, err := service.PersistNamedFactory(
		t.Context(),
		factorydefinitions.NamedFactoryPersistenceRequest{
			Mode:    factorydefinitions.NamedFactoryPersistenceModeCreate,
			RootDir: rootDir,
			Name:    "@you/goal",
			Payload: []byte("payload"),
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result.FactoryDir != filepath.Join(rootDir, "@you", "goal") {
		t.Fatalf("result.FactoryDir = %q", result.FactoryDir)
	}
}
