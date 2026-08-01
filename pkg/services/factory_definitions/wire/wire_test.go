package wire_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"runtime"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorydefaultscaffold "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/defaultscaffold"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
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
			name:   "validator",
			mutate: func(ports *constructionPorts) { ports.validator = nil },
			want:   "validator is required",
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
		{
			name:   "required tool checker",
			mutate: func(ports *constructionPorts) { ports.requiredToolChecker = nil },
			want:   "required tool checker is required",
		},
		{
			name:   "orchestrator definition validator",
			mutate: func(ports *constructionPorts) { ports.orchestratorValidator = nil },
			want:   "orchestrator definition validator is required",
		},
		{
			name:   "portable filesystem",
			mutate: func(ports *constructionPorts) { ports.portableFileSystem = nil },
			want:   "portable filesystem is required",
		},
		{
			name:   "directory replacement store",
			mutate: func(ports *constructionPorts) { ports.directoryReplacementStore = nil },
			want:   "directory replacement store is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := base
			test.mutate(&ports)
			service, err := factorydefinitionswire.NewService(definitionDependencies(ports))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() error = %v, want %q", err, test.want)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	baselineGoroutines := runtime.NumGoroutine()
	namedPaths := &recordingNamedPathResolver{}
	namedFactoryCatalogFileSystem := &recordingNamedFactoryCatalogFileSystem{}

	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/wire-inert",
		Project: "wire-inert",
		JSON:    []byte(`{"name":"wire-inert"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	service, err := factorydefinitionswire.NewService(factorydefinitionswire.Dependencies{
		Validator:                     stubValidator{},
		DefinitionValidation:          stubValidator{},
		EffectiveDefinitionValidation: stubValidator{},
		Loader:                        &compilationwire.Loader{},
		ApplySupportedFiles:           func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		ApplyStarterWork:              func(string, *factorydefinitions.FactoryConfig) error { return nil },
		NamedPaths:                    namedPaths,
		NamedFactoryCatalogFileSystem: namedFactoryCatalogFileSystem,
		ListEffective: func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
		PackagedCatalog: packagedCatalog,
		PackagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		}},
		RequiredToolChecker:       stubRequiredToolChecker{},
		OrchestratorValidator:     stubOrchestratorValidator{},
		PortableFileSystem:        platformfilesystem.Local{},
		DirectoryReplacementStore: stubDirectoryReplacementStore{},
		Representation:            testRepresentation(),
		MapFactoryJSONForPersistence: func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{}, nil
		},
	})
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

	if namedPaths.calls != 0 {
		t.Fatalf("construction invoked named path resolver %d times, want no filesystem activity", namedPaths.calls)
	}
	if namedFactoryCatalogFileSystem.calls != 0 {
		t.Fatalf(
			"construction invoked named Factory catalog filesystem %d times, want no filesystem activity",
			namedFactoryCatalogFileSystem.calls,
		)
	}
	if leaked := runtime.NumGoroutine() - baselineGoroutines; leaked > 4 {
		t.Fatalf(
			"construction started background goroutines: baseline=%d current=%d delta=%d",
			baselineGoroutines,
			runtime.NumGoroutine(),
			leaked,
		)
	}
}

func TestNewServiceResolvesInvocationDefinitionThroughRoot(t *testing.T) {
	t.Parallel()

	service, err := factorydefinitionswire.NewService(definitionDependencies(validConstructionPorts(t)))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	resolved, err := service.ResolveInvocationDefinition(
		t.Context(),
		factorydefinitions.ResolveInvocationDefinitionRequest{
			Definition: factorydefinitions.EffectiveFactorySource{
				Factory: &factorydefinitions.FactoryConfig{
					Name: "root-invocation",
					WorkTypes: []factorydefinitions.WorkTypeConfig{{
						Name:             "default-work",
						HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
					}},
					Workstations: []factorydefinitions.FactoryWorkstationConfig{{
						ID:   "main",
						Name: "main",
					}},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ResolveInvocationDefinition() error = %v", err)
	}
	if resolved.DefaultWork != "default-work" {
		t.Fatalf("DefaultWork = %q, want default-work", resolved.DefaultWork)
	}
	if got := resolved.Workstations["main"].DecisionMode; got != factorydefinitions.DecisionEnvelopeModeNone {
		t.Fatalf("main DecisionMode = %q, want none", got)
	}
}

func TestNewServiceServesPublishedPackagedCatalogPeerBehavior(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{
		{
			Name: "@you/review", Project: "builtin-review",
			JSON: []byte(`{"name":"review"}`),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
			},
		},
		{
			Name: "@you/goal", Project: "builtin-goal",
			JSON: []byte(`{"name":"goal"}`),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	ports := validConstructionPorts(t)
	ports.packagedCatalog = packagedCatalog
	service, err := factorydefinitionswire.NewService(definitionDependencies(ports))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root factorydefinitions.Service = service

	listed, err := root.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	gotNames := []string{listed.Entries[0].Name, listed.Entries[1].Name}
	if !reflect.DeepEqual(gotNames, []string{"@you/goal", "@you/review"}) {
		t.Fatalf("ListBuiltInPackagedFactories names = %v, want [@you/goal @you/review]", gotNames)
	}

	resolved, err := root.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil {
		t.Fatalf("ResolveBuiltInPackagedFactory() error = %v", err)
	}
	if resolved.Definition.Project != "builtin-goal" {
		t.Fatalf("ResolveBuiltInPackagedFactory result = %#v, want builtin-goal project", resolved)
	}

	_, err = root.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/missing"},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("ResolveBuiltInPackagedFactory(missing) error = %v, want ErrUnknownPackagedFactoryIdentity", err)
	}
}

func TestNewServiceServesPublishedCompilePeerBehavior(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := factorydefinitionswire.NewService(definitionDependencies(ports))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root factorydefinitions.Service = service

	_, invalidErr := root.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource invalid-source error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidAuthoredFactorySource,
		)
	}

	_, unresolvedErr := root.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	)
	if !errors.Is(unresolvedErr, factorydefinitions.ErrUnresolvedDefinitionReference) {
		t.Fatalf(
			"CompileEffectiveFactorySource unresolved error = %v, want %v",
			unresolvedErr,
			factorydefinitions.ErrUnresolvedDefinitionReference,
		)
	}
	if errors.Is(unresolvedErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatal("unresolved definition reference must not also match ErrInvalidAuthoredFactorySource")
	}
}

func TestEffectiveFactoryDefinitionNormalizerFromMapperHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	normalizer := factorydefinitionswire.EffectiveFactoryDefinitionNormalizerFromMapper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := normalizer(ctx, factorydefinitions.EffectiveFactoryCatalogCandidate{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("normalizer() error = %v, want context.Canceled", err)
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := factorydefinitionswire.NewService(definitionDependencies(ports))
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

func TestNewServiceDelegatesSnapshotPortabilityThroughRoot(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := factorydefinitionswire.NewService(definitionDependencies(ports))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.CaptureFactorySnapshot(
		t.Context(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"not-object"`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("CaptureFactorySnapshot() error = %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	_, err = service.PrepareFactorySnapshotImport(
		t.Context(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("PrepareFactorySnapshotImport() error = %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	_, err = service.MaterializeFactorySnapshot(
		t.Context(),
		factorydefinitions.MaterializeFactorySnapshotRequest{},
	)
	if !errors.Is(err, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf("MaterializeFactorySnapshot() error = %v, want ErrUnsafeFactorySnapshotMaterialize", err)
	}
}

type stubRequiredToolChecker struct{}

func (stubRequiredToolChecker) Check(
	factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	return factorydefinitions.RequiredToolCheckResult{}
}

type stubOrchestratorValidator struct{}

func (stubOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	context.Context,
	*factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	return nil
}

type constructionPorts struct {
	validator                     factorydefinitions.Validator
	definitionValidation          factorydefinitions.DefinitionValidationOperation
	effectiveDefinitionValidation factorydefinitions.EffectiveDefinitionValidationOperation
	loader                        *compilationwire.Loader
	applySupportedFiles           factorydefinitions.PortableBundledFilesApplier
	applyStarterWork              factorydefinitions.FactoryStarterWorkApplier
	namedPaths                    factoryeffect.NamedPathResolver
	namedFactoryCatalogFileSystem factoryeffect.NamedFactoryCatalogFileSystem
	listEffective                 factorydefinitions.EffectiveFactoryCatalogOperation
	packagedCatalog               factorydefinitions.PackagedFactoryCatalogOperations
	packagedInstaller             factorydefinitions.PackagedFactoryInstallationOperations
	requiredToolChecker           factorydefinitions.RequiredToolChecker
	orchestratorValidator         factorydefinitions.OrchestratorDefinitionValidator
	portableFileSystem            portablefiles.FileSystem
	directoryReplacementStore     factoryeffect.DirectoryReplacementStore
}

func definitionDependencies(ports constructionPorts) factorydefinitionswire.Dependencies {
	return factorydefinitionswire.Dependencies{
		Validator:                     ports.validator,
		DefinitionValidation:          ports.definitionValidation,
		EffectiveDefinitionValidation: ports.effectiveDefinitionValidation,
		Loader:                        ports.loader,
		ApplySupportedFiles:           ports.applySupportedFiles,
		ApplyStarterWork:              ports.applyStarterWork,
		NamedPaths:                    ports.namedPaths,
		NamedFactoryCatalogFileSystem: ports.namedFactoryCatalogFileSystem,
		ListEffective:                 ports.listEffective,
		PackagedCatalog:               ports.packagedCatalog,
		PackagedInstaller:             ports.packagedInstaller,
		RequiredToolChecker:           ports.requiredToolChecker,
		OrchestratorValidator:         ports.orchestratorValidator,
		PortableFileSystem:            ports.portableFileSystem,
		DirectoryReplacementStore:     ports.directoryReplacementStore,
		Representation:                testRepresentation(),
		MapFactoryJSONForPersistence: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, ports.loader.LoadSourceFromCanonicalJSON)
		},
	}
}

func validConstructionPorts(t *testing.T) constructionPorts {
	t.Helper()

	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
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
		validator:                     stubValidator{},
		definitionValidation:          stubValidator{},
		effectiveDefinitionValidation: stubValidator{},
		loader:                        &compilationwire.Loader{},
		applySupportedFiles:           func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		applyStarterWork:              func(string, *factorydefinitions.FactoryConfig) error { return nil },
		namedPaths:                    stubNamedPathResolver{},
		namedFactoryCatalogFileSystem: platformfilesystem.Local{},
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
		requiredToolChecker:       stubRequiredToolChecker{},
		orchestratorValidator:     stubOrchestratorValidator{},
		portableFileSystem:        platformfilesystem.Local{},
		directoryReplacementStore: stubDirectoryReplacementStore{},
	}
}

