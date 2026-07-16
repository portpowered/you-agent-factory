// backendsizecheck:ignore-file definition and save host glue remains with runtime host until dedicated definition seams split.
package runtimehost

import (
	"context"
	"fmt"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// FactoryDefinitionService owns current-factory read models for the phase-one
// compatibility facade.
type FactoryDefinitionService interface {
	GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error)
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
}

type factoryDefinitionAPI struct {
	definitions FactoryDefinitionService
	saver       FactorySaveSaver
}

var _ apisurface.FactorySaveAPI = factoryDefinitionAPI{}

// FactoryDefinitionAPI returns the bounded canonical definition collaborator
// used by the composed HTTP surface and the Host compatibility facade.
func (h *Host) FactoryDefinitionAPI() apisurface.FactorySaveAPI {
	if h == nil {
		return factoryDefinitionAPI{}
	}
	return factoryDefinitionAPI{definitions: h.requireDefinitions(), saver: h.factorySave}
}

func (api factoryDefinitionAPI) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if api.definitions == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	return api.definitions.GetCurrentFactoryForSession(ctx, sessionID)
}

func (api factoryDefinitionAPI) SaveFactoryForSession(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	if api.saver == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	return api.saver.Save(ctx, sessionID, mode, request)
}

func (api factoryDefinitionAPI) SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error) {
	return api.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (h *Host) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return h.requireCoordinator().GetCurrentFactory(ctx)
}

func (h *Host) buildSessionEditableFactoryReplacement(
	ctx context.Context,
	sessionRootDir string,
	factoryDir string,
	sessionID string,
	name factoryapi.FactoryName,
) (*factoryRuntimeBundle, error) {
	replacement, err := h.buildReplacementFactoryRuntime(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return replacement, nil
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (h *Host) GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error) {
	return h.requireDefinitions().GetCurrentNamedFactory(ctx)
}

type factoryDefinitionHost struct {
	*Host
}

var _ factorydefinition.Host = factoryDefinitionHost{}

func (dh factoryDefinitionHost) PersistRootDir() string {
	if dh.Host == nil {
		return ""
	}
	rootDir := dh.Host.factoryRootDir
	if rootDir == "" && dh.Host.cfg != nil {
		rootDir = dh.Host.cfg.Dir
	}
	return rootDir
}

func (dh factoryDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	if dh.Host == nil || dh.Host.cfg == nil {
		return nil
	}
	return dh.Host.cfg.WorkstationLoader
}

func (dh factoryDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if dh.Host == nil {
		return nil
	}
	return dh.Host.currentRuntimeConfig()
}

func (dh factoryDefinitionHost) WorkflowID() string {
	if dh.Host == nil {
		return ""
	}
	return dh.Host.workflowID()
}

func (dh factoryDefinitionHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if dh.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return dh.Host.requireSession(sessionID)
}

func (dh factoryDefinitionHost) SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	if dh.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return dh.Host.sessionRuntimeConfig(sessionID)
}

func (dh factoryDefinitionHost) SessionFactoryPersistRoot(session *factorysessions.LiveSession) string {
	if dh.Host == nil {
		return ""
	}
	return sessionFactoryPersistRoot(dh.Host.factoryRootDir, session)
}

func (dh factoryDefinitionHost) ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	return validationentry.ValidateEditableFactorySnapshot(snapshot, dh.WorkstationLoader())
}

func (dh factoryDefinitionHost) GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*interfaces.FactorySnapshot, error) {
	if dh.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	current, err := dh.Host.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := interfaces.NewFactorySnapshot(current)
	if err != nil {
		return nil, fmt.Errorf("capture current factory snapshot: %w", err)
	}
	return snapshot, nil
}

func (dh factoryDefinitionHost) WithActivationLock(fn func() error) error {
	if dh.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return dh.Host.withActivationLock(fn)
}

func (dh factoryDefinitionHost) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	if dh.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return dh.Host.requireIdleRuntimeForSession(ctx, sessionID)
}

func (dh factoryDefinitionHost) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	if dh.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return dh.Host.activateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, factoryapi.FactoryName(name), runtimeName)
}

func (dh factoryDefinitionHost) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	if dh.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return dh.Host.replaceFactoryLayoutAtDir(targetDir, prepared)
}

func (dh factoryDefinitionHost) SaveNow() time.Time {
	if dh.Host == nil || dh.Host.clock == nil {
		return time.Now().UTC()
	}
	return dh.Host.clock.Now().UTC()
}

