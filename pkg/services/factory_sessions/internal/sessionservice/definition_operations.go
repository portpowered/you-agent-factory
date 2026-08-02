package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// ReadCurrentFactoryForSession reads detached editable Factory facts for one
// live session. Sessions selects the runtime, resolves the current pointer,
// and owns the durable optimistic-concurrency version.
func (fs *SessionRuntime) ReadCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factorydefinitions.EditableFactory, error) {
	if fs == nil || fs.definitions == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Definitions service is required")
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	session, runtimeConfig, persistRoot, rootDir, name, err := fs.currentDefinitionState(ctx, sessionID)
	if err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	snapshot, err := fs.captureDefinitionSnapshot(ctx, runtimeConfig, name)
	if err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	version, err := fs.currentDefinitionVersion(ctx, persistRoot, rootDir, name, runtimeConfig)
	if err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	_ = session
	return factorydefinitions.EditableFactory{
		Name:     name,
		Snapshot: snapshot,
		Version:  &version,
	}, nil
}

// SaveFactoryForSession owns validation, optimistic concurrency, durable
// layout persistence, activation locking, and rollback for one session.
func (fs *SessionRuntime) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factorydefinitions.SaveMode,
	request factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	if fs == nil || fs.definitions == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Definitions service is required")
	}
	if request.Snapshot == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("editable factory snapshot is required")
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	if mode == factorydefinitions.SaveModeUpsertNamedAndActivate {
		return fs.saveNamedFactoryForSession(ctx, sessionID, request)
	}
	return fs.replaceCurrentFactoryForSession(ctx, sessionID, request)
}

func (fs *SessionRuntime) replaceCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	coordinator := fs.activationCoordinator()
	if coordinator == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Session activation coordinator is required")
	}
	err := coordinator.WithActivationLock(func() error {
		session, runtimeConfig, persistRoot, rootDir, currentName, err := fs.currentDefinitionState(ctx, sessionID)
		if err != nil {
			return err
		}
		if currentName == "" {
			return factorydefinitions.ErrCurrentFactoryNotFound
		}
		sanitized, err := request.Snapshot.WithName(currentName)
		if err != nil {
			return fmt.Errorf("name editable factory snapshot: %w", err)
		}
		if err := fs.validateEditableDefinition(ctx, sanitized); err != nil {
			return err
		}
		currentVersion, err := fs.currentDefinitionVersion(ctx, persistRoot, rootDir, currentName, runtimeConfig)
		if err != nil {
			return err
		}
		if err := requireFreshDefinitionVersion(request.Version, currentVersion); err != nil {
			return err
		}
		nextVersion := nextDefinitionVersion(&currentVersion, fs.definitionSaveNow())
		prepared, err := fs.preparePersistedDefinition(ctx, currentName, sanitized, nextVersion)
		if err != nil {
			return err
		}
		targetDir := rootDir
		if currentName != factorydefinitions.DefaultCurrentFactoryName {
			entry, getErr := fs.definitions.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
				RootDir: persistRoot,
				Name:    currentName,
			})
			if getErr != nil {
				return fmt.Errorf("resolve current factory %q: %w", currentName, getErr)
			}
			targetDir = entry.Entry.FactoryDir
		}
		replacementResult, err := fs.definitions.ReplaceFactoryLayoutAtDir(ctx, factorydefinitions.ReplaceFactoryLayoutAtDirRequest{
			TargetDir: targetDir,
			Prepared:  prepared,
		})
		if err != nil {
			return err
		}
		replacement := replacementResult.Replacement
		if err := coordinator.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			restoreLayout(replacement)
			return err
		}
		if err := coordinator.ActivateSessionEditableFactory(
			ctx,
			projectDefinitionSession(session),
			sessionID,
			rootDir,
			targetDir,
			currentName,
			currentName,
		); err != nil {
			restoreLayout(replacement)
			return err
		}
		discardLayout(replacement)
		return nil
	})
	if err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	return fs.ReadCurrentFactoryForSession(ctx, sessionID)
}

