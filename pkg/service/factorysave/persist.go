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
	sanitized := request
	sanitized.Version = nil
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return "", err
	}
	var factoryDir string
	if replaceExisting {
		factoryDir, err = configpersist.ReplaceNamedFactory(sessionRootDir, string(request.Name), payload)
	} else {
		factoryDir, err = configpersist.PersistNamedFactory(sessionRootDir, string(request.Name), payload)
	}
	if err != nil {
		return "", mapUpsertNamedFactoryPersistError(err)
	}
	return factoryDir, nil
}

func replaceEditableFactoryDefinition(
	sessionRootDir string,
	name factoryapi.FactoryName,
	payload []byte,
) (string, error) {
	factoryDir, err := configpersist.ReplaceNamedFactory(sessionRootDir, string(name), payload)
	if err == nil {
		return factoryDir, nil
	}
	if configpersist.IsInvalidNamedFactory(err) {
		return "", fmt.Errorf("%w: %w", apisurface.ErrInvalidNamedFactory, err)
	}
	return "", err
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