type stubDirectoryReplacementStore struct{}

func (stubDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}
func (stubDirectoryReplacementStore) Restore(string, string) {}

type stubValidator struct{}

func (stubValidator) ValidateDefinition(
	context.Context,
	factorydefinitions.DefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return factorydefinitions.ValidationResult{}, nil
}

func (stubValidator) ValidateEffectiveDefinition(
	context.Context,
	factorydefinitions.EffectiveDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return factorydefinitions.ValidationResult{}, nil
}

func (stubValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (stubValidator) ValidateBlockingLoad(context.Context, *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (stubValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	return factorydefinitions.TopologyValidationResult{}
}
func (stubValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (stubValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (stubValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
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

type recordingNamedPathResolver struct{ calls int }

func (r *recordingNamedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

func (r *recordingNamedPathResolver) ResolveExistingDir(string, string) (string, error) {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

func (r *recordingNamedPathResolver) RequireDefinitionDir(string) error {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

func (r *recordingNamedPathResolver) ResolveCurrentDir(string) (string, error) {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

func (r *recordingNamedPathResolver) ReadCurrentPointer(string) (string, error) {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

func (r *recordingNamedPathResolver) WriteCurrentPointer(string, string) error {
	r.calls++
	panic("named path resolver invoked during inert construction")
}

type recordingNamedFactoryCatalogFileSystem struct{ calls int }

func (f *recordingNamedFactoryCatalogFileSystem) Stat(string) (fs.FileInfo, error) {
	f.calls++
	panic("named Factory catalog filesystem invoked during inert construction")
}

func (f *recordingNamedFactoryCatalogFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	f.calls++
	panic("named Factory catalog filesystem invoked during inert construction")
}

func (f *recordingNamedFactoryCatalogFileSystem) RemoveAll(string) error {
	f.calls++
	panic("named Factory catalog filesystem invoked during inert construction")
}

func TestNewServiceInstallAndScaffoldReturnMatchingDistributedFacts(t *testing.T) {
	t.Parallel()

	goalJSON, err := json.Marshal(map[string]string{"name": "goal", "project": "builtin-goal"})
	if err != nil {
		t.Fatalf("marshal goal factory: %v", err)
	}
	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Project: "builtin-goal",
		JSON:    goalJSON,
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	fileSystem := platformfilesystem.Local{}
	output := &bytes.Buffer{}
	scaffoldInitializer, err := factorydefaultscaffold.NewScaffoldInitializer(fileSystem, output)
	if err != nil {
		t.Fatalf("NewScaffoldInitializer() error = %v", err)
	}

	ports := validConstructionPorts(t)
	ports.packagedCatalog = packagedCatalog
	ports.packagedInstaller = factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			_ context.Context,
			params factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			return factorydefinitions.PackagedFactoryInstallResult{
				Name:       params.Definition.Name,
				FactoryDir: "/customer/factories/@you/goal",
				Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
				Format:     params.Format,
			}, nil
		},
	}

	service, err := factorydefinitionswire.NewService(
		definitionDependencies(ports),
		factorydefinitionswire.WithDistributionScaffold(
			scaffoldInitializer,
			distributionwire.LocalFactoryNameResolver(),
		),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	installed, err := service.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if installed.Definition.Name == "" || installed.Definition.FactoryDir == "" {
		t.Fatalf("InstallPackagedFactory() facts = %#v, want populated Name and FactoryDir", installed.Definition)
	}

	scaffoldDir := t.TempDir()
	scaffolded, err := service.CreateFactoryScaffold(
		t.Context(),
		factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: scaffoldDir},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold() error = %v", err)
	}
	if scaffolded.Definition.Name == "" || scaffolded.Definition.FactoryDir == "" {
		t.Fatalf("CreateFactoryScaffold() facts = %#v, want populated Name and FactoryDir", scaffolded.Definition)
	}
	if reflect.TypeOf(installed.Definition) != reflect.TypeOf(scaffolded.Definition) {
		t.Fatalf(
			"distributed facts types = %T vs %T, want matching DistributedFactoryDefinitionFacts",
			installed.Definition,
			scaffolded.Definition,
		)
	}
}