func (fs *SessionRuntime) saveNamedFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	coordinator := fs.activationCoordinator()
	if coordinator == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Session activation coordinator is required")
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || name == factorydefinitions.DefaultCurrentFactoryName {
		return factorydefinitions.EditableFactory{}, fmt.Errorf(
			"%w: %q is not a persistable named Factory",
			factorydefinitions.ErrInvalidNamedFactoryName,
			name,
		)
	}
	err := coordinator.WithActivationLock(func() error {
		session, _, persistRoot, _, _, err := fs.currentDefinitionState(ctx, sessionID)
		if err != nil {
			return err
		}
		if err := fs.validateEditableDefinition(ctx, request.Snapshot); err != nil {
			return err
		}
		entry, currentErr := fs.definitions.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
			RootDir: persistRoot,
			Name:    name,
		})
		replaceExisting := currentErr == nil
		if currentErr != nil && !errors.Is(currentErr, factorydefinitions.ErrNamedFactoryNotFound) {
			return currentErr
		}
		var currentVersion *factorydefinitions.FactoryVersion
		if replaceExisting {
			version, versionErr := fs.currentDefinitionVersion(ctx, persistRoot, persistRoot, name, nil)
			if versionErr != nil {
				return versionErr
			}
			if err := requireFreshDefinitionVersion(request.Version, version); err != nil {
				return err
			}
			currentVersion = &version
		}
		nextVersion := nextDefinitionVersion(currentVersion, fs.definitionSaveNow())
		prepared, err := fs.preparePersistedDefinition(ctx, name, request.Snapshot, nextVersion)
		if err != nil {
			return err
		}

		previousPointer, pointerErr := fs.definitions.GetCurrentFactoryPointer(ctx, factorydefinitions.GetCurrentFactoryPointerRequest{
			RootDir: persistRoot,
		})
		previousPointerKnown := pointerErr == nil
		if pointerErr != nil && !errors.Is(pointerErr, factorydefinitions.ErrCurrentFactoryNotFound) {
			return pointerErr
		}

		var factoryDir string
		var replacement *factorydefinitions.FactorySplitLayoutReplaceResult
		created := false
		if replaceExisting {
			factoryDir = entry.Entry.FactoryDir
			replacementResult, replaceErr := fs.definitions.ReplaceFactoryLayoutAtDir(ctx, factorydefinitions.ReplaceFactoryLayoutAtDirRequest{
				TargetDir: factoryDir,
				Prepared:  prepared,
			})
			if replaceErr != nil {
				return replaceErr
			}
			replacement = replacementResult.Replacement
		} else {
			result, createErr := fs.definitions.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
				RootDir:  persistRoot,
				Name:     name,
				Prepared: prepared,
			})
			if createErr != nil {
				return createErr
			}
			factoryDir = result.FactoryDir
			created = true
		}

		rollback := func() error {
			var rollbackErr error
			if previousPointerKnown {
				if _, restoreErr := fs.definitions.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
					RootDir: persistRoot,
					Name:    previousPointer.Name,
				}); restoreErr != nil {
					rollbackErr = fmt.Errorf("restore previous current Factory pointer: %w", restoreErr)
				}
			} else {
				if _, clearErr := fs.definitions.ClearCurrentFactoryPointer(ctx, factorydefinitions.ClearCurrentFactoryPointerRequest{
					RootDir: persistRoot,
				}); clearErr != nil {
					rollbackErr = fmt.Errorf("clear previous current Factory pointer: %w", clearErr)
				}
			}
			restoreLayout(replacement)
			if created {
				if _, deleteErr := fs.definitions.DeleteNamedFactory(ctx, factorydefinitions.DeleteNamedFactoryRequest{
					RootDir: persistRoot,
					Name:    name,
				}); deleteErr != nil && rollbackErr == nil {
					rollbackErr = fmt.Errorf("remove newly created Factory %q: %w", name, deleteErr)
				}
			}
			return rollbackErr
		}
		failWithRollback := func(primary error) error {
			if rollbackErr := rollback(); rollbackErr != nil {
				return fmt.Errorf("%w (rollback failed: %v)", primary, rollbackErr)
			}
			return primary
		}

		if _, err := fs.definitions.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
			RootDir: persistRoot,
			Name:    name,
		}); err != nil {
			return failWithRollback(err)
		}
		if err := coordinator.RequireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return failWithRollback(err)
		}
		if err := coordinator.ActivateSessionEditableFactory(
			ctx,
			projectDefinitionSession(session),
			sessionID,
			persistRoot,
			factoryDir,
			name,
			name,
		); err != nil {
			return failWithRollback(err)
		}
		discardLayout(replacement)
		return nil
	})
	if err != nil {
		return factorydefinitions.EditableFactory{}, err
	}
	return fs.ReadCurrentFactoryForSession(ctx, sessionID)
}

