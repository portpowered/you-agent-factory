package factorydefinition_test

import (
	"context"
	"time"

	apitypes "github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

var testDefinitionService = factorydefinition.New(stubDefinitionHost{}, factorydefinition.StubActivationGateway())

func prepareExternalFactoryLayoutForDefinitionTest(
	ctx context.Context,
	segment string,
	payload []byte,
	validator factorydefinitions.Validator,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	mapper := factoryconfigmapping.NewFactoryConfigMapper()
	return factoryauthoredlayout.Prepare(
		ctx,
		segment,
		payload,
		validator,
		mapper.Expand,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		mapper.Flatten,
	)
}

type stubDefinitionHost struct{}

func (stubDefinitionHost) PersistRootDir() string { return "" }
func (stubDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (stubDefinitionHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}
func (stubDefinitionHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return externalDefinitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (stubDefinitionHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return externalDefinitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}
func (stubDefinitionHost) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return prepareExternalFactoryLayoutForDefinitionTest(
		context.Background(),
		segment,
		payload,
		factoryvalidation.New(nil),
	)
}
func (stubDefinitionHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}
func (stubDefinitionHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	return externalDefinitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}
func (stubDefinitionHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return portableconfig.Prepare(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}
func (stubDefinitionHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorysnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}
func (stubDefinitionHost) CurrentRuntimeConfig() loadedFactorySource {
	return nil
}
func (stubDefinitionHost) WorkflowID() string { return "" }
func (stubDefinitionHost) RequireSession(string) (*interfaces.DefinitionSession, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionFactoryPersistRoot(*interfaces.DefinitionSession) string {
	return ""
}
func (h stubDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *interfaces.FactorySnapshot) error {
	return validateEditableFactorySnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (stubDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*interfaces.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (stubDefinitionHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (stubDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (stubDefinitionHost) ActivateSessionEditableFactory(context.Context, *interfaces.DefinitionSession, string, string, string, string, string) error {
	return nil
}

func (stubDefinitionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*interfaces.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (stubDefinitionHost) SaveNow() time.Time {
	return time.Time{}
}

func (stubDefinitionHost) RunSessionID() string { return "" }

func (stubDefinitionHost) SessionForActivation(string) *interfaces.DefinitionSession {
	return nil
}

func (stubDefinitionHost) NamedFactoryActivationPaths(*interfaces.DefinitionSession) (string, string) {
	return "", ""
}

func (stubDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *interfaces.DefinitionSession) error {
	return nil
}

func (stubDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *interfaces.DefinitionSession, string, string, string, string) error {
	return nil
}

func mustFactorySnapshot(factory factoryapi.Factory) *interfaces.FactorySnapshot {
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		panic(err)
	}
	return snapshot
}

func requireFreshEditableFactoryVersion(
	baseVersion *factoryapi.HybridLogicalTimestamp,
	currentVersion factoryapi.HybridLogicalTimestamp,
) error {
	return testDefinitionService.RequireFreshEditableFactoryVersion(testFactoryVersionFromAPI(baseVersion), *testFactoryVersionFromAPI(&currentVersion))
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	version := testDefinitionService.NextEditableFactoryVersion(testFactoryVersionFromAPI(current), now)
	return factoryapi.HybridLogicalTimestamp{Logical: apitypes.Int64String(version.Logical), Physical: version.Physical}
}

func preparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		return nil, err
	}
	return testDefinitionService.PreparePersistedFactoryPayload(segment, snapshot, *testFactoryVersionFromAPI(&version))
}

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		return nil, err
	}
	return testDefinitionService.PrepareEditableFactoryPersistView(segment, snapshot)
}

func testFactoryVersionFromAPI(version *factoryapi.HybridLogicalTimestamp) *interfaces.FactoryVersion {
	if version == nil {
		return nil
	}
	return &interfaces.FactoryVersion{Logical: version.Logical.Int64(), Physical: version.Physical.UTC()}
}

type saveDefinitionHostAdapter struct {
	saveHost definitionSaveHost
	rootDir  string
}

type definitionSaveHost interface {
	RequireSession(sessionID string) (*interfaces.DefinitionSession, error)
	SessionRuntimeConfig(sessionID string) (loadedFactorySource, error)
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	ActivateSessionEditableFactory(
		ctx context.Context,
		session *interfaces.DefinitionSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name factoryapi.FactoryName,
		runtimeName string,
	) error
	ReplaceFactoryLayoutAtDir(
		targetDir string,
		prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	) (*interfaces.FactorySplitLayoutReplaceResult, error)
}

func saveFactoryThroughDefinition(
	rootDir string,
	saveHost definitionSaveHost,
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	adapter := saveDefinitionHostAdapter{
		saveHost: saveHost,
		rootDir:  rootDir,
	}
	return factorydefinitionmapping.New(factorydefinition.New(adapter, adapter)).Save(ctx, sessionID, mode, request)
}

func (h saveDefinitionHostAdapter) PersistRootDir() string { return h.rootDir }

func (h saveDefinitionHostAdapter) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}

func (h saveDefinitionHostAdapter) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}

func (h saveDefinitionHostAdapter) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return externalDefinitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (h saveDefinitionHostAdapter) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return externalDefinitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}

func (h saveDefinitionHostAdapter) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return prepareExternalFactoryLayoutForDefinitionTest(
		context.Background(),
		segment,
		payload,
		factoryvalidation.New(nil),
	)
}

