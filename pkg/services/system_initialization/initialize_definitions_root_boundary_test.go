package systeminitialization_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
)

type definitionsCatalogRecorder struct {
	listCalls    int
	resolveCalls int
	definitions  []factorydefinitions.PackagedDefinition
}

func (catalog *definitionsCatalogRecorder) list(
	_ context.Context,
	_ factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	catalog.listCalls++
	entries := make([]factorydefinitions.BuiltInPackagedFactoryEntry, len(catalog.definitions))
	for index, definition := range catalog.definitions {
		entries[index] = factorydefinitions.BuiltInPackagedFactoryEntry{
			Name:    definition.Name,
			Project: definition.Project,
			Formats: append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
		}
	}
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
}

func (catalog *definitionsCatalogRecorder) resolve(
	_ context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	catalog.resolveCalls++
	for _, definition := range catalog.definitions {
		if definition.Name == request.Name {
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
				Definition: definition,
				Formats:    append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
			}, nil
		}
	}
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{},
		factorydefinitions.ErrUnknownPackagedFactoryIdentity
}

type definitionsService struct {
	factorydefinitions.Service
	listFn    func(context.Context, factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error)
	resolveFn func(context.Context, factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error)
	installFn func(context.Context, factorydefinitions.InstallPackagedFactoryRequest) (factorydefinitions.InstallPackagedFactoryResult, error)
}

func (service *definitionsService) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return service.listFn(ctx, request)
}

func (service *definitionsService) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	return service.resolveFn(ctx, request)
}

func (service *definitionsService) InstallPackagedFactory(
	ctx context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	return service.installFn(ctx, request)
}

func (catalog *definitionsCatalogRecorder) service(
	installer func(context.Context, factorydefinitions.InstallPackagedFactoryRequest) (factorydefinitions.InstallPackagedFactoryResult, error),
) factorydefinitions.Service {
	return &definitionsService{
		listFn: catalog.list, resolveFn: catalog.resolve, installFn: installer,
	}
}

// filePreservingPackagedInstaller exercises the Definitions root ensure-installer
// seam: first ensure creates on-disk Factory content; repeat ensures report skipped
// without rewriting customer-owned Factory files.
type filePreservingPackagedInstaller struct {
	calls int
}

func (installer *filePreservingPackagedInstaller) install(
	_ context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	installer.calls++
	outcome := factorydefinitions.PackagedFactoryInstallCreated
	if installer.calls > 1 {
		outcome = factorydefinitions.PackagedFactoryInstallSkipped
	}

	factoryDir := filepath.Join(request.RootDir, strings.TrimPrefix(request.Name, "@you/"))
	if installer.calls == 1 {
		if err := os.MkdirAll(factoryDir, 0o755); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		if err := os.WriteFile(filepath.Join(factoryDir, "customer-owned.txt"), []byte("bootstrap-created\n"), 0o600); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
	}
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name: request.Name, FactoryDir: factoryDir,
		},
		Outcome: outcome,
		Format:  request.Format,
	}, nil
}

type failingDefinitionsInstaller struct {
	err error
}

func (installer *failingDefinitionsInstaller) install(
	context.Context,
	factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	return factorydefinitions.InstallPackagedFactoryResult{}, installer.err
}

