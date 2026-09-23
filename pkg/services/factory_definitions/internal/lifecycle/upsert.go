package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// SaveUpsertNamedSnapshotAndActivateForSession persists one named Factory
// definition for a live session and activates it as the session current Factory.
func (s *Service) SaveUpsertNamedSnapshotAndActivateForSession(
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
	err = s.activationGateway.WithActivationLock(func() error {
		if err := s.activationGateway.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}

		currentVersion, err := s.upsertCurrentVersionAtSessionRoot(sessionRootDir, request, replaceExisting)
		if err != nil {
			return err
		}

		nextVersion := s.NextEditableFactoryVersion(currentVersion, s.activationGateway.SaveNow())
		prepared, err := s.PreparePersistedFactoryPayload(request.Name, request.Snapshot, nextVersion)
		if err != nil {
			return err
		}
		pointerState, err := readCurrentFactoryPointerState(
			func(rootDir string) (string, error) {
				return readCurrentFactoryPointerFromHost(s.host, rootDir)
			},
			sessionRootDir,
		)
		if err != nil {
			return err
		}
		persisted, err := persistUpsertNamedFactoryPrepared(
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
			persisted,
			pointerState,
			request,
			nextVersion,
		)
		return finalizeErr
	})
	if err != nil {
		return EditableFactory{}, err
	}
	return saved, nil
}

type persistedNamedFactory struct {
	factoryDir   string
	replace      *factorydefinitions.FactorySplitLayoutReplaceResult
	created      bool
	replaceKnown bool
}

func (p persistedNamedFactory) rollback(host Host, rootDir, name string) error {
	if p.replaceKnown {
		if p.replace == nil || p.replace.Restore == nil {
			return fmt.Errorf("named Factory layout rollback handle is required")
		}
		p.replace.Restore()
		return nil
	}
	if p.created {
		return discardNamedFactoryFromHost(host, rootDir, name)
	}
	return nil
}

func (p persistedNamedFactory) commit() {
	if p.replace != nil && p.replace.DiscardBackup != nil {
		p.replace.DiscardBackup()
	}
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
	persisted persistedNamedFactory,
	pointerState currentFactoryPointerState,
	request EditableFactory,
	nextVersion factorydefinitions.FactoryVersion,
) (EditableFactory, error) {
	var responseSnapshot *factorydefinitions.FactorySnapshot
	var responseVersion *factorydefinitions.FactoryVersion
	if s.activationResultSupported() {
		var err error
		responseSnapshot, responseVersion, err = s.prepareActivationResponse(
			request.Name,
			persisted.factoryDir,
			false,
			nextVersion,
		)
		if err != nil {
			return EditableFactory{}, rollbackUpsertFailure(
				err,
				s.host,
				sessionRootDir,
				request.Name,
				persisted,
				nil,
			)
		}
	}

	restorePointer, err := s.publishUpsertCurrentPointer(
		pointerState,
		sessionRootDir,
		request.Name,
	)
	if err != nil {
		return EditableFactory{}, rollbackUpsertFailure(
			err,
			s.host,
			sessionRootDir,
			request.Name,
			persisted,
			nil,
		)
	}

	activatedSource, resultAvailable, err := s.activateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		persisted.factoryDir,
		request.Name,
		request.Name,
	)
	if err != nil {
		return EditableFactory{}, rollbackUpsertFailure(
			err,
			s.host,
			sessionRootDir,
			request.Name,
			persisted,
			restorePointer,
		)
	}
	if resultAvailable {
		saved := EditableFactory{
			Name:       request.Name,
			Snapshot:   responseSnapshot.Clone(),
			Version:    responseVersion,
			Activation: currentFactoryActivationProvenance(activatedSource),
		}
		persisted.commit()
		return saved, nil
	}

	saved, err := s.upsertActivationResponse(sessionID, sessionRootDir, request.Name)
	if err != nil {
		return EditableFactory{}, rollbackUpsertFailure(
			err,
			s.host,
			sessionRootDir,
			request.Name,
			persisted,
			restorePointer,
		)
	}
	persisted.commit()
	return saved, nil
}

