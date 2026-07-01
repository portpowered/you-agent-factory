package factorysave

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
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

func (stubDefinitionHost) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (stubDefinitionHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (stubDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (stubDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, factoryapi.FactoryName, string) error {
	return nil
}

func (stubDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (stubDefinitionHost) SaveNow() time.Time {
	return time.Time{}
}

func requireFreshEditableFactoryVersion(
	baseVersion *factoryapi.HybridLogicalTimestamp,
	currentVersion factoryapi.HybridLogicalTimestamp,
) error {
	return testDefinitionService.RequireFreshEditableFactoryVersion(baseVersion, currentVersion)
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	return testDefinitionService.NextEditableFactoryVersion(current, now)
}

func preparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	return testDefinitionService.PreparePersistedFactoryPayload(segment, factory, version)
}

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	return testDefinitionService.PrepareEditableFactoryPersistView(segment, factory)
}

type saveDefinitionHostAdapter struct {
	saveHost Host
	rootDir  string
}

func saveReplaceCurrentThroughDefinition(
	rootDir string,
	saveHost Host,
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return factorydefinition.New(saveDefinitionHostAdapter{
		saveHost: saveHost,
		rootDir:  rootDir,
	}).SaveReplaceCurrentForSession(ctx, sessionID, request)
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
	return SessionFactoryPersistRoot(h.rootDir, session)
}

func (h saveDefinitionHostAdapter) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return h.saveHost.GetCurrentFactoryForSession(ctx, sessionID)
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
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	return h.saveHost.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (h saveDefinitionHostAdapter) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return h.saveHost.ReplaceFactoryLayoutAtDir(targetDir, prepared)
}

func (h saveDefinitionHostAdapter) SaveNow() time.Time {
	return time.Now().UTC()
}