func (h saveDefinitionHostAdapter) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}

func (h saveDefinitionHostAdapter) WriteCurrentFactoryPointer(rootDir, name string) error {
	return externalDefinitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}

func (h saveDefinitionHostAdapter) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return portableconfig.Prepare(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h saveDefinitionHostAdapter) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorysnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}

func (h saveDefinitionHostAdapter) CurrentRuntimeConfig() loadedFactorySource {
	return nil
}

func (h saveDefinitionHostAdapter) WorkflowID() string { return "" }

func (h saveDefinitionHostAdapter) RequireSession(sessionID string) (*interfaces.DefinitionSession, error) {
	return h.saveHost.RequireSession(sessionID)
}

func (h saveDefinitionHostAdapter) SessionRuntimeConfig(sessionID string) (loadedFactorySource, error) {
	return h.saveHost.SessionRuntimeConfig(sessionID)
}

func (h saveDefinitionHostAdapter) SessionFactoryPersistRoot(session *interfaces.DefinitionSession) string {
	return factorydefinition.SessionFactoryPersistRoot(h.rootDir, session)
}

func (h saveDefinitionHostAdapter) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *interfaces.FactorySnapshot) error {
	return validateEditableFactorySnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (h saveDefinitionHostAdapter) GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*interfaces.FactorySnapshot, error) {
	current, err := h.saveHost.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return mustFactorySnapshot(current), nil
}

func (h saveDefinitionHostAdapter) WithActivationLock(fn func() error) error {
	return h.saveHost.WithActivationLock(fn)
}

func (h saveDefinitionHostAdapter) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	return h.saveHost.RequireIdleRuntimeForSession(ctx, sessionID)
}

func (h saveDefinitionHostAdapter) ActivateSessionEditableFactory(
	ctx context.Context,
	session *interfaces.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	return h.saveHost.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, factoryapi.FactoryName(name), runtimeName)
}

func (h saveDefinitionHostAdapter) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*interfaces.FactorySplitLayoutReplaceResult, error) {
	return h.saveHost.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}

func (h saveDefinitionHostAdapter) SaveNow() time.Time {
	return time.Now().UTC()
}

func (h saveDefinitionHostAdapter) RunSessionID() string { return "" }

func (h saveDefinitionHostAdapter) SessionForActivation(string) *interfaces.DefinitionSession {
	return nil
}

func (h saveDefinitionHostAdapter) NamedFactoryActivationPaths(*interfaces.DefinitionSession) (string, string) {
	return h.rootDir, h.rootDir
}

func (h saveDefinitionHostAdapter) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *interfaces.DefinitionSession) error {
	return nil
}

func (h saveDefinitionHostAdapter) SwapPersistedNamedFactoryRuntime(context.Context, string, *interfaces.DefinitionSession, string, string, string, string) error {
	return nil
}
