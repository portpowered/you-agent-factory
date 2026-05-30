package factorysave

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

func (s *Service) saveReplaceCurrentForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	current, err := s.host.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir, sanitized, err := s.prepareEditableFactoryDefinitionSave(
		factorysessions.SessionFactoryRootDir(s.factoryRootDir, session),
		current,
		request,
	)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return s.saveDefaultCurrentFactoryForSession(ctx, sessionID, session, sessionRootDir, current, request, sanitized)
	}

	var saved factoryapi.Factory
	err = s.host.WithActivationLock(func() error {
		if err := s.host.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}
		if err := requireFreshEditableFactoryVersionAtRoot(s.host, request.Version, sessionRootDir, current.Name); err != nil {
			return err
		}
		nextVersion := nextEditableFactoryVersion(current.Version, s.now().Now().UTC())
		payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
		if err != nil {
			return err
		}

		factoryDir, err := replaceEditableFactoryDefinition(sessionRootDir, request.Name, payload)
		if err != nil {
			return err
		}
		if err := s.host.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, request.Name, string(request.Name)); err != nil {
			return err
		}
		var readbackErr error
		saved, readbackErr = s.host.GetCurrentFactoryForSession(ctx, sessionID)
		return readbackErr
	})
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return saved, nil
}

func (s *Service) prepareEditableFactoryDefinitionSave(
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
) (string, factoryapi.Factory, error) {
	if request.Name != current.Name {
		return "", factoryapi.Factory{}, fmt.Errorf("%w: editable save must preserve current factory name %q", apisurface.ErrInvalidNamedFactoryName, current.Name)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
			return "", factoryapi.Factory{}, err
		}
	}
	sanitized := request
	sanitized.Version = nil
	if err := validateEditableFactoryTopology(sanitized, s.workstationLoader()); err != nil {
		return "", factoryapi.Factory{}, err
	}
	return sessionRootDir, sanitized, nil
}

func (s *Service) saveDefaultCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
	sanitized factoryapi.Factory,
) (factoryapi.Factory, error) {
	var saved factoryapi.Factory
	err := s.host.WithActivationLock(func() error {
		if err := s.host.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}
		if err := requireFreshEditableFactoryVersionAtRoot(s.host, request.Version, sessionRootDir, current.Name); err != nil {
			return err
		}
		nextVersion := nextEditableFactoryVersion(current.Version, s.now().Now().UTC())
		payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
		if err != nil {
			return err
		}
		restore, err := s.host.ReplaceDefaultFactoryDefinition(sessionRootDir, payload)
		if err != nil {
			return err
		}

		if err := s.host.ActivateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, sessionRootDir, current.Name, string(current.Name)); err != nil {
			restore()
			return err
		}

		var readbackErr error
		saved, readbackErr = s.host.GetCurrentFactoryForSession(ctx, sessionID)
		return readbackErr
	})
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return saved, nil
}