func (dh factoryDefinitionHost) RunSessionID() string {
	if dh.Host == nil {
		return ""
	}
	return dh.Host.runSessionID()
}

func (dh factoryDefinitionHost) SessionForActivation(sessionID string) *factorysessions.LiveSession {
	if dh.Host == nil {
		return nil
	}
	return dh.Host.sessionByID(sessionID)
}

func (dh factoryDefinitionHost) NamedFactoryActivationPaths(session *factorysessions.LiveSession) (persistRoot, folderPath string) {
	if dh.Host == nil {
		return "", ""
	}
	return dh.Host.namedFactoryActivationPaths(session)
}

func (dh factoryDefinitionHost) RequireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
) error {
	if dh.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return dh.Host.requireIdleBeforeNamedFactoryActivation(ctx, sessionID, session)
}

func (dh factoryDefinitionHost) SwapPersistedNamedFactoryRuntime(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	persistRoot string,
	folderPath string,
	factoryDir string,
	name string,
) error {
	if dh.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	replacement, err := dh.Host.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return dh.Host.applyNamedFactoryReplacement(ctx, sessionID, session, persistRoot, name, replacement)
}

var _ FactoryDefinitionService = (*factorydefinitionmapping.Service)(nil)

func newFactoryDefinitionService(h *Host) FactoryDefinitionService {
	return factorydefinitionmapping.New(factorydefinition.New(factoryDefinitionHost{Host: h}))
}

func (h *Host) requireDefinitions() FactoryDefinitionService {
	if h == nil {
		return factorydefinitionmapping.New(factorydefinition.New(factoryDefinitionHost{}))
	}
	if h.definitions == nil {
		h.definitions = newFactoryDefinitionService(h)
	}
	return h.definitions
}

func (h *Host) definitionService() *factorydefinitionmapping.Service {
	if svc, ok := h.requireDefinitions().(*factorydefinitionmapping.Service); ok {
		return svc
	}
	return nil
}

// FactorySaveSaver is the injectable factory-save collaborator seam.
type FactorySaveSaver interface {
	Save(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
}

var _ FactorySaveSaver = (*factorysave.Service)(nil)

// SaveFactoryForSession is the single orchestrated pipeline for session-scoped
// factory submission. It delegates to the factorysave collaborator.
func (h *Host) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return h.FactoryDefinitionAPI().SaveFactoryForSession(ctx, sessionID, mode, request)
}

// SaveCurrentFactoryForSession replaces the current factory definition for one
// live session using REPLACE_CURRENT semantics.
func (h *Host) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return h.FactoryDefinitionAPI().SaveCurrentFactoryForSession(ctx, sessionID, request)
}

func sessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	return factorydefinition.SessionFactoryPersistRoot(serviceRootDir, session)
}

type factorySaveHost struct {
	*Host
}

var _ factorysave.Host = factorySaveHost{}

func (sh factorySaveHost) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	svc := sh.Host.definitionService()
	if svc == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return svc.Save(ctx, sessionID, mode, request)
}

func newFactorySaveService(h *Host) *factorysave.Service {
	return factorysave.New(factorySaveHost{Host: h})
}

func (h *Host) withActivationLock(fn func() error) error {
	h.activationMu.Lock()
	defer h.activationMu.Unlock()
	return fn()
}

func (h *Host) activateSessionEditableFactory(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	replacement, err := h.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, name)
	if err != nil {
		return err
	}
	if err := h.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return err
	}
	return h.ReplaceSessionRuntime(ctx, session, runtimeName, replacement)
}

func (h *Host) replaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return configpersist.ReplaceFactoryLayoutAtDirWithPreparedWithResult(
		targetDir,
		prepared,
		configpersist.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
}
func wireFactorySaveCollaborator(h *Host, cfg *Config) FactorySaveSaver {
	if cfg != nil && cfg.FactorySave != nil {
		return cfg.FactorySave
	}
	return newFactorySaveService(h)
}

func ProvideFactorySaveCollaborator(shell HostShell, cfg *Config) FactorySaveSaver {
	return wireFactorySaveCollaborator(shell.Host, cfg)
}

func AttachFactorySaveCollaborator(shell HostShell, factorySave FactorySaveSaver) *Host {
	if shell.Host != nil {
		shell.Host.factorySave = factorySave
	}
	return shell.Host
}
