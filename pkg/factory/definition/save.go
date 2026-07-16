package factorydefinition

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// SaveReplaceCurrentForSession replaces the current factory definition for one
// live session using REPLACE_CURRENT semantics.
func (s *Service) SaveReplaceCurrentForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	current, err := s.host.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	sessionRootDir := factorysessions.SessionFactoryRootDir(s.host.PersistRootDir(), session)
	sessionRootDir, sanitized, err := s.prepareEditableFactoryDefinitionSave(sessionRootDir, current, request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	targetDir, activateFactoryDir, err := resolveReplaceCurrentLayoutTarget(sessionRootDir, current.Name)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	return s.replaceCurrentFactoryLayoutLocked(
		ctx,
		sessionID,
		session,
		current,
		request,
		sessionRootDir,
		targetDir,
		activateFactoryDir,
		sanitized,
	)
}

func (s *Service) prepareEditableFactoryDefinitionSave(
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
) (string, factoryapi.Factory, error) {
	if current.Name != apisurface.DefaultCurrentFactoryName {
		if err := apisurface.ValidateWritableNamedFactoryName(current.Name); err != nil {
			return "", factoryapi.Factory{}, err
		}
	}
	sanitized := request
	sanitized.Name = current.Name
	sanitized.Version = nil
	if err := s.ValidateEditableFactoryTopology(sanitized); err != nil {
		return "", factoryapi.Factory{}, err
	}
	return sessionRootDir, sanitized, nil
}

func (s *Service) replaceCurrentFactoryLayoutLocked(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	current factoryapi.Factory,
	request factoryapi.Factory,
	sessionRootDir string,
	targetDir string,
	activateFactoryDir string,
	sanitized factoryapi.Factory,
) (factoryapi.Factory, error) {
	var saved factoryapi.Factory
	err := s.host.WithActivationLock(func() error {
		if err := s.host.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}
		currentVersion, err := s.CurrentFactoryDefinitionVersionAtRoot(sessionRootDir, current.Name)
		if err != nil {
			return err
		}
		if err := s.RequireFreshEditableFactoryVersion(factoryVersionFromAPI(request.Version), factoryVersionFromAPIValue(currentVersion)); err != nil {
			return err
		}
		currentDomainVersion := factoryVersionFromAPIValue(currentVersion)
		nextVersion := s.NextEditableFactoryVersion(&currentDomainVersion, s.host.SaveNow())
		snapshot, err := interfaces.NewFactorySnapshot(sanitized)
		if err != nil {
			return fmt.Errorf("capture editable factory snapshot: %w", err)
		}
		prepared, err := s.PreparePersistedFactoryPayload(string(current.Name), snapshot, nextVersion)
		if err != nil {
			return err
		}

		replaceResult, err := s.host.ReplaceFactoryLayoutAtDir(targetDir, prepared)
		if err != nil {
			return err
		}

		if err := s.host.ActivateSessionEditableFactory(
			ctx,
			session,
			sessionID,
			sessionRootDir,
			activateFactoryDir,
			current.Name,
			string(current.Name),
		); err != nil {
			if replaceResult != nil && replaceResult.Restore != nil {
				replaceResult.Restore()
			}
			return err
		}
		if replaceResult != nil && replaceResult.DiscardBackup != nil {
			replaceResult.DiscardBackup()
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

func resolveReplaceCurrentLayoutTarget(
	sessionRootDir string,
	name factoryapi.FactoryName,
) (targetDir string, activateFactoryDir string, err error) {
	if name == apisurface.DefaultCurrentFactoryName {
		return sessionRootDir, sessionRootDir, nil
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(name))
	if err != nil {
		return "", "", err
	}
	return factoryDir, factoryDir, nil
}
