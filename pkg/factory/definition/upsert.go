package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"os"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// SaveUpsertNamedAndActivateForSession persists one named factory definition
// for a live session and activates it as the session current factory.
func (s *Service) SaveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	if err := s.ValidateUpsertNamedFactoryRequest(request); err != nil {
		return factoryapi.Factory{}, err
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir := s.host.SessionFactoryPersistRoot(session)

	replaceExisting, err := namedFactoryExistsAtSessionRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	var saved factoryapi.Factory
	err = s.host.WithActivationLock(func() error {
		if err := s.host.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}

		currentVersion, err := s.upsertCurrentVersionAtSessionRoot(sessionRootDir, request, replaceExisting)
		if err != nil {
			return err
		}

		nextVersion := s.NextEditableFactoryVersion(currentVersion, s.host.SaveNow())
		snapshot, err := interfaces.NewFactorySnapshot(request)
		if err != nil {
			return fmt.Errorf("capture editable factory snapshot: %w", err)
		}
		prepared, err := s.PreparePersistedFactoryPayload(string(request.Name), snapshot, nextVersion)
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
		return factoryapi.Factory{}, err
	}
	return saved, nil
}

func (s *Service) upsertCurrentVersionAtSessionRoot(
	sessionRootDir string,
	request factoryapi.Factory,
	replaceExisting bool,
) (*interfaces.FactoryVersion, error) {
	if !replaceExisting {
		return nil, nil
	}
	version, err := s.CurrentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return nil, err
	}
	domainVersion := factoryVersionFromAPIValue(version)
	if err := s.RequireFreshEditableFactoryVersion(factoryVersionFromAPI(request.Version), domainVersion); err != nil {
		return nil, err
	}
	return &domainVersion, nil
}

func (s *Service) finalizeUpsertNamedAndActivateForSession(
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

	if err := s.host.ActivateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		string(request.Name),
		string(request.Name),
	); err != nil {
		return factoryapi.Factory{}, err
	}

	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	snapshot, err := s.SerializeNamedFactoryUpsertResponse(string(request.Name), runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map upsert factory snapshot: %w", err)
	}
	version, err := s.CurrentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return *serialized, nil
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

func persistUpsertNamedFactoryPrepared(
	host Host,
	sessionRootDir string,
	request factoryapi.Factory,
	prepared *factoryconfig.PreparedFactoryLayoutPayload,
	replaceExisting bool,
) (string, error) {
	if replaceExisting {
		targetDir, err := factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(request.Name))
		if err != nil {
			return "", mapUpsertNamedFactoryPersistError(err)
		}
		if _, err := host.ReplaceFactoryLayoutAtDir(targetDir, prepared); err != nil {
			return "", mapUpsertNamedFactoryPersistError(err)
		}
		return targetDir, nil
	}
	factoryDir, err := configpersist.PersistNamedFactoryWithPrepared(sessionRootDir, string(request.Name), prepared)
	if err != nil {
		return "", mapUpsertNamedFactoryPersistError(err)
	}
	return factoryDir, nil
}

func mapUpsertNamedFactoryPersistError(err error) error {
	switch {
	case configpersist.IsNamedFactoryAlreadyExists(err):
		return configpersist.ErrNamedFactoryAlreadyExists
	case configpersist.IsInvalidNamedFactory(err):
		return fmt.Errorf("%w: %w", apisurface.ErrInvalidNamedFactory, err)
	default:
		return err
	}
}

func isNamedFactoryResolveNotFound(err error) bool {
	for err != nil {
		if factoryconfig.IsNamedFactoryNotFound(err) {
			return true
		}
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
