package factorydefinition

import (
	"fmt"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

func (s *Service) serializeNamedFactory(
	name string,
	current factorydefinitions.LoadedFactorySource,
	inlineBundledFiles bool,
) (*interfaces.FactorySnapshot, error) {
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

func (s *Service) currentFactoryDefinitionVersionAtRoot(rootDir string, name string) (interfaces.FactoryVersion, error) {
	factoryDir := rootDir
	if name != interfaces.DefaultCurrentFactoryName {
		resolved, err := resolveExistingFactoryDirFromHost(s.host, rootDir, name)
		if err != nil {
			return interfaces.FactoryVersion{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factorydefinitions.WorkstationLoader
	if s != nil && s.host != nil {
		workstationLoader = s.host.WorkstationLoader()
	}
	current, err := loadFactoryFromHost(s.host, factoryDir, workstationLoader)
	if err != nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return interfaces.FactoryVersion{
			Logical:  version.Logical,
			Physical: version.Physical.UTC(),
		}, nil
	}

	if s.versionFileSystem == nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("Factory Definition version filesystem is required")
	}
	info, err := s.versionFileSystem.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return interfaces.FactoryVersion{
		Logical:  logical,
		Physical: modified,
	}, nil
}

// SerializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (s *Service) SerializeNamedFactoryUpsertResponse(
	name string,
	current factorydefinitions.LoadedFactorySource,
) (*interfaces.FactorySnapshot, error) {
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
