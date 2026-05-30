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

	replaceExisting, err := upsertNamedReplacesExistingAtSessionRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}

	currentVersion, err := fs.currentVersionForUpsertReplace(sessionRootDir, request, replaceExisting)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	nextVersion := nextEditableFactoryVersion(currentVersion, factory.EnsureClock(fs.clock).Now().UTC())
	sanitized := request
	sanitized.Version = nil
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	factoryDir, err := persistUpsertNamedFactoryAtSessionRoot(sessionRootDir, request.Name, payload, replaceExisting)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	if err := factoryconfig.WriteCurrentFactoryPointer(sessionRootDir, string(request.Name)); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("write session current factory pointer: %w", err)
	}

	if err := fs.activateUpsertNamedFactoryForSession(ctx, sessionID, session, sessionRootDir, factoryDir, request.Name); err != nil {
		return factoryapi.Factory{}, err
	}

	return fs.serializeUpsertNamedActivateResponse(sessionRootDir, request.Name, sessionID)
}

func upsertNamedReplacesExistingAtSessionRoot(sessionRootDir string, name factoryapi.FactoryName) (bool, error) {
	if _, err := factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(name)); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) && !isNamedFactoryResolveNotFound(err) {
		return false, err
	}
	return false, nil
}

func (fs *FactoryService) currentVersionForUpsertReplace(
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

func (fs *FactoryService) activateUpsertNamedFactoryForSession(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	sessionRootDir string,
	factoryDir string,
	name factoryapi.FactoryName,
) error {
	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, name)
	if err != nil {
		return err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return err
	}
	return fs.replaceSessionRuntime(ctx, session, string(name), replacement)
}

func (fs *FactoryService) serializeUpsertNamedActivateResponse(
	sessionRootDir string,
	name factoryapi.FactoryName,
	sessionID string,
) (factoryapi.Factory, error) {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized, err := fs.serializeNamedFactoryUpsertResponse(name, runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	version, err := fs.currentFactoryDefinitionVersionAtRoot(sessionRootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

func persistUpsertNamedFactoryAtSessionRoot(
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
	if err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
			return "", factoryconfig.ErrNamedFactoryAlreadyExists
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return "", fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
		default:
			return "", err
		}
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
