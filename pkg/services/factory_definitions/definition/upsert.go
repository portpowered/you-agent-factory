package factorydefinition

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// SaveUpsertNamedSnapshotAndActivateForSession persists one named Factory
// definition for a live session and activates it as the session current Factory.
func (s *Service) SaveUpsertNamedSnapshotAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request EditableFactory,
) (EditableFactory, error) {
	if s == nil || s.host == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	if request.Snapshot == nil {
		return EditableFactory{}, fmt.Errorf("editable factory snapshot is required")
	}
	if err := s.ValidateUpsertNamedFactoryRequest(ctx, request.Name, request.Snapshot); err != nil {
		return EditableFactory{}, err
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	sessionRootDir := s.host.SessionFactoryPersistRoot(session)

	replaceExisting, err := namedFactoryExistsAtSessionRoot(s.host, sessionRootDir, request.Name)
	if err != nil {
		return EditableFactory{}, err
	}

	var saved EditableFactory
	err = s.host.WithActivationLock(func() error {
		if err := s.host.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}

		currentVersion, err := s.upsertCurrentVersionAtSessionRoot(sessionRootDir, request, replaceExisting)
		if err != nil {
			return err
		}

		nextVersion := s.NextEditableFactoryVersion(currentVersion, s.host.SaveNow())
		prepared, err := s.PreparePersistedFactoryPayload(request.Name, request.Snapshot, nextVersion)
		if err != nil {
			return err
		}
		factoryDir, err := persistUpsertNamedFactoryPrepared(
			s.host,
			sessionRootDir,
			request,
			prepared,
			replaceExisting,
		)
		if err != nil {
			return err
		}

		var finalizeErr error
		saved, finalizeErr = s.finalizeUpsertNamedAndActivateForSession(
			ctx,
			session,
			sessionID,
			sessionRootDir,
			factoryDir,
			request,
		)
		return finalizeErr
	})
	if err != nil {
		return EditableFactory{}, err
	}
	return saved, nil
}

func (s *Service) upsertCurrentVersionAtSessionRoot(
	sessionRootDir string,
	request EditableFactory,
	replaceExisting bool,
) (*factorydefinitions.FactoryVersion, error) {
	if !replaceExisting {
		return nil, nil
	}
	version, err := s.currentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return nil, err
	}
	if err := s.RequireFreshEditableFactoryVersion(request.Version, version); err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *Service) finalizeUpsertNamedAndActivateForSession(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	request EditableFactory,
) (EditableFactory, error) {
	if err := writeCurrentFactoryPointerFromHost(s.host, sessionRootDir, request.Name); err != nil {
		return EditableFactory{}, fmt.Errorf("write session current factory pointer: %w", err)
	}

	if err := s.host.ActivateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		request.Name,
		request.Name,
	); err != nil {
		return EditableFactory{}, err
	}

	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	snapshot, err := s.SerializeNamedFactoryUpsertResponse(request.Name, runtimeCfg)
	if err != nil {
		return EditableFactory{}, err
	}
	version, err := s.currentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{Name: request.Name, Snapshot: snapshot, Version: &version}, nil
}

func namedFactoryExistsAtSessionRoot(
	host Host,
	sessionRootDir string,
	name string,
) (bool, error) {
	_, err := resolveExistingFactoryDirFromHost(host, sessionRootDir, string(name))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		return false, nil
	}
	return false, err
}

func persistUpsertNamedFactoryPrepared(
	host Host,
	sessionRootDir string,
	request EditableFactory,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	replaceExisting bool,
) (string, error) {
	if replaceExisting {
		targetDir, err := resolveExistingFactoryDirFromHost(host, sessionRootDir, request.Name)
		if err != nil {
			return "", mapUpsertNamedFactoryPersistError(err)
		}
		if _, err := host.ReplaceFactoryLayoutAtDir(targetDir, prepared); err != nil {
			return "", mapUpsertNamedFactoryPersistError(err)
		}
		return targetDir, nil
	}
	factoryDir, err := persistNamedFactoryWithPreparedFromHost(host, sessionRootDir, request.Name, prepared)
	if err != nil {
		return "", mapUpsertNamedFactoryPersistError(err)
	}
	return factoryDir, nil
}

func mapUpsertNamedFactoryPersistError(err error) error {
	switch {
	case errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists):
		return factorydefinitions.ErrNamedFactoryAlreadyExists
	case errors.Is(err, factorydefinitions.ErrInvalidNamedFactory):
		return fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactory, err)
	default:
		return err
	}
}
