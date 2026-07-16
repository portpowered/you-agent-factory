package factorysave

import (
	"context"
	"time"

	apitypes "github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

var testDefinitionService = factorydefinition.New(stubDefinitionHost{})

type stubDefinitionHost struct{}

func (stubDefinitionHost) PersistRootDir() string { return "" }
func (stubDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	return nil
}
func (stubDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return nil
}
func (stubDefinitionHost) WorkflowID() string { return "" }
func (stubDefinitionHost) RequireSession(string) (*factorysessions.LiveSession, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionFactoryPersistRoot(*factorysessions.LiveSession) string {
	return ""
}
func (h stubDefinitionHost) ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	return validationentry.ValidateEditableFactorySnapshot(snapshot, h.WorkstationLoader())
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

func (stubDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, string, string) error {
	return nil
}

func (stubDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (stubDefinitionHost) SaveNow() time.Time {
	return time.Time{}
}

func (stubDefinitionHost) RunSessionID() string { return "" }

func (stubDefinitionHost) SessionForActivation(string) *factorysessions.LiveSession {
	return nil
}

func (stubDefinitionHost) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return "", ""
}

func (stubDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorysessions.LiveSession) error {
	return nil
}

func (stubDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorysessions.LiveSession, string, string, string, string) error {
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
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		return nil, err
	}
	return testDefinitionService.PreparePersistedFactoryPayload(segment, snapshot, *testFactoryVersionFromAPI(&version))
}

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
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
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error)
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	ActivateSessionEditableFactory(
		ctx context.Context,
		session *factorysessions.LiveSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name factoryapi.FactoryName,
		runtimeName string,
	) error
	ReplaceFactoryLayoutAtDir(
		targetDir string,
		prepared *factoryconfig.PreparedFactoryLayoutPayload,
	) (*factoryconfig.FactorySplitLayoutReplaceResult, error)
}

func saveFactoryThroughDefinition(
	rootDir string,
	saveHost definitionSaveHost,
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return factorydefinitionmapping.New(factorydefinition.New(saveDefinitionHostAdapter{
		saveHost: saveHost,
		rootDir:  rootDir,
	})).Save(ctx, sessionID, mode, request)
}

func (h saveDefinitionHostAdapter) PersistRootDir() string { return h.rootDir }

func (h saveDefinitionHostAdapter) WorkstationLoader() factoryconfig.WorkstationLoader {
	return nil
}

func (h saveDefinitionHostAdapter) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return nil
}

func (h saveDefinitionHostAdapter) WorkflowID() string { return "" }

func (h saveDefinitionHostAdapter) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return h.saveHost.RequireSession(sessionID)
}

func (h saveDefinitionHostAdapter) SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	return h.saveHost.SessionRuntimeConfig(sessionID)
}

func (h saveDefinitionHostAdapter) SessionFactoryPersistRoot(session *factorysessions.LiveSession) string {
	return factorydefinition.SessionFactoryPersistRoot(h.rootDir, session)
}

func (h saveDefinitionHostAdapter) ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	return validationentry.ValidateEditableFactorySnapshot(snapshot, h.WorkstationLoader())
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
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	return h.saveHost.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, factoryapi.FactoryName(name), runtimeName)
}

func (h saveDefinitionHostAdapter) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return h.saveHost.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}

func (h saveDefinitionHostAdapter) SaveNow() time.Time {
	return time.Now().UTC()
}

func (h saveDefinitionHostAdapter) RunSessionID() string { return "" }

func (h saveDefinitionHostAdapter) SessionForActivation(string) *factorysessions.LiveSession {
	return nil
}

func (h saveDefinitionHostAdapter) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return h.rootDir, h.rootDir
}

func (h saveDefinitionHostAdapter) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorysessions.LiveSession) error {
	return nil
}

func (h saveDefinitionHostAdapter) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorysessions.LiveSession, string, string, string, string) error {
	return nil
}
