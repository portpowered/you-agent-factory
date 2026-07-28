package internal_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

func TestNewWithAuthoringLayoutConstructsPublishedRootCatalogSurface(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factoryinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/internal-root",
		Project: "internal-root",
		JSON:    []byte(`{"name":"internal-root"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	root := factoryinternal.NewWithAuthoringLayout(
		rootSurfaceSessionHost{},
		factorydefinition.StubActivationGateway(),
		staticClock{instant: time.Unix(0, 0)},
		platformfilesystem.Local{},
		rootSurfaceValidator{},
		func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, nil
		},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, nil
		},
		func(string) (string, error) { return "", nil },
		func(context.Context, string, []byte, factorydefinitions.Validator) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return &factorydefinitions.PreparedFactoryLayoutPayload{}, nil
		},
		func(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "", nil
		},
		func(string, string) error { return nil },
		func(string, *factorydefinitions.FactoryConfig, bool) (*factorydefinitions.FactoryConfig, error) {
			return &factorydefinitions.FactoryConfig{}, nil
		},
		func(
			string,
			*factorydefinitions.FactoryConfig,
			factorydefinitions.RuntimeDefinitionLookup,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return &factorydefinitions.FactorySnapshot{}, nil
		},
		func(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
			return nil, nil
		},
		rootSurfaceNamedPaths{},
		platformfilesystem.Local{},
		packagedCatalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		rootSurfaceRequiredToolChecker{},
		rootSurfaceOrchestratorValidator{},
		stubAuthoringLayout{},
	)
	if root == nil {
		t.Fatal("NewWithAuthoringLayout() = nil, want composed Factory Definitions root")
	}

	payload := []byte(`{"name":"alpha"}`)
	prepared, err := root.PrepareFactoryLayout(
		t.Context(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout() error = %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(payload) {
		t.Fatalf("PrepareFactoryLayout canonical = %q, want %q", prepared.Prepared.Canonical, payload)
	}

	listed, err := root.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/internal-root" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, want one @you/internal-root entry", listed.Entries)
	}
}

type staticClock struct{ instant time.Time }

func (c staticClock) Now() time.Time { return c.instant }

type rootSurfaceSessionHost struct{}

func (rootSurfaceSessionHost) PersistRootDir() string { return "/persist" }
func (rootSurfaceSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (rootSurfaceSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource { return nil }
func (rootSurfaceSessionHost) WorkflowID() string                                           { return "workflow" }
func (rootSurfaceSessionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{}, nil
}
func (rootSurfaceSessionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, nil
}
func (rootSurfaceSessionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return "/persist"
}
func (rootSurfaceSessionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (rootSurfaceSessionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, nil
}
func (rootSurfaceSessionHost) WithActivationLock(func() error) error { return nil }
func (rootSurfaceSessionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}
func (rootSurfaceSessionHost) ActivateSessionEditableFactory(
	context.Context,
	*factorydefinitions.DefinitionSession,
	string, string, string, string, string,
) error {
	return nil
}
func (rootSurfaceSessionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}
func (rootSurfaceSessionHost) SaveNow() time.Time   { return time.Unix(0, 0) }
func (rootSurfaceSessionHost) RunSessionID() string { return "" }
func (rootSurfaceSessionHost) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}
func (rootSurfaceSessionHost) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}
func (rootSurfaceSessionHost) RequireIdleBeforeNamedFactoryActivation(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
) error {
	return nil
}
func (rootSurfaceSessionHost) SwapPersistedNamedFactoryRuntime(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
	string, string, string, string,
) error {
	return nil
}
func (rootSurfaceSessionHost) AttachFactoryDefinitions(
	definitions factorydefinitions.Service,
) factorydefinitions.Service {
	return definitions
}

type rootSurfaceValidator struct{}

func (rootSurfaceValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (rootSurfaceValidator) ValidateBlockingLoad(context.Context, *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (rootSurfaceValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	return factorydefinitions.TopologyValidationResult{}
}
func (rootSurfaceValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (rootSurfaceValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (rootSurfaceValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

type rootSurfaceNamedPaths struct{}

func (rootSurfaceNamedPaths) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (rootSurfaceNamedPaths) ResolveExistingDir(string, string) (string, error) {
	return "/factories", nil
}
func (rootSurfaceNamedPaths) RequireDefinitionDir(string) error { return nil }
func (rootSurfaceNamedPaths) ResolveCurrentDir(string) (string, error) {
	return "", fs.ErrNotExist
}
func (rootSurfaceNamedPaths) ReadCurrentPointer(string) (string, error) { return "", nil }
func (rootSurfaceNamedPaths) WriteCurrentPointer(string, string) error  { return nil }

type rootSurfaceRequiredToolChecker struct{}

func (rootSurfaceRequiredToolChecker) Check(
	factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	return factorydefinitions.RequiredToolCheckResult{}
}

type rootSurfaceOrchestratorValidator struct{}

func (rootSurfaceOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	context.Context,
	*factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	return nil
}
