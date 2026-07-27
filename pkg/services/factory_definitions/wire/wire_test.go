package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	base := validConstructionPorts(t)
	tests := []struct {
		name   string
		mutate func(*constructionPorts)
		want   string
	}{
		{
			name:   "session host",
			mutate: func(ports *constructionPorts) { ports.sessionHost = nil },
			want:   "session host is required",
		},
		{
			name:   "validator",
			mutate: func(ports *constructionPorts) { ports.validator = nil },
			want:   "validator is required",
		},
		{
			name:   "persistence",
			mutate: func(ports *constructionPorts) { ports.persistence = nil },
			want:   "persistence is required",
		},
		{
			name:   "loader",
			mutate: func(ports *constructionPorts) { ports.loader = nil },
			want:   "loader is required",
		},
		{
			name:   "portable bundled files applier",
			mutate: func(ports *constructionPorts) { ports.applySupportedFiles = nil },
			want:   "portable bundled files applier is required",
		},
		{
			name:   "starter Work applier",
			mutate: func(ports *constructionPorts) { ports.applyStarterWork = nil },
			want:   "starter Work applier is required",
		},
		{
			name:   "named path resolver",
			mutate: func(ports *constructionPorts) { ports.namedPaths = nil },
			want:   "named path resolver is required",
		},
		{
			name:   "named Factory catalog filesystem",
			mutate: func(ports *constructionPorts) { ports.namedFactoryCatalogFileSystem = nil },
			want:   "named Factory catalog filesystem is required",
		},
		{
			name:   "clock",
			mutate: func(ports *constructionPorts) { ports.clock = nil },
			want:   "clock is required",
		},
		{
			name:   "version filesystem",
			mutate: func(ports *constructionPorts) { ports.versionFileSystem = nil },
			want:   "version filesystem is required",
		},
		{
			name:   "effective Factory catalog",
			mutate: func(ports *constructionPorts) { ports.listEffective = nil },
			want:   "effective Factory catalog is required",
		},
		{
			name: "packaged Factory catalog list operation",
			mutate: func(ports *constructionPorts) {
				ports.packagedCatalog.List = nil
			},
			want: "packaged Factory catalog list operation is required",
		},
		{
			name: "packaged Factory catalog resolve operation",
			mutate: func(ports *constructionPorts) {
				ports.packagedCatalog.Resolve = nil
			},
			want: "packaged Factory catalog resolve operation is required",
		},
		{
			name: "packaged Factory installer",
			mutate: func(ports *constructionPorts) {
				ports.packagedInstaller.Install = nil
			},
			want: "packaged Factory installer is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := base
			test.mutate(&ports)
			service, err := factorydefinitionswire.NewService(
				ports.sessionHost,
				ports.validator,
				ports.persistence,
				ports.loader,
				ports.applySupportedFiles,
				ports.applyStarterWork,
				ports.namedPaths,
				ports.namedFactoryCatalogFileSystem,
				ports.clock,
				ports.versionFileSystem,
				ports.listEffective,
				ports.packagedCatalog,
				ports.packagedInstaller,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() error = %v, want %q", err, test.want)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := factorydefinitionswire.NewService(
		ports.sessionHost,
		ports.validator,
		ports.persistence,
		ports.loader,
		ports.applySupportedFiles,
		ports.applyStarterWork,
		ports.namedPaths,
		ports.namedFactoryCatalogFileSystem,
		ports.clock,
		ports.versionFileSystem,
		ports.listEffective,
		ports.packagedCatalog,
		ports.packagedInstaller,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factorydefinitions.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}
}

type constructionPorts struct {
	sessionHost                   factorydefinitions.SessionHost
	validator                     factorydefinitions.Validator
	persistence                   factorydefinitions.Persistence
	loader                        *factoryloading.Loader
	applySupportedFiles           factorydefinitions.PortableBundledFilesApplier
	applyStarterWork              factorydefinitions.FactoryStarterWorkApplier
	namedPaths                    factorydefinitions.NamedPathResolver
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem
	clock                         factorydefinitions.Clock
	versionFileSystem             factorydefinitions.VersionFileSystem
	listEffective                 factorydefinitions.EffectiveFactoryCatalogOperation
	packagedCatalog               factorydefinitions.PackagedFactoryCatalogOperations
	packagedInstaller             factorydefinitions.PackagedFactoryInstallationOperations
}

func validConstructionPorts(t *testing.T) constructionPorts {
	t.Helper()

	packagedCatalog, err := factorydefinitionsservice.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/wire-test",
		Project: "wire-test",
		JSON:    []byte(`{"name":"wire-test"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	return constructionPorts{
		sessionHost:                   &stubSessionHost{},
		validator:                     stubValidator{},
		persistence:                   stubPersistence{},
		loader:                        &factoryloading.Loader{},
		applySupportedFiles:           func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		applyStarterWork:              func(string, *factorydefinitions.FactoryConfig) error { return nil },
		namedPaths:                    stubNamedPathResolver{},
		namedFactoryCatalogFileSystem: platformfilesystem.Local{},
		clock:                         factorydefinitionswire.StaticClock(time.Unix(0, 0)),
		versionFileSystem:             platformfilesystem.Local{},
		listEffective: func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
		packagedCatalog: packagedCatalog,
		packagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
	}
}

type stubSessionHost struct{}

func (stubSessionHost) PersistRootDir() string { return "" }
func (stubSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (stubSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource { return nil }
func (stubSessionHost) WorkflowID() string                                           { return "" }
func (stubSessionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, errors.New("session not found")
}
func (stubSessionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, errors.New("session not found")
}
func (stubSessionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (stubSessionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (stubSessionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, errors.New("session not found")
}
func (stubSessionHost) WithActivationLock(func() error) error { return nil }
func (stubSessionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}
func (stubSessionHost) ActivateSessionEditableFactory(
	context.Context,
	*factorydefinitions.DefinitionSession,
	string, string, string, string, string,
) error {
	return nil
}
func (stubSessionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}
func (stubSessionHost) SaveNow() time.Time   { return time.Unix(0, 0) }
func (stubSessionHost) RunSessionID() string { return "" }
func (stubSessionHost) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}
func (stubSessionHost) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}
func (stubSessionHost) RequireIdleBeforeNamedFactoryActivation(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
) error {
	return nil
}
func (stubSessionHost) SwapPersistedNamedFactoryRuntime(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
	string, string, string, string,
) error {
	return nil
}
func (stubSessionHost) AttachFactoryDefinitions(
	definitions factorydefinitions.Service,
) factorydefinitions.Service {
	return definitions
}

type stubValidator struct{}

func (stubValidator) Validate(
	context.Context,
	*factorycontracts.FactoryConfig,
	factorycontracts.WorkflowSourceReader,
) factorycontracts.ValidationResult {
	return factorycontracts.ValidationResult{}
}
func (stubValidator) ValidateBlockingLoad(context.Context, *factorycontracts.FactoryConfig) factorycontracts.ValidationResult {
	return factorycontracts.ValidationResult{}
}
func (stubValidator) ValidateTopology(
	context.Context,
	*factorycontracts.FactoryConfig,
	factorycontracts.RequiredToolChecker,
) factorycontracts.TopologyValidationResult {
	return factorycontracts.TopologyValidationResult{}
}
func (stubValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorycontracts.FactoryConfig,
) []factorycontracts.ValidationTarget {
	return nil
}
func (stubValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorycontracts.FactoryConfig,
	bool,
) []factorycontracts.ValidationTarget {
	return nil
}
func (stubValidator) PruneLayout(
	context.Context,
	*factorycontracts.FactoryConfig,
	factorycontracts.PendingFactoryGraphTopology,
) factorycontracts.ValidationResult {
	return factorycontracts.ValidationResult{}
}

type stubPersistence struct{}

func (stubPersistence) PersistNamedFactory(
	context.Context,
	factorydefinitions.NamedFactoryPersistenceRequest,
) (factorydefinitions.NamedFactoryPersistenceResult, error) {
	return factorydefinitions.NamedFactoryPersistenceResult{}, nil
}
func (stubPersistence) PrepareFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return nil, nil
}
func (stubPersistence) ValidateFactoryLayout(string) error          { return nil }
func (stubPersistence) FlattenFactoryLayout(string) ([]byte, error) { return nil, nil }
func (stubPersistence) ExpandFactoryLayout(string) (string, factorydefinitions.LayoutExpansionReport, error) {
	return "", factorydefinitions.LayoutExpansionReport{}, nil
}
func (stubPersistence) CreateNamedFactory(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	return "", nil
}
func (stubPersistence) ReplaceNamedFactory(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	return "", nil
}
func (stubPersistence) ReplaceFactoryLayout(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type stubNamedPathResolver struct{}

func (stubNamedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (stubNamedPathResolver) ResolveExistingDir(string, string) (string, error) {
	return "", factorydefinitions.ErrNamedFactoryNotFound
}
func (stubNamedPathResolver) RequireDefinitionDir(string) error { return fs.ErrNotExist }
func (stubNamedPathResolver) ResolveCurrentDir(string) (string, error) {
	return "", fs.ErrNotExist
}
func (stubNamedPathResolver) ReadCurrentPointer(string) (string, error) {
	return "", fs.ErrNotExist
}
func (stubNamedPathResolver) WriteCurrentPointer(string, string) error { return nil }
