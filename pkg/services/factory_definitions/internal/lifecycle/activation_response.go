package lifecycle

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// prepareActivationResponse loads and serializes the persisted candidate
// before the live runtime is replaced. Any filesystem, decoding, materialized
// response, or version failure therefore leaves the previous runtime active.
func (s *Service) prepareActivationResponse(
	name string,
	factoryDir string,
	inlineBundledFiles bool,
	fallbackVersion factorydefinitions.FactoryVersion,
) (*factorydefinitions.FactorySnapshot, *factorydefinitions.FactoryVersion, error) {
	candidate, err := loadFactoryFromHost(s.host, factoryDir, s.host.WorkstationLoader())
	if err != nil {
		return nil, nil, fmt.Errorf("load activation response candidate: %w", err)
	}
	if candidate == nil {
		return nil, nil, fmt.Errorf("activation response candidate is unavailable")
	}

	var snapshot *factorydefinitions.FactorySnapshot
	if inlineBundledFiles {
		snapshot, err = s.serializeNamedFactory(name, candidate, true)
	} else {
		snapshot, err = s.SerializeNamedFactoryUpsertResponse(name, candidate)
	}
	if err != nil {
		return nil, nil, err
	}
	if snapshot == nil {
		return nil, nil, fmt.Errorf("activation response snapshot is unavailable")
	}

	version := fallbackVersion
	if loadedVersion, loaded := loadedFactoryDefinitionVersion(candidate); loaded {
		version = *loadedVersion
	}
	return snapshot, &version, nil
}

func (s *Service) activationResultSupported() bool {
	if s == nil || s.activationGateway == nil {
		return false
	}
	_, ok := s.activationGateway.(factorydefinitions.DefinitionActivationResultGateway)
	return ok
}

func (s *Service) activateSessionEditableFactory(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) (factorydefinitions.LoadedFactorySource, bool, error) {
	if resultGateway, ok := s.activationGateway.(factorydefinitions.DefinitionActivationResultGateway); ok {
		result, err := resultGateway.ActivateSessionEditableFactoryWithResult(
			ctx,
			session,
			sessionID,
			sessionRootDir,
			factoryDir,
			name,
			runtimeName,
		)
		if err != nil {
			return nil, false, err
		}
		if result.Available {
			if result.LoadedSource == nil {
				return nil, true, fmt.Errorf("activation returned no loaded Factory source")
			}
			return result.LoadedSource, true, nil
		}
	}

	if err := s.activationGateway.ActivateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		name,
		runtimeName,
	); err != nil {
		return nil, false, err
	}
	return nil, false, nil
}
