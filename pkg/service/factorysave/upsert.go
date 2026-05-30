package factorysave

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

func (s *Service) saveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if err := validateUpsertNamedFactoryRequest(request, s.workstationLoader()); err != nil {
		return factoryapi.Factory{}, err
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir := SessionFactoryPersistRoot(s.factoryRootDir, session)

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

		nextVersion := nextEditableFactoryVersion(currentVersion, s.now().Now().UTC())
		factoryDir, err := persistUpsertNamedFactoryPayload(
			sessionRootDir,
			request,
			nextVersion,
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
) (*factoryapi.HybridLogicalTimestamp, error) {
	if !replaceExisting {
		return nil, nil
	}
	version, err := s.host.CurrentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return nil, err
	}
	if err := requireFreshEditableFactoryVersionAtRoot(s.host, request.Version, sessionRootDir, request.Name); err != nil {
		return nil, err
	}
	return &version, nil
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

	if err := s.host.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, request.Name, string(request.Name)); err != nil {
		return factoryapi.Factory{}, err
	}

	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized, err := s.host.SerializeNamedFactoryUpsertResponse(request.Name, runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	version, err := s.host.CurrentFactoryDefinitionVersionAtRoot(sessionRootDir, request.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}