func newDefinitionsRootService(
	t *testing.T,
	settings operatorsettings.Service,
	definitions factorydefinitions.Service,
) systeminitialization.Service {
	t.Helper()

	service, err := systeminitializationwire.NewService(
		settings,
		definitions,
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

// TestInitializeDefinitionsRootBoundary_CreatedThenSkippedPreservesCustomerFactoryContent
// proves Initialize reports packaged-Factory created outcomes on first ensure and
// skipped outcomes on repeat through injected Definitions root catalog and
// ensure-installer collaborators without rewriting customer-owned Factory content.
func TestInitializeDefinitionsRootBoundary_CreatedThenSkippedPreservesCustomerFactoryContent(t *testing.T) {
	t.Parallel()

	definitions := []factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		JSON:    []byte(`{}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}}
	catalog := &definitionsCatalogRecorder{definitions: definitions}
	installer := &filePreservingPackagedInstaller{}
	service := newDefinitionsRootService(t, &routingOperatorSettings{}, catalog.service(installer.install))

	homeDir := t.TempDir()
	first, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if len(first.PackagedFactories) != 1 ||
		first.PackagedFactories[0].Name != "@you/goal" ||
		first.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("first packaged factories = %#v, want one created @you/goal", first.PackagedFactories)
	}

	factoryMarker := filepath.Join(first.PackagedFactories[0].FactoryDir, "customer-owned.txt")
	customerContent := []byte("customer-edited\n")
	if err := os.WriteFile(factoryMarker, customerContent, 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if len(second.PackagedFactories) != 1 ||
		second.PackagedFactories[0].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf("second packaged factories = %#v, want one skipped @you/goal", second.PackagedFactories)
	}

	after, err := os.ReadFile(factoryMarker)
	if err != nil {
		t.Fatalf("read factory marker after repeat = %v", err)
	}
	if string(after) != string(customerContent) {
		t.Fatalf("factory content rewritten on repeat: before %q after %q", customerContent, after)
	}
	if catalog.listCalls < 1 || catalog.resolveCalls < 1 {
		t.Fatalf("catalog list/resolve calls = %d/%d, want collaborator catalog usage", catalog.listCalls, catalog.resolveCalls)
	}
	if installer.calls != 2 {
		t.Fatalf("installer calls = %d, want one per Initialize invocation", installer.calls)
	}
}

// TestInitializeDefinitionsRootBoundary_PartialFailureSurfacesRollbackFactsWhenEnsureFails
// proves partial-failure Initialize still surfaces Bootstrap-owned rollback facts
// when a Definitions ensure collaborator fails after earlier successful work.
func TestInitializeDefinitionsRootBoundary_PartialFailureSurfacesRollbackFactsWhenEnsureFails(t *testing.T) {
	t.Parallel()

	installErr := errors.New("packaged factory ensure denied")
	installer := &failingDefinitionsInstaller{err: installErr}
	service := newDefinitionsRootService(
		t,
		&routingOperatorSettings{},
		&definitionsService{
			listFn: func(
				context.Context,
				factorydefinitions.ListBuiltInPackagedFactoriesRequest,
			) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
				return factorydefinitions.ListBuiltInPackagedFactoriesResult{
					Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
						Name:    "@you/goal",
						Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
					}},
				}, nil
			},
			resolveFn: func(
				_ context.Context,
				request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
			) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
				if request.Name != "@you/goal" {
					return factorydefinitions.ResolveBuiltInPackagedFactoryResult{},
						factorydefinitions.ErrUnknownPackagedFactoryIdentity
				}
				return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
					Definition: factorydefinitions.PackagedDefinition{
						Name:    "@you/goal",
						JSON:    []byte(`{}`),
						Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
					},
				}, nil
			},
			installFn: installer.install,
		},
	)

	_, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	if !errors.Is(err, installErr) {
		t.Fatalf("Initialize() error = %v, want wrapped ensure cause", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if !errors.As(err, &partialFailure) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(partialFailure.Facts) != 3 ||
		partialFailure.Facts[0].Step != systeminitialization.InitializeStepLegacyMigration ||
		partialFailure.Facts[0].Outcome != systeminitialization.RollbackStepCompleted ||
		partialFailure.Facts[1].Step != systeminitialization.InitializeStepSystemConfig ||
		partialFailure.Facts[1].Outcome != systeminitialization.RollbackStepRolledBackOrPreserved ||
		partialFailure.Facts[2].Step != systeminitialization.InitializeStepPackagedFactories ||
		partialFailure.Facts[2].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
	}
}

// TestPackageBoundary_InitializeProductionDoesNotRequireAttachFactoryDefinitions
// proves Bootstrap production Initialize paths under this packet do not call or
// require AttachFactoryDefinitions; collaborators remain Definitions root contracts.
func TestPackageBoundary_InitializeProductionDoesNotRequireAttachFactoryDefinitions(t *testing.T) {
	t.Parallel()

	packageDirs := []string{
		".",
		"wire",
		"internal/workflow",
	}
	for _, packageDir := range packageDirs {
		entries, err := os.ReadDir(packageDir)
		if err != nil {
			t.Fatalf("read %s: %v", packageDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
				strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(packageDir, entry.Name())
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(source), "AttachFactoryDefinitions") {
				t.Fatalf(
					"%s references AttachFactoryDefinitions; Initialize must use Definitions root catalog and ensure collaborators only",
					path,
				)
			}
		}
	}
}
