package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"runtime"
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

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	baselineGoroutines := runtime.NumGoroutine()
	sessionHost := &recordingSessionHost{}
	namedPaths := &recordingNamedPathResolver{}
	namedFactoryCatalogFileSystem := &recordingNamedFactoryCatalogFileSystem{}
	versionFileSystem := &recordingVersionFileSystem{}
	clock := &recordingClock{}

	packagedCatalog, err := factorydefinitionsservice.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
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

	service, err := factorydefinitionswire.NewService(
		sessionHost,
		stubValidator{},
		stubPersistence{},
		&factoryloading.Loader{},
		func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		func(string, *factorydefinitions.FactoryConfig) error { return nil },
		namedPaths,
		namedFactoryCatalogFileSystem,
		clock,
		versionFileSystem,
		func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
		packagedCatalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
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

	if namedPaths.calls != 0 {
		t.Fatalf("construction invoked named path resolver %d times, want no filesystem activity", namedPaths.calls)
	}
	if namedFactoryCatalogFileSystem.calls != 0 {
		t.Fatalf(
			"construction invoked named Factory catalog filesystem %d times, want no filesystem activity",
			namedFactoryCatalogFileSystem.calls,
		)
	}
	if versionFileSystem.calls != 0 {
		t.Fatalf("construction invoked version filesystem %d times, want no filesystem activity", versionFileSystem.calls)
	}
	if clock.calls != 0 {
		t.Fatalf("construction read clock %d times, want no runtime activity", clock.calls)
	}
	if sessionHost.runtimeCalls != 0 {
		t.Fatalf(
			"construction invoked session-host runtime ports %d times, want no disk or runtime activity",
			sessionHost.runtimeCalls,
		)
	}
	if sessionHost.attachCalls != 1 {
		t.Fatalf("AttachFactoryDefinitions calls = %d, want exactly one construction-time wiring call", sessionHost.attachCalls)
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

func TestNewServiceServesPublishedPackagedCatalogPeerBehavior(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factorydefinitionsservice.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{
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

type recordingVersionFileSystem struct{ calls int }

func (v *recordingVersionFileSystem) Stat(string) (fs.FileInfo, error) {
	v.calls++
	panic("version filesystem invoked during inert construction")
}

type recordingClock struct{ calls int }

func (c *recordingClock) Now() time.Time {
	c.calls++
	panic("clock invoked during inert construction")
}

type recordingSessionHost struct {
	runtimeCalls int
	attachCalls  int
}

func (h *recordingSessionHost) recordRuntimeCall() {
	h.runtimeCalls++
	panic("session-host runtime port invoked during inert construction")
}

func (h *recordingSessionHost) PersistRootDir() string {
	h.recordRuntimeCall()
	return ""
}

func (h *recordingSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) WorkflowID() string {
	h.recordRuntimeCall()
	return ""
}

func (h *recordingSessionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	h.recordRuntimeCall()
	return nil, errors.New("session not found")
}

func (h *recordingSessionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	h.recordRuntimeCall()
	return nil, errors.New("session not found")
}

func (h *recordingSessionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	h.recordRuntimeCall()
	return ""
}

func (h *recordingSessionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	h.recordRuntimeCall()
	return nil, errors.New("session not found")
}

func (h *recordingSessionHost) WithActivationLock(func() error) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) ActivateSessionEditableFactory(
	context.Context,
	*factorydefinitions.DefinitionSession,
	string, string, string, string, string,
) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	h.recordRuntimeCall()
	return nil, nil
}

func (h *recordingSessionHost) SaveNow() time.Time {
	h.recordRuntimeCall()
	return time.Unix(0, 0)
}

func (h *recordingSessionHost) RunSessionID() string {
	h.recordRuntimeCall()
	return ""
}

func (h *recordingSessionHost) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	h.recordRuntimeCall()
	return "", ""
}

func (h *recordingSessionHost) RequireIdleBeforeNamedFactoryActivation(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) SwapPersistedNamedFactoryRuntime(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
	string, string, string, string,
) error {
	h.recordRuntimeCall()
	return nil
}

func (h *recordingSessionHost) AttachFactoryDefinitions(
	definitions factorydefinitions.Service,
) factorydefinitions.Service {
	h.attachCalls++
	return definitions
}
