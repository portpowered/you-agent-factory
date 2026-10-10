package internal_test

import (
	"context"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorylifecycle "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
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

	distribution, err := distributionwire.NewService(packagedCatalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := factorylifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		rootSurfaceSessionHost{}, factorylifecycle.StubActivationGateway(),
		factorydefinitions.UnimplementedService{}, factorydefinitions.UnimplementedService{},
		rootSurfaceAuthoring{}, distribution, factorydefinitions.UnimplementedService{},
		factorydefinitions.UnimplementedService{}, platformfilesystem.Local{},
		factorydefinitions.UnimplementedService{}.ListEffectiveFactories, factorydefinitions.UnimplementedService{},
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

type rootSurfaceSessionHost struct{}

func (rootSurfaceSessionHost) PersistRootDir() string { return "/persist" }
func (rootSurfaceSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (rootSurfaceSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}
func (rootSurfaceSessionHost) WorkflowID() string { return "workflow" }
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

type rootSurfaceAuthoring struct {
	factorydefinitions.UnimplementedService
}

func (rootSurfaceAuthoring) PrepareFactoryLayout(_ context.Context, request factorydefinitions.PrepareFactoryLayoutRequest) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return factorydefinitions.PrepareFactoryLayoutResult{Prepared: factorydefinitions.PreparedFactoryLayoutPayload{Canonical: request.Payload}}, nil
}
