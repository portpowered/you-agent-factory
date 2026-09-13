package lifecycle

import (
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func loadedFactoryDefinitionVersion(
	current factorydefinitions.LoadedFactorySource,
) (*factorydefinitions.FactoryVersion, bool) {
	if current == nil {
		return nil, false
	}
	if versionSource, ok := current.(factorydefinitions.LoadedFactoryVersionSource); ok {
		if version := versionSource.LoadedFactoryVersion(); version != nil {
			version.Physical = version.Physical.UTC()
			return version, true
		}
	}
	if factoryConfig := current.FactoryConfig(); factoryConfig != nil && factoryConfig.Version != nil {
		version := *factoryConfig.Version
		version.Physical = version.Physical.UTC()
		return &version, true
	}
	return nil, false
}

func currentFactoryActivationProvenance(
	current factorydefinitions.LoadedFactorySource,
) *factorydefinitions.FactoryActivationProvenance {
	activationSource, ok := current.(factorydefinitions.LoadedFactoryActivationSource)
	if !ok {
		return nil
	}
	activation := activationSource.FactoryActivationProvenance()
	if activation == nil ||
		strings.TrimSpace(activation.ActivationID) == "" ||
		strings.TrimSpace(activation.LoadedSourceDigest) == "" {
		return nil
	}
	activation.State = factorydefinitions.FactoryActivationStateNotActivated
	if comparator, ok := current.(factorydefinitions.LoadedFactoryAuthoredSourceComparator); ok {
		state, err := comparator.CompareAuthoredSource()
		if err != nil {
			state = factorydefinitions.FactoryActivationStateAuthoredSourceUnavailable
		}
		switch state {
		case factorydefinitions.FactoryActivationStateActive,
			factorydefinitions.FactoryActivationStateAuthoredChanged,
			factorydefinitions.FactoryActivationStateNotActivated,
			factorydefinitions.FactoryActivationStateAuthoredSourceUnavailable:
			activation.State = state
		default:
			activation.State = factorydefinitions.FactoryActivationStateAuthoredSourceUnavailable
		}
	}
	return activation
}

func (s *Service) serializeNamedFactory(
	name string,
	current factorydefinitions.LoadedFactorySource,
	inlineBundledFiles bool,
) (*factorydefinitions.FactorySnapshot, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		portableFactoryConfig, err := preparePortableFactoryConfigFromHost(
			s.host,
			current.FactoryDir(),
			factoryCfg,
			true,
		)
		if err != nil {
			return nil, fmt.Errorf("prepare named Factory snapshot: %w", err)
		}
		factoryCfg = portableFactoryConfig
	}
	snapshot, err := captureFactorySnapshotFromHost(
		s.host,
		current.FactoryDir(),
		factoryCfg,
		current,
		current.FactoryDir(), nil)

	if err != nil {
		return nil, fmt.Errorf("serialize current factory: %w", err)
	}
	namedSnapshot, err := snapshot.WithName(name)
	if err != nil {
		return nil, fmt.Errorf("name current factory snapshot: %w", err)
	}
	return namedSnapshot, nil
}

func (s *Service) currentFactoryDefinitionVersionAtRoot(rootDir string, name string) (factorydefinitions.FactoryVersion, error) {
	factoryDir := rootDir
	if name != factorydefinitions.DefaultCurrentFactoryName {
		resolved, err := resolveExistingFactoryDirFromHost(s.host, rootDir, name)
		if err != nil {
			return factorydefinitions.FactoryVersion{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factorydefinitions.WorkstationLoader
	if s != nil && s.host != nil {
		workstationLoader = s.host.WorkstationLoader()
	}
	current, err := loadFactoryFromHost(s.host, factoryDir, workstationLoader)
	if err != nil {
		return factorydefinitions.FactoryVersion{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return factorydefinitions.FactoryVersion{
			Logical:  version.Logical,
			Physical: version.Physical.UTC(),
		}, nil
	}

	if s.versionFileSystem == nil {
		return factorydefinitions.FactoryVersion{}, fmt.Errorf("Factory Definition version filesystem is required")
	}
	info, err := s.versionFileSystem.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		return factorydefinitions.FactoryVersion{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factorydefinitions.FactoryVersion{
		Logical:  logical,
		Physical: modified,
	}, nil
}

// SerializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (s *Service) SerializeNamedFactoryUpsertResponse(
	name string,
	current factorydefinitions.LoadedFactorySource,
) (*factorydefinitions.FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	factoryCfg := current.FactoryConfig()
	if factoryCfg != nil {
		portableFactoryConfig, err := preparePortableFactoryConfigFromHost(
			s.host,
			current.FactoryDir(),
			factoryCfg,
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("prepare named Factory response: %w", err)
		}
		factoryCfg = portableFactoryConfig
	}
	snapshot, err := captureFactorySnapshotFromHost(
		s.host,
		current.FactoryDir(),
		factoryCfg,
		current,
		current.FactoryDir(), nil)

	if err != nil {
		return nil, fmt.Errorf("serialize upsert factory: %w", err)
	}
	namedSnapshot, err := snapshot.WithName(name)
	if err != nil {
		return nil, fmt.Errorf("name upsert factory snapshot: %w", err)
	}
	return namedSnapshot, nil
}