// ActivateFactory is the Sessions-owned named-Factory activation command.
func (fs *SessionRuntime) ActivateFactory(ctx context.Context, name string) error {
	if fs == nil || fs.definitions == nil {
		return fmt.Errorf("Factory Definitions service is required")
	}
	coordinator := fs.activationCoordinator()
	if coordinator == nil {
		return fmt.Errorf("Factory Session activation coordinator is required")
	}
	return coordinator.WithActivationLock(func() error {
		sessionID := coordinator.RunSessionID()
		session := coordinator.SessionForActivation(sessionID)
		if err := coordinator.RequireIdleBeforeNamedFactoryActivation(ctx, sessionID, session); err != nil {
			return err
		}
		persistRoot, folderPath := coordinator.NamedFactoryActivationPaths(session)
		entry, err := fs.definitions.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
			RootDir: persistRoot,
			Name:    name,
		})
		if err != nil {
			return err
		}
		previousPointer, pointerErr := fs.definitions.GetCurrentFactoryPointer(ctx, factorydefinitions.GetCurrentFactoryPointerRequest{
			RootDir: persistRoot,
		})
		previousPointerKnown := pointerErr == nil
		if pointerErr != nil && !errors.Is(pointerErr, factorydefinitions.ErrCurrentFactoryNotFound) {
			return pointerErr
		}
		activationErr := coordinator.SwapPersistedNamedFactoryRuntime(
			ctx,
			sessionID,
			session,
			persistRoot,
			folderPath,
			entry.Entry.FactoryDir,
			name,
		)
		if activationErr == nil {
			return nil
		}
		var rollbackErr error
		if previousPointerKnown {
			_, rollbackErr = fs.definitions.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
				RootDir: persistRoot,
				Name:    previousPointer.Name,
			})
		} else {
			_, rollbackErr = fs.definitions.ClearCurrentFactoryPointer(ctx, factorydefinitions.ClearCurrentFactoryPointerRequest{
				RootDir: persistRoot,
			})
		}
		if rollbackErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", activationErr, rollbackErr)
		}
		return activationErr
	})
}

