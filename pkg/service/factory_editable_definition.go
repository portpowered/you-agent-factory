package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (fs *FactoryService) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.GetCurrentNamedFactory(ctx)
}

func (fs *FactoryService) prepareEditableFactoryDefinitionSave(
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
) (string, factoryapi.Factory, error) {
	if request.Name != current.Name {
		return "", factoryapi.Factory{}, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
			return "", factoryapi.Factory{}, err
		}
	}
	sanitized := request
	sanitized.Version = nil
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	if err := validateEditableFactoryTopology(sanitized, workstationLoader); err != nil {
		return "", factoryapi.Factory{}, err
	}
	return sessionRootDir, sanitized, nil
}

func (fs *FactoryService) saveDefaultCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	session *liveFactorySession,
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
	sanitized factoryapi.Factory,
) (factoryapi.Factory, error) {
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	nextVersion := nextEditableFactoryVersion(current.Version, factory.EnsureClock(fs.clock).Now().UTC())
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	restore, err := configpersist.ReplaceDefaultFactoryDefinition(sessionRootDir, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, sessionRootDir, sessionID, current.Name)
	if err != nil {
		restore()
		return factoryapi.Factory{}, err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(current.Name), replacement); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *FactoryService) replaceEditableFactoryDefinition(
	sessionRootDir string,
	name factoryapi.FactoryName,
	payload []byte,
) (string, error) {
	factoryDir, err := configpersist.ReplaceNamedFactory(sessionRootDir, string(name), payload)
	if err == nil {
		return factoryDir, nil
	}
	if configpersist.IsInvalidNamedFactory(err) {
		return "", fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	}
	return "", err
}

func (fs *FactoryService) buildSessionEditableFactoryReplacement(
	ctx context.Context,
	sessionRootDir string,
	factoryDir string,
	sessionID string,
	name factoryapi.FactoryName,
) (*factoryRuntimeBundle, error) {
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return replacement, nil
}

func (fs *FactoryService) requireFreshEditableFactoryVersionAtRoot(baseVersion *factoryapi.HybridLogicalTimestamp, rootDir string, name factoryapi.FactoryName) error {
	if baseVersion == nil {
		return fmt.Errorf("%w: save request must include an advanced factory version", apisurface.ErrFactoryVersionStale)
	}
	currentVersion, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	if !isEditableFactoryVersionAdvanced(*baseVersion, currentVersion) {
		return fmt.Errorf("%w: submitted version logical=%d physical=%s must advance current logical=%d physical=%s",
			apisurface.ErrFactoryVersionStale,
			baseVersion.Logical,
			baseVersion.Physical.UTC().Format(time.RFC3339Nano),
			currentVersion.Logical,
			currentVersion.Physical.UTC().Format(time.RFC3339Nano),
		)
	}
	return nil
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical.Int64() + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: physical,
	}
}

func marshalPersistedFactoryPayload(
	sanitized factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) ([]byte, error) {
	persisted := sanitized
	persisted.Version = &version
	payload, err := json.Marshal(persisted)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return payload, nil
}

func isEditableFactoryVersionAdvanced(candidate, current factoryapi.HybridLogicalTimestamp) bool {
	return candidate.Logical > current.Logical && candidate.Physical.UTC().After(current.Physical.UTC())
}

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factoryconfig.WorkstationLoader) error {
	payload, err := json.Marshal(submitted)
	if err != nil {
		return fmt.Errorf("marshal editable factory payload: %w", err)
	}
	_, loadErr := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{
		WorkstationLoader: workstationLoader,
	})
	cfg, mapErr := factoryconfig.FactoryConfigFromOpenAPI(submitted)
	if mapErr != nil {
		return fmt.Errorf("%w: %v", ErrInvalidNamedFactory, mapErr)
	}
	if loadErr != nil {
		if configload.IsInvalidNamedFactory(loadErr) {
			blocking := factoryvalidation.ValidateBlockingLoad(&cfg)
			if len(blocking.Targets) > 0 {
				return topologyValidationErrorFromTargets(blocking.Targets)
			}
		}
		return fmt.Errorf("%w: %v", ErrInvalidNamedFactory, loadErr)
	}
	result := factoryvalidation.Validate(&cfg)
	if len(result.Targets) == 0 {
		return nil
	}
	return topologyValidationErrorFromTargets(result.Targets)
}

func topologyValidationErrorFromTargets(targets []factoryvalidation.Target) *apisurface.TopologyValidationError {
	return apisurface.NewTopologyValidationError(
		"Factory topology contains invalid graph references.",
		factoryvalidation.ToValidationTargets(targets),
	)
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (fs *FactoryService) GetCurrentNamedFactory(_ context.Context) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	name, err := configpersist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := fs.currentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return fs.serializeNamedFactory(apisurface.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return factoryapi.Factory{}, ErrCurrentFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current factory %q: %w", name, err)
	}

	return fs.serializeNamedFactory(factoryapi.FactoryName(name), current, true)
}

func (fs *FactoryService) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return factoryapi.HybridLogicalTimestamp{
			Logical:  apitypes.Int64String(version.Logical),
			Physical: version.Physical.UTC(),
		}, nil
	}

	info, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: modified,
	}, nil
}

