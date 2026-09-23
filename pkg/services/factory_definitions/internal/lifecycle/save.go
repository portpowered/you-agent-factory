package lifecycle

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
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
	saved, err := s.saveReplaceCurrentFactoryForSession(ctx, sessionID, request)
	if err != nil {
		return nil, err
	}
	return saved.Snapshot, nil
}

func (s *Service) saveReplaceCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request EditableFactory,
) (EditableFactory, error) {
	if s == nil || s.host == nil || s.activationGateway == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	if request.Snapshot == nil {
		return EditableFactory{}, fmt.Errorf("editable factory snapshot is required")
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	currentSnapshot, err := s.host.GetCurrentFactorySnapshotForSession(ctx, sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	currentName, err := factorySnapshotName(currentSnapshot)
	if err != nil {
		return EditableFactory{}, fmt.Errorf("read current factory snapshot identity: %w", err)
	}
	sessionRootDir := sessionFactoryRootDir(s.host.PersistRootDir(), session)
	sessionRootDir, sanitized, err := s.prepareEditableFactoryDefinitionSave(ctx, sessionRootDir, currentName, request.Snapshot)
	if err != nil {
		return EditableFactory{}, err
	}
	targetDir, activateFactoryDir, err := resolveReplaceCurrentLayoutTarget(s.host, sessionRootDir, currentName)
	if err != nil {
		return EditableFactory{}, err
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
) (EditableFactory, error) {
	currentName, err := factorySnapshotName(sanitized)
	if err != nil {
		return EditableFactory{}, fmt.Errorf("read editable factory snapshot identity: %w", err)
	}
	var saved EditableFactory
	err = s.activationGateway.WithActivationLock(func() error {
		return s.persistAndActivateCurrentFactory(
			ctx,
			sessionID,
			session,
			request,
			sessionRootDir,
			targetDir,
			activateFactoryDir,
			currentName,
			sanitized,
			&saved,
		)
	})
	if err != nil {
		return EditableFactory{}, err
	}
	return saved, nil
}

func (s *Service) persistAndActivateCurrentFactory(
	ctx context.Context,
	sessionID string,
	session *factorydefinitions.DefinitionSession,
	request EditableFactory,
	sessionRootDir string,
	targetDir string,
	activateFactoryDir string,
	currentName string,
	sanitized *factorydefinitions.FactorySnapshot,
	saved *EditableFactory,
) error {
	if err := s.activationGateway.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return err
	}
	nextVersion, prepared, err := s.prepareCurrentFactoryPersistence(
		sessionRootDir,
		currentName,
		request,
		sanitized,
	)
	if err != nil {
		return err
	}

	replaceResult, err := s.host.ReplaceFactoryLayoutAtDir(targetDir, prepared)
	if err != nil {
		return err
	}
	commit := false
	defer func() {
		if !commit && replaceResult != nil && replaceResult.Restore != nil {
			replaceResult.Restore()
		}
	}()

	responseSnapshot, responseVersion, err := s.prepareActivationResponseIfSupported(
		currentName,
		activateFactoryDir,
		true,
		nextVersion,
	)
	if err != nil {
		return err
	}
	activatedSource, resultAvailable, err := s.activateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		activateFactoryDir,
		currentName,
		currentName,
	)
	if err != nil {
		return err
	}
	*saved, err = s.currentFactorySaveResult(
		ctx,
		sessionID,
		currentName,
		nextVersion,
		activatedSource,
		resultAvailable,
		responseSnapshot,
		responseVersion,
	)
	if err != nil {
		return err
	}
	if replaceResult != nil && replaceResult.DiscardBackup != nil {
		replaceResult.DiscardBackup()
	}
	commit = true
	return nil
}

func (s *Service) prepareCurrentFactoryPersistence(
	sessionRootDir string,
	currentName string,
	request EditableFactory,
	sanitized *factorydefinitions.FactorySnapshot,
) (factorydefinitions.FactoryVersion, *factorydefinitions.PreparedFactoryLayoutPayload, error) {
	currentVersion, err := s.currentFactoryDefinitionVersionAtRoot(sessionRootDir, currentName)
	if err != nil {
		return factorydefinitions.FactoryVersion{}, nil, err
	}
	if err := s.RequireFreshEditableFactoryVersion(request.Version, currentVersion); err != nil {
		return factorydefinitions.FactoryVersion{}, nil, err
	}
	nextVersion := s.NextEditableFactoryVersion(&currentVersion, s.activationGateway.SaveNow())
	prepared, err := s.PreparePersistedFactoryPayload(currentName, sanitized, nextVersion)
	if err != nil {
		return factorydefinitions.FactoryVersion{}, nil, err
	}
	return nextVersion, prepared, nil
}

func (s *Service) currentFactorySaveResult(
	ctx context.Context,
	sessionID string,
	currentName string,
	nextVersion factorydefinitions.FactoryVersion,
	activatedSource factorydefinitions.LoadedFactorySource,
	resultAvailable bool,
	responseSnapshot *factorydefinitions.FactorySnapshot,
	responseVersion *factorydefinitions.FactoryVersion,
) (EditableFactory, error) {
	if resultAvailable {
		if responseSnapshot == nil || responseVersion == nil {
			return EditableFactory{}, fmt.Errorf("activation response is unavailable")
		}
		return EditableFactory{
			Name:       currentName,
			Snapshot:   responseSnapshot.Clone(),
			Version:    responseVersion,
			Activation: currentFactoryActivationProvenance(activatedSource),
		}, nil
	}
	savedSnapshot, err := s.host.GetCurrentFactorySnapshotForSession(
		withActivationReadBypass(ctx),
		sessionID,
	)
	if err != nil {
		return EditableFactory{}, err
	}
	version := &nextVersion
	var activation *factorydefinitions.FactoryActivationProvenance
	if runtimeCfg, runtimeErr := s.host.SessionRuntimeConfig(sessionID); runtimeErr == nil {
		if loadedVersion, loaded := loadedFactoryDefinitionVersion(runtimeCfg); loaded {
			version = loadedVersion
		}
		activation = currentFactoryActivationProvenance(runtimeCfg)
	}
	return EditableFactory{
		Name:       currentName,
		Snapshot:   savedSnapshot.Clone(),
		Version:    version,
		Activation: activation,
	}, nil
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