func (fs *SessionRuntime) currentDefinitionState(
	ctx context.Context,
	sessionID string,
) (*livesession.LiveSession, factorydefinitions.LoadedFactorySource, string, string, string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil, "", "", "", fmt.Errorf("factory session id is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, "", "", "", err
	}
	session, err := runtimebinding.RequireLiveSession(fs.sessionState, sessionID)
	if err != nil {
		return nil, nil, "", "", "", err
	}
	runtimeConfig, err := runtimebinding.RuntimeConfigForSession(fs.sessionState, sessionID)
	if err != nil {
		return nil, nil, "", "", "", err
	}
	persistRoot := logicaltarget.SessionFactoryPersistRoot(fs.factoryRootDir, session)
	rootDir := sessionFactoryRootDir(fs.factoryRootDir, session)
	name := sessionFactoryName(rootDir, runtimeConfig)
	pointer, pointerErr := fs.definitions.GetCurrentFactoryPointer(ctx, factorydefinitions.GetCurrentFactoryPointerRequest{
		RootDir: persistRoot,
	})
	if pointerErr != nil && !errors.Is(pointerErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		return nil, nil, "", "", "", pointerErr
	}
	if pointerErr == nil && (session.IsDefault || pointer.Name == name) {
		name = pointer.Name
	}
	if name == "" {
		name = factorydefinitions.DefaultCurrentFactoryName
	}
	return session, runtimeConfig, persistRoot, rootDir, name, nil
}

func (fs *SessionRuntime) captureDefinitionSnapshot(
	ctx context.Context,
	runtimeConfig factorydefinitions.LoadedFactorySource,
	name string,
) (*factorydefinitions.FactorySnapshot, error) {
	if runtimeConfig == nil || runtimeConfig.FactoryConfig() == nil {
		return nil, fmt.Errorf("current factory snapshot is unavailable")
	}
	canonical, err := json.Marshal(runtimeConfig.FactoryConfig())
	if err != nil {
		return nil, fmt.Errorf("serialize current factory: %w", err)
	}
	result, err := fs.definitions.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: runtimeConfig.FactoryDir(),
		Canonical:  canonical,
		Name:       name,
	})
	if err != nil {
		return nil, fmt.Errorf("capture current factory snapshot: %w", err)
	}
	if result.Snapshot == nil {
		return nil, fmt.Errorf("current factory snapshot is unavailable")
	}
	return result.Snapshot.WithName(name)
}

func (fs *SessionRuntime) validateEditableDefinition(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
) error {
	if snapshot == nil {
		return fmt.Errorf("editable factory snapshot is required")
	}
	result, err := fs.definitions.ValidateStructuralFactoryDefinition(ctx, factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
		Canonical: []byte(*snapshot),
		Profile:   factorydefinitions.ValidationProfilePrePersist,
	})
	if err != nil {
		return err
	}
	if result.Validation.HasBlockingTargets() {
		return &factorydefinitions.FactoryDefinitionValidationFailure{Validation: result.Validation}
	}
	return nil
}

func (fs *SessionRuntime) preparePersistedDefinition(
	ctx context.Context,
	name string,
	snapshot *factorydefinitions.FactorySnapshot,
	version factorydefinitions.FactoryVersion,
) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
	payload, err := snapshotWithoutVersion(snapshot)
	if err != nil {
		return factorydefinitions.PreparedFactoryLayoutPayload{}, err
	}
	prepared, err := fs.definitions.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name:    name,
		Payload: payload,
	})
	if err != nil {
		return factorydefinitions.PreparedFactoryLayoutPayload{}, err
	}
	canonical, err := withDefinitionVersion(prepared.Prepared.Canonical, version)
	if err != nil {
		return factorydefinitions.PreparedFactoryLayoutPayload{}, err
	}
	prepared.Prepared.Canonical = canonical
	return prepared.Prepared, nil
}