func (s *Service) publishUpsertCurrentPointer(
	state currentFactoryPointerState,
	rootDir string,
	name string,
) (func() error, error) {
	restore, err := writeCurrentFactoryPointerAfterState(
		state,
		rootDir,
		name,
		func(rootDir, name string) error {
			return writeCurrentFactoryPointerFromHost(s.host, rootDir, name)
		},
		func(rootDir string) error {
			return removeCurrentFactoryPointerFromHost(s.host, rootDir)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("write session current factory pointer: %w", err)
	}
	return restore, nil
}

type currentFactoryPointerState struct {
	name   string
	exists bool
}

func readCurrentFactoryPointerState(
	read factorydefinitions.CurrentFactoryPointerReader,
	rootDir string,
) (currentFactoryPointerState, error) {
	if read == nil {
		return currentFactoryPointerState{}, fmt.Errorf("current Factory pointer reader is required")
	}
	name, err := read(rootDir)
	if err == nil {
		return currentFactoryPointerState{name: name, exists: true}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return currentFactoryPointerState{}, nil
	}
	return currentFactoryPointerState{}, err
}

func writeCurrentFactoryPointerAfterState(
	state currentFactoryPointerState,
	rootDir string,
	name string,
	write factorydefinitions.CurrentFactoryPointerWriter,
	remove func(string) error,
) (func() error, error) {
	if write == nil {
		return nil, fmt.Errorf("current Factory pointer writer is required")
	}
	restore := func() error {
		if state.exists {
			return write(rootDir, state.name)
		}
		if remove == nil {
			return fmt.Errorf("current Factory pointer remover is required to restore an absent pointer")
		}
		return remove(rootDir)
	}
	if err := write(rootDir, name); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return nil, errors.Join(err, fmt.Errorf("restore current Factory pointer after write failure: %w", restoreErr))
		}
		return nil, err
	}
	return restore, nil
}

func (s *Service) upsertActivationResponse(
	sessionID string,
	rootDir string,
	name string,
) (EditableFactory, error) {
	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	snapshot, err := s.SerializeNamedFactoryUpsertResponse(name, runtimeCfg)
	if err != nil {
		return EditableFactory{}, err
	}
	version, err := s.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{
		Name:       name,
		Snapshot:   snapshot,
		Version:    &version,
		Activation: currentFactoryActivationProvenance(runtimeCfg),
	}, nil
}

func rollbackUpsertFailure(
	primary error,
	host Host,
	rootDir string,
	name string,
	persisted persistedNamedFactory,
	restorePointer func() error,
) error {
	var rollbackErrs []error
	if restorePointer != nil {
		if err := restorePointer(); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore current Factory pointer: %w", err))
		}
	}
	if err := persisted.rollback(host, rootDir, name); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback persisted named Factory: %w", err))
	}
	if len(rollbackErrs) == 0 {
		return primary
	}
	return errors.Join(append([]error{primary}, rollbackErrs...)...)
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
) (persistedNamedFactory, error) {
	if replaceExisting {
		targetDir, err := resolveExistingFactoryDirFromHost(host, sessionRootDir, request.Name)
		if err != nil {
			return persistedNamedFactory{}, mapUpsertNamedFactoryPersistError(err)
		}
		replace, err := host.ReplaceFactoryLayoutAtDir(targetDir, prepared)
		if err != nil {
			return persistedNamedFactory{}, mapUpsertNamedFactoryPersistError(err)
		}
		return persistedNamedFactory{
			factoryDir:   targetDir,
			replace:      replace,
			replaceKnown: true,
		}, nil
	}
	factoryDir, err := persistNamedFactoryWithPreparedFromHost(host, sessionRootDir, request.Name, prepared)
	if err != nil {
		return persistedNamedFactory{}, mapUpsertNamedFactoryPersistError(err)
	}
	return persistedNamedFactory{factoryDir: factoryDir, created: true}, nil
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
