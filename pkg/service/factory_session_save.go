package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

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
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := validateEditableFactoryTopology(request); err != nil {
		return factoryapi.Factory{}, err
	}

	session, err := fs.requireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir := sessionFactoryPersistRoot(fs.factoryRootDir, session)

	replaceExisting := false
	if _, err := factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(request.Name)); err == nil {
		replaceExisting = true
	} else if !errors.Is(err, os.ErrNotExist) && !isNamedFactoryResolveNotFound(err) {
		return factoryapi.Factory{}, err
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}

	payload, err := fs.marshalUpsertNamedFactoryPayload(sessionRootDir, request, replaceExisting)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	factoryDir, err := fs.persistUpsertNamedFactoryAtSessionRoot(sessionRootDir, request.Name, payload, replaceExisting)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	if err := factoryconfig.WriteCurrentFactoryPointer(sessionRootDir, string(request.Name)); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("write session current factory pointer: %w", err)
	}

	return fs.activateUpsertNamedFactorySave(ctx, sessionID, session, sessionRootDir, factoryDir, request)
}

func (fs *FactoryService) marshalUpsertNamedFactoryPayload(
	sessionRootDir string,
	request factoryapi.Factory,
	replaceExisting bool,
) ([]byte, error) {
	var currentVersion *factoryapi.HybridLogicalTimestamp
	if replaceExisting {
		version, err := fs.currentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
		if err != nil {
			return nil, err
		}
		currentVersion = &version
		if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, request.Name); err != nil {
			return nil, err
		}
	}

	nextVersion := nextEditableFactoryVersion(currentVersion, factory.EnsureClock(fs.clock).Now().UTC())
	sanitized := request
	sanitized.Version = nil
	return marshalPersistedFactoryPayload(sanitized, nextVersion)
}

func (fs *FactoryService) activateUpsertNamedFactorySave(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	sessionRootDir string,
	factoryDir string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
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

func (fs *FactoryService) persistUpsertNamedFactoryAtSessionRoot(
	sessionRootDir string,
	name factoryapi.FactoryName,
	payload []byte,
	replaceExisting bool,
) (string, error) {
	var factoryDir string
	var err error
	if replaceExisting {
		factoryDir, err = factoryconfig.ReplaceNamedFactory(sessionRootDir, string(name), payload)
	} else {
		factoryDir, err = factoryconfig.PersistNamedFactory(sessionRootDir, string(name), payload)
	}
	if err == nil {
		return factoryDir, nil
	}
	switch {
	case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
		return "", factoryconfig.ErrNamedFactoryAlreadyExists
	case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
		return "", fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	default:
		return "", err
	}
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
