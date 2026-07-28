package factorydefinition_test

import (
	"context"
	"time"

	apitypes "github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
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
	return authoringlayoutprepare.FactoryLayout(
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
	return snapshotsportabilityprepare.PrepareConfig(
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
func (stubDefinitionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (h stubDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return validateEditableFactorySnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (stubDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (stubDefinitionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func mustFactorySnapshot(factory factoryapi.Factory) *factorydefinitions.FactorySnapshot {
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
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
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		return nil, err
	}
	return testDefinitionService.PreparePersistedFactoryPayload(segment, snapshot, *testFactoryVersionFromAPI(&version))
}

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		return nil, err
	}
	return testDefinitionService.PrepareEditableFactoryPersistView(segment, snapshot)
}

func testFactoryVersionFromAPI(version *factoryapi.HybridLogicalTimestamp) *factorydefinitions.FactoryVersion {
	if version == nil {
		return nil
	}
	return &factorydefinitions.FactoryVersion{Logical: version.Logical.Int64(), Physical: version.Physical.UTC()}
}

type saveDefinitionHostAdapter struct {
	saveHost definitionSaveHost
	rootDir  string
}

type definitionSaveHost interface {
	RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error)
	SessionRuntimeConfig(sessionID string) (loadedFactorySource, error)
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	ActivateSessionEditableFactory(
		ctx context.Context,
		session *factorydefinitions.DefinitionSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name factoryapi.FactoryName,
		runtimeName string,
	) error
	ReplaceFactoryLayoutAtDir(
		targetDir string,
		prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	) (*factorydefinitions.FactorySplitLayoutReplaceResult, error)
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
	return factorydefinitionmapping.New(factorydefinition.New(adapter, saveDefinitionActivationGateway{saveHost: saveHost})).Save(ctx, sessionID, mode, request)
}

type saveDefinitionActivationGateway struct {
	saveHost definitionSaveHost
}

func (g saveDefinitionActivationGateway) RunSessionID() string { return "" }

func (g saveDefinitionActivationGateway) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}

func (g saveDefinitionActivationGateway) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return g.saveHost.RequireSession(sessionID)
}

func (g saveDefinitionActivationGateway) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}

func (g saveDefinitionActivationGateway) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}

func (g saveDefinitionActivationGateway) SaveNow() time.Time { return time.Time{} }

func (g saveDefinitionActivationGateway) WithActivationLock(fn func() error) error {
	return g.saveHost.WithActivationLock(fn)
}

func (g saveDefinitionActivationGateway) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	return g.saveHost.RequireIdleRuntimeForSession(ctx, sessionID)
}

func (g saveDefinitionActivationGateway) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return nil
}

func (g saveDefinitionActivationGateway) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	return g.saveHost.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, factoryapi.FactoryName(name), runtimeName)
}

func (g saveDefinitionActivationGateway) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error {
	return nil
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
	return snapshotsportabilityprepare.PrepareConfig(
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

func (h saveDefinitionHostAdapter) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return h.saveHost.RequireSession(sessionID)
}

func (h saveDefinitionHostAdapter) SessionRuntimeConfig(sessionID string) (loadedFactorySource, error) {
	return h.saveHost.SessionRuntimeConfig(sessionID)
}

func (h saveDefinitionHostAdapter) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	return factorydefinition.SessionFactoryPersistRoot(h.rootDir, session)
}

func (h saveDefinitionHostAdapter) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return validateEditableFactorySnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (h saveDefinitionHostAdapter) GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*factorydefinitions.FactorySnapshot, error) {
	current, err := h.saveHost.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return mustFactorySnapshot(current), nil
}

func (h saveDefinitionHostAdapter) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return h.saveHost.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}