func (fs *FactoryService) withCurrentFactoryVersion(
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

func (fs *FactoryService) serializeNamedFactory(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize current factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

// serializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (fs *FactoryService) serializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, false); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("merge named factory portable bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize upsert factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

func sameFactoryDir(left, right string) bool {
	return factorysessions.SameFactoryDir(left, right)
}

// SaveFactoryForSession is the single orchestrated pipeline for session-scoped
// factory submission. It resolves session scope, validates the payload, persists
// under the session factory root, activates via replaceSessionRuntime, and
// returns the saved factory readback.
func (fs *FactoryService) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	switch mode {
	case factoryapi.FactorySaveModeUpsertNamedAndActivate:
		return fs.saveUpsertNamedAndActivateForSession(ctx, sessionID, request)
	default:
		return fs.saveReplaceCurrentForSession(ctx, sessionID, request)
	}
}

// SaveCurrentFactoryForSession replaces the current factory definition for one
// live session using REPLACE_CURRENT semantics.
func (fs *FactoryService) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return fs.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

func (fs *FactoryService) saveReplaceCurrentForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	current, err := fs.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir, sanitized, err := fs.prepareEditableFactoryDefinitionSave(
		factorysessions.SessionFactoryRootDir(fs.factoryRootDir, session),
		current,
		request,
	)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return fs.saveDefaultCurrentFactoryForSession(ctx, sessionID, session, sessionRootDir, current, request, sanitized)
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	nextVersion := nextEditableFactoryVersion(current.Version, factory.EnsureClock(fs.clock).Now().UTC())
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	factoryDir, err := fs.replaceEditableFactoryDefinition(sessionRootDir, request.Name, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(request.Name), replacement); err != nil {
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *FactoryService) saveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	if err := validateUpsertNamedFactoryRequest(request, workstationLoader); err != nil {
		return factoryapi.Factory{}, err
	}

	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir := sessionFactoryPersistRoot(fs.factoryRootDir, session)

	replaceExisting, err := namedFactoryExistsAtSessionRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}

	currentVersion, err := fs.upsertCurrentVersionAtSessionRoot(sessionRootDir, request, replaceExisting)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	nextVersion := nextEditableFactoryVersion(currentVersion, factory.EnsureClock(fs.clock).Now().UTC())
	factoryDir, err := persistUpsertNamedFactoryPayload(
		sessionRootDir,
		request,
		nextVersion,
		replaceExisting,
	)
	if err != nil {
		return factoryapi.Factory{}, mapUpsertNamedFactoryPersistError(err)
	}

	return fs.finalizeUpsertNamedAndActivateForSession(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		request,
	)
}

func validateUpsertNamedFactoryRequest(
	request factoryapi.Factory,
	workstationLoader factoryconfig.WorkstationLoader,
) error {
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return err
	}
	return validateEditableFactoryTopology(request, workstationLoader)
}

func mapUpsertNamedFactoryPersistError(err error) error {
	switch {
	case configpersist.IsNamedFactoryAlreadyExists(err):
		return configpersist.ErrNamedFactoryAlreadyExists
	case configpersist.IsInvalidNamedFactory(err):
		return fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	default:
		return err
	}
}

func (fs *FactoryService) upsertCurrentVersionAtSessionRoot(
	sessionRootDir string,
	request factoryapi.Factory,
	replaceExisting bool,
) (*factoryapi.HybridLogicalTimestamp, error) {
	if !replaceExisting {
		return nil, nil
	}
	version, err := fs.currentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return nil, err
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, request.Name); err != nil {
		return nil, err
	}
	return &version, nil
}

func (fs *FactoryService) finalizeUpsertNamedAndActivateForSession(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if err := configpersist.WriteCurrentFactoryPointer(sessionRootDir, string(request.Name)); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("write session current factory pointer: %w", err)
	}

	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(request.Name), replacement); err != nil {
		return factoryapi.Factory{}, err
	}

	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized, err := fs.serializeNamedFactoryUpsertResponse(request.Name, runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	version, err := fs.currentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

func namedFactoryExistsAtSessionRoot(
	sessionRootDir string,
	name factoryapi.FactoryName,
) (bool, error) {
	_, err := factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(name))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) || isNamedFactoryResolveNotFound(err) {
		return false, nil
	}
	return false, err
}

func persistUpsertNamedFactoryPayload(
	sessionRootDir string,
	request factoryapi.Factory,
	nextVersion factoryapi.HybridLogicalTimestamp,
	replaceExisting bool,
) (string, error) {
	sanitized := request
	sanitized.Version = nil
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return "", err
	}
	var factoryDir string
	if replaceExisting {
		factoryDir, err = configpersist.ReplaceNamedFactory(sessionRootDir, string(request.Name), payload)
	} else {
		factoryDir, err = configpersist.PersistNamedFactory(sessionRootDir, string(request.Name), payload)
	}
	if err != nil {
		return "", mapUpsertNamedFactoryPersistError(err)
	}
	return factoryDir, nil
}

func sessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return factorysessions.SessionFactoryRootDir(serviceRootDir, session)
}

func isNamedFactoryResolveNotFound(err error) bool {
	for err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