func (fs *SessionRuntime) currentDefinitionVersion(
	ctx context.Context,
	persistRoot string,
	rootDir string,
	name string,
	fallback factorydefinitions.LoadedFactorySource,
) (factorydefinitions.FactoryVersion, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.FactoryVersion{}, err
	}
	factoryDir := rootDir
	if name != factorydefinitions.DefaultCurrentFactoryName {
		entry, err := fs.definitions.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
			RootDir: persistRoot,
			Name:    name,
		})
		if err != nil {
			return factorydefinitions.FactoryVersion{}, err
		}
		factoryDir = entry.Entry.FactoryDir
	}
	var loaded factorydefinitions.LoadedFactorySource
	if fallback != nil && sameDefinitionDir(fallback.FactoryDir(), factoryDir) {
		loaded = fallback
	} else if fs.loadFactory != nil {
		loaded, _ = fs.loadFactory(factoryDir, fs.workstationLoader)
	}
	if loaded != nil && loaded.FactoryConfig() != nil && loaded.FactoryConfig().Version != nil {
		version := *loaded.FactoryConfig().Version
		version.Physical = version.Physical.UTC()
		return version, nil
	}
	if fs.directoryInspection == nil {
		return factorydefinitions.FactoryVersion{}, fmt.Errorf("Factory Definition version filesystem is required")
	}
	info, err := fs.directoryInspection.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		return factorydefinitions.FactoryVersion{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factorydefinitions.FactoryVersion{Logical: logical, Physical: modified}, nil
}

func snapshotWithoutVersion(snapshot *factorydefinitions.FactorySnapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("editable factory snapshot is required")
	}
	var object map[string]json.RawMessage
	if err := snapshot.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode editable factory snapshot: %w", err)
	}
	delete(object, "version")
	payload, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return payload, nil
}

func withDefinitionVersion(canonical []byte, version factorydefinitions.FactoryVersion) ([]byte, error) {
	var object map[string]any
	if err := json.Unmarshal(canonical, &object); err != nil {
		return nil, fmt.Errorf("unmarshal canonical factory payload: %w", err)
	}
	object["version"] = map[string]any{
		"logical":  version.Logical,
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted factory payload: %w", err)
	}
	return payload, nil
}

func requireFreshDefinitionVersion(candidate *factorydefinitions.FactoryVersion, current factorydefinitions.FactoryVersion) error {
	if candidate == nil || candidate.Logical <= current.Logical || !candidate.Physical.UTC().After(current.Physical.UTC()) {
		return fmt.Errorf("%w: submitted version must advance the current Factory definition", factorydefinitions.ErrFactoryVersionStale)
	}
	return nil
}

func nextDefinitionVersion(current *factorydefinitions.FactoryVersion, now time.Time) factorydefinitions.FactoryVersion {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factorydefinitions.FactoryVersion{Logical: logical, Physical: physical}
}

func (fs *SessionRuntime) definitionSaveNow() time.Time {
	if fs != nil && fs.clock != nil {
		return fs.clock.Now().UTC()
	}
	return time.Now().UTC()
}

func sessionFactoryRootDir(serviceRootDir string, session *livesession.LiveSession) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FactoryDir == "" || !sameDefinitionDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	cleanServiceRoot := filepath.Clean(serviceRootDir)
	if cleanServiceRoot != "" && filepath.Dir(session.FactoryDir) == cleanServiceRoot {
		return cleanServiceRoot
	}
	return rootDir
}

func sessionFactoryName(rootDir string, runtimeConfig factorydefinitions.RuntimeConfigLookup) string {
	if runtimeConfig == nil {
		return factorydefinitions.DefaultCurrentFactoryName
	}
	factoryDir := runtimeConfig.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if sameDefinitionDir(factoryDir, cleanRoot) {
		return factorydefinitions.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		if name := strings.TrimSpace(filepath.Base(factoryDir)); name != "" && name != "." {
			return name
		}
	}
	if cfg := runtimeConfig.FactoryConfig(); cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return name
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return project
		}
	}
	return "factory"
}

func sameDefinitionDir(left, right string) bool {
	return strings.TrimSpace(left) != "" && strings.TrimSpace(right) != "" && filepath.Clean(left) == filepath.Clean(right)
}

func restoreLayout(replacement *factorydefinitions.FactorySplitLayoutReplaceResult) {
	if replacement != nil && replacement.Restore != nil {
		replacement.Restore()
	}
}

func discardLayout(replacement *factorydefinitions.FactorySplitLayoutReplaceResult) {
	if replacement != nil && replacement.DiscardBackup != nil {
		replacement.DiscardBackup()
	}
}
