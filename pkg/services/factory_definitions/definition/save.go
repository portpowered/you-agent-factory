package factorydefinition

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

// EditableFactory carries one detached Factory definition and the optimistic-
// concurrency version supplied with it. Definition policy consumes this
// domain-owned value; transport adapters are responsible for capturing and
// decoding generated API values.
type EditableFactory = factorydefinitions.EditableFactory

// SaveReplaceCurrentSnapshotForSession replaces the current Factory definition
// for one live session using replace-current semantics.
func (s *Service) SaveReplaceCurrentSnapshotForSession(
	ctx context.Context,
	sessionID string,
	request EditableFactory,
) (*factorydefinitions.FactorySnapshot, error) {
	if s == nil || s.host == nil || s.activationGateway == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	if request.Snapshot == nil {
		return nil, fmt.Errorf("editable factory snapshot is required")
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return nil, err
	}
	currentSnapshot, err := s.host.GetCurrentFactorySnapshotForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	currentName, err := factorySnapshotName(currentSnapshot)
	if err != nil {
		return nil, fmt.Errorf("read current factory snapshot identity: %w", err)
	}
	sessionRootDir := sessionFactoryRootDir(s.host.PersistRootDir(), session)
	sessionRootDir, sanitized, err := s.prepareEditableFactoryDefinitionSave(ctx, sessionRootDir, currentName, request.Snapshot)
	if err != nil {
		return nil, err
	}
	targetDir, activateFactoryDir, err := resolveReplaceCurrentLayoutTarget(s.host, sessionRootDir, currentName)
	if err != nil {
		return nil, err
	}

	return s.replaceCurrentFactoryLayoutLocked(
		ctx,
		sessionID,
		session,
		request,
		sessionRootDir,
		targetDir,
		activateFactoryDir,
		sanitized,
	)
}

func (s *Service) prepareEditableFactoryDefinitionSave(
	ctx context.Context,
	sessionRootDir string,
	currentName string,
	request *factorydefinitions.FactorySnapshot,
) (string, *factorydefinitions.FactorySnapshot, error) {
	if currentName != factorydefinitions.DefaultCurrentFactoryName {
		if err := namedfactorypath.ValidateName(currentName); err != nil {
			return "", nil, fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactoryName, err)
		}
	}
	sanitized, err := request.WithName(currentName)
	if err != nil {
		return "", nil, fmt.Errorf("name editable factory snapshot: %w", err)
	}
	if err := s.ValidateEditableFactoryTopology(ctx, sanitized); err != nil {
		return "", nil, err
	}
	return sessionRootDir, sanitized, nil
}

func (s *Service) replaceCurrentFactoryLayoutLocked(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
	request EditableFactory,
	sessionRootDir string,
	targetDir string,
	activateFactoryDir string,
	sanitized *factorydefinitions.FactorySnapshot,
) (*factorydefinitions.FactorySnapshot, error) {
	currentName, err := factorySnapshotName(sanitized)
	if err != nil {
		return nil, fmt.Errorf("read editable factory snapshot identity: %w", err)
	}
	var saved *factorydefinitions.FactorySnapshot
	err = s.activationGateway.WithActivationLock(func() error {
		if err := s.activationGateway.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}
		currentVersion, err := s.currentFactoryDefinitionVersionAtRoot(sessionRootDir, currentName)
		if err != nil {
			return err
		}
		if err := s.RequireFreshEditableFactoryVersion(request.Version, currentVersion); err != nil {
			return err
		}
		nextVersion := s.NextEditableFactoryVersion(&currentVersion, s.activationGateway.SaveNow())
		prepared, err := s.PreparePersistedFactoryPayload(currentName, sanitized, nextVersion)
		if err != nil {
			return err
		}

		replaceResult, err := s.host.ReplaceFactoryLayoutAtDir(targetDir, prepared)
		if err != nil {
			return err
		}

		if err := s.activationGateway.ActivateSessionEditableFactory(
			ctx,
			session,
			sessionID,
			sessionRootDir,
			activateFactoryDir,
			currentName,
			currentName,
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
		savedSnapshot, readbackErr := s.host.GetCurrentFactorySnapshotForSession(ctx, sessionID)
		if readbackErr != nil {
			return readbackErr
		}
		saved = savedSnapshot.Clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func resolveReplaceCurrentLayoutTarget(
	host Host,
	sessionRootDir string,
	name string,
) (targetDir string, activateFactoryDir string, err error) {
	if name == factorydefinitions.DefaultCurrentFactoryName {
		return sessionRootDir, sessionRootDir, nil
	}
	factoryDir, err := resolveExistingFactoryDirFromHost(host, sessionRootDir, name)
	if err != nil {
		return "", "", err
	}
	return factoryDir, factoryDir, nil
}

func factorySnapshotName(snapshot *factorydefinitions.FactorySnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("factory snapshot is required")
	}
	var identity struct {
		Name string `json:"name"`
	}
	if err := snapshot.Decode(&identity); err != nil {
		return "", err
	}
	return identity.Name, nil
}
