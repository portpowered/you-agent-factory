package factorysave

import (
	"errors"
	"fmt"
	"os"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// SessionFactoryPersistRoot resolves the on-disk factory root for session-scoped persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return factorysessions.SessionFactoryRootDir(serviceRootDir, session)
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

func persistUpsertNamedFactoryPayload(
	sessionRootDir string,
	request factoryapi.Factory,
	nextVersion factoryapi.HybridLogicalTimestamp,
	replaceExisting bool,
) (string, error) {
	prepared, err := preparePersistedFactoryPayload(string(request.Name), request, nextVersion)
	if err != nil {
		return "", err
	}
	if replaceExisting {
		targetDir, err := resolveNamedFactoryLayoutTargetDir(sessionRootDir, request.Name)
		if err != nil {
			return "", mapUpsertNamedFactoryPersistError(err)
		}
		if _, err := persistFactoryLayoutAtDir(targetDir, prepared); err != nil {
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

func resolveNamedFactoryLayoutTargetDir(sessionRootDir string, name factoryapi.FactoryName) (string, error) {
	return factoryconfig.ResolveNamedFactoryDir(sessionRootDir, string(name))
}

func persistFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	result, err := configpersist.ReplaceFactoryLayoutAtDirWithPreparedWithResult(
		targetDir,
		prepared,
		configpersist.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		if configpersist.IsInvalidNamedFactory(err) {
			return nil, fmt.Errorf("%w: %w", apisurface.ErrInvalidNamedFactory, err)
		}
		return nil, err
	}
	return result, nil
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
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
